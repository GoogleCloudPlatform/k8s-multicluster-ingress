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
	"fmt"

	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/status"
)

type FakeForwardingRule struct {
	LBName    string
	IPAddress string
	TPLink    string
	Clusters  []string
	IsHTTPS   bool
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

func (f *FakeForwardingRuleSyncer) EnsureHttpForwardingRule(lbName, ipAddress, targetProxyLink string, clusters []string, forceUpdate bool) error {
	f.EnsuredForwardingRules = append(f.EnsuredForwardingRules, FakeForwardingRule{
		LBName:    lbName,
		IPAddress: ipAddress,
		TPLink:    targetProxyLink,
		Clusters:  clusters,
	})
	return nil
}

func (f *FakeForwardingRuleSyncer) EnsureHttpsForwardingRule(lbName, ipAddress, targetProxyLink string, clusters []string, forceUpdate bool) error {
	f.EnsuredForwardingRules = append(f.EnsuredForwardingRules, FakeForwardingRule{
		LBName:    lbName,
		IPAddress: ipAddress,
		TPLink:    targetProxyLink,
		Clusters:  clusters,
		IsHTTPS:   true,
	})
	return nil
}

func (f *FakeForwardingRuleSyncer) DeleteForwardingRules() error {
	f.EnsuredForwardingRules = nil
	return nil
}

func (f *FakeForwardingRuleSyncer) GetLoadBalancerStatus(lbName string) (*status.LoadBalancerStatus, error) {
	for _, fr := range f.EnsuredForwardingRules {
		if fr.LBName == lbName {
			return &status.LoadBalancerStatus{
				LoadBalancerName: lbName,
				Clusters:         fr.Clusters,
				IPAddress:        fr.IPAddress,
			}, nil
		}
	}
	return nil, fmt.Errorf("load balancer %s does not exist", lbName)
}

func (f *FakeForwardingRuleSyncer) ListLoadBalancerStatuses() ([]status.LoadBalancerStatus, error) {
	var ret []status.LoadBalancerStatus
	for _, fr := range f.EnsuredForwardingRules {
		status := status.LoadBalancerStatus{
			LoadBalancerName: fr.LBName,
			Clusters:         fr.Clusters,
			IPAddress:        fr.IPAddress,
		}
		ret = append(ret, status)
	}
	return ret, nil
}

func (f *FakeForwardingRuleSyncer) RemoveClustersFromStatus(clusters []string) error {
	// TODO: Implement this.
	return nil
}
