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
	"math/rand"
	"reflect"
	"testing"

	"google.golang.org/api/compute/v1"
	"k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	kubeclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
	"k8s.io/ingress-gce/pkg/annotations"
	ingressig "k8s.io/ingress-gce/pkg/instances"
	ingresslb "k8s.io/ingress-gce/pkg/loadbalancers"

	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/backendservice"
	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/firewallrule"
	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/forwardingrule"
	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/healthcheck"
	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/sslcert"
	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/status"
	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/targetproxy"
	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/urlmap"
	"github.com/golang/glog"
)

func newSyncer(lbName string) *Syncer {
	return &Syncer{
		lbName: lbName,
		hcs:    healthcheck.NewFakeHealthCheckSyncer(),
		bss:    backendservice.NewFakeBackendServiceSyncer(),
		ums:    urlmap.NewFakeURLMapSyncer(),
		tps:    targetproxy.NewFakeTargetProxySyncer(),
		frs:    forwardingrule.NewFakeForwardingRuleSyncer(),
		fws:    firewallrule.NewFakeFirewallRuleSyncer(),
		scs:    sslcert.NewFakeSSLCertSyncer(),
		clients: map[string]kubeclient.Interface{
			"cluster1": &fake.Clientset{},
			"cluster2": &fake.Clientset{},
		},
		igp: ingressig.NewFakeInstanceGroups(nil /*nodes*/, nil /*namer*/),
		ipp: ingresslb.NewFakeLoadBalancers("" /*name*/, nil /*namer*/),
	}
}

// TODO(nikhiljindal): Add a https ingress test.

