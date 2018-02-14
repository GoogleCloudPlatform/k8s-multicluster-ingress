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
	"reflect"
	"testing"

	ingresslb "k8s.io/ingress-gce/pkg/loadbalancers"

	utilsnamer "github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/namer"
	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/status"
	"github.com/golang/glog"
)

func TestEnsureHttpForwardingRule(t *testing.T) {
	lbName := "lb-name"
	namer := utilsnamer.NewNamer("mci1", lbName)
	frName := namer.HttpForwardingRuleName()
	frp := ingresslb.NewFakeLoadBalancers("" /*name*/, nil /*namer*/)
	// GET should return NotFound.
	if _, err := frp.GetGlobalForwardingRule(frName); err == nil {
		t.Errorf("expected NotFound error, got nil.")
	}
	testCases := []struct {
		desc string
		// In's
		ipAddr      string
		forceUpdate bool
		// Out's
		ensureErr      bool
		expectedIpAddr string
	}{
		{
			desc:           "initial write (force=false)",
			ipAddr:         "1.2.3.4",
			forceUpdate:    false,
			ensureErr:      false,
			expectedIpAddr: "1.2.3.4",
		},
		{
			desc:           "write same (force=false)",
			ipAddr:         "1.2.3.4",
			forceUpdate:    false,
			ensureErr:      false,
			expectedIpAddr: "1.2.3.4",
		},
		{
			desc:           "write different (force=false)",
			ipAddr:         "2.2.2.2",
			forceUpdate:    false,
			ensureErr:      true,
			expectedIpAddr: "1.2.3.4",
		},
		{
			desc:           "write different (force=true)",
			ipAddr:         "2.2.2.2",
			forceUpdate:    true,
			ensureErr:      false,
			expectedIpAddr: "2.2.2.2",
		},
	}
	// We want the load balancers to exist across test cases.
	for _, c := range testCases {
		glog.Infof("===============testing case: %s=================", c.desc)
		tpLink := "fakeLink"
		clusters := []string{"cluster1", "cluster2"}
		// Should create the forwarding rule as expected.
		namer := utilsnamer.NewNamer("mci1", lbName)
		frs := NewForwardingRuleSyncer(namer, frp)
		err := frs.EnsureHttpForwardingRule(lbName, c.ipAddr, tpLink, clusters, c.forceUpdate)
		if (err != nil) != c.ensureErr {
			t.Errorf("in ensuring forwarding rule, expected err? %v, actual: %v", c.ensureErr, err)
		}
		if c.ensureErr {
			glog.Infof("Skipping validation for this test case.")
			continue
		}
		// Verify that GET does not return NotFound and the returned forwarding rule is as expected.
		fr, err := frp.GetGlobalForwardingRule(frName)
		if err != nil {
			t.Errorf("expected nil error getting forwarding rules, actual: %v", err)
		}
		if fr.IPAddress != c.expectedIpAddr {
			t.Errorf("unexpected ip address. expected: %s, got: %s", c.expectedIpAddr, fr.IPAddress)
		}
		if fr.Target != tpLink {
			t.Errorf("unexpected target proxy link. expected: %s, got: %s", tpLink, fr.Target)
		}
		if fr.PortRange != httpDefaultPortRange {
			t.Errorf("unexpected port range: %s, expected: %s", fr.PortRange, httpDefaultPortRange)
		}
		expectedDescr := status.LoadBalancerStatus{
			LoadBalancerName: lbName,
			Clusters:         clusters,
			IPAddress:        c.expectedIpAddr,
		}
		status, err := status.FromString(fr.Description)
		if err != nil {
			t.Errorf("unexpected error %s in unmarshalling forwarding rule description %v", err, fr.Description)
		}
		if status.LoadBalancerName != expectedDescr.LoadBalancerName || !reflect.DeepEqual(status.Clusters, expectedDescr.Clusters) || status.IPAddress != expectedDescr.IPAddress {
			t.Errorf("unexpected description: %v, expected: %v", status, expectedDescr)
		}
	}
}

