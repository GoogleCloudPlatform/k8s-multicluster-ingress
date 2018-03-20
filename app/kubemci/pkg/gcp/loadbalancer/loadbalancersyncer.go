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
	"strings"
	"time"

	"github.com/golang/glog"
	multierror "github.com/hashicorp/go-multierror"
	"k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
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
	ums urlmap.URLMapSyncerInterface
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

// ServicesNodePortsSame checks that for each backend/service, the services are
// all listening on the same NodePort.
func (l *LoadBalancerSyncer) ServicesNodePortsSame(clients map[string]kubeclient.Interface, ing *v1beta1.Ingress) error {
	for _, rule := range ing.Spec.Rules {
		if rule.HTTP == nil {
			glog.Errorf("ignoring non http Ingress rule")
			continue
		}
		for _, path := range rule.HTTP.Paths {
			glog.Infof("Validating path:%s", path.Path)
			if err := l.NodePortSameInAllClusters(path.Backend, ing.Namespace); err != nil {
				return fmt.Errorf("NodePort validation error for service '%s/%s': %s", ing.Namespace, path.Backend.ServiceName, err)
			}

		}
	}
	glog.Infof("Checking default backend's nodeports.")

	if ing.Spec.Backend != nil {
		if err := l.NodePortSameInAllClusters(*ing.Spec.Backend, ing.Namespace); err != nil {
			glog.Errorf("Node port mismatch on default service: %s", err)
			return err
		}
		glog.Infof("Default backend's nodeports passed validation.")
	} else {
		return fmt.Errorf("Ingress Spec is missing default backend")
	}
	return nil
}