func TestCreateLoadBalancer(t *testing.T) {
	lbName := "lb-name"
	nodePort := int64(32211)
	igName := "my-fake-ig"
	igZone := "my-fake-zone"
	zoneLink := fmt.Sprintf("https://www.googleapis.com/compute/v1/projects/fake-project/zones/%s", igZone)
	clusters := []string{"cluster1", "cluster2"}
	lbc := newSyncer(lbName)
	ipAddress := &compute.Address{
		Name:    "ipAddressName",
		Address: "1.2.3.4",
	}
	// Reserve a global address. User is supposed to do this before calling CreateLoadBalancer.
	lbc.ipp.ReserveGlobalAddress(ipAddress)
	ing, err := setupLBCForCreateIng(lbc, nodePort, false /*randNodePort*/, igName, igZone, zoneLink, ipAddress)
	if err != nil {
		t.Fatalf("unexpected error in test setup: %s", err)
	}
	if err := lbc.CreateLoadBalancer(ing, true /*forceUpdate*/, true /*validate*/, clusters); err != nil {
		t.Fatalf("unexpected error %s", err)
	}
	// Verify client actions.
	fetchedSvc := false
	for k, v := range lbc.clients {
		client := v.(*fake.Clientset)
		actions := client.Actions()
		// Validation (same for each cluster):
		//   1x GET-Service for path=/foo.
		//   1x GET-Service for default backend.
		// Start of setup (using a random cluster's client):
		//   1x GET-Service for default backend.
		//   1x GET-Service for path=/foo.
		// The last action should always be GET-Ingress, once for each cluster.
		// This is how we get 3 actions for 1 cluster and 5 actions for the other.
		fetchedSvc = fetchedSvc || len(actions) == 3
		if !(len(actions) == 1 || len(actions) == 3) {
			t.Errorf("Bad number of actions for cluster '%s'. Expected 3 or 5. Got %d: %v", k, len(actions), actions)
		} else {
			for i := 0; i < len(actions)-1; i++ {
				getSvcName := actions[i].(core.GetAction).GetName()
				if getSvcName != "my-svc" {
					t.Errorf("unexpected get for %s, expected: my-svc", getSvcName)
				}
			}
			getIngName := actions[len(actions)-1].(core.GetAction).GetName()
			if getIngName != ing.Name {
				t.Errorf("unexpected get for %s, expected: %s", getIngName, ing.Name)
			}
		}
	}
	if !fetchedSvc {
		t.Errorf("None of the clients were used to fetch the service")
	}
	// Verify that the expected healthcheck was created.
	fhc := lbc.hcs.(*healthcheck.FakeHealthCheckSyncer)
	if len(fhc.EnsuredHealthChecks) != 2 {
		t.Errorf("unexpected number of health checks. expected: 2, got: %d: %+v", len(fhc.EnsuredHealthChecks),
			fhc.EnsuredHealthChecks)
	}
	hc := fhc.EnsuredHealthChecks[0]
	if hc.LBName != lbName || hc.Port.NodePort != nodePort {
		t.Errorf("unexpected health check: %v\nexpected: lbname: %s, port: %d", hc, lbName, nodePort)
	}
	// Verify that the expected backend service was created.
	fbs := lbc.bss.(*backendservice.FakeBackendServiceSyncer)
	if len(fbs.EnsuredBackendServices) != 2 {
		t.Errorf("unexpected number of backend services. expected: 2, got: %d", len(fbs.EnsuredBackendServices))
	}
	bs := fbs.EnsuredBackendServices[0]
	if bs.LBName != lbName {
		t.Errorf("unexpected lb name in backend service. expected: %s, got: %s", lbName, bs.LBName)
	}
	if bs.Port.NodePort != nodePort {
		t.Errorf("unexpected port in backend service. expected port %d, got: %d", nodePort, bs.Port.NodePort)
	}
	if len(bs.HCMap) != 1 || bs.HCMap[nodePort] == nil {
		t.Errorf("unexpected health check map in backend service. expected an entry for port %d, got: %v", nodePort, bs.HCMap)
	}
	if len(bs.NPMap) != 1 || bs.NPMap[nodePort] == nil {
		t.Errorf("unexpected node port map in backend service. expected an entry for port %d, got: %v", nodePort, bs.NPMap)
	}
	// Verify that the instance group from both clusters (same in this case) is added to the backend service.
	expectedIGLink := fmt.Sprintf("%s/instanceGroups/%s", zoneLink, igName)
	if len(bs.IGLinks) != 2 || bs.IGLinks[0] != expectedIGLink || bs.IGLinks[1] != expectedIGLink {
		t.Errorf("unexpected instance group in backend service. expected 2 links %s, got: %s", expectedIGLink, bs.IGLinks)
	}
	// Verify that the expected urlmap was created.
	fum := lbc.ums.(*urlmap.FakeURLMapSyncer)
	if len(fum.EnsuredURLMaps) != 1 {
		t.Fatalf("unexpected number of url maps. expected: %d, got: %d", 1, len(fum.EnsuredURLMaps))
	}
	um := fum.EnsuredURLMaps[0]
	if hc.LBName != lbName {
		t.Errorf("unexpected url map: %v\nexpected: lbname: %s", um, lbName)
	}
	// Verify that the expected target proxy was created.
	ftp := lbc.tps.(*targetproxy.FakeTargetProxySyncer)
	if len(ftp.EnsuredTargetProxies) != 1 {
		t.Fatalf("unexpected number of target proxies. expected: %d, got: %d", 1, len(ftp.EnsuredTargetProxies))
	}
	tp := ftp.EnsuredTargetProxies[0]
	if tp.UmLink != urlmap.FakeURLSelfLink {
		t.Errorf("unexpected url map link in target proxy. expected: %s, got: %s", urlmap.FakeURLSelfLink, tp.UmLink)
	}
	// Verify that the expected forwarding rule was created.
	ffr := lbc.frs.(*forwardingrule.FakeForwardingRuleSyncer)
	if len(ffr.EnsuredForwardingRules) != 1 {
		t.Fatalf("unexpected number of forwarding rules. expected: %d, got: %d", 1, len(ffr.EnsuredForwardingRules))
	}
	fr := ffr.EnsuredForwardingRules[0]
	if fr.LBName != lbName {
		t.Errorf("unexpected lbname in forwarding rule. expected: %s, got: %s", lbName, fr.LBName)
	}
	if fr.TPLink != targetproxy.FakeTargetProxySelfLink {
		t.Errorf("unexpected target proxy link in forwarding rule. expected: %s, got: %s", targetproxy.FakeTargetProxySelfLink, fr.TPLink)
	}
	if fr.IPAddress != ipAddress.Address {
		t.Errorf("unexpected ip address in forwarding rule. expected: %s, got: %s", ipAddress.Address, fr.IPAddress)
	}
	if fr.LBName != lbName {
		t.Errorf("unexpected lbname in forwarding rule. expected: %s, got: %s", lbName, fr.LBName)
	}
	// Verify that the expected firewall rule was created.
	ffw := lbc.fws.(*firewallrule.FakeFirewallRuleSyncer)
	if len(ffw.EnsuredFirewallRules) != 1 {
		t.Fatalf("unexpected number of firewall rules. expected: %d, got: %d", 1, len(ffw.EnsuredFirewallRules))
	}
	fw := ffw.EnsuredFirewallRules[0]
	if fw.LBName != lbName {
		t.Errorf("unexpected lbname in forwarding rule. expected: %s, got: %s", lbName, fw.LBName)
	}
}

