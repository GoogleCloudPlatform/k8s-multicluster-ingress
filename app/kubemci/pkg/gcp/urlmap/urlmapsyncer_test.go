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
	"strings"
	"testing"

	compute "google.golang.org/api/compute/v1"

	"k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/util/intstr"
	ingresslb "k8s.io/ingress-gce/pkg/loadbalancers"
	"k8s.io/ingress-gce/pkg/utils"

	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/backendservice"
	utilsnamer "github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/namer"
	"github.com/golang/glog"
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
	namer := utilsnamer.NewNamer("mci1" /*prefix*/, lbName)
	ingressNamer := utils.NewNamer("cluster1", "firewall1")
	ump := ingresslb.NewFakeLoadBalancers("" /*name*/, ingressNamer)
	umName := namer.URLMapName()
	ums := NewURLMapSyncer(namer, ump)
	// GET should return NotFound.
	if _, err := ump.GetUrlMap(umName); err == nil {
		t.Fatalf("expected NotFound error, got nil")
	}
	testCases := []struct {
		desc string
		// In's
		lBName      string
		forceUpdate bool
		// Out's
		ensureErr bool
	}{
		{
			desc:        "initial write (force=false)",
			lBName:      "lb-name",
			forceUpdate: false,
			ensureErr:   false,
		},
		{
			desc:        "write same (force=false)",
			lBName:      "lb-name",
			forceUpdate: false,
			ensureErr:   false,
		},
		{
			desc:        "write different (force=false)",
			lBName:      "lb-name2",
			forceUpdate: false,
			ensureErr:   true,
		},
		{
			desc:        "write different (force=true)",
			lBName:      "lb-name3",
			forceUpdate: true,
			ensureErr:   false,
		},
	}

	for _, c := range testCases {
		glog.Infof("\nTest case: %s", c.desc)
		umLink, err := ums.EnsureURLMap(c.lBName, ing, beMap, c.forceUpdate)
		if (err != nil) != c.ensureErr {
			t.Errorf("Ensuring URL map... expected err? %v. actual: %v", c.ensureErr, err)
		}
		if c.ensureErr {
			t.Logf("Skipping validation.")
			continue
		}
		// Verify that GET does not return NotFound.
		um, err := ump.GetUrlMap(umName)
		if err != nil {
			t.Errorf("expected nil error, actual: %v", err)
		}
		if um.SelfLink != umLink {
			t.Errorf("unexpected self link in returned url map. expected: %s, got: %s", umLink, um.SelfLink)
		}
		if !strings.Contains(um.Description, c.lBName) {
			t.Errorf("Expected description to contain lb name (%s). got:%s", c.lBName, um.Description)
		}
	}
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

	// TODO: get rid of duplicated namers by moving to the ingress namer.
	namer := utilsnamer.NewNamer("mci1" /*prefix*/, lbName)
	ingressNamer := utils.NewNamer("cluster1", "firewall1")
	ump := ingresslb.NewFakeLoadBalancers("" /*name*/, ingressNamer)
	umName := namer.URLMapName()
	ums := NewURLMapSyncer(namer, ump)
	if _, err := ums.EnsureURLMap(lbName, ing, beMap, false /*forceUpdate*/); err != nil {
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
