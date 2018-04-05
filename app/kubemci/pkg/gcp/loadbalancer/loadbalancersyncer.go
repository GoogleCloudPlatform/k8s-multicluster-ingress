// Copyright 2017 Google Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package loadbalancer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang/glog"
	multierror "github.com/hashicorp/go-multierror"
	"k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeclient "k8s.io/client-go/kubernetes"
	"k8s.io/ingress-gce/pkg/annotations"
	ingressbe "k8s.io/ingress-gce/pkg/backends"
	ingressig "k8s.io/ingress-gce/pkg/instances"
	ingresslb "k8s.io/ingress-gce/pkg/loadbalancers"
	ingressutils "k8s.io/ingress-gce/pkg/utils"
	"k8s.io/kubernetes/pkg/cloudprovider/providers/gce"
	"k8s.io/kubernetes/pkg/printers"

	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/backendservice"
	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/firewallrule"
	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/forwardingrule"
	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/healthcheck"
	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/instances"
	utilsnamer "github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/namer"
	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/sslcert"
	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/status"
	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/targetproxy"
	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/urlmap"
	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/utils"
	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/kubeutils"
	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/validations"
)

const (
	// Prefix used by the namer to generate names.
	// This is used to identify resources created by this code.
	MciPrefix = "mci1"
)

// LoadBalancerSyncer manages the GCP resources necessary for an L7 GCP Load balancer.
type LoadBalancerSyncer struct {
	lbName string
	// Health check syncer to sync the required health checks.
	hcs healthcheck.HealthCheckSyncerInterface
	// Backend service syncer to sync the required backend services.
	bss backendservice.BackendServiceSyncerInterface
	// URL map syncer to sync the required URL map.
	ums urlmap.SyncerInterface
	// Target proxy syncer to sync the required target proxy.
	tps targetproxy.TargetProxySyncerInterface
	// Forwarding rule syncer to sync the required forwarding rule.
	frs forwardingrule.ForwardingRuleSyncerInterface
	// Firewall rule syncer to sync the required firewall rule.
	fws firewallrule.FirewallRuleSyncerInterface
	// SSL Certificates syncer to sync the required ssl certificates.
	scs sslcert.SSLCertSyncerInterface
	// Kubernetes clients to send requests to kubernetes apiservers.
	// This is a map from cluster name to the client for that cluster.
	clients map[string]kubeclient.Interface
	// Instance groups provider to call GCE APIs to manage GCE instance groups.
	igp ingressig.InstanceGroups
	// IP addresses provider to manage GCP IP addresses.
	// We do not have a specific ip address provider interface, so we use the larger LoadBalancers interface.
	ipp ingresslb.LoadBalancers
}

func NewLoadBalancerSyncer(lbName string, clients map[string]kubeclient.Interface, cloud *gce.GCECloud, gcpProjectId string) (*LoadBalancerSyncer, error) {

	namer := utilsnamer.NewNamer(MciPrefix, lbName)
	ig, err := instances.NewInstanceGetter(gcpProjectId)
	if err != nil {
		return nil, err
	}
	return &LoadBalancerSyncer{
		lbName:  lbName,
		hcs:     healthcheck.NewHealthCheckSyncer(namer, cloud),
		bss:     backendservice.NewBackendServiceSyncer(namer, cloud),
		ums:     urlmap.NewURLMapSyncer(namer, cloud),
		tps:     targetproxy.NewTargetProxySyncer(namer, cloud),
		frs:     forwardingrule.NewForwardingRuleSyncer(namer, cloud),
		fws:     firewallrule.NewFirewallRuleSyncer(namer, cloud, ig),
		scs:     sslcert.NewSSLCertSyncer(namer, cloud),
		clients: clients,
		igp:     cloud,
		ipp:     cloud,
	}, nil
}