func TestCreateLoadBalancerBadNodePortsNoValidate(t *testing.T) {
	lbName := "lb-name"
	igName := "my-fake-ig"
	igZone := "my-fake-zone"
	zoneLink := fmt.Sprintf("https://www.googleapis.com/compute/v1/projects/fake-project/zones/%s", igZone)
	clusters := []string{"cluster1", "cluster2"}
	lbc := newSyncer(lbName)
	ipAddress := &compute.Address{
		Name:    "ipAddressName",
		Address: "1.2.3.4",
	}
	// Reserve a global address. User is supposed to do this before calling CreateLoadBalancer.
	lbc.ipp.ReserveGlobalAddress(ipAddress)
	ing, err := setupLBCForCreateIng(lbc, -1 /*nodePort*/, true /*randNodePort*/, igName, igZone, zoneLink, ipAddress)
	if err != nil {
		t.Fatalf("%s", err)
	}
	if err := lbc.CreateLoadBalancer(ing, true /*forceUpdate*/, false /*validate*/, clusters); err != nil {
		t.Errorf("expected no error Creating Load Balancer. Got: %s", err)
	}
}

func setupLBCForCreateIng(lbc *Syncer, nodePort int64, randNodePort bool, igName, igZone, zoneLink string, ipAddress *compute.Address) (*v1beta1.Ingress, error) {
	portName := "my-port-name"
	// Create ingress with instance groups annotation.
	annotationsValue := []struct {
		Name string
		Zone string
	}{
		{
			Name: igName,
			Zone: zoneLink,
		},
	}
	jsonValue, err := json.Marshal(annotationsValue)
	if err != nil {
		return nil, fmt.Errorf("error %s in marshalling annotations %v", err, annotationsValue)
	}
	ing := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-ing",
			Namespace: "my-ns",
			Annotations: map[string]string{
				annotations.InstanceGroupsAnnotationKey: string(jsonValue),
				annotations.StaticIPNameKey:             ipAddress.Name,
			},
		},
		Spec: v1beta1.IngressSpec{
			Rules: []v1beta1.IngressRule{
				{
					IngressRuleValue: v1beta1.IngressRuleValue{
						HTTP: &v1beta1.HTTPIngressRuleValue{
							Paths: []v1beta1.HTTPIngressPath{
								{
									Path: "foo",
									Backend: v1beta1.IngressBackend{
										ServiceName: "my-svc",
										ServicePort: intstr.FromInt(80),
									},
								},
							},
						},
					},
				},
			},
			Backend: &v1beta1.IngressBackend{
				ServiceName: "my-svc",
				ServicePort: intstr.FromInt(80),
			},
		},
	}

	for _, c := range lbc.clients {
		client := c.(*fake.Clientset)
		// Add a reaction to return a fake service.
		client.AddReactor("get", "services", func(action core.Action) (handled bool, ret runtime.Object, err error) {
			if randNodePort {
				nodePort = int64(rand.Int31n(1000) + 30000)
				glog.Infof("Using a randomized NodePort: %d", nodePort)
			}
			ret = &v1.Service{
				Spec: v1.ServiceSpec{
					Ports: []v1.ServicePort{
						{
							Port:     80,
							NodePort: int32(nodePort),
						},
					},
				},
			}
			return true, ret, nil
		})
		// Return a fake ingress with the instance groups annotation.
		client.AddReactor("get", "ingresses", func(action core.Action) (handled bool, ret runtime.Object, err error) {
			return true, ing, nil
		})
	}
	// Setup the fake instance group provider to return the expected instance group with fake named ports.
	ig := &compute.InstanceGroup{
		Name: igName,
		NamedPorts: []*compute.NamedPort{
			{
				Name: portName,
				Port: nodePort,
			},
		},
	}
	if igErr := lbc.igp.CreateInstanceGroup(ig, igZone); igErr != nil {
		return nil, igErr
	}
	return ing, nil
}

