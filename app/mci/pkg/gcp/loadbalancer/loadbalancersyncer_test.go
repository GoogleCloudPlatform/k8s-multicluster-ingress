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
	"testing"

	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/mci/pkg/gcp/healthcheck"
	"k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
)

func newLoadBalancerSyncer(lbName string) *LoadBalancerSyncer {
	return &LoadBalancerSyncer{
		lbName: lbName,
		hcs:    healthcheck.NewFakeHealthCheckSyncer(),
		client: &fake.Clientset{},
	}
}

func TestCreateLoadBalancer(t *testing.T) {
	lbName := "lb-name"
	nodePort := 32211
	lbc := newLoadBalancerSyncer(lbName)

	ing := v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-ing",
			Namespace: "my-ns",
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
	if err := lbc.CreateLoadBalancer(&ing); err != nil {
		t.Fatalf("unexpected error %s", err)
	}
	actions := client.Actions()
	// Verify that the client should have been used to fetch service "my-svc".
	if len(actions) != 1 {
		t.Fatalf("unexpected actions: %v, expected 1 get", actions)
	}
	getName := actions[0].(core.GetAction).GetName()
	if getName != "my-svc" {
		t.Fatalf("unexpected get for %s, expected: my-svc", getName)
	}
	fhc := lbc.hcs.(*healthcheck.FakeHealthCheckSyncer)
	// Verify that the expected healthcheck was ensured.
	if len(fhc.EnsuredHealthChecks) != 1 {
		t.Fatalf("unexpected number of health checks. expected: %d, got: %d", 1, len(fhc.EnsuredHealthChecks))
	}
	hc := fhc.EnsuredHealthChecks[0]
	if hc.LBName != lbName || hc.Port.Port != int64(nodePort) {
		t.Fatalf("unexpected health check: %v\nexpected: lbname: %s, port: %d", hc, lbName, nodePort)
	}
}
