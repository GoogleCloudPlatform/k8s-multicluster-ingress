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

	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/mci/pkg/gcp/backendservice"
	utilsnamer "github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/mci/pkg/gcp/namer"
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
	namer := utilsnamer.NewNamer("mci", lbName)
	umName := namer.URLMapName()
	ums := NewURLMapSyncer(namer, ump)
	// GET should return NotFound.
	if _, err := ump.GetUrlMap(umName); err == nil {
		t.Fatalf("expected NotFound error, got nil")
	}
	_, err := ums.EnsureURLMap(lbName, ing, beMap)
	if err != nil {
		t.Fatalf("expected no error in ensuring url map, actual: %v", err)
	}
	// Verify that GET does not return NotFound.
	if _, err := ump.GetUrlMap(umName); err != nil {
		t.Fatalf("expected nil error, actual: %v", err)
	}
	// TODO(nikhiljindal): Verify that the returned urlmap is as expected.
	// TODO(nikhiljindal): Test update existing url map.
}
