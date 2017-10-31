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

	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/backendservice"
	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/forwardingrule"
	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/healthcheck"
	utilsnamer "github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/namer"
	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/targetproxy"
	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/urlmap"
)

const (
	// Prefix used by the namer to generate names.
	// This is used to identify resources created by this code.
	mciPrefix = "mci1"
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
	// Kubernetes clients to send requests to kubernetes apiservers.
	// This is a map from cluster name to the client for that cluster.
	clients map[string]kubeclient.Interface
	// Instance groups provider to call GCE APIs to manage GCE instance groups.
	igp ingressig.InstanceGroups
	// IP addresses provider to manage GCP IP addresses.
	// We do not have a specific ip address provider interface, so we use the larger LoadBalancers interface.
	ipp ingresslb.LoadBalancers
}

func NewLoadBalancerSyncer(lbName string, clients map[string]kubeclient.Interface, cloud *gce.GCECloud) *LoadBalancerSyncer {
	namer := utilsnamer.NewNamer(mciPrefix, lbName)
	return &LoadBalancerSyncer{
		lbName:  lbName,
		hcs:     healthcheck.NewHealthCheckSyncer(namer, cloud),
		bss:     backendservice.NewBackendServiceSyncer(namer, cloud),
		ums:     urlmap.NewURLMapSyncer(namer, cloud),
		tps:     targetproxy.NewTargetProxySyncer(namer, cloud),
		frs:     forwardingrule.NewForwardingRuleSyncer(namer, cloud),
		clients: clients,
		igp:     cloud,
		ipp:     cloud,
	}
}

// CreateLoadBalancer creates the GCP resources necessary for an L7 GCP load balancer corresponding to the given ingress.
// clusters is the list of clusters that this load balancer is spread to.
func (l *LoadBalancerSyncer) CreateLoadBalancer(ing *v1beta1.Ingress, forceUpdate bool, clusters []string) error {
	var err error
	ports := l.ingToNodePorts(ing)
	ipAddr, err := l.getIPAddress(ing)
	if err != nil {
		// No point continuing if the user has not specified a valid ip address.
		return err
	}
	// Create health checks to be used by the backend service.
	healthChecks, hcErr := l.hcs.EnsureHealthCheck(l.lbName, ports, forceUpdate)
	if hcErr != nil {
		// Keep aggregating errors and return all at the end, rather than giving up on the first error.
		err = multierror.Append(err, hcErr)
	}
	igs, namedPorts, igErr := l.getIGsAndNamedPorts(ing)
	// Cant really create any backend service without named ports. No point continuing.
	if igErr != nil {
		return multierror.Append(err, igErr)
	}
	// Create backend service. This should always be called after the health check since backend service needs to point to the health check.
	backendServices, beErr := l.bss.EnsureBackendService(l.lbName, ports, healthChecks, namedPorts, igs)
	if beErr != nil {
		// Aggregate errors and return all at the end.
		err = multierror.Append(err, beErr)
	}
	umLink, umErr := l.ums.EnsureURLMap(l.lbName, ing, backendServices)
	if umErr != nil {
		// Aggregate errors and return all at the end.
		err = multierror.Append(err, umErr)
	}
	ingAnnotations := annotations.IngAnnotations(ing.ObjectMeta.Annotations)
	// Configure HTTP target proxy and forwarding rule, if required.
	if ingAnnotations.AllowHTTP() {
		tpLink, tpErr := l.tps.EnsureHttpTargetProxy(l.lbName, umLink)
		if tpErr != nil {
			// Aggregate errors and return all at the end.
			err = multierror.Append(err, tpErr)
		}
		frErr := l.frs.EnsureHttpForwardingRule(l.lbName, ipAddr, tpLink, clusters)
		if frErr != nil {
			// Aggregate errors and return all at the end.
			err = multierror.Append(err, frErr)
		}
	}
	// Configure HTTPS target proxy and forwarding rule, if required.
	if (ingAnnotations.UseNamedTLS() != "") || len(ing.Spec.TLS) > 0 {
		// TODO(nikhiljindal): Support creating https target proxy and forwarding rule.
		err = multierror.Append(err, fmt.Errorf("This tool does not support creating HTTPS target proxy and forwarding rule yet."))
	}
	return err
}