func TestDeleteLoadBalancer(t *testing.T) {
	lbName := "lb-name"
	nodePort := int64(32211)
	igName := "my-fake-ig"
	igZone := "my-fake-zone"
	zoneLink := fmt.Sprintf("https://www.googleapis.com/compute/v1/projects/fake-project/zones/%s", igZone)
	clusters := []string{"cluster1", "cluster2"}
	lbc := newSyncer(lbName)
	ipAddress := &compute.Address{
		Name:    "ipAddressName",
		Address: "1.2.3.4",
	}
	// Reserve a global address. User is supposed to do this before calling CreateLoadBalancer.
	lbc.ipp.ReserveGlobalAddress(ipAddress)
	ing, err := setupLBCForCreateIng(lbc, nodePort, false /*randNodePort*/, igName, igZone, zoneLink, ipAddress)
	if err != nil {
		t.Fatalf("unexpected error in test setup: %s", err)
	}
	// Verify that trying to delete when no load balancer exists does not return any error.
	if err := lbc.DeleteLoadBalancer(ing, false /* forceDelete */); err != nil {
		t.Fatalf("unexpected error while deleting load balancer when none exist: %s", err)
	}
	if err := lbc.CreateLoadBalancer(ing, true /*forceUpdate*/, true /*validate*/, clusters); err != nil {
		t.Fatalf("unexpected error %s", err)
	}
	expectLBResources(lbc)
	// Verify that all expected resources are deleted.
	if err := lbc.DeleteLoadBalancer(ing, false /* forceDelete */); err != nil {
		t.Fatalf("unexpected error in deleting load balancer: %s", err)
	}
	if err := expectNoLBResources(lbc); err != nil {
		t.Errorf("%s", err)
	}
}

