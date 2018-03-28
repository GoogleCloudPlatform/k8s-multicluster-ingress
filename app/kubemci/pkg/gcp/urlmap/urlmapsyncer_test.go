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
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"

	compute "google.golang.org/api/compute/v1"

	"k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/util/intstr"
	ingresslb "k8s.io/ingress-gce/pkg/loadbalancers"
	"k8s.io/ingress-gce/pkg/utils"

	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/backendservice"
	utilsnamer "github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/namer"
	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/status"
	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/goutils"
	"github.com/golang/glog"
)

func TestEnsureURLMap(t *testing.T) {
	lbName := "lb-name"
	ipAddr := "1.2.3.4"
	clusters := []string{"cluster1", "cluster2"}
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
		umLink, err := ums.EnsureURLMap(c.lBName, ipAddr, clusters, ing, beMap, c.forceUpdate)
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
	ipAddr := "1.2.3.4"
	clusters := []string{"cluster1", "cluster2"}
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
	if _, err := ums.EnsureURLMap(lbName, ipAddr, clusters, ing, beMap, false /*forceUpdate*/); err != nil {
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

func TestListLoadBalancerStatuses(t *testing.T) {
	lbName := "lb-name"
	ipAddr := "1.2.3.4"
	clusters := []string{"cluster1", "cluster2"}
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
	type ensureUM struct {
		lbName   string
		ipAddr   string
		clusters []string
		// True if the ensured forwarding rule should also have the right load balancer status.
		// False by default.
		hasStatus bool
	}
	testCases := []struct {
		description  string
		urlMaps      []ensureUM
		expectedList map[string]status.LoadBalancerStatus
	}{
		{
			"Should get an empty list when there are no url maps",
			[]ensureUM{},
			map[string]status.LoadBalancerStatus{},
		},
		{
			"Load balancer should be listed as expected when url map exists",
			[]ensureUM{
				{
					lbName,
					ipAddr,
					clusters,
					true,
				},
			},
			map[string]status.LoadBalancerStatus{
				lbName: {
					"",
					lbName,
					clusters,
					ipAddr,
				},
			},
		},
		{
			"Should ignore the load balancer without status",
			[]ensureUM{
				{
					lbName + "1",
					ipAddr,
					clusters,
					true,
				},
				{
					lbName + "2",
					ipAddr,
					clusters,
					false,
				},
			},
			map[string]status.LoadBalancerStatus{
				lbName + "1": {
					"",
					lbName + "1",
					clusters,
					ipAddr,
				},
			},
		},
	}
	for i, c := range testCases {
		glog.Infof("Test case: %d: %s", i, c.description)
		frp := ingresslb.NewFakeLoadBalancers("" /*name*/, nil /*namer*/)
		// Create all url maps.
		for _, um := range c.urlMaps {
			namer := utilsnamer.NewNamer("mci1", um.lbName)
			ums := NewURLMapSyncer(namer, frp)
			typedUMS := ums.(*URLMapSyncer)
			if _, err := ums.EnsureURLMap(um.lbName, um.ipAddr, um.clusters, ing, beMap, false /*force*/); err != nil {
				t.Errorf("expected no error in ensuring url map, actual: %v", err)
			}
			if !um.hasStatus {
				if err := removeStatus(typedUMS); err != nil {
					t.Errorf("unpexpected error in removing status from url map: %s. Moving to next test case", err)
					continue
				}
			}
		}
		namer := utilsnamer.NewNamer("mci1", lbName)
		ums := NewURLMapSyncer(namer, frp)
		// List them to verify that they are as expected.
		list, err := ums.ListLoadBalancerStatuses()
		if err != nil {
			t.Errorf("unexpected error listing load balancer statuses: %v. Moving to next test case", err)
			continue
		}
		if len(list) != len(c.expectedList) {
			t.Errorf("unexpected list of load balancers, expected: %v, actual: %v. Moving to next test case", c.expectedList, list)
			continue
		}
		for _, rule := range list {
			expectedRule, ok := c.expectedList[rule.LoadBalancerName]
			if !ok {
				t.Errorf("unexpected rule %v. expected rules: %v, actual: %v", rule, c.expectedList, list)
				continue
			}
			if rule.LoadBalancerName != expectedRule.LoadBalancerName {
				t.Errorf("Unexpected name. expected: %s, actual: %s", expectedRule.LoadBalancerName, rule.LoadBalancerName)
			}
			if rule.IPAddress != expectedRule.IPAddress {
				t.Errorf("Unexpected IP addr. expected %s, actual: %s", expectedRule.IPAddress, rule.IPAddress)
			}
			actualClusters := goutils.MapFromSlice(rule.Clusters)
			for _, expected := range expectedRule.Clusters {
				if _, ok := actualClusters[expected]; !ok {
					t.Errorf("expected cluster %s is missing from resultant cluster list %v", expected, rule.Clusters)
				}
			}
		}
	}
}

func TestGetLoadBalancerStatus(t *testing.T) {
	lbName := "lb-name"
	ipAddr := "1.2.3.4"
	clusters := []string{"cluster1", "cluster2"}
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

	frp := ingresslb.NewFakeLoadBalancers("" /*name*/, nil /*namer*/)
	namer := utilsnamer.NewNamer("mci1", lbName)
	ums := NewURLMapSyncer(namer, frp)
	typedUMS := ums.(*URLMapSyncer)

	// Should return http.StatusNotFound when the url map does not exist.
	status, err := ums.GetLoadBalancerStatus(lbName)
	if !utils.IsHTTPErrorCode(err, http.StatusNotFound) {
		t.Fatalf("expected error with status code http.StatusNotFound, found: %v", err)
	}

	// Should return the status as expected when the url map exists with status.
	if _, err := ums.EnsureURLMap(lbName, ipAddr, clusters, ing, beMap, false /*force*/); err != nil {
		t.Errorf("expected no error in ensuring url map, actual: %v", err)
	}
	status, err = ums.GetLoadBalancerStatus(lbName)
	if err != nil {
		t.Errorf("unexpected error in getting status description for load balancer %s, expected err = nil, actual err: %s", lbName, err)
	}

	// Verify that status description has the expected values set.
	if status.LoadBalancerName != lbName {
		t.Errorf("unexpected load balancer name, expected: %s, got: %s", lbName, status.LoadBalancerName)
	}
	if !reflect.DeepEqual(status.Clusters, clusters) {
		t.Errorf("unexpected list of clusters, expected: %v, got: %v", clusters, status.Clusters)
	}
	if status.IPAddress != ipAddr {
		t.Errorf("unpexpected ip address, expected: %s, got: %s", status.IPAddress, ipAddr)
	}

	// Remove the status from url map's description to simulate old url maps that have status on forwarding rule.
	// It should return a non-nil error in this case.
	removeStatus(typedUMS)
	status, err = ums.GetLoadBalancerStatus(lbName)
	if err == nil {
		t.Fatalf("unexpected nil error for getting load balancer status from a url map with non status description")
	}
	// Cleanup
	if err := ums.DeleteURLMap(); err != nil {
		t.Fatalf("unexpected error in deleting url map: %s", err)
	}
}

// removeStatus updates the existing url map of the given name to remove load balancer status.
func removeStatus(ums *URLMapSyncer) error {
	// Update the forwarding rule to not have a status to simulate old url maps that do not have the a status.
	name := ums.namer.URLMapName()
	um, err := ums.ump.GetUrlMap(name)
	if err != nil {
		return fmt.Errorf("unexpected error in fetching existing url map: %s", err)
	}
	// Shallow copy is fine since we only change the description.
	newUM := *um
	newUM.Description = "Basic description"
	if _, err := ums.updateURLMap(&newUM); err != nil {
		return fmt.Errorf("unexpected error in updating url map: %s", err)
	}
	return nil
}
