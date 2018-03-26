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
)

const (
	FakeUrlSelfLink = "url/map/self/link"
)

type FakeURLMap struct {
	LBName    string
	IPAddress string
	Clusters  []string
	Ingress   *v1beta1.Ingress
	BeMap     backendservice.BackendServicesMap
}

type FakeURLMapSyncer struct {
	// List of url maps that this has been asked to ensure.
	EnsuredURLMaps []FakeURLMap
}

// Fake url map syncer to be used for tests.
func NewFakeURLMapSyncer() URLMapSyncerInterface {
	return &FakeURLMapSyncer{}
}

// Ensure this implements URLMapSyncerInterface.
var _ URLMapSyncerInterface = &FakeURLMapSyncer{}

func (f *FakeURLMapSyncer) EnsureURLMap(lbName, ipAddress string, clusters []string, ing *v1beta1.Ingress, beMap backendservice.BackendServicesMap, forceUpdate bool) (string, error) {
	f.EnsuredURLMaps = append(f.EnsuredURLMaps, FakeURLMap{
		LBName:    lbName,
		IPAddress: ipAddress,
		Clusters:  clusters,
		Ingress:   ing,
		BeMap:     beMap,
	})
	return FakeUrlSelfLink, nil
}

func (f *FakeURLMapSyncer) DeleteURLMap() error {
	f.EnsuredURLMaps = nil
	return nil
}

func (f *FakeURLMapSyncer) GetLoadBalancerStatus(lbName string) (*status.LoadBalancerStatus, error) {
	for _, fr := range f.EnsuredURLMaps {
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

func (f *FakeURLMapSyncer) ListLoadBalancerStatuses() ([]status.LoadBalancerStatus, error) {
	var ret []status.LoadBalancerStatus
	for _, fr := range f.EnsuredURLMaps {
		status := status.LoadBalancerStatus{
			LoadBalancerName: fr.LBName,
			Clusters:         fr.Clusters,
			IPAddress:        fr.IPAddress,
		}
		ret = append(ret, status)
	}
	return ret, nil
}