func TestDeleteLoadBalancerWithForce(t *testing.T) {
	lbName := "lb-name"
	nodePort := int64(32211)
	igName := "my-fake-ig"
	igZone := "my-fake-zone"
	zoneLink := fmt.Sprintf("https://www.googleapis.com/compute/v1/projects/fake-project/zones/%s", igZone)
	clusters := []string{"cluster1", "cluster2"}
	lbc := newSyncer(lbName)
	ipAddress := &compute.Address{
		Name:    "ipAddressName",
		Address: "1.2.3.4",
	}
	// Reserve a global address. User is supposed to do this before calling CreateLoadBalancer.
	lbc.ipp.ReserveGlobalAddress(ipAddress)
	ing, err := setupLBCForCreateIng(lbc, nodePort, false /*randNodePort*/, igName, igZone, zoneLink, ipAddress)
	if err != nil {
		t.Fatalf("unexpected error in test setup: %s", err)
	}
	if err := lbc.CreateLoadBalancer(ing, false /*forceUpdate*/, true /*validate*/, clusters); err != nil {
		t.Fatalf("unexpected error %s", err)
	}
	if err := expectLBResources(lbc); err != nil {
		t.Fatalf("%s", err)
	}
	// Delete should return an error when no clients are found and we are
	// not force deleting.
	clients := lbc.clients
	lbc.clients = nil
	if err := lbc.DeleteLoadBalancer(ing, false /* forceDelete */); err == nil {
		t.Errorf("unexpected nil error in deleting load balancer. expected to get no client found error")
	}
	// All resources should still exist.
	if err := expectLBResources(lbc); err != nil {
		t.Errorf("%s", err)
	}
	if err := lbc.DeleteLoadBalancer(ing, true /* forceDelete */); err == nil {
		t.Errorf("unexpected nil error in deleting load balancer. expected to get no client found error")
	}

	// All resources except backend services and health checks should be deleted.
	ffw := lbc.fws.(*firewallrule.FakeFirewallRuleSyncer)
	if len(ffw.EnsuredFirewallRules) != 0 {
		t.Errorf("unexpected firewall rules have not been deleted: %v", ffw.EnsuredFirewallRules)
	}
	ffr := lbc.frs.(*forwardingrule.FakeForwardingRuleSyncer)
	if len(ffr.EnsuredForwardingRules) != 0 {
		t.Errorf("unexpected forwarding rules have not been deleted: %v", ffr.EnsuredForwardingRules)
	}
	ftp := lbc.tps.(*targetproxy.FakeTargetProxySyncer)
	if len(ftp.EnsuredTargetProxies) != 0 {
		t.Errorf("unexpected target proxies have not been deleted: %v", ftp.EnsuredTargetProxies)
	}
	fum := lbc.ums.(*urlmap.FakeURLMapSyncer)
	if len(fum.EnsuredURLMaps) != 0 {
		t.Errorf("unexpected url maps have not been deleted: %v", fum.EnsuredURLMaps)
	}
	fbs := lbc.bss.(*backendservice.FakeBackendServiceSyncer)
	if len(fbs.EnsuredBackendServices) == 0 {
		t.Errorf("unexpected backend services should not have been deleted: %v", fbs.EnsuredBackendServices)
	}
	fhc := lbc.hcs.(*healthcheck.FakeHealthCheckSyncer)
	if len(fhc.EnsuredHealthChecks) == 0 {
		t.Errorf("unexpected health checks should not have been deleted: %v", fhc.EnsuredHealthChecks)
	}
	// Set the clients back to their original value and verify that deleting now deletes all resources.
	lbc.clients = clients
	if err := lbc.DeleteLoadBalancer(ing, false /* forceDelete */); err != nil {
		t.Errorf("unexpected error in deleting load balancer: %s", err)
	}
	if err := expectNoLBResources(lbc); err != nil {
		t.Errorf("%s", err)
	}
}

// Verifies that all expected LB resources exist. Returns an error if that is not true.
func expectLBResources(lbc *Syncer) error {
	fhc := lbc.hcs.(*healthcheck.FakeHealthCheckSyncer)
	if len(fhc.EnsuredHealthChecks) == 0 {
		return fmt.Errorf("unexpected health checks do not exist")
	}
	fbs := lbc.bss.(*backendservice.FakeBackendServiceSyncer)
	if len(fbs.EnsuredBackendServices) == 0 {
		return fmt.Errorf("unexpected backend services do not exist")
	}
	fum := lbc.ums.(*urlmap.FakeURLMapSyncer)
	if len(fum.EnsuredURLMaps) == 0 {
		return fmt.Errorf("unexpected url maps do not exist")
	}
	ftp := lbc.tps.(*targetproxy.FakeTargetProxySyncer)
	if len(ftp.EnsuredTargetProxies) == 0 {
		return fmt.Errorf("unexpected target proxies do not exist")
	}
	ffr := lbc.frs.(*forwardingrule.FakeForwardingRuleSyncer)
	if len(ffr.EnsuredForwardingRules) == 0 {
		return fmt.Errorf("unexpected forwarding rules do not exist")
	}
	ffw := lbc.fws.(*firewallrule.FakeFirewallRuleSyncer)
	if len(ffw.EnsuredFirewallRules) == 0 {
		return fmt.Errorf("unexpected firewall rules do not exist")
	}
	return nil
}

