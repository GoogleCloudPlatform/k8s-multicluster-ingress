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

package firewallrule

import (
	"reflect"
	"strconv"
	"testing"

	"k8s.io/apimachinery/pkg/types"
	ingressbe "k8s.io/ingress-gce/pkg/backends"
	ingressfw "k8s.io/ingress-gce/pkg/firewalls"

	utilsnamer "github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/namer"
	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/networktags"
)

func TestEnsureFirewallRule(t *testing.T) {
	lbName := "lb-name"
	port := int64(32211)
	kubeSvcName := "svc-name"
	igLink := "https://www.googleapis.com/compute/v1/projects/abc/zones/def/instanceGroups/ig1"
	networkTag := "fake-network-tag"
	// Should create the firewall rule as expected.
	fwp := ingressfw.NewFakeFirewallsProvider(false /* xpn */, false /* read only */)
	ntg := networktags.NewFakeNetworkTagsGetter([]string{networkTag})
	namer := utilsnamer.NewNamer("mci1", lbName)
	fwName := namer.FirewallRuleName()
	fws := NewFirewallRuleSyncer(namer, fwp, ntg)
	// GET should return NotFound.
	if _, err := fwp.GetFirewall(fwName); err == nil {
		t.Fatalf("expected NotFound error, actual: nil")
	}
	err := fws.EnsureFirewallRule(lbName, []ingressbe.ServicePort{
		{
			Port:     port,
			Protocol: "HTTP",
			SvcName:  types.NamespacedName{Name: kubeSvcName},
		},
	}, map[string][]string{"cluster1": {igLink}})
	if err != nil {
		t.Fatalf("expected no error in ensuring firewall rule, actual: %v", err)
	}
	// Verify that the created firewall rule is as expected.
	fw, err := fwp.GetFirewall(fwName)
	if err != nil {
		t.Fatalf("expected nil error, actual: %v", err)
	}
	if !reflect.DeepEqual(fw.SourceRanges, l7SrcRanges) {
		t.Errorf("unexpected source ranges, expected: %s, got: %s", l7SrcRanges, fw.SourceRanges)
	}
	expectedPort := strconv.Itoa(int(port))
	if len(fw.Allowed) != 1 || len(fw.Allowed[0].Ports) != 1 || fw.Allowed[0].Ports[0] != expectedPort {
		t.Errorf("unexpected allowed, expected only one port item with port %s, got: %v", expectedPort, fw.Allowed)
	}
	if len(fw.TargetTags) != 1 || fw.TargetTags[0] != networkTag {
		t.Errorf("unexpected target tags in firewall rule, expected only on item for %s, got: %v", networkTag, fw.TargetTags)
	}
	// TODO(nikhiljindal): Test update existing firewall rule.
}

func TestDeleteFirewallRule(t *testing.T) {
	lbName := "lb-name"
	port := int64(32211)
	kubeSvcName := "svc-name"
	igLink := "https://www.googleapis.com/compute/v1/projects/abc/zones/def/instanceGroups/ig1"
	networkTag := "fake-network-tag"
	// Should create the firewall rule as expected.
	fwp := ingressfw.NewFakeFirewallsProvider(false /* xpn */, false /* read only */)
	ntg := networktags.NewFakeNetworkTagsGetter([]string{networkTag})
	namer := utilsnamer.NewNamer("mci1", lbName)
	fwName := namer.FirewallRuleName()
	fws := NewFirewallRuleSyncer(namer, fwp, ntg)
	// GET should return NotFound.
	if _, err := fwp.GetFirewall(fwName); err == nil {
		t.Fatalf("expected NotFound error, actual: nil")
	}
	err := fws.EnsureFirewallRule(lbName, []ingressbe.ServicePort{
		{
			Port:     port,
			Protocol: "HTTP",
			SvcName:  types.NamespacedName{Name: kubeSvcName},
		},
	}, map[string][]string{"cluster1": {igLink}})
	if err != nil {
		t.Fatalf("expected no error in ensuring firewall rule, actual: %v", err)
	}
	// Verify that the created firewall rule is as expected.
	_, err = fwp.GetFirewall(fwName)
	if err != nil {
		t.Fatalf("expected nil error, actual: %v", err)
	}

	// Verify that GET fails after DELETE.
	if err := fws.DeleteFirewallRules(); err != nil {
		t.Fatalf("unexpected error in deleting firewall rules: %s", err)
	}
	if _, err := fwp.GetFirewall(fwName); err == nil {
		t.Errorf("unexpected nil error, expected NotFound")
	}
}