// CreateLoadBalancer creates the GCP resources necessary for an L7 GCP load balancer corresponding to the given ingress.
// clusters is the list of clusters that this load balancer is spread to.
func (l *LoadBalancerSyncer) CreateLoadBalancer(ing *v1beta1.Ingress, forceUpdate, validate bool, clusters []string) error {
	client, cErr := getAnyClient(l.clients)
	if cErr != nil {
		// No point in continuing without a client.
		return cErr
	}

	if validate {
		// TODO(G-Harmon): Move this earlier to create.go and consolidate with the other validation there.
		if err := validations.Validate(l.clients, ing); err != nil {
			return fmt.Errorf("validation failed: %s", err)
		}
		glog.Infof("Validation passed.")
	} else {
		fmt.Println("Validation skipped. (Set --validate to enable.)")
	}

	ports := l.ingToNodePorts(ing, client)
	ipAddr, err := l.getIPAddress(ing)
	if err != nil {
		// No point continuing if the user has not specified a valid ip address.
		return err
	}
	// Create health checks to be used by the backend service.
	healthChecks, hcErr := l.hcs.EnsureHealthCheck(l.lbName, ports, l.clients, forceUpdate)
	if hcErr != nil {
		// Keep aggregating errors and return all at the end, rather than giving up on the first error.
		hcErr = fmt.Errorf("Error ensuring health check for %s: %s", l.lbName, hcErr)
		fmt.Println(hcErr)
		err = multierror.Append(err, hcErr)
	}
	igs, igErr := l.getIGs(ing, l.clients)
	if igErr != nil {
		err = multierror.Append(err, igErr)
	}
	namedPorts, portsErr := l.getNamedPorts(igs)
	if portsErr != nil {
		err = multierror.Append(err, portsErr)
	}
	// Can not really create any backend service without named ports and instance groups. No point continuing.
	if len(igs) == 0 {
		err = multierror.Append(err, fmt.Errorf("No instance group found. Can not continue"))
		return err
	}
	if len(namedPorts) == 0 {
		err = multierror.Append(err, fmt.Errorf("No named ports found. Can not continue"))
		return err
	}
	igsForBE := []string{}
	for k := range igs {
		igsForBE = append(igsForBE, igs[k]...)
	}
	// Create backend service. This should always be called after the health check since backend service needs to point to the health check.
	backendServices, beErr := l.bss.EnsureBackendService(l.lbName, ports, healthChecks, namedPorts, igsForBE, forceUpdate)
	if beErr != nil {
		// Aggregate errors and return all at the end.
		beErr = fmt.Errorf("Error ensuring backend service for %s: %s", l.lbName, beErr)
		fmt.Println(beErr)
		err = multierror.Append(err, beErr)
	}
	umLink, umErr := l.ums.EnsureURLMap(l.lbName, ipAddr, clusters, ing, backendServices, forceUpdate)
	if umErr != nil {
		// Aggregate errors and return all at the end.
		umErr = fmt.Errorf("Error ensuring urlmap for %s: %s", l.lbName, umErr)
		fmt.Println(umErr)
		err = multierror.Append(err, umErr)
	}
	ingAnnotations := annotations.FromIngress(ing)
	// Configure HTTP target proxy and forwarding rule, if required.
	if ingAnnotations.AllowHTTP() {
		tpLink, tpErr := l.tps.EnsureHttpTargetProxy(l.lbName, umLink, forceUpdate)
		if tpErr != nil {
			// Aggregate errors and return all at the end.
			tpErr = fmt.Errorf("Error ensuring HTTP target proxy: %s", tpErr)
			fmt.Println(tpErr)
			err = multierror.Append(err, tpErr)
		}
		frErr := l.frs.EnsureHttpForwardingRule(l.lbName, ipAddr, tpLink, forceUpdate)
		if frErr != nil {
			// Aggregate errors and return all at the end.
			frErr = fmt.Errorf("Error ensuring http forwarding rule: %s", frErr)
			fmt.Println(frErr)
			err = multierror.Append(err, frErr)
		}
	}
	// Configure HTTPS target proxy and forwarding rule, if required.
	if (ingAnnotations.UseNamedTLS() != "") || len(ing.Spec.TLS) > 0 {
		// Note that we expect to load the cert from any cluster,
		// so users are required to create the secret in all clusters.
		certLink, cErr := l.scs.EnsureSSLCert(l.lbName, ing, client, forceUpdate)
		if cErr != nil {
			// Aggregate errors and return all at the end.
			cErr = fmt.Errorf("Error ensuring SSL certs: %s", cErr)
			fmt.Println(cErr)
			err = multierror.Append(err, cErr)
		}

		tpLink, tpErr := l.tps.EnsureHttpsTargetProxy(l.lbName, umLink, certLink, forceUpdate)
		if tpErr != nil {
			// Aggregate errors and return all at the end.
			tpErr = fmt.Errorf("Error ensuring HTTPS target proxy: %s", tpErr)
			fmt.Println(tpErr)
			err = multierror.Append(err, tpErr)
		}
		frErr := l.frs.EnsureHttpsForwardingRule(l.lbName, ipAddr, tpLink, forceUpdate)
		if frErr != nil {
			// Aggregate errors and return all at the end.
			frErr = fmt.Errorf("Error ensuring https forwarding rule: %s", frErr)
			fmt.Println(frErr)
			err = multierror.Append(err, frErr)
		}
	}
	if fwErr := l.fws.EnsureFirewallRule(l.lbName, ports, igs, forceUpdate); fwErr != nil {
		// Aggregate errors and return all at the end.
		fmt.Println("Error ensuring firewall rule:", fwErr)
		err = multierror.Append(err, fwErr)
	}
	return err
}