// Verifies that no expected LB resources exist. Returns an error if that is not true.
func expectNoLBResources(lbc *Syncer) error {
	fhc := lbc.hcs.(*healthcheck.FakeHealthCheckSyncer)
	if len(fhc.EnsuredHealthChecks) != 0 {
		return fmt.Errorf("unexpected health checks exist: %v", fhc.EnsuredHealthChecks)
	}
	fbs := lbc.bss.(*backendservice.FakeBackendServiceSyncer)
	if len(fbs.EnsuredBackendServices) != 0 {
		return fmt.Errorf("unexpected backend services exist: %v", fbs.EnsuredBackendServices)
	}
	fum := lbc.ums.(*urlmap.FakeURLMapSyncer)
	if len(fum.EnsuredURLMaps) != 0 {
		return fmt.Errorf("unexpected url maps exist: %v", fum.EnsuredURLMaps)
	}
	ftp := lbc.tps.(*targetproxy.FakeTargetProxySyncer)
	if len(ftp.EnsuredTargetProxies) != 0 {
		return fmt.Errorf("unexpected target proxies exist: %v", ftp.EnsuredTargetProxies)
	}
	ffr := lbc.frs.(*forwardingrule.FakeForwardingRuleSyncer)
	if len(ffr.EnsuredForwardingRules) != 0 {
		return fmt.Errorf("unexpected forwarding rules exist: %v", ffr.EnsuredForwardingRules)
	}
	ffw := lbc.fws.(*firewallrule.FakeFirewallRuleSyncer)
	if len(ffw.EnsuredFirewallRules) != 0 {
		return fmt.Errorf("unexpected firewall rules exist: %v", ffw.EnsuredFirewallRules)
	}
	return nil
}

func TestRemoveFromClusters(t *testing.T) {
	lbName := "lb-name"
	nodePort := int64(32211)
	igName := "my-fake-ig"
	igZone := "my-fake-zone"
	zoneLink := fmt.Sprintf("https://www.googleapis.com/compute/v1/projects/fake-project/zones/%s", igZone)
	igLink := fmt.Sprintf("%s/instanceGroups/%s", zoneLink, igName)
	clusters := []string{"cluster1", "cluster2"}
	lbc := newSyncer(lbName)
	ipAddress := &compute.Address{
		Name:    "ipAddressName",
		Address: "1.2.3.4",
	}
	// Reserve a global address. User is supposed to do this before calling CreateLoadBalancer.
	lbc.ipp.ReserveGlobalAddress(ipAddress)
	ing, err := setupLBCForCreateIng(lbc, nodePort, false /*randNodePort*/, igName, igZone, zoneLink, ipAddress)
	if err != nil {
		t.Fatalf("unexpected error in test setup: %s", err)
	}
	if err := lbc.CreateLoadBalancer(ing, true /*forceUpdate*/, true /*validate*/, clusters); err != nil {
		t.Fatalf("unexpected error %s", err)
	}
	// Verify that backend service, firewall rule and status are as expected.
	expectedClusters := clusters
	expectedIGlinks := []string{igLink, igLink}
	expectedFWIGLinks := map[string][]string{
		"cluster1": {igLink},
		"cluster2": {igLink},
	}
	if err := verifyRemoveClustersResult(lbc, expectedClusters, expectedIGlinks, expectedFWIGLinks); err != nil {
		t.Errorf("%s", err)
	}

	// Remove the load balancer from the first cluster and verify that the load balancer status is updated appropriately.
	removeClients := map[string]kubeclient.Interface{
		"cluster1": lbc.clients["cluster1"],
	}
	if err := lbc.RemoveFromClusters(ing, removeClients, false /*forceUpdate*/); err != nil {
		t.Errorf("unexpected error in removing existing load balancer from clusters: %s", err)
	}
	expectedClusters = []string{"cluster2"}
	expectedIGlinks = []string{igLink}
	expectedFWIGLinks = map[string][]string{
		"cluster2": {igLink},
	}
	if err := verifyRemoveClustersResult(lbc, expectedClusters, expectedIGlinks, expectedFWIGLinks); err != nil {
		t.Errorf("%s", err)
	}

	// Try removing it from the second cluster as well and verify that it returns an error.
	removeClients = map[string]kubeclient.Interface{
		"cluster2": lbc.clients["cluster2"],
	}
	if err := lbc.RemoveFromClusters(ing, removeClients, false /*forceUpdate*/); err == nil {
		t.Errorf("unexpected nil error in removing load balancer from all clusters, expected an error to prompt using delete command instead")
	}
	// Verify that nothing was changed.
	if err := verifyRemoveClustersResult(lbc, expectedClusters, expectedIGlinks, expectedFWIGLinks); err != nil {
		t.Errorf("%s", err)
	}

	// Verify that remove succeeds with force=true.
	if err := lbc.RemoveFromClusters(ing, removeClients, true /*forceUpdate*/); err != nil {
		t.Errorf("unexpected error in removing load balancer from all clusters with force=true: %s", err)
	}
	expectedClusters = []string{}
	expectedIGlinks = []string{}
	expectedFWIGLinks = map[string][]string{}
	if err := verifyRemoveClustersResult(lbc, expectedClusters, expectedIGlinks, expectedFWIGLinks); err != nil {
		t.Errorf("%s", err)
	}

	// Cleanup.
	if err := lbc.DeleteLoadBalancer(ing, false /* forceDelete */); err != nil {
		t.Fatalf("unexpected error in deleting load balancer: %s", err)
	}
}