// DeleteLoadBalancer deletes the GCP resources associated with the L7 GCP load balancer for the given ingress.
func (l *LoadBalancerSyncer) DeleteLoadBalancer(ing *v1beta1.Ingress) error {
	// TODO(nikhiljindal): Dont require the ingress yaml from users. Just the name should be enough. We can fetch ingress YAML from one of the clusters.
	var err error
	ports := l.ingToNodePorts(ing)
	// We delete resources in the reverse order in which they were created.
	// For ex: we create health checks before creating backend services (so that backend service can point to the health check),
	// but delete backend service before deleting health check. We cannot delete the health check when backend service is still pointing to it.
	if frErr := l.frs.DeleteForwardingRules(); frErr != nil {
		// Aggregate errors and return all at the end.
		err = multierror.Append(err, frErr)
	}
	if tpErr := l.tps.DeleteTargetProxies(); tpErr != nil {
		// Aggregate errors and return all at the end.
		err = multierror.Append(err, tpErr)
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

// PrintStatus prints the current status of the load balancer.
func (l *LoadBalancerSyncer) PrintStatus() (string, error) {
	sd, err := l.frs.GetLoadBalancerStatus(l.lbName)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Load balancer %s has IPAddress %s and is spread across %d clusters (%s)", l.lbName, sd.IPAddress, len(sd.Clusters), strings.Join(sd.Clusters, ",")), nil
}

func (l *LoadBalancerSyncer) getIPAddress(ing *v1beta1.Ingress) (string, error) {
	key := annotations.StaticIPNameKey
	if ing.ObjectMeta.Annotations == nil || ing.ObjectMeta.Annotations[key] == "" {
		// TODO(nikhiljindal): Add logic to reserve a new IP address if user has not specified any.
		// If we do that then we should also add the new IP address as annotation to the ingress created in clusters.
		return "", fmt.Errorf("No static ip specified. Multicluster ingresses require a pre-reserved static IP, which can be specified using %s annotation", key)
	}
	ipName := ing.ObjectMeta.Annotations[key]
	ip, err := l.ipp.GetGlobalAddress(ipName)
	if err != nil {
		return "", fmt.Errorf("unexpected error in getting ip address %s: %s", ipName, err)
	}
	return ip.Address, nil
}

// Returns links to all instance groups and named ports on any one of them.
// Note that it picks an instance group at random and returns the named ports for that instance group, assuming that the named ports are same on all instance groups.
// Also note that this returns all named ports on the instance group and not just the ones relevant to the given ingress.
func (l *LoadBalancerSyncer) getIGsAndNamedPorts(ing *v1beta1.Ingress) ([]string, backendservice.NamedPortsMap, error) {
	var err error
	var igs []string
	// Get instance groups from all clusters.
	for cluster, client := range l.clients {
		igsFromCluster, getIGsErr := l.getIGs(ing, client, cluster)
		if getIGsErr != nil {
			err = multierror.Append(err, getIGsErr)
			continue
		}
		igs = append(igs, igsFromCluster...)
	}
	if len(igs) == 0 {
		err = multierror.Append(err, fmt.Errorf("Cannot fetch named ports since fetching instance groups failed"))
		return nil, backendservice.NamedPortsMap{}, err
	}
	// Compute named ports from the first instance group.
	namedPorts, namedPortsErr := l.getNamedPorts(igs[0])
	if namedPortsErr != nil {
		err = multierror.Append(err, namedPortsErr)
	}
	return igs, namedPorts, err
}

func (l *LoadBalancerSyncer) getIGs(ing *v1beta1.Ingress, client kubeclient.Interface, cluster string) ([]string, error) {
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

func (l *LoadBalancerSyncer) getNamedPorts(igUrl string) (backendservice.NamedPortsMap, error) {
	zone, name, err := getZoneAndNameFromIGUrl(igUrl)
	if err != nil {
		return nil, err
	}
	fmt.Println("Fetching instance group:", name, zone)
	ig, err := l.igp.GetInstanceGroup(name, zone)
	if err != nil {
		return nil, err
	}
	fmt.Println("Fetched instance group:", zone, "/", name, "got named ports:", ig.NamedPorts)
	namedPorts := backendservice.NamedPortsMap{}
	for _, np := range ig.NamedPorts {
		namedPorts[np.Port] = np
	}
	return namedPorts, nil
}

// Returns the zone from a GCP Zone URL.
func getZoneAndNameFromIGUrl(igUrl string) (string, string, error) {
	// Split the string by "/instanceGroups/".
	components := strings.Split(igUrl, "/instanceGroups/")
	if len(components) != 2 {
		return "", "", fmt.Errorf("Error in parsing instance groups URL: %s, expected it to contain /instanceGroups", igUrl)
	}
	zoneUrl := components[0]
	name := components[1]
	// To get zone name from zone Url, split the string by "/" and use the last element.
	components = strings.Split(zoneUrl, "/")
	if len(components) == 0 {
		return "", "", fmt.Errorf("Error in parsing instance groups URL: %s, expected it to contain zone Url")
	}
	return components[len(components)-1], name, nil
}

func (l *LoadBalancerSyncer) ingToNodePorts(ing *v1beta1.Ingress) []ingressbe.ServicePort {
	var knownPorts []ingressbe.ServicePort
	defaultBackend := ing.Spec.Backend
	if defaultBackend != nil {
		port, err := l.getServiceNodePort(*defaultBackend, ing.Namespace)
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
			port, err := l.getServiceNodePort(path.Backend, ing.Namespace)
			if err != nil {
				glog.Errorf("%v", err)
				continue
			}
			knownPorts = append(knownPorts, port)
		}
	}
	return knownPorts
}

func (l *LoadBalancerSyncer) getServiceNodePort(be v1beta1.IngressBackend, namespace string) (ingressbe.ServicePort, error) {
	svc, err := l.getSvc(be.ServiceName, namespace)
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
		return ingressbe.ServicePort{}, fmt.Errorf("could not find matching nodeport for backend %v and service %s/%s", be, be.ServiceName, namespace)
	}

	proto := ingressutils.ProtocolHTTP
	if protoStr, exists := appProtocols[port.Name]; exists {
		proto = ingressutils.AppProtocol(protoStr)
	}

	p := ingressbe.ServicePort{
		Port:     int64(port.NodePort),
		Protocol: proto,
		SvcName:  types.NamespacedName{Namespace: namespace, Name: be.ServiceName},
		SvcPort:  be.ServicePort,
	}
	return p, nil
}

func (l *LoadBalancerSyncer) getSvc(svcName, nsName string) (*v1.Service, error) {
	client, err := getClient(l.clients)
	if err != nil {
		return nil, err
	}
	return client.CoreV1().Services(nsName).Get(svcName, metav1.GetOptions{})
}

func getIng(ingName, nsName string, client kubeclient.Interface) (*v1beta1.Ingress, error) {
	return client.ExtensionsV1beta1().Ingresses(nsName).Get(ingName, metav1.GetOptions{})
}

// Returns a client at random from the given map of clients.
// Returns an error if the map is empty.
func getClient(clients map[string]kubeclient.Interface) (kubeclient.Interface, error) {
	if len(clients) == 0 {
		return nil, fmt.Errorf("could not find client to send requests to kubernetes cluster")
	}
	// Return the client for any cluster.
	for k, _ := range clients {
		return clients[k], nil
	}
	return nil, nil
}