// DeleteLoadBalancer deletes the GCP resources associated with the L7 GCP load balancer for the given ingress.
// Continues in case of errors and deletes the resources that it can when forceDelete is set to true.
// TODO(nikhiljindal): Do not require the ingress yaml from users. Just the name should be enough. We can fetch ingress YAML from one of the clusters.
func (l *LoadBalancerSyncer) DeleteLoadBalancer(ing *v1beta1.Ingress, forceDelete bool) error {
	var err error
	client, cErr := getAnyClient(l.clients)
	if cErr != nil {
		if !forceDelete {
			// No point in continuing without a client.
			return cErr
		}
		fmt.Printf("%s. Continuing with force delete. Will not be able to delete backend services and healthchecks\n", cErr)
		err = multierror.Append(err, cErr)
	}
	// We delete resources in the reverse order in which they were created.
	// For ex: we create health checks before creating backend services (so that backend service can point to the health check),
	// but delete backend service before deleting health check. We cannot delete the health check when backend service is still pointing to it.
	if fwErr := l.fws.DeleteFirewallRules(); fwErr != nil {
		// Aggregate errors and return all at the end.
		err = multierror.Append(err, fwErr)
	}

	if frErr := l.frs.DeleteForwardingRules(); frErr != nil {
		// Aggregate errors and return all at the end.
		err = multierror.Append(err, frErr)
	}
	if tpErr := l.tps.DeleteTargetProxies(); tpErr != nil {
		// Aggregate errors and return all at the end.
		err = multierror.Append(err, tpErr)
	}
	if scErr := l.scs.DeleteSSLCert(); scErr != nil {
		// Aggregate errors and return all at the end.
		err = multierror.Append(err, scErr)
	}
	if umErr := l.ums.DeleteURLMap(); umErr != nil {
		// Aggregate errors and return all at the end.
		err = multierror.Append(err, umErr)
	}
	if cErr == nil {
		ports := l.ingToNodePorts(ing, client)
		if beErr := l.bss.DeleteBackendServices(ports); beErr != nil {
			// Aggregate errors and return all at the end.
			err = multierror.Append(err, beErr)
		}
		if hcErr := l.hcs.DeleteHealthChecks(ports); hcErr != nil {
			// Aggregate errors and return all at the end.
			err = multierror.Append(err, hcErr)
		}
	}
	return err
}

// RemoveFromClusters removes the given ingress from the clusters corresponding to the given clients.
// TODO(nikhiljindal): Do not require the ingress yaml from users. Just the name should be enough. We can fetch ingress YAML from one of the clusters.
func (l *LoadBalancerSyncer) RemoveFromClusters(ing *v1beta1.Ingress, removeClients map[string]kubeclient.Interface, forceUpdate bool) error {
	// First verify that removing it from the given clusters will not remove it from all clusters.
	// If yes, then prompt users to use delete instead or use --force.
	lbStatus, statusErr := l.getLoadBalancerStatus()
	if statusErr != nil {
		return fmt.Errorf("error in fetching status of existing load balancer: %s", statusErr)
	}
	if !hasSomeRemainingClusters(lbStatus.Clusters, removeClients) {
		if !forceUpdate {
			return fmt.Errorf("This will remove the ingress from all clusters. You should use kubemci delete to delete the ingress completely. If you really want to just remove it from existing clusters without deleting it completely, use --force flag with remove-clusters command")
		}
		fmt.Println("This will remove the ingress from all clusters. Continuing with force update")
	}

	client, cErr := getAnyClient(l.clients)
	if cErr != nil {
		// No point in continuing without a client.
		return cErr
	}
	var err error
	ports := l.ingToNodePorts(ing, client)
	removeIGLinks, igErr := l.getIGs(ing, removeClients)
	if igErr == nil && len(removeIGLinks) == 0 {
		igErr = fmt.Errorf("No instance group found")
	}
	// Can not really update resources without igs. So return on error.
	// Allow users to continue using force since they can run into this if they are trying to remove their already deleted clusters.
	if igErr != nil {
		if !forceUpdate {
			return igErr
		}
		fmt.Printf("Error in fetching instance groups: %s. Continuing with force update\n", igErr)
		err = multierror.Append(err, igErr)
	}

	if igErr == nil {
		if fwErr := l.fws.RemoveFromClusters(l.lbName, removeIGLinks); fwErr != nil {
			// Aggregate errors and return all at the end.
			err = multierror.Append(err, fwErr)
		}

		// Convert the map of instance groups to a flat array.
		igsForBE := []string{}
		for k := range removeIGLinks {
			igsForBE = append(igsForBE, removeIGLinks[k]...)
		}

		if beErr := l.bss.RemoveFromClusters(ports, igsForBE); beErr != nil {
			// Aggregate errors and return all at the end.
			err = multierror.Append(err, beErr)
		}
	}

	// Convert the client map to an array of cluster names.
	clustersToRemove := make([]string, len(removeClients))
	removeIndex := 0
	for k := range removeClients {
		clustersToRemove[removeIndex] = k
		removeIndex++
	}
	// Update the status at the end.
	if statusErr := l.removeClustersFromStatus(l.lbName, clustersToRemove); statusErr != nil {
		// Aggregate errors and return all at the end.
		err = multierror.Append(err, statusErr)
	}
	return err
}

