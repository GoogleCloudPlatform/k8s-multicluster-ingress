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
	ingressbe "k8s.io/ingress-gce/pkg/backends"

	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/mci/pkg/gcp/healthcheck"
)

type FakeBackendService struct {
	LBName  string
	Ports   []ingressbe.ServicePort
	HCMap   healthcheck.HealthChecksMap
	NPMap   NamedPortsMap
	IGLinks []string
}

type FakeBackendServiceSyncer struct {
	// List of backend services that this has been asked to ensure.
	EnsuredBackendServices []FakeBackendService
}

// Fake backend service syncer to be used for tests.
func NewFakeBackendServiceSyncer() BackendServiceSyncerInterface {
	return &FakeBackendServiceSyncer{}
}

// Ensure this implements BackendServiceSyncerInterface.
var _ BackendServiceSyncerInterface = &FakeBackendServiceSyncer{}

func (h *FakeBackendServiceSyncer) EnsureBackendService(lbName string, ports []ingressbe.ServicePort, hcMap healthcheck.HealthChecksMap, npMap NamedPortsMap, igLinks []string) error {
	h.EnsuredBackendServices = append(h.EnsuredBackendServices, FakeBackendService{
		LBName:  lbName,
		Ports:   ports,
		HCMap:   hcMap,
		NPMap:   npMap,
		IGLinks: igLinks,
	})
	return nil
}