// NodePortSameInAllClusters checks that the given backend's service is running
// on the same NodePort in all clusters (defined by l.Clients).
func (l *LoadBalancerSyncer) NodePortSameInAllClusters(backend v1beta1.IngressBackend, namespace string) error {
	var node_port int64 = -1
	var first_cluster_name string
	for client_name, client := range l.clients {
		glog.Infof("Checking client/cluster: %s", client_name)

		service_port, err := l.getServiceNodePort(backend, namespace, client)
		if err != nil {
			glog.Errorf("Could not get service NodePort in cluster %s: %s", client_name, err)
			return err
		}
		glog.Infof("Service's servicePort: %+v", service_port)
		// The NodePort is stored in 'Port' by getServiceNodePort.
		cluster_node_port := service_port.Port

		if node_port == -1 {
			node_port = cluster_node_port
			first_cluster_name = client_name
			continue
		}
		if cluster_node_port != node_port {
			return fmt.Errorf("some Services (e.g. in '%s') are on NodePort %v, but '%s' is on %v. All clusters must use same NodePort",
				first_cluster_name, node_port, client_name, cluster_node_port)
		}
	}
	return nil
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
		if err := l.ServicesNodePortsSame(l.clients, ing); err != nil {
			glog.Errorf("Validation of service NodePorts failed: %s", err)
			return err
		}
		glog.Infof("Validation of NodePorts passed.")
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
	umLink, umErr := l.ums.EnsureURLMap(l.lbName, ing, backendServices, forceUpdate)
	if umErr != nil {
		// Aggregate errors and return all at the end.
		umErr = fmt.Errorf("Error ensuring urlmap for %s: %s", l.lbName, umErr)
		fmt.Println(umErr)
		err = multierror.Append(err, umErr)
	}
	ingAnnotations := annotations.IngAnnotations(ing.ObjectMeta.Annotations)
	// Configure HTTP target proxy and forwarding rule, if required.
	if ingAnnotations.AllowHTTP() {
		tpLink, tpErr := l.tps.EnsureHttpTargetProxy(l.lbName, umLink, forceUpdate)
		if tpErr != nil {
			// Aggregate errors and return all at the end.
			tpErr = fmt.Errorf("Error ensuring HTTP target proxy: %s", tpErr)
			fmt.Println(tpErr)
			err = multierror.Append(err, tpErr)
		}
		frErr := l.frs.EnsureHttpForwardingRule(l.lbName, ipAddr, tpLink, clusters, forceUpdate)
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
		frErr := l.frs.EnsureHttpsForwardingRule(l.lbName, ipAddr, tpLink, clusters, forceUpdate)
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
// TODO(nikhiljindal): Do not require the ingress yaml from users. Just the name should be enough. We can fetch ingress YAML from one of the clusters.
func (l *LoadBalancerSyncer) DeleteLoadBalancer(ing *v1beta1.Ingress) error {
	client, cErr := getAnyClient(l.clients)
	if cErr != nil {
		// No point in continuing without a client.
		return cErr
	}
	ports := l.ingToNodePorts(ing, client)
	var err error
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
	if beErr := l.bss.DeleteBackendServices(ports); beErr != nil {
		// Aggregate errors and return all at the end.
		err = multierror.Append(err, beErr)
	}
	if hcErr := l.hcs.DeleteHealthChecks(ports); hcErr != nil {
		// Aggregate errors and return all at the end.
		err = multierror.Append(err, hcErr)
	}
	return err
}

// DeleteLoadBalancer deletes the GCP resources associated with the L7 GCP load balancer for the given ingress.
// TODO(nikhiljindal): Do not require the ingress yaml from users. Just the name should be enough. We can fetch ingress YAML from one of the clusters.
func (l *LoadBalancerSyncer) RemoveFromClusters(ing *v1beta1.Ingress, removeClients map[string]kubeclient.Interface, forceUpdate bool) error {
	client, cErr := getAnyClient(l.clients)
	if cErr != nil {
		// No point in continuing without a client.
		return cErr
	}
	var err error
	ports := l.ingToNodePorts(ing, client)
	removeIGLinks, igErr := l.getIGs(ing, removeClients)
	if igErr != nil {
		return multierror.Append(err, igErr)
	}
	// Can not really update any resource without igs. No point continuing.
	// Note: User can run into this if they are trying to remove their already deleted clusters.
	// TODO: Allow them to proceed to clean up whatever they can by using --force.
	if len(removeIGLinks) == 0 {
		err := multierror.Append(err, fmt.Errorf("No instance group found. Can not continue"))
		return err
	}

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

	// Convert the client map to an array of cluster names.
	removeClusters := make([]string, len(removeClients))
	removeIndex := 0
	for k := range removeClients {
		removeClusters[removeIndex] = k
		removeIndex++
	}
	// Update the forwarding rule status at the end.
	if frErr := l.frs.RemoveClustersFromStatus(removeClusters); frErr != nil {
		// Aggregate errors and return all at the end.
		err = multierror.Append(err, frErr)
	}

	return err
}

// PrintStatus prints the current status of the load balancer.
func (l *LoadBalancerSyncer) PrintStatus() (string, error) {
	sd, err := l.frs.GetLoadBalancerStatus(l.lbName)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Load balancer %s has IPAddress %s and is spread across %d clusters (%s)", l.lbName, sd.IPAddress, len(sd.Clusters), strings.Join(sd.Clusters, ",")), nil
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
	balancers, err := l.frs.ListLoadBalancerStatuses()
	if err != nil {
		return "", err
	}
	return formatLoadBalancersList(balancers)

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
		port, err := l.getServiceNodePort(*defaultBackend, ing.Namespace, client)
		if err != nil {
			glog.Errorf("%v", err)
		} else {
			knownPorts = append(knownPorts, port)
		}
	}
	for _, rule := range ing.Spec.Rules {
		if rule.HTTP == nil {
			glog.Errorf("ignoring non http Ingress rule")
			continue
		}
		for _, path := range rule.HTTP.Paths {
			port, err := l.getServiceNodePort(path.Backend, ing.Namespace, client)
			if err != nil {
				glog.Errorf("%v", err)
				continue
			}
			knownPorts = append(knownPorts, port)
		}
	}
	return knownPorts
}

// getServiceNodePort takes an IngressBackend and returns a corresponding ServicePort
func (l *LoadBalancerSyncer) getServiceNodePort(be v1beta1.IngressBackend, namespace string, client kubeclient.Interface) (ingressbe.ServicePort, error) {
	svc, err := getSvc(be.ServiceName, namespace, client)
	// Refactor this code to get serviceport from a given service and share it with kubernetes/ingress.
	appProtocols, err := annotations.SvcAnnotations(svc.GetAnnotations()).ApplicationProtocols()
	if err != nil {
		return ingressbe.ServicePort{}, err
	}

	var port *v1.ServicePort
PortLoop:
	for _, p := range svc.Spec.Ports {
		np := p
		switch be.ServicePort.Type {
		case intstr.Int:
			if p.Port == be.ServicePort.IntVal {
				port = &np
				break PortLoop
			}
		default:
			if p.Name == be.ServicePort.StrVal {
				port = &np
				break PortLoop
			}
		}
	}

	if port == nil {
		return ingressbe.ServicePort{}, fmt.Errorf("could not find matching nodeport for backend %+v and service %s/%s. Looking for port %+v in %v", be, namespace, be.ServiceName, be.ServicePort, svc.Spec.Ports)
	}

	proto := ingressutils.ProtocolHTTP
	if protoStr, exists := appProtocols[port.Name]; exists {
		glog.Infof("changing protocol to %q", protoStr)
		proto = ingressutils.AppProtocol(protoStr)
	}

	p := ingressbe.ServicePort{
		Port:     int64(port.NodePort),
		Protocol: proto,
		SvcName:  types.NamespacedName{Namespace: namespace, Name: be.ServiceName},
		SvcPort:  be.ServicePort,
	}
	glog.Infof("Found ServicePort: %+v", p)
	return p, nil
}

func getSvc(svcName, nsName string, client kubeclient.Interface) (*v1.Service, error) {
	return client.CoreV1().Services(nsName).Get(svcName, metav1.GetOptions{})
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
		glog.Infof("getAnyClient: using client for cluster %s", k)
		return clients[k], nil
	}
	return nil, nil
}