// hasSomeRemainingClusters returns true if not all clusters are in the clustersToRemove list.
func hasSomeRemainingClusters(clusters []string, clustersToRemove map[string]kubeclient.Interface) bool {
	for _, v := range clusters {
		if _, has := clustersToRemove[v]; !has {
			return true
		}
	}
	return false
}

// removeClustersFromStatus updates the status of the given load balancer to remove the given list of clusters from it.
// Since the status could be stored on either the url map or the forwarding rule, it tries updating both and returns an error if that fails.
// Returns an http.StatusNotFound error if either the url map or the forwarding rule does not exist.
func (l *LoadBalancerSyncer) removeClustersFromStatus(lbName string, clustersToRemove []string) error {
	// Try updating status in both url map and forwarding rule.
	frErr := l.frs.RemoveClustersFromStatus(clustersToRemove)
	umErr := l.ums.RemoveClustersFromStatus(clustersToRemove)
	if ingressutils.IsHTTPErrorCode(frErr, http.StatusNotFound) || ingressutils.IsHTTPErrorCode(umErr, http.StatusNotFound) {
		return fmt.Errorf("load balancer %s does not exist", lbName)
	}
	if frErr == nil {
		return umErr
	}
	if umErr == nil {
		return frErr
	}
	return fmt.Errorf("errors in updating both forwarding rule and url map, forwarding rule error: %s, url map error: %s", frErr, umErr)
}

// PrintStatus prints the current status of the load balancer.
func (l *LoadBalancerSyncer) PrintStatus() (string, error) {
	status, err := l.getLoadBalancerStatus()
	if err != nil {
		return "", err
	}
	return formatLoadBalancerStatus(l.lbName, *status), nil
}

// getLoadBalancerStatus returns the current status of the load balancer.
func (l *LoadBalancerSyncer) getLoadBalancerStatus() (*status.LoadBalancerStatus, error) {
	// First try to fetch the status from url map.
	// If that fails, then we fetch it from forwarding rule.
	// This is because we first used to store the status on forwarding rules and then migrated to url maps.
	// https://github.com/GoogleCloudPlatform/k8s-multicluster-ingress/issues/145 has more details.
	umStatus, umErr := l.ums.GetLoadBalancerStatus(l.lbName)
	if umStatus != nil && umErr == nil {
		return umStatus, nil
	}
	if ingressutils.IsHTTPErrorCode(umErr, http.StatusNotFound) {
		return nil, fmt.Errorf("Load balancer %s does not exist", l.lbName)
	}
	// Try forwarding rule.
	frStatus, frErr := l.frs.GetLoadBalancerStatus(l.lbName)
	if frStatus != nil && frErr == nil {
		return frStatus, nil
	}
	if ingressutils.IsHTTPErrorCode(frErr, http.StatusNotFound) {
		return nil, fmt.Errorf("Load balancer %s does not exist", l.lbName)
	}
	// Failed to get status from both url map and forwarding rule.
	return nil, fmt.Errorf("failed to get status from both url map and forwarding rule. Error in extracting status from url map: %s. Error in extracting status from forwarding rule: %s", umErr, frErr)
}