func TestEnsureHttpsForwardingRule(t *testing.T) {
	lbName := "lb-name"
	namer := utilsnamer.NewNamer("mci1", lbName)
	frName := namer.HttpsForwardingRuleName()
	frp := ingresslb.NewFakeLoadBalancers("" /*name*/, nil /*namer*/)
	// GET should return NotFound.
	if _, err := frp.GetGlobalForwardingRule(frName); err == nil {
		t.Errorf("expected NotFound error, got nil.")
	}
	testCases := []struct {
		desc string
		// In's
		ipAddr      string
		forceUpdate bool
		// Out's
		ensureErr      bool
		expectedIpAddr string
	}{
		{
			desc:           "initial write (force=false)",
			ipAddr:         "1.2.3.4",
			forceUpdate:    false,
			ensureErr:      false,
			expectedIpAddr: "1.2.3.4",
		},
		{
			desc:           "write same (force=false)",
			ipAddr:         "1.2.3.4",
			forceUpdate:    false,
			ensureErr:      false,
			expectedIpAddr: "1.2.3.4",
		},
		{
			desc:           "write different (force=false)",
			ipAddr:         "2.2.2.2",
			forceUpdate:    false,
			ensureErr:      true,
			expectedIpAddr: "1.2.3.4",
		},
		{
			desc:           "write different (force=true)",
			ipAddr:         "2.2.2.2",
			forceUpdate:    true,
			ensureErr:      false,
			expectedIpAddr: "2.2.2.2",
		},
	}
	// We want the load balancers to exist across test cases.
	for _, c := range testCases {
		glog.Infof("===============testing case: %s=================", c.desc)
		tpLink := "fakeLink"
		clusters := []string{"cluster1", "cluster2"}
		// Should create the forwarding rule as expected.
		namer := utilsnamer.NewNamer("mci1", lbName)
		frs := NewForwardingRuleSyncer(namer, frp)
		err := frs.EnsureHttpsForwardingRule(lbName, c.ipAddr, tpLink, clusters, c.forceUpdate)
		if (err != nil) != c.ensureErr {
			t.Errorf("in ensuring forwarding rule, expected err? %v, actual: %v", c.ensureErr, err)
		}
		if c.ensureErr {
			glog.Infof("Skipping validation for this test case.")
			continue
		}
		// Verify that GET does not return NotFound and the returned forwarding rule is as expected.
		fr, err := frp.GetGlobalForwardingRule(frName)
		if err != nil {
			t.Errorf("expected nil error, actual: %v", err)
		}
		if fr.IPAddress != c.expectedIpAddr {
			t.Errorf("unexpected ip address. expected: %s, got: %s", c.expectedIpAddr, fr.IPAddress)
		}
		if fr.Target != tpLink {
			t.Errorf("unexpected target proxy link. expected: %s, got: %s", tpLink, fr.Target)
		}
		if fr.PortRange != httpsDefaultPortRange {
			t.Errorf("unexpected port range: %s, expected: %s", fr.PortRange, httpsDefaultPortRange)
		}
		expectedDescr := status.LoadBalancerStatus{
			LoadBalancerName: lbName,
			Clusters:         clusters,
			IPAddress:        c.expectedIpAddr,
		}
		status, err := status.FromString(fr.Description)
		if err != nil {
			t.Errorf("unexpected error %s in unmarshalling forwarding rule description %v", err, fr.Description)
		}
		if status.LoadBalancerName != expectedDescr.LoadBalancerName || !reflect.DeepEqual(status.Clusters, expectedDescr.Clusters) || status.IPAddress != expectedDescr.IPAddress {
			t.Errorf("unexpected description: %v, expected: %v", status, expectedDescr)
		}
	}
}