// verifyRemoveClustersResult verifies that backend services, firewall rules and load balancer status are as expected after a remove clusters command.
// Returns an error if not true.
func verifyRemoveClustersResult(lbc *Syncer, expectedClusters, expectedIGlinks []string, expectedFWIGLinks map[string][]string) error {
	fbs := lbc.bss.(*backendservice.FakeBackendServiceSyncer)
	if len(fbs.EnsuredBackendServices) != 2 {
		return fmt.Errorf("unexpected backend services, expected 2 backend services, got: %v", fbs.EnsuredBackendServices)
	}
	if err := verifyBackendService(fbs.EnsuredBackendServices[0], expectedIGlinks); err != nil {
		return err
	}
	if err := verifyBackendService(fbs.EnsuredBackendServices[1], expectedIGlinks); err != nil {
		return err
	}
	ffw := lbc.fws.(*firewallrule.FakeFirewallRuleSyncer)
	if len(ffw.EnsuredFirewallRules) != 1 {
		return fmt.Errorf("unexpected firewall rules, expected 1, got: %v", ffw.EnsuredFirewallRules)
	}
	fw := ffw.EnsuredFirewallRules[0]
	if !reflect.DeepEqual(fw.IGLinks, expectedFWIGLinks) {
		return fmt.Errorf("unexpected IG links on firewall rule, expected: %v, got: %v", expectedFWIGLinks, fw.IGLinks)
	}
	// Verify that the load balancer is spread to both clusters.
	err := verifyClusters(lbc, expectedClusters)
	return err
}

func verifyBackendService(be backendservice.FakeBackendService, expectedIGlinks []string) error {
	if !reflect.DeepEqual(be.IGLinks, expectedIGlinks) {
		return fmt.Errorf("unexpected IG links on backend service. expected: %v, got: %v", expectedIGlinks, be.IGLinks)
	}
	return nil
}