// formatLoadBalancerStatus formats the given status to be printed for get-status output.
func formatLoadBalancerStatus(lbName string, s status.LoadBalancerStatus) string {
	return fmt.Sprintf("Load balancer %s has IPAddress %s and is spread across %d clusters (%s)", lbName, s.IPAddress, len(s.Clusters), strings.Join(s.Clusters, ","))
}

func formatLoadBalancersList(balancers []status.LoadBalancerStatus) (string, error) {
	if len(balancers) == 0 {
		return "No multicluster ingresses found.", nil
	}

	buf := bytes.NewBuffer([]byte{})
	out := printers.GetNewTabWriter(buf)
	columnNames := []string{"NAME", "IP", "CLUSTERS"}
	fmt.Fprintf(out, "%s\n", strings.Join(columnNames, "\t"))
	for _, lbStatus := range balancers {
		fmt.Fprintf(out, "%s\t%s\t%s\n", lbStatus.LoadBalancerName, lbStatus.IPAddress, strings.Join(lbStatus.Clusters, ", "))
	}
	out.Flush()
	return buf.String(), nil
}

func (l *LoadBalancerSyncer) ListLoadBalancers() (string, error) {
	var err error
	// We fetch the list from both url maps and forwarding rules and then merge them.
	// This is because we first used to store the status on forwarding rules and then migrated to url maps.
	// https://github.com/GoogleCloudPlatform/k8s-multicluster-ingress/issues/145 has more details.
	umBalancers, umErr := l.ums.ListLoadBalancerStatuses()
	if umErr != nil {
		// Aggregate errors and return all at the end.
		err = multierror.Append(err, umErr)
	}

	frBalancers, frErr := l.frs.ListLoadBalancerStatuses()
	if frErr != nil {
		// Aggregate errors and return all at the end.
		err = multierror.Append(err, frErr)
	}
	if err != nil {
		return "", err
	}
	// We merge the list of load balancers from both url map syncer and forwarding rule syncer.
	// We assume that every load balancer has status on either url map or forwarding rule and only on one of those.
	return formatLoadBalancersList(append(umBalancers, frBalancers...))
}

func (l *LoadBalancerSyncer) getIPAddress(ing *v1beta1.Ingress) (string, error) {
	key := annotations.StaticIPNameKey
	if ing.ObjectMeta.Annotations == nil || ing.ObjectMeta.Annotations[key] == "" {
		// TODO(nikhiljindal): Add logic to reserve a new IP address if user has not specified any.
		// If we do that then we should also add the new IP address as annotation to the ingress created in clusters.
		return "", fmt.Errorf("The annotation %v has not been specified. Multicluster ingresses require a pre-reserved static IP", key)
	}
	ipName := ing.ObjectMeta.Annotations[key]
	ip, err := l.ipp.GetGlobalAddress(ipName)
	if err != nil {
		return "", fmt.Errorf("unexpected error in getting ip address %s: %s", ipName, err)
	}
	return ip.Address, nil
}

// Returns links to all instance groups corresponding the given ingress for the given clients.
func (l *LoadBalancerSyncer) getIGs(ing *v1beta1.Ingress, clients map[string]kubeclient.Interface) (map[string][]string, error) {
	var err error
	igs := make(map[string][]string, len(l.clients))
	// Get instance groups from all clusters.
	for cluster, client := range clients {
		igsFromCluster, getIGsErr := getIGsForCluster(ing, client, cluster)
		if getIGsErr != nil {
			err = multierror.Append(err, getIGsErr)
			continue
		}
		igs[cluster] = igsFromCluster
	}
	return igs, err
}

// Returns named ports on an instance from the given list.
// Note that it picks an instance group at random and returns the named ports for that instance group, assuming that the named ports are same on all instance groups.
// Also note that this returns all named ports on the instance group and not just the ones relevant to the given ingress.
func (l *LoadBalancerSyncer) getNamedPorts(igs map[string][]string) (backendservice.NamedPortsMap, error) {
	if len(igs) == 0 {
		return backendservice.NamedPortsMap{}, fmt.Errorf("Cannot fetch named ports since instance groups list is empty")
	}

	// Pick an IG at random.
	var ig string
	for _, v := range igs {
		ig = v[0]
		break
	}
	return l.getNamedPortsForIG(ig)
}

