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

package targetproxy

import (
	"testing"

	ingresslb "k8s.io/ingress-gce/pkg/loadbalancers"

	utilsnamer "github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/namer"
	"github.com/golang/glog"
)

func TestEnsureTargetHttpProxy(t *testing.T) {
	lbName := "lb-name"
	// Should create the target proxy as expected.
	tpp := ingresslb.NewFakeLoadBalancers("" /*name*/, nil /*namer*/)
	namer := utilsnamer.NewNamer("mci1", lbName)
	tpName := namer.TargetHttpProxyName()
	tps := NewTargetProxySyncer(namer, tpp)
	// GET should return NotFound.
	if _, err := tpp.GetTargetHttpProxy(tpName); err == nil {
		t.Fatalf("expected NotFound error, got nil")
	}
	testCases := []struct {
		desc string
		// In's
		umLink      string
		forceUpdate bool
		// Out's
		ensureErr bool
	}{
		{
			desc:        "initial write (force=false)",
			umLink:      "http://google/compute/v1/projects/p/global/urlMaps/map",
			forceUpdate: false,
			ensureErr:   false,
		},
		{
			desc:        "write same (force=false)",
			umLink:      "http://google/compute/v1/projects/p/global/urlMaps/map",
			forceUpdate: false,
			ensureErr:   false,
		},
		{
			desc:        "write different (force=false)",
			umLink:      "http://google/compute/v1/projects/p/global/urlMaps/map2",
			forceUpdate: false,
			ensureErr:   true,
		},
		{
			desc:        "write different (force=true)",
			umLink:      "http://google/compute/v1/projects/p/global/urlMaps/map3",
			forceUpdate: true,
			ensureErr:   false,
		},
	}
	for _, c := range testCases {
		glog.Infof("test case:%v", c.desc)
		tpLink, err := tps.EnsureHttpTargetProxy(lbName, c.umLink, c.forceUpdate)
		if (err != nil) != c.ensureErr {
			glog.Errorf("expected_error:%v Got Error:%v", c.ensureErr, err)
			t.Errorf("in ensuring target proxy, expected error? %v, actual: %v", c.ensureErr, err)
		}
		// Verify that GET does not return NotFound.
		tp, err := tpp.GetTargetHttpProxy(tpName)
		if err != nil {
			t.Errorf("expected nil error, actual: %v", err)
		}
		if c.ensureErr {
			// Skip validation in error scenarios.
			continue
		}
		if tp.UrlMap != c.umLink {
			t.Errorf("unexpected UrlMap link in target proxy. expected: %s, actual: %s", c.umLink, tp.UrlMap)
		}
		if tp.SelfLink != tpLink {
			t.Errorf("unexpected target proxy self link. expected: %s, got: %s", tpLink, tp.SelfLink)
		}
	}
	// TODO(nikhiljindal): Test update existing target proxy.
}

func TestDeleteTargetProxies(t *testing.T) {
	lbName := "lb-name"
	umLink := "selfLink"
	// Should create the target proxy as expected.
	tpp := ingresslb.NewFakeLoadBalancers("" /*name*/, nil /*namer*/)
	namer := utilsnamer.NewNamer("mci1", lbName)
	tpName := namer.TargetHttpProxyName()
	tps := NewTargetProxySyncer(namer, tpp)
	if _, err := tps.EnsureHttpTargetProxy(lbName, umLink, false /*forceUpdate*/); err != nil {
		t.Fatalf("expected no error in ensuring target proxy, actual: %v", err)
	}
	if _, err := tpp.GetTargetHttpProxy(tpName); err != nil {
		t.Fatalf("expected nil error, actual: %v", err)
	}
	// Verify that GET fails after DELETE.
	if err := tps.DeleteTargetProxies(); err != nil {
		t.Fatalf("unexpected error while deleting target proxies: %s", err)
	}
	if _, err := tpp.GetTargetHttpProxy(tpName); err == nil {
		t.Fatalf("unexpected nil error, expected NotFound")
	}
}
