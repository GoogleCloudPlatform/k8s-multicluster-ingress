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
	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/goutils"
)

type fakeForwardingRule struct {
	LBName    string
	IPAddress string
	TPLink    string
	IsHTTPS   bool
	Status    *status.LoadBalancerStatus
}

// FakeForwardingRuleSyncer is a fake implementation of SyncerInterface to be used in tests.
type FakeForwardingRuleSyncer struct {
	// List of forwarding rules that this has been asked to ensure.
	EnsuredForwardingRules []fakeForwardingRule
}

// NewFakeForwardingRuleSyncer returns a new instance of fake syncer.
func NewFakeForwardingRuleSyncer() SyncerInterface {
	return &FakeForwardingRuleSyncer{}
}

// Ensure this implements SyncerInterface.
var _ SyncerInterface = &FakeForwardingRuleSyncer{}

// EnsureHTTPForwardingRule ensures that a http forwarding rule exists for the given load balancer.
// See interface for more details.
func (f *FakeForwardingRuleSyncer) EnsureHTTPForwardingRule(lbName, ipAddress, targetProxyLink string, forceUpdate bool) error {
	f.EnsuredForwardingRules = append(f.EnsuredForwardingRules, fakeForwardingRule{
		LBName:    lbName,
		IPAddress: ipAddress,
		TPLink:    targetProxyLink,
	})
	return nil
}

// EnsureHTTPSForwardingRule ensures that a https forwarding rule exists for the given load balancer.
// See interface for more details.
func (f *FakeForwardingRuleSyncer) EnsureHTTPSForwardingRule(lbName, ipAddress, targetProxyLink string, forceUpdate bool) error {
	f.EnsuredForwardingRules = append(f.EnsuredForwardingRules, fakeForwardingRule{
		LBName:    lbName,
		IPAddress: ipAddress,
		TPLink:    targetProxyLink,
		IsHTTPS:   true,
	})
	return nil
}

// DeleteForwardingRules deletes the forwarding rules that EnsureHTTPForwardingRule and EnsureHTTPSForwardingRule would have created.
// See interface for more details.
func (f *FakeForwardingRuleSyncer) DeleteForwardingRules() error {
	f.EnsuredForwardingRules = nil
	return nil
}

// GetLoadBalancerStatus returns the status of the given load balancer.
// See interface for more details.
func (f *FakeForwardingRuleSyncer) GetLoadBalancerStatus(lbName string) (*status.LoadBalancerStatus, error) {
	for _, fr := range f.EnsuredForwardingRules {
		if fr.LBName == lbName {
			if fr.Status != nil {
				return fr.Status, nil
			}
			return nil, nil
		}
	}
	return nil, fmt.Errorf("load balancer %s does not exist", lbName)
}

// ListLoadBalancerStatuses returns the list of load balancer statuses.
// See interface for more details.
func (f *FakeForwardingRuleSyncer) ListLoadBalancerStatuses() ([]status.LoadBalancerStatus, error) {
	var ret []status.LoadBalancerStatus
	for _, fr := range f.EnsuredForwardingRules {
		if fr.Status != nil {
			ret = append(ret, *fr.Status)
		}
	}
	return ret, nil
}

// RemoveClustersFromStatus removes the given clusters from forwarding rules.
// See interface for more details.
func (f *FakeForwardingRuleSyncer) RemoveClustersFromStatus(clusters []string) error {
	clustersToRemove := goutils.MapFromSlice(clusters)
	for i, fr := range f.EnsuredForwardingRules {
		if fr.Status == nil {
			continue
		}
		newClusters := []string{}
		for _, c := range fr.Status.Clusters {
			if _, has := clustersToRemove[c]; !has {
				newClusters = append(newClusters, c)
			}
		}
		f.EnsuredForwardingRules[i].Status.Clusters = newClusters
	}
	return nil
}

// AddStatus adds the given status to forwarding rule for the given load balancer.
func (f *FakeForwardingRuleSyncer) AddStatus(lbName string, status *status.LoadBalancerStatus) {
	for i, fr := range f.EnsuredForwardingRules {
		if fr.LBName == lbName {
			f.EnsuredForwardingRules[i].Status = status
		}
	}
}

// RemoveStatus removes status of the given load balancer.
func (f *FakeForwardingRuleSyncer) RemoveStatus(lbName string) {
	for i, fr := range f.EnsuredForwardingRules {
		if fr.LBName == lbName {
			f.EnsuredForwardingRules[i].Status = nil
		}
	}
}
