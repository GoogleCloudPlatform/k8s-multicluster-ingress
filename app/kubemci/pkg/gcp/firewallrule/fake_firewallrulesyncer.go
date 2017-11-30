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

type FakeFirewallRule struct {
	LBName  string
	Ports   []ingressbe.ServicePort
	IGLinks map[string][]string
}

type FakeFirewallRuleSyncer struct {
	// List of firewall rules that this has been asked to ensure.
	EnsuredFirewallRules []FakeFirewallRule
}

// Fake firewall rule syncer to be used for tests.
func NewFakeFirewallRuleSyncer() FirewallRuleSyncerInterface {
	return &FakeFirewallRuleSyncer{}
}

// Ensure this implements FirewallRuleSyncerInterface.
var _ FirewallRuleSyncerInterface = &FakeFirewallRuleSyncer{}

func (h *FakeFirewallRuleSyncer) EnsureFirewallRule(lbName string, ports []ingressbe.ServicePort, igLinks map[string][]string, forceUpdate bool) error {
	h.EnsuredFirewallRules = append(h.EnsuredFirewallRules, FakeFirewallRule{
		LBName:  lbName,
		Ports:   ports,
		IGLinks: igLinks,
	})
	return nil
}

func (h *FakeFirewallRuleSyncer) DeleteFirewallRules() error {
	h.EnsuredFirewallRules = nil
	return nil
}
