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

// SyncerInterface is an interface to manage GCP forwarding rules.
type SyncerInterface interface {
	// EnsureHTTPForwardingRule ensures that the required http forwarding rule exists.
	// Will only change an existing rule if forceUpdate = True.
	EnsureHTTPForwardingRule(lbName, ipAddress, targetProxyLink string, forceUpdate bool) error
	// EnsureHTTPSForwardingRule ensures that the required https forwarding rule exists.
	// Will only change an existing rule if forceUpdate = True.
	EnsureHTTPSForwardingRule(lbName, ipAddress, targetProxyLink string, forceUpdate bool) error
	// DeleteForwardingRules deletes the forwarding rules that
	// EnsureHTTPForwardingRule and EnsureHTTPSForwardingRule would have created.
	DeleteForwardingRules() error

	// GetLoadBalancerStatus returns the status of the given load balancer if it is stored on the forwarding rule.
	// Returns an error with http.StatusNotFound if forwarding rule does not exist.
	GetLoadBalancerStatus(lbName string) (*status.LoadBalancerStatus, error)
	// ListLoadBalancerStatuses returns status of all MCI ingresses (load balancers) that have statuses stored on forwarding rules.
	ListLoadBalancerStatuses() ([]status.LoadBalancerStatus, error)
	// RemoveClustersFromStatus removes the given clusters from the LoadBalancerStatus.
	RemoveClustersFromStatus(clusters []string) error
}
