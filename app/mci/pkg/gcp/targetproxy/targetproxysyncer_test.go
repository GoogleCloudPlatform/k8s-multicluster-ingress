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

	utilsnamer "github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/mci/pkg/gcp/namer"
)

func TestEnsureTargetHttpProxy(t *testing.T) {
	lbName := "lb-name"
	umLink := "selfLink"
	// Should create the target proxy as expected.
	tpp := ingresslb.NewFakeLoadBalancers("")
	namer := utilsnamer.NewNamer("mci", lbName)
	tpName := namer.TargetHttpProxyName()
	tps := NewTargetProxySyncer(namer, tpp)
	// GET should return NotFound.
	if _, err := tpp.GetTargetHttpProxy(tpName); err == nil {
		t.Fatalf("expected NotFound error, got nil")
	}
	tpLink, err := tps.EnsureHttpTargetProxy(lbName, umLink)
	if err != nil {
		t.Fatalf("expected no error in ensuring target proxy, actual: %v", err)
	}
	// Verify that GET does not return NotFound.
	tp, err := tpp.GetTargetHttpProxy(tpName)
	if err != nil {
		t.Fatalf("expected nil error, actual: %v", err)
	}
	if tp.UrlMap != umLink {
		t.Errorf("unexpected UrlMap link in target proxy. expected: %s, actual: %s", umLink, tp.UrlMap)
	}
	if tp.SelfLink != tpLink {
		t.Errorf("unexpected target proxy self link. expected: %s, got: %s", tpLink, tp.SelfLink)
	}
	// TODO(nikhiljindal): Test update existing target proxy.
}

func TestDeleteTargetProxies(t *testing.T) {
	lbName := "lb-name"
	umLink := "selfLink"
	// Should create the target proxy as expected.
	tpp := ingresslb.NewFakeLoadBalancers("")
	namer := utilsnamer.NewNamer("mci", lbName)
	tpName := namer.TargetHttpProxyName()
	tps := NewTargetProxySyncer(namer, tpp)
	if _, err := tps.EnsureHttpTargetProxy(lbName, umLink); err != nil {
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
