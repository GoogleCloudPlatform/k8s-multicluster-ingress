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
	ingressbe "k8s.io/ingress-gce/pkg/backends"
)

type fakeFirewallRule struct {
	LBName  string
	Ports   []ingressbe.ServicePort
	IGLinks map[string][]string
}

// FakeFirewallRuleSyncer is a fake implementation of SyncerInterface to be used in tests.
type FakeFirewallRuleSyncer struct {
	// List of firewall rules that this has been asked to ensure.
	EnsuredFirewallRules []fakeFirewallRule
}

// NewFakeFirewallRuleSyncer returns a new instance of the fake syncer.
func NewFakeFirewallRuleSyncer() SyncerInterface {
	return &FakeFirewallRuleSyncer{}
}

// Ensure this implements SyncerInterface.
var _ SyncerInterface = &FakeFirewallRuleSyncer{}

// EnsureFirewallRule ensures that the required firewall rules exist.
// See the interface for more details.
func (h *FakeFirewallRuleSyncer) EnsureFirewallRule(lbName string, ports []ingressbe.ServicePort, igLinks map[string][]string, forceUpdate bool) error {
	h.EnsuredFirewallRules = append(h.EnsuredFirewallRules, fakeFirewallRule{
		LBName:  lbName,
		Ports:   ports,
		IGLinks: igLinks,
	})
	return nil
}

// DeleteFirewallRules deletes the firewall rules that EnsureFirewallRule would have created.
// See the interface for more details.
func (h *FakeFirewallRuleSyncer) DeleteFirewallRules() error {
	h.EnsuredFirewallRules = nil
	return nil
}

// RemoveFromClusters removes the clusters corresponding to the given instance groups from the firewall rule.
// See the interface for more details.
func (h *FakeFirewallRuleSyncer) RemoveFromClusters(lbName string, removeIGLinks map[string][]string) error {
	for i, v := range h.EnsuredFirewallRules {
		if v.LBName != lbName {
			continue
		}
		newIGLinks := map[string][]string{}
		for clusterName, igValues := range v.IGLinks {
			if _, has := removeIGLinks[clusterName]; !has {
				newIGLinks[clusterName] = igValues
			}
		}
		h.EnsuredFirewallRules[i].IGLinks = newIGLinks
	}
	return nil
}
