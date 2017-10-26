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
	"github.com/hashicorp/go-multierror"
	"k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	kubeclient "k8s.io/client-go/kubernetes"
	ingressbe "k8s.io/ingress-gce/pkg/backends"
	ingressig "k8s.io/ingress-gce/pkg/instances"
	ingresslb "k8s.io/ingress-gce/pkg/loadbalancers"
	ingressutils "k8s.io/ingress-gce/pkg/utils"
	"k8s.io/kubernetes/pkg/cloudprovider/providers/gce"

	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/mci/pkg/gcp/backendservice"
	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/mci/pkg/gcp/forwardingrule"
	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/mci/pkg/gcp/healthcheck"
	utilsnamer "github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/mci/pkg/gcp/namer"
	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/mci/pkg/gcp/targetproxy"
	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/mci/pkg/gcp/urlmap"
)

const (
	// TODO(nikhiljindal): refactor code in kubernetes/ingress to reuse this.
	// serviceApplicationProtocolKey is a stringified JSON map of port names to
	// protocol names. Possible values are HTTP, HTTPS.
	// Example annotation:
	// {
	//   "service.alpha.kubernetes.io/app-protocols" : '{"my-https-port":"HTTPS","my-http-port":"HTTP"}'
	// }
	serviceApplicationProtocolKey = "service.alpha.kubernetes.io/app-protocols"

	// Prefix used by the namer to generate names.
	// This is used to identify resources created by this code.
	mciPrefix = "mci1"

	// instanceGroupsAnnotationKey is the annotation key used by gclb ingress
	// controller to specify the name and zone of instance groups created
	// for the ingress.
	// TODO(nikhiljindal): Refactor the code in kubernetes/ingress-gce to
	// export it and then reuse it here.
	instanceGroupsAnnotationKey = "ingress.gcp.kubernetes.io/instance-groups"

	// Annotation key for user to specify the static IP to be used by this load balancer.
	// TODO(nikhiljindal): Share this key with kubernetes/ingress-gce
	staticIPNameKey = "kubernetes.io/ingress.global-static-ip-name"
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
	// kubernetes client to send requests to kubernetes apiserver.
	client kubeclient.Interface
	// Instance groups provider to call GCE APIs to manage GCE instance groups.
	igp ingressig.InstanceGroups
	// LoadBalancers interface to manage IP addresses.
	ipp ingresslb.LoadBalancers
}

func NewLoadBalancerSyncer(lbName string, client kubeclient.Interface, cloud *gce.GCECloud) *LoadBalancerSyncer {
	namer := utilsnamer.NewNamer(mciPrefix, lbName)
	return &LoadBalancerSyncer{
		lbName: lbName,
		hcs:    healthcheck.NewHealthCheckSyncer(namer, cloud),
		bss:    backendservice.NewBackendServiceSyncer(namer, cloud),
		ums:    urlmap.NewURLMapSyncer(namer, cloud),
		tps:    targetproxy.NewTargetProxySyncer(namer, cloud),
		frs:    forwardingrule.NewForwardingRuleSyncer(namer, cloud),
		client: client,
		igp:    cloud,
		ipp:    cloud,
	}
}

// CreateLoadBalancer creates the GCP resources necessary for an L7 GCP load balancer corresponding to the given ingress.
func (l *LoadBalancerSyncer) CreateLoadBalancer(ing *v1beta1.Ingress, forceUpdate bool) error {
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
	tpLink, tpErr := l.tps.EnsureTargetProxy(l.lbName, umLink)
	if tpErr != nil {
		// Aggregate errors and return all at the end.
		err = multierror.Append(err, tpErr)
	}
	frErr := l.frs.EnsureForwardingRule(l.lbName, ipAddr, tpLink)
	if frErr != nil {
		// Aggregate errors and return all at the end.
		err = multierror.Append(err, frErr)
	}
	return err
}

