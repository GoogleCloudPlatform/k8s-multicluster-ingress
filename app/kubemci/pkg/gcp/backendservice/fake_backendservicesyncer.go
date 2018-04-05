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

package backendservice

import (
	compute "google.golang.org/api/compute/v1"
	ingressbe "k8s.io/ingress-gce/pkg/backends"

	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/healthcheck"
)

// FakeBackendService is a fake representation of a backend service ensured by invoking EnsureBackendService.
type FakeBackendService struct {
	LBName  string
	Port    ingressbe.ServicePort
	HCMap   healthcheck.HealthChecksMap
	NPMap   NamedPortsMap
	IGLinks []string
}

// FakeBackendServiceSyncer is a fake implementation of SyncerInterface to be used in tests.
type FakeBackendServiceSyncer struct {
	// List of backend services that this has been asked to ensure.
	EnsuredBackendServices []FakeBackendService
}

// NewFakeBackendServiceSyncer returns a new instance of the fake syncer.
func NewFakeBackendServiceSyncer() SyncerInterface {
	return &FakeBackendServiceSyncer{}
}

// Ensure this implements SyncerInterface.
var _ SyncerInterface = &FakeBackendServiceSyncer{}

// EnsureBackendService ensures that the backend service exists.
// See interface for more details.
func (h *FakeBackendServiceSyncer) EnsureBackendService(lbName string, ports []ingressbe.ServicePort, hcMap healthcheck.HealthChecksMap, npMap NamedPortsMap, igLinks []string, forceUpdate bool) (BackendServicesMap, error) {
	beMap := BackendServicesMap{}
	for _, p := range ports {
		h.EnsuredBackendServices = append(h.EnsuredBackendServices, FakeBackendService{
			LBName:  lbName,
			Port:    p,
			HCMap:   hcMap,
			NPMap:   npMap,
			IGLinks: igLinks,
		})
		beMap[p.SvcName.Name] = &compute.BackendService{}
	}
	return beMap, nil
}

// DeleteBackendServices deletes the backend services that EnsureBackendService would have created.
// See the interface for more details.
func (h *FakeBackendServiceSyncer) DeleteBackendServices(ports []ingressbe.ServicePort) error {
	h.EnsuredBackendServices = nil
	return nil
}

// RemoveFromClusters removes the given backend services corresponsding to the given ports from clusters corresponding to the given removeIGLinks.
// See the interface for more details.
func (h *FakeBackendServiceSyncer) RemoveFromClusters(ports []ingressbe.ServicePort, removeIGLinks []string) error {
	// Convert array to maps for easier lookups.
	affectedPorts := make(map[int64]bool, len(ports))
	for _, v := range ports {
		affectedPorts[v.NodePort] = true
	}
	for i, v := range h.EnsuredBackendServices {
		if _, has := affectedPorts[v.Port.NodePort]; !has {
			continue
		}
		// We remove the given instance group links from each backend service.
		// For a given backend service, we remove an instance group link only once.
		// This is because we use duplicate ig links in our tests.
		removeLinksEachBe := sliceToMap(removeIGLinks)
		newIGLinks := []string{}
		for _, ig := range v.IGLinks {
			if !removeLinksEachBe[ig] {
				newIGLinks = append(newIGLinks, ig)
			} else {
				// Mark the link as removed.
				// This is to handle duplicate ig links in our tests.
				removeLinksEachBe[ig] = false
			}
		}
		h.EnsuredBackendServices[i].IGLinks = newIGLinks
	}
	return nil
}

func sliceToMap(slice []string) map[string]bool {
	desiredMap := make(map[string]bool, len(slice))
	for _, v := range slice {
		desiredMap[v] = true
	}
	return desiredMap
}
