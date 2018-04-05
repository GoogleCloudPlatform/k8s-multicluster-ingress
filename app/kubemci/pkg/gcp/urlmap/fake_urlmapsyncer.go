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
	"fmt"

	"k8s.io/api/extensions/v1beta1"

	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/backendservice"
	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/status"
	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/goutils"
)

const (
	// FakeURLSelfLink is a fake self link used for url maps.
	FakeURLSelfLink = "url/map/self/link"
)

type fakeURLMap struct {
	LBName    string
	IPAddress string
	Clusters  []string
	Ingress   *v1beta1.Ingress
	BeMap     backendservice.BackendServicesMap
	HasStatus bool
}

// FakeURLMapSyncer is a fake implementation of SyncerInterface to be used in tests.
type FakeURLMapSyncer struct {
	// List of url maps that this has been asked to ensure.
	EnsuredURLMaps []fakeURLMap
}

// NewFakeURLMapSyncer returns a new instance of the fake syncer.
func NewFakeURLMapSyncer() SyncerInterface {
	return &FakeURLMapSyncer{}
}

// Ensure this implements SyncerInterface.
var _ SyncerInterface = &FakeURLMapSyncer{}

// EnsureURLMap ensures that the required url map exists for the given ingress.
// See the interface for more details.
func (f *FakeURLMapSyncer) EnsureURLMap(lbName, ipAddress string, clusters []string, ing *v1beta1.Ingress, beMap backendservice.BackendServicesMap, forceUpdate bool) (string, error) {
	f.EnsuredURLMaps = append(f.EnsuredURLMaps, fakeURLMap{
		LBName:    lbName,
		IPAddress: ipAddress,
		Clusters:  clusters,
		Ingress:   ing,
		BeMap:     beMap,
		// URL map has status by default.
		HasStatus: true,
	})
	return FakeURLSelfLink, nil
}

// DeleteURLMap deletes the url map that EnsureURLMap would have created.
// See the interface for more details.
func (f *FakeURLMapSyncer) DeleteURLMap() error {
	f.EnsuredURLMaps = nil
	return nil
}

// GetLoadBalancerStatus returns the status of the given load balancer.
// See the interface for more details.
func (f *FakeURLMapSyncer) GetLoadBalancerStatus(lbName string) (*status.LoadBalancerStatus, error) {
	for _, fr := range f.EnsuredURLMaps {
		if fr.LBName == lbName {
			if !fr.HasStatus {
				return nil, nil
			}
			return &status.LoadBalancerStatus{
				LoadBalancerName: lbName,
				Clusters:         fr.Clusters,
				IPAddress:        fr.IPAddress,
			}, nil
		}
	}
	return nil, fmt.Errorf("load balancer %s does not exist", lbName)
}

// ListLoadBalancerStatuses lists all the load balancers.
// See the interface for more details.
func (f *FakeURLMapSyncer) ListLoadBalancerStatuses() ([]status.LoadBalancerStatus, error) {
	var ret []status.LoadBalancerStatus
	for _, fr := range f.EnsuredURLMaps {
		if !fr.HasStatus {
			continue
		}
		status := status.LoadBalancerStatus{
			LoadBalancerName: fr.LBName,
			Clusters:         fr.Clusters,
			IPAddress:        fr.IPAddress,
		}
		ret = append(ret, status)
	}
	return ret, nil
}

// RemoveClustersFromStatus removes the given clusters from the LoadBalancerStatus.
// See the interface for more details.
func (f *FakeURLMapSyncer) RemoveClustersFromStatus(clusters []string) error {
	clustersToRemove := goutils.MapFromSlice(clusters)
	for i, fr := range f.EnsuredURLMaps {
		if !fr.HasStatus {
			continue
		}
		newClusters := []string{}
		for _, c := range fr.Clusters {
			if _, has := clustersToRemove[c]; !has {
				newClusters = append(newClusters, c)
			}
		}
		f.EnsuredURLMaps[i].Clusters = newClusters
	}
	return nil
}

// AddStatus adds a status to the url map corresponding to the given load balancer.
func (f *FakeURLMapSyncer) AddStatus(lbName string) {
	for i, um := range f.EnsuredURLMaps {
		if um.LBName == lbName {
			f.EnsuredURLMaps[i].HasStatus = true
		}
	}
}

// RemoveStatus removes the status for the given load balancer.
func (f *FakeURLMapSyncer) RemoveStatus(lbName string) {
	for i, um := range f.EnsuredURLMaps {
		if um.LBName == lbName {
			f.EnsuredURLMaps[i].HasStatus = false
		}
	}
}
