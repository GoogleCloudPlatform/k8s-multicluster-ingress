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

type FakeForwardingRule struct {
	LBName    string
	IPAddress string
	TPLink    string
}

type FakeForwardingRuleSyncer struct {
	// List of forwarding rules that this has been asked to ensure.
	EnsuredForwardingRules []FakeForwardingRule
}

// Fake forwarding rule syncer to be used for tests.
func NewFakeForwardingRuleSyncer() ForwardingRuleSyncerInterface {
	return &FakeForwardingRuleSyncer{}
}

// Ensure this implements ForwardingRuleSyncerInterface.
var _ ForwardingRuleSyncerInterface = &FakeForwardingRuleSyncer{}

func (f *FakeForwardingRuleSyncer) EnsureForwardingRule(lbName, ipAddress, targetProxyLink string) error {
	f.EnsuredForwardingRules = append(f.EnsuredForwardingRules, FakeForwardingRule{
		LBName:    lbName,
		IPAddress: ipAddress,
		TPLink:    targetProxyLink,
	})
	return nil
}
