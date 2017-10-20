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
	"testing"

	"google.golang.org/api/compute/v1"
	"k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
	ingressig "k8s.io/ingress-gce/pkg/instances"

	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/mci/pkg/gcp/backendservice"
	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/mci/pkg/gcp/healthcheck"
)

func newLoadBalancerSyncer(lbName string) *LoadBalancerSyncer {
	return &LoadBalancerSyncer{
		lbName: lbName,
		hcs:    healthcheck.NewFakeHealthCheckSyncer(),
		client: &fake.Clientset{},
		igp:    ingressig.NewFakeInstanceGroups(nil),
		bss:    backendservice.NewFakeBackendServiceSyncer(),
	}
}

func TestCreateLoadBalancer(t *testing.T) {
	lbName := "lb-name"
	nodePort := int64(32211)
	igName := "my-fake-ig"
	igZone := "my-fake-zone"
	portName := "my-port-name"
	lbc := newLoadBalancerSyncer(lbName)
	zoneLink := fmt.Sprintf("https://www.googleapis.com/compute/v1/projects/fake-project/zones/%s", igZone)
	expectedIGLink := fmt.Sprintf("%s/instanceGroups/%s", zoneLink, igName)
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
		t.Fatalf("unexpected error in marshalling json: %s", err)
	}
	ing := v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-ing",
			Namespace: "my-ns",
			Annotations: map[string]string{
				instanceGroupsAnnotationKey: string(jsonValue),
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
		},
	}
	client := lbc.client.(*fake.Clientset)
	// Add a reaction to return a fake service.
	client.AddReactor("get", "services", func(action core.Action) (handled bool, ret runtime.Object, err error) {
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
		return true, &ing, nil
	})

	// Setup the fake instance group provider to return the expected instance group with fake named ports.
	err = lbc.igp.CreateInstanceGroup(&compute.InstanceGroup{
		Name: igName,
		NamedPorts: []*compute.NamedPort{
			{
				Name: portName,
				Port: nodePort,
			},
		},
	}, igZone)
	if err := lbc.CreateLoadBalancer(&ing, true /*forceUpdate*/); err != nil {
		t.Fatalf("unexpected error %s", err)
	}
	// Verify client actions.
	actions := client.Actions()
	// Verify that the client should have been used to fetch service and ingress.
	if len(actions) != 2 {
		t.Fatalf("unexpected actions: %v, expected 2 (get svc and get ing)", actions)
	}
	getSvcName := actions[0].(core.GetAction).GetName()
	if getSvcName != "my-svc" {
		t.Fatalf("unexpected get for %s, expected: my-svc", getSvcName)
	}
	getIngName := actions[1].(core.GetAction).GetName()
	if getIngName != ing.Name {
		t.Fatalf("unexpected get for %s, expected: %s", getIngName, ing.Name)
	}
	// Verify that the expected healthcheck was created.
	fhc := lbc.hcs.(*healthcheck.FakeHealthCheckSyncer)
	if len(fhc.EnsuredHealthChecks) != 1 {
		t.Fatalf("unexpected number of health checks. expected: %d, got: %d", 1, len(fhc.EnsuredHealthChecks))
	}
	hc := fhc.EnsuredHealthChecks[0]
	if hc.LBName != lbName || hc.Port.Port != nodePort {
		t.Fatalf("unexpected health check: %v\nexpected: lbname: %s, port: %d", hc, lbName, nodePort)
	}
	// Verify that the expected backend service was created.
	fbs := lbc.bss.(*backendservice.FakeBackendServiceSyncer)
	if len(fbs.EnsuredBackendServices) != 1 {
		t.Fatalf("unexpected number of backend services. expected: %d, got: %d", 1, len(fbs.EnsuredBackendServices))
	}
	bs := fbs.EnsuredBackendServices[0]
	if bs.LBName != lbName {
		t.Errorf("unexpected lb name in backend service. expected: %s, got: %s", lbName, bs.LBName)
	}
	if len(bs.Ports) != 1 || bs.Ports[0].Port != nodePort {
		t.Errorf("unexpected ports in backend service. expected port %d, got: %v", nodePort, bs.Ports)
	}
	if len(bs.HCMap) != 1 || bs.HCMap[nodePort] == nil {
		t.Errorf("unexpected health check map in backend service. expected an entry for port %d, got: %v", nodePort, bs.HCMap)
	}
	if len(bs.NPMap) != 1 || bs.NPMap[nodePort] == nil {
		t.Errorf("unexpected node port map in backend service. expected an entry for port %d, got: %v", nodePort, bs.NPMap)
	}
	if len(bs.IGLinks) != 1 || bs.IGLinks[0] != expectedIGLink {
		t.Errorf("unexpected instance group in backend service. expected %s, got: %s", expectedIGLink, bs.IGLinks)
	}
}
