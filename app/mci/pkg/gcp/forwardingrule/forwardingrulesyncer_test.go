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

package forwardingrule

import (
	"testing"

	ingresslb "k8s.io/ingress-gce/pkg/loadbalancers"

	utilsnamer "github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/mci/pkg/gcp/namer"
)

func TestEnsureHttpForwardingRule(t *testing.T) {
	lbName := "lb-name"
	ipAddr := "1.2.3.4"
	tpLink := "fakeLink"
	// Should create the forwarding rule as expected.
	frp := ingresslb.NewFakeLoadBalancers("")
	namer := utilsnamer.NewNamer("mci", lbName)
	frName := namer.HttpForwardingRuleName()
	frs := NewForwardingRuleSyncer(namer, frp)
	// GET should return NotFound.
	if _, err := frp.GetGlobalForwardingRule(frName); err == nil {
		t.Fatalf("expected NotFound error, got nil")
	}
	err := frs.EnsureHttpForwardingRule(lbName, ipAddr, tpLink)
	if err != nil {
		t.Fatalf("expected no error in ensuring forwarding rule, actual: %v", err)
	}
	// Verify that GET does not return NotFound and the returned forwarding rule is as expected.
	fr, err := frp.GetGlobalForwardingRule(frName)
	if err != nil {
		t.Fatalf("expected nil error, actual: %v", err)
	}
	if fr.IPAddress != ipAddr {
		t.Errorf("unexpected ip address. expected: %s, got: %s", ipAddr, fr.IPAddress)
	}
	if fr.Target != tpLink {
		t.Errorf("unexpected target proxy link. expected: %s, got: %s", tpLink, fr.Target)
	}
	// TODO(nikhiljindal): Test update existing forwarding rule.
}

func TestDeleteForwardingRule(t *testing.T) {
	lbName := "lb-name"
	ipAddr := "1.2.3.4"
	tpLink := "fakeLink"
	// Should create the forwarding rule as expected.
	frp := ingresslb.NewFakeLoadBalancers("")
	namer := utilsnamer.NewNamer("mci", lbName)
	frName := namer.HttpForwardingRuleName()
	frs := NewForwardingRuleSyncer(namer, frp)
	if err := frs.EnsureHttpForwardingRule(lbName, ipAddr, tpLink); err != nil {
		t.Fatalf("expected no error in ensuring forwarding rule, actual: %v", err)
	}
	if _, err := frp.GetGlobalForwardingRule(frName); err != nil {
		t.Fatalf("expected nil error, actual: %v", err)
	}
	// GET should return NotFound after DELETE.
	if err := frs.DeleteForwardingRules(); err != nil {
		t.Fatalf("unexpeted error in deleting forwarding rule: %s", err)
	}
	if _, err := frp.GetGlobalForwardingRule(frName); err == nil {
		t.Fatalf("expected not found error, actual: nil")
	}
}