func TestListLoadBalancerStatuses(t *testing.T) {
	lbName := "lb-name"
	ipAddr := "1.2.3.4"
	tpLink := "fakeLink"
	clusters := []string{"cluster1", "cluster2", "cluster3"}
	type ensureFR struct {
		lbName   string
		ipAddr   string
		tpLink   string
		clusters []string
		// True if this is a https forwarding rule,
		// false for http forwarding rule.
		https bool
	}
	testCases := []struct {
		description     string
		forwardingRules []ensureFR
		expectedList    map[string]status.LoadBalancerStatus
	}{
		{
			"Should get an empty list when there are no forwarding rules",
			[]ensureFR{},
			map[string]status.LoadBalancerStatus{},
		},
		{
			"Forwarding rule should be listed as expected",
			[]ensureFR{
				{
					lbName,
					ipAddr,
					tpLink,
					clusters,
					false,
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
			"Load balancer should be listed only once, when both http and https forwarding rules exist for the same load balancer",
			[]ensureFR{
				{
					lbName,
					ipAddr,
					tpLink,
					clusters,
					false,
				},
				{
					lbName,
					ipAddr,
					tpLink,
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
			"Both http and https load balancers should be listed",
			[]ensureFR{
				{
					lbName + "1",
					ipAddr,
					tpLink,
					clusters,
					false,
				},
				{
					lbName + "2",
					ipAddr,
					tpLink,
					clusters,
					true,
				},
			},
			map[string]status.LoadBalancerStatus{
				lbName + "1": {
					"",
					lbName + "1",
					clusters,
					ipAddr,
				},
				lbName + "2": {
					"",
					lbName + "2",
					clusters,
					ipAddr,
				},
			},
		},
	}
	namer := utilsnamer.NewNamer("mci1", lbName)
	for i, c := range testCases {
		glog.Infof("Test case: %d: %s", i, c.description)
		frp := ingresslb.NewFakeLoadBalancers("" /*name*/, nil /*namer*/)
		frs := NewForwardingRuleSyncer(namer, frp)
		// Create all forwarding rules.
		for _, fr := range c.forwardingRules {
			if fr.https {
				if err := frs.EnsureHttpsForwardingRule(fr.lbName, fr.ipAddr, fr.tpLink, fr.clusters, false /*force*/); err != nil {
					t.Errorf("expected no error in ensuring https forwarding rule, actual: %v", err)
				}
			} else {
				if err := frs.EnsureHttpForwardingRule(fr.lbName, fr.ipAddr, fr.tpLink, fr.clusters, false /*force*/); err != nil {
					t.Errorf("expected no error in ensuring http forwarding rule, actual: %v", err)
				}
			}
		}
		// List them to verify that they are as expected.
		list, err := frs.ListLoadBalancerStatuses()
		if err != nil {
			t.Errorf("unexpected error listing load balancer statuses: %v", err)
		}
		if len(list) != len(c.expectedList) {
			t.Errorf("unexpected list of firewall rules, expected: %v, actual: %v", c.expectedList, list)
		}
		for _, rule := range list {
			expectedRule, ok := c.expectedList[rule.LoadBalancerName]
			if !ok {
				t.Errorf("unexpected rule %v. expected rules: %v, actual: %v", rule, c.expectedList, list)
			}
			if rule.LoadBalancerName != expectedRule.LoadBalancerName {
				t.Errorf("Unexpected name. expected: %s, acctual: %s", expectedRule.LoadBalancerName, rule.LoadBalancerName)
			}
			if rule.IPAddress != expectedRule.IPAddress {
				t.Errorf("Unexpected IP addr. expected %s, actual: %s", expectedRule.IPAddress, rule.IPAddress)
			}
			actualClusters := mapFromSlice(rule.Clusters)
			for _, expected := range expectedRule.Clusters {
				if _, ok := actualClusters[expected]; !ok {
					t.Errorf("expected cluster %s is missing from resultant cluster list %v", expected, rule.Clusters)
				}
			}
		}
	}
}

func mapFromSlice(strs []string) map[string]struct{} {
	mymap := make(map[string]struct{})
	for _, s := range strs {
		mymap[s] = struct{}{}
	}
	return mymap
}

func TestListLoadBalancersWithSkippedRules(t *testing.T) {
	lbName := "lb-name"
	// Should create the forwarding rule as expected.
	frp := ingresslb.NewFakeLoadBalancers("" /*name*/, nil /*namer*/)
	namer := utilsnamer.NewNamer("mci1", lbName)
	frs := NewForwardingRuleSyncer(namer, frp)
	ipAddr := "1.2.3.4"
	tpLink := "fakeLink"

	clusters := []string{"cluster1", "cluster2", "cluster3", "fooCluster"}

	if err := frs.EnsureHttpForwardingRule(lbName, ipAddr, tpLink, clusters, false /*force*/); err != nil {
		t.Fatalf("expected no error in ensuring forwarding rule, actual: %v", err)
	}

	// Add anoter forwarding rule that does *not* match our normal prefix.
	namer = utilsnamer.NewNamer("otherFR", "blah")
	frs = NewForwardingRuleSyncer(namer, frp)
	if err := frs.EnsureHttpForwardingRule(lbName, ipAddr, tpLink, clusters, false /*force*/); err != nil {
		t.Fatalf("expected no error in ensuring forwarding rule, actual: %v", err)
	}

	list, err := frs.ListLoadBalancerStatuses()
	if err != nil {
		t.Errorf("unexpected error listing load balancer statuses: %v", err)
	}
	for _, rule := range list {
		if rule.LoadBalancerName != lbName {
			t.Errorf("Unexpected name. expected: %s, acctual: %s", lbName, rule.LoadBalancerName)
		}
		if rule.IPAddress != ipAddr {
			t.Errorf("Unexpected IP addr. expected %s, actual: %s", ipAddr, rule.IPAddress)
		}
		actualClusters := mapFromSlice(rule.Clusters)
		for _, expected := range clusters {
			if _, ok := actualClusters[expected]; !ok {
				t.Errorf("cluster %s is missing from resultant cluster list %v", expected, rule.Clusters)
			}
		}
	}
}

func TestDeleteForwardingRule(t *testing.T) {
	lbName := "lb-name"
	ipAddr := "1.2.3.4"
	tpLink := "fakeLink"
	frp := ingresslb.NewFakeLoadBalancers("" /*name*/, nil /*namer*/)
	namer := utilsnamer.NewNamer("mci1", lbName)
	httpFrName := namer.HttpForwardingRuleName()
	httpsFrName := namer.HttpsForwardingRuleName()
	frs := NewForwardingRuleSyncer(namer, frp)
	// Create both the http and https forwarding rules and verify that it deletes both.
	if err := frs.EnsureHttpForwardingRule(lbName, ipAddr, tpLink, []string{}, false /*force*/); err != nil {
		t.Fatalf("expected no error in ensuring http forwarding rule, actual: %v", err)
	}
	if err := frs.EnsureHttpsForwardingRule(lbName, ipAddr, tpLink, []string{}, false /*force*/); err != nil {
		t.Fatalf("expected no error in ensuring https forwarding rule, actual: %v", err)
	}
	if _, err := frp.GetGlobalForwardingRule(httpFrName); err != nil {
		t.Fatalf("expected nil error in getting http forwarding rule, actual: %v", err)
	}
	if _, err := frp.GetGlobalForwardingRule(httpsFrName); err != nil {
		t.Fatalf("expected nil error in getting https forwarding rule, actual: %v", err)
	}
	// GET should return NotFound after DELETE.
	if err := frs.DeleteForwardingRules(); err != nil {
		t.Fatalf("unexpeted error in deleting forwarding rule: %s", err)
	}
	if _, err := frp.GetGlobalForwardingRule(httpFrName); err == nil {
		t.Fatalf("expected not found error in getting http forwarding rule, actual: nil")
	}
	if _, err := frp.GetGlobalForwardingRule(httpsFrName); err == nil {
		t.Fatalf("expected not found error in getting https forwarding rule, actual: nil")
	}
}

// Tests that the Load Balancer status contains the expected data (mci metadata).
func TestGetLoadBalancerStatus(t *testing.T) {
	lbName := "lb-name"
	ipAddr := "1.2.3.4"
	tpLink := "fakeLink"
	clusters := []string{"cluster1", "cluster2"}
	// Should create the forwarding rule as expected.
	frp := ingresslb.NewFakeLoadBalancers("" /*name*/, nil /*namer*/)
	namer := utilsnamer.NewNamer("mci1", lbName)
	frs := NewForwardingRuleSyncer(namer, frp)
	testCases := []struct {
		// Should the http forwarding rule be created.
		http bool
		// Should the https forwarding rule be created.
		https bool
		// Is get status expected to return an error.
		shouldErr bool
	}{
		{
			http:      true,
			https:     false,
			shouldErr: false,
		},
		{
			http:      false,
			https:     true,
			shouldErr: false,
		},
		{
			http:      true,
			https:     true,
			shouldErr: false,
		},
		{
			http:      false,
			https:     false,
			shouldErr: true,
		},
	}
	for i, c := range testCases {
		glog.Infof("===============testing case: %d=================", i)
		if c.http {
			// Ensure http forwarding rule.
			if err := frs.EnsureHttpForwardingRule(lbName, ipAddr, tpLink, clusters, false /*force*/); err != nil {
				t.Errorf("expected no error in ensuring http forwarding rule, actual: %v", err)
			}
		}
		if c.https {
			// Ensure https forwarding rule.
			if err := frs.EnsureHttpsForwardingRule(lbName, ipAddr, tpLink, clusters, false /*force*/); err != nil {
				t.Errorf("expected no error in ensuring https forwarding rule, actual: %v", err)
			}
		}
		status, err := frs.GetLoadBalancerStatus(lbName)
		if c.shouldErr != (err != nil) {
			t.Errorf("unexpected error in getting status description for load balancer %s, expected err != nil: %v, actual err: %s", lbName, c.shouldErr, err)
		}
		if c.shouldErr {
			continue
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
		// Delete the forwarding rules if we created at least one.
		if c.http || c.https {
			if err := frs.DeleteForwardingRules(); err != nil {
				t.Errorf("unexpeted error in deleting forwarding rules: %s", err)
			}
		}
	}
}