// Returns the instance groups corresponding to the given cluster.
// It fetches the given ingress from the cluster and extracts the instance group annotations on it to get the list of instance groups.
func getIGsForCluster(ing *v1beta1.Ingress, client kubeclient.Interface, cluster string) ([]string, error) {
	fmt.Println("Determining instance groups for cluster", cluster)
	key := annotations.InstanceGroupsAnnotationKey
	// Keep trying until ingress gets the instance group annotation.
	// TODO(nikhiljindal): Check if we need exponential backoff.
	for try := true; try; time.Sleep(1 * time.Second) {
		// Fetch the ingress from a cluster.
		ing, err := getIng(ing.Name, ing.Namespace, client)
		if err != nil {
			return nil, fmt.Errorf("error %s in fetching ingress", err)
		}
		if ing.Annotations == nil || ing.Annotations[key] == "" {
			// keep trying
			fmt.Println("Waiting for ingress (", ing.Namespace, ":", ing.Name, ") to get", key, "annotation.....")
			glog.Infof("Waiting for annotation. Current annotation(s):%+v\n", ing.Annotations)
			continue
		}
		annotationValue := ing.Annotations[key]
		glog.V(3).Infof("Found instance groups annotation value: %s", annotationValue)
		// Get the instance group name.
		type InstanceGroupsAnnotationValue struct {
			Name string
			Zone string
		}
		var values []InstanceGroupsAnnotationValue
		if err := json.Unmarshal([]byte(annotationValue), &values); err != nil {
			return nil, fmt.Errorf("error in parsing annotation key %s, value %s: %s", key, annotationValue, err)
		}
		if len(values) == 0 {
			// keep trying
			fmt.Println("Waiting for ingress to get", key, "annotation...")
			continue
		}
		// Compute link to all instance groups.
		var igs []string
		for _, ig := range values {
			igs = append(igs, fmt.Sprintf("%s/instanceGroups/%s", ig.Zone, ig.Name))
		}
		return igs, nil
	}
	return nil, nil
}

func (l *LoadBalancerSyncer) getNamedPortsForIG(igUrl string) (backendservice.NamedPortsMap, error) {
	zone, name, err := utils.GetZoneAndNameFromIGUrl(igUrl)
	if err != nil {
		return nil, err
	}
	fmt.Println("Fetching instance group:", zone, name)
	ig, err := l.igp.GetInstanceGroup(name, zone)
	if err != nil {
		return nil, err
	}
	fmt.Printf("Fetched instance group: %s/%s, got named ports: ", zone, name)
	namedPorts := backendservice.NamedPortsMap{}
	for _, np := range ig.NamedPorts {
		fmt.Printf("port: %+v ", np)
		namedPorts[np.Port] = np
	}
	fmt.Printf("\n")
	return namedPorts, nil
}
func (l *LoadBalancerSyncer) ingToNodePorts(ing *v1beta1.Ingress, client kubeclient.Interface) []ingressbe.ServicePort {
	var knownPorts []ingressbe.ServicePort
	defaultBackend := ing.Spec.Backend
	if defaultBackend != nil {
		port, err := kubeutils.GetServiceNodePort(*defaultBackend, ing.Namespace, client)
		if err != nil {
			fmt.Println("error getting service nodeport:", err, ". Ignoring.")
		} else {
			knownPorts = append(knownPorts, port)
		}
	}
	for _, rule := range ing.Spec.Rules {
		if rule.HTTP == nil {
			glog.Warningf("ignoring non http Ingress rule: %v", rule)
			continue
		}
		for _, path := range rule.HTTP.Paths {
			port, err := kubeutils.GetServiceNodePort(path.Backend, ing.Namespace, client)
			if err != nil {
				fmt.Println("error getting service nodeport:", err, ". Ignoring.")
				continue
			}
			knownPorts = append(knownPorts, port)
		}
	}
	return knownPorts
}

func getIng(ingName, nsName string, client kubeclient.Interface) (*v1beta1.Ingress, error) {
	return client.ExtensionsV1beta1().Ingresses(nsName).Get(ingName, metav1.GetOptions{})
}

// Returns a client at random from the given map of clients.
// Returns an error if the map is empty.
func getAnyClient(clients map[string]kubeclient.Interface) (kubeclient.Interface, error) {
	if len(clients) == 0 {
		return nil, fmt.Errorf("could not find client to send requests to kubernetes cluster")
	}
	// Return the client for any cluster.
	for k := range clients {
		glog.V(2).Infof("getAnyClient: using client for cluster %s", k)
		return clients[k], nil
	}
	return nil, nil
}
