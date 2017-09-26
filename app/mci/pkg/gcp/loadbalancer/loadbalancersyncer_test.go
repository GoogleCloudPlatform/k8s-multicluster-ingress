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

	"k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
)

func TestCreateLoadBalancer(t *testing.T) {
	fakeClient := fake.Clientset{}
	lbc := NewLoadBalancerSyncer("lb-name", &fakeClient)

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
									},
								},
							},
						},
					},
				},
			},
		},
	}
	if err := lbc.CreateLoadBalancer(&ing); err != nil {
		t.Fatalf("unexpected error %s", err)
	}
	// Verify that the client should have been used to fetch service "my-svc".
	if len(fakeClient.Actions()) != 1 {
		t.Fatalf("unexpected actions: %v, expected 1 get", fakeClient.Actions())
	}
	getName := fakeClient.Actions()[0].(core.GetAction).GetName()
	if getName != "my-svc" {
		t.Fatalf("unexpected get for %s, expected: my-svc", getName)
	}
}
