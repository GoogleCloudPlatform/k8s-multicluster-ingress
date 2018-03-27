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

package urlmap

import (
	"k8s.io/api/extensions/v1beta1"

	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/backendservice"
	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/status"
)

// URLMapSyncerInterface is an interface to manage GCP url maps.
type URLMapSyncerInterface interface {
	// EnsureURLMap ensures that the required url map exists for the given ingress.
	// Uses beMap to extract the backend services to link to in the url map.
	// clusters is the list of clusters across which the load balancer is spread.
	// Will only change an existing URL map if forceUpdate=True.
	// Returns the self link for the ensured url map.
	EnsureURLMap(lbName, ipAddress string, clusters []string, ing *v1beta1.Ingress, beMap backendservice.BackendServicesMap, forceUpdate bool) (string, error)
	// DeleteURLMap deletes the url map that EnsureURLMap would have created.
	DeleteURLMap() error

	// GetLoadBalancerStatus returns the status of the given load balancer if it is stored on the url map.
	// Returns an error with http.StatusNotFound if url map does not exist.
	GetLoadBalancerStatus(lbName string) (*status.LoadBalancerStatus, error)
	// ListLoadBalancerStatuses returns status of all MCI ingresses (load balancers) that have statuses stored on url maps.
	ListLoadBalancerStatuses() ([]status.LoadBalancerStatus, error)
	// RemoveClustersFromStatus removes the given clusters from the LoadBalancerStatus.
	RemoveClustersFromStatus(clusters []string) error
}
