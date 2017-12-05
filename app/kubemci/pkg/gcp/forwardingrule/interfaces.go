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
	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/status"
)

// ForwardingRuleSyncerInterface is an interface to manage GCP forwarding rules.
type ForwardingRuleSyncerInterface interface {
	// EnsureHttpForwardingRule ensures that the required http forwarding rule exists.
	// clusters is the list of clusters across which the load balancer is spread.
	// Will only change an existing rule if forceUpdate = True.
	EnsureHttpForwardingRule(lbName, ipAddress, targetProxyLink string, clusters []string, forceUpdate bool) error
	// DeleteForwardingRules deletes the forwarding rules that EnsureForwardingRule would have created.
	DeleteForwardingRules() error
	// GetLoadBalancerStatus returns the struct describing the status of the given load balancer.
	GetLoadBalancerStatus(lbName string) (*status.LoadBalancerStatus, error)
	// ListLoadBalancerStatuses returns status of all MCI ingresses (load balancers).
	ListLoadBalancerStatuses() ([]status.LoadBalancerStatus, error)
}