func (l *LoadBalancerSyncer) getIPAddress(ing *v1beta1.Ingress) (string, error) {
	if ing.ObjectMeta.Annotations == nil || ing.ObjectMeta.Annotations[staticIPNameKey] == "" {
		// TODO(nikhiljindal): Add logic to reserve a new IP address if user has not specified any.
		// If we do that then we should also add the new IP address as annotation to the ingress created in clusters.
		return "", fmt.Errorf("No static ip specified. Multicluster ingresses require a pre-reserved static IP, which can be specified using %s annotation", staticIPNameKey)
	}
	ipName := ing.ObjectMeta.Annotations[staticIPNameKey]
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
	// Keep trying until ingress gets the instance group annotation.
	// TODO(nikhiljindal): Check if we need exponential backoff.
	for try := true; try; time.Sleep(1 * time.Second) {
		// Fetch the ingress from a cluster.
		ing, err := l.getIng(ing.Name, ing.Namespace)
		if err != nil {
			return nil, nil, fmt.Errorf("error %s in fetching ingress", err)
		}
		if ing.Annotations == nil || ing.Annotations[instanceGroupsAnnotationKey] == "" {
			// keep trying
			fmt.Println("Waiting for ingress to get", instanceGroupsAnnotationKey, "annotation.....")
			continue
		}
		annotationValue := ing.Annotations[instanceGroupsAnnotationKey]
		glog.V(3).Infof("Found instance groups annotation value: %s", annotationValue)
		// Get the instance group name.
		type InstanceGroupsAnnotationValue struct {
			Name string
			Zone string
		}
		var values []InstanceGroupsAnnotationValue
		if err := json.Unmarshal([]byte(annotationValue), &values); err != nil {
			return nil, nil, fmt.Errorf("error in parsing annotation key %s, value %s: %s", instanceGroupsAnnotationKey, annotationValue, err)
		}
		if len(values) == 0 {
			// keep trying
			fmt.Printf("Waiting for ingress to get", instanceGroupsAnnotationKey, "annotation.....")
			continue
		}
		// Compute link to all instance groups.
		var igs []string
		for _, ig := range values {
			igs = append(igs, fmt.Sprintf("%s/instanceGroups/%s", ig.Zone, ig.Name))
		}
		// Compute named ports from the first instance group.
		zone := getZoneFromURL(values[0].Zone)
		ig, err := l.igp.GetInstanceGroup(values[0].Name, zone)
		if err != nil {
			return nil, nil, err
		}
		fmt.Printf("Fetched instance group: %s/%s, got named ports: %#v", zone, values[0].Name, ig.NamedPorts)
		namedPorts := backendservice.NamedPortsMap{}
		for _, np := range ig.NamedPorts {
			namedPorts[np.Port] = np
		}
		return igs, namedPorts, nil
	}
	return nil, nil, nil
}

// Returns the zone from a GCP Zone URL.
func getZoneFromURL(zoneURL string) string {
	// Split the string by "/" and return the last element.
	components := strings.Split(zoneURL, "/")
	return components[len(components)-1]
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

func svcProtocolsFromAnnotation(annotations map[string]string) (map[string]string, error) {
	val, ok := annotations[serviceApplicationProtocolKey]
	if !ok {
		return map[string]string{}, nil
	}

	var svcProtocols map[string]string
	if err := json.Unmarshal([]byte(val), &svcProtocols); err != nil {
		return nil, fmt.Errorf("error in parsing annotation with key %s, error: %v", serviceApplicationProtocolKey, err)
	}
	return svcProtocols, nil
}

func (l *LoadBalancerSyncer) getServiceNodePort(be v1beta1.IngressBackend, namespace string) (ingressbe.ServicePort, error) {
	svc, err := l.getSvc(be.ServiceName, namespace)
	// Refactor this code to get serviceport from a given service and share it with kubernetes/ingress.
	appProtocols, err := svcProtocolsFromAnnotation(svc.GetAnnotations())
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
	return l.client.CoreV1().Services(nsName).Get(svcName, metav1.GetOptions{})
}

func (l *LoadBalancerSyncer) getIng(ingName, nsName string) (*v1beta1.Ingress, error) {
	return l.client.ExtensionsV1beta1().Ingresses(nsName).Get(ingName, metav1.GetOptions{})
}
