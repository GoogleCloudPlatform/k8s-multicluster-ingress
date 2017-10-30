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

package urlmap

import (
	"testing"

	"google.golang.org/api/compute/v1"
	"k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/util/intstr"
	ingresslb "k8s.io/ingress-gce/pkg/loadbalancers"

	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/backendservice"
	utilsnamer "github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/namer"
)

func TestEnsureURLMap(t *testing.T) {
	lbName := "lb-name"
	svcName := "svcName"
	svcPort := "svcPort"
	ing := &v1beta1.Ingress{
		Spec: v1beta1.IngressSpec{
			Backend: &v1beta1.IngressBackend{
				ServiceName: svcName,
				ServicePort: intstr.FromString(svcPort),
			},
		},
	}
	beMap := backendservice.BackendServicesMap{
		svcName: &compute.BackendService{},
	}
	// Should create the url map as expected.
	ump := ingresslb.NewFakeLoadBalancers("")
	namer := utilsnamer.NewNamer("mci1", lbName)
	umName := namer.URLMapName()
	ums := NewURLMapSyncer(namer, ump)
	// GET should return NotFound.
	if _, err := ump.GetUrlMap(umName); err == nil {
		t.Fatalf("expected NotFound error, got nil")
	}
	umLink, err := ums.EnsureURLMap(lbName, ing, beMap)
	if err != nil {
		t.Fatalf("expected no error in ensuring url map, actual: %v", err)
	}
	// Verify that GET does not return NotFound.
	um, err := ump.GetUrlMap(umName)
	if err != nil {
		t.Fatalf("expected nil error, actual: %v", err)
	}
	if um.SelfLink != umLink {
		t.Errorf("unexpected self link in returned url map. expected: %s, got: %s", umLink, um.SelfLink)
	}
	// TODO(nikhiljindal): Test update existing url map.
}

func TestDeleteURLMap(t *testing.T) {
	lbName := "lb-name"
	svcName := "svcName"
	svcPort := "svcPort"
	ing := &v1beta1.Ingress{
		Spec: v1beta1.IngressSpec{
			Backend: &v1beta1.IngressBackend{
				ServiceName: svcName,
				ServicePort: intstr.FromString(svcPort),
			},
		},
	}
	beMap := backendservice.BackendServicesMap{
		svcName: &compute.BackendService{},
	}
	// Should create the url map as expected.
	ump := ingresslb.NewFakeLoadBalancers("")
	namer := utilsnamer.NewNamer("mci1", lbName)
	umName := namer.URLMapName()
	ums := NewURLMapSyncer(namer, ump)
	if _, err := ums.EnsureURLMap(lbName, ing, beMap); err != nil {
		t.Fatalf("expected no error in ensuring url map, actual: %v", err)
	}
	if _, err := ump.GetUrlMap(umName); err != nil {
		t.Fatalf("expected nil error, actual: %v", err)
	}
	// Verify that GET fails after DELETE
	if err := ums.DeleteURLMap(); err != nil {
		t.Fatalf("unexpected err while deleting URL map: %s", err)
	}
	if _, err := ump.GetUrlMap(umName); err == nil {
		t.Errorf("unexpected nil error, expected NotFound")
	}
}