func TestRemoveClustersFromStatus(t *testing.T) {
	lbName := "lb-name"
	nodePort := int64(32211)
	igName := "my-fake-ig"
	igZone := "my-fake-zone"
	zoneLink := fmt.Sprintf("https://www.googleapis.com/compute/v1/projects/fake-project/zones/%s", igZone)
	clusters := []string{"cluster1", "cluster2"}
	lbc := newSyncer(lbName)
	ipAddress := &compute.Address{
		Name:    "ipAddressName",
		Address: "1.2.3.4",
	}
	// Reserve a global address. User is supposed to do this before calling CreateLoadBalancer.
	lbc.ipp.ReserveGlobalAddress(ipAddress)
	ing, err := setupLBCForCreateIng(lbc, nodePort, false /*randNodePort*/, igName, igZone, zoneLink, ipAddress)
	if err != nil {
		t.Fatalf("unexpected error in test setup: %s", err)
	}
	testCases := []struct {
		frHasStatus      bool
		umHasStatus      bool
		errorInGetStatus bool
	}{
		{
			// Default case.
			false,
			true,
			false,
		},
		{
			// Old setup - forwarding rule has the status, url map does not.
			true,
			false,
			false,
		},
		{
			true,
			true,
			false,
		},
		{
			false,
			false,
			true,
		},
	}
	for _, c := range testCases {
		if err := lbc.CreateLoadBalancer(ing, false /*forceUpdate*/, true /*validate*/, clusters); err != nil {
			t.Fatalf("unexpected error %s", err)
		}
		if c.frHasStatus {
			// Explicitly add status, since forwarding rule will not have status by default.
			lbc.frs.(*forwardingrule.FakeForwardingRuleSyncer).AddStatus(lbName, &status.LoadBalancerStatus{Clusters: clusters})
		}
		if !c.umHasStatus {
			// Explicitly remove status, since url map will have status by default.
			lbc.ums.(*urlmap.FakeURLMapSyncer).RemoveStatus(lbName)
		}
		// Verify that the load balancer is spread to both clusters.
		if err := verifyClusters(lbc, clusters); c.errorInGetStatus != (err != nil) {
			t.Errorf("expected error in verifying status: %v, got err: %s", c.errorInGetStatus, err)
		}

		// Remove the load balancer from the first cluster and verify that the load balancer status is updated appropriately.
		clustersToRemove := []string{"cluster1"}
		if err := lbc.removeClustersFromStatus(lbName, clustersToRemove); err != nil {
			t.Errorf("unexpected error in removing existing load balancer from clusters: %s", err)
		}
		if err := verifyClusters(lbc, []string{"cluster2"}); c.errorInGetStatus != (err != nil) {

			t.Errorf("expected error in verifying status: %v, got err: %s", c.errorInGetStatus, err)
		}
		if err := lbc.DeleteLoadBalancer(ing, false /* forceDelete */); err != nil {
			t.Fatalf("unexpected error %s", err)
		}
	}
}

func verifyClusters(lbc *Syncer, expectedClusters []string) error {
	status, err := lbc.getLoadBalancerStatus()
	if status == nil || err != nil {
		return fmt.Errorf("unexpected error in getting load balancer status: %s", err)
	}
	if !reflect.DeepEqual(status.Clusters, expectedClusters) {
		return fmt.Errorf("unexpected list of clusters, expected: %v, got: %v", expectedClusters, status.Clusters)
	}
	return nil
}

func TestFormatLoadBalancersList(t *testing.T) {
	testCases := []struct {
		statuses       []status.LoadBalancerStatus
		expectedOutput string
	}{
		{
			// No ingresses.
			[]status.LoadBalancerStatus{},
			"No multicluster ingresses found.",
		},
		{
			// Name only.
			[]status.LoadBalancerStatus{
				{
					LoadBalancerName: "lb",
				},
			},
			"NAME      IP        CLUSTERS\nlb                  \n",
		},
		{
			// Missing IP.
			[]status.LoadBalancerStatus{
				{
					LoadBalancerName: "lb",
					Clusters:         []string{"cluster1", "cluster2"},
				},
			},
			"NAME      IP        CLUSTERS\nlb                  cluster1, cluster2\n",
		},
		{
			// All information.
			[]status.LoadBalancerStatus{
				{
					LoadBalancerName: "lb",
					IPAddress:        "192.168.1.5",
					Clusters:         []string{"cluster1", "cluster2"},
				},
			},
			"NAME      IP            CLUSTERS\nlb        192.168.1.5   cluster1, cluster2\n",
		},
		{
			// Multiple ingresses.
			[]status.LoadBalancerStatus{
				{
					LoadBalancerName: "lb1",
					IPAddress:        "192.168.1.5",
					Clusters:         []string{"cluster1", "cluster2"},
				},
				{
					LoadBalancerName: "lb2",
					IPAddress:        "192.168.1.7",
					Clusters:         []string{"cluster1", "cluster2"},
				},
			},
			"NAME      IP            CLUSTERS\nlb1       192.168.1.5   cluster1, cluster2\nlb2       192.168.1.7   cluster1, cluster2\n",
		},
	}
	for i, c := range testCases {
		output, err := formatLoadBalancersList(c.statuses)
		if err != nil {
			t.Errorf("case %d: unexpected error: %s", i, err)
		}
		if output != c.expectedOutput {
			t.Errorf("case %d: unexpected output %s, expected: %s", i, output, c.expectedOutput)
		}
	}
}
