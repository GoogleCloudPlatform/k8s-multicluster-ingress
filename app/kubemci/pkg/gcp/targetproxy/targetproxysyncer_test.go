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
		tpLink, err := tps.EnsureHTTPTargetProxy(lbName, c.umLink, c.forceUpdate)
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
}

func TestEnsureTargetHttpsProxy(t *testing.T) {
	lbName := "lb-name"
	// Should create the target proxy as expected.
	tpp := ingresslb.NewFakeLoadBalancers("" /*name*/, nil /*namer*/)
	namer := utilsnamer.NewNamer("mci1", lbName)
	tpName := namer.TargetHttpsProxyName()
	tps := NewTargetProxySyncer(namer, tpp)
	// GET should return NotFound.
	if _, err := tpp.GetTargetHttpProxy(tpName); err == nil {
		t.Fatalf("expected NotFound error, got nil")
	}
	testCases := []struct {
		desc string
		// In's
		umLink      string
		certLink    string
		forceUpdate bool
		// Out's
		ensureErr bool
	}{
		{
			desc:        "initial write (force=false)",
			umLink:      "http://google/compute/v1/projects/p/global/urlMaps/map",
			certLink:    "http://google/compute/v1/projects/p/global/sslCerts/cert",
			forceUpdate: false,
			ensureErr:   false,
		},
		{
			desc:        "write same (force=false)",
			umLink:      "http://google/compute/v1/projects/p/global/urlMaps/map",
			certLink:    "http://google/compute/v1/projects/p/global/sslCerts/cert",
			forceUpdate: false,
			ensureErr:   false,
		},
		{
			desc:        "write different (force=false)",
			umLink:      "http://google/compute/v1/projects/p/global/urlMaps/map2",
			certLink:    "http://google/compute/v1/projects/p/global/sslCerts/cert",
			forceUpdate: false,
			ensureErr:   true,
		},
		{
			desc:        "write different (force=true)",
			umLink:      "http://google/compute/v1/projects/p/global/urlMaps/map3",
			certLink:    "http://google/compute/v1/projects/p/global/sslCerts/cert",
			forceUpdate: true,
			ensureErr:   false,
		},
	}
	for _, c := range testCases {
		glog.Infof("test case:%v", c.desc)
		tpLink, err := tps.EnsureHTTPSTargetProxy(lbName, c.umLink, c.certLink, c.forceUpdate)
		if (err != nil) != c.ensureErr {
			glog.Errorf("expected_error:%v Got Error:%v", c.ensureErr, err)
			t.Errorf("in ensuring target proxy, expected error? %v, actual: %v", c.ensureErr, err)
		}
		// Verify that GET does not return NotFound.
		tp, err := tpp.GetTargetHttpsProxy(tpName)
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
		if len(tp.SslCertificates) != 1 || tp.SslCertificates[0] != c.certLink {
			t.Errorf("unexpected certificates link in target proxy. expected: %s, actual: %s", c.certLink, tp.SslCertificates)
		}
		if tp.SelfLink != tpLink {
			t.Errorf("unexpected target proxy self link. expected: %s, got: %s", tpLink, tp.SelfLink)
		}
	}
}

func TestDeleteTargetProxies(t *testing.T) {
	lbName := "lb-name"
	umLink := "selfLink"
	certLink := "certSelfLink"
	tpp := ingresslb.NewFakeLoadBalancers("" /*name*/, nil /*namer*/)
	namer := utilsnamer.NewNamer("mci1", lbName)
	httpTpName := namer.TargetHttpProxyName()
	httpsTpName := namer.TargetHttpsProxyName()
	tps := NewTargetProxySyncer(namer, tpp)
	// Verify that trying to delete when no proxy exists does not return any error.
	if err := tps.DeleteTargetProxies(); err != nil {
		t.Errorf("unexpected error in deleting target proxies when none exist: %s", err)
	}
	// Create both http and https and verify that it deletes both.
	if _, err := tps.EnsureHTTPTargetProxy(lbName, umLink, false /*forceUpdate*/); err != nil {
		t.Fatalf("expected no error in ensuring http target proxy, actual: %v", err)
	}
	if _, err := tpp.GetTargetHttpProxy(httpTpName); err != nil {
		t.Fatalf("expected nil error, actual: %v", err)
	}
	if _, err := tps.EnsureHTTPSTargetProxy(lbName, umLink, certLink, false /*forceUpdate*/); err != nil {
		t.Fatalf("expected no error in ensuring https target proxy, actual: %v", err)
	}
	if _, err := tpp.GetTargetHttpsProxy(httpsTpName); err != nil {
		t.Fatalf("expected nil error, actual: %v", err)
	}
	// Verify that GET fails after DELETE.
	if err := tps.DeleteTargetProxies(); err != nil {
		t.Fatalf("unexpected error while deleting target proxies: %s", err)
	}
	if _, err := tpp.GetTargetHttpProxy(httpTpName); err == nil {
		t.Fatalf("unexpected nil error, expected NotFound")
	}
	if _, err := tpp.GetTargetHttpsProxy(httpTpName); err == nil {
		t.Fatalf("unexpected nil error, expected NotFound")
	}
}
