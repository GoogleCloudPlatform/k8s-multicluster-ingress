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

// NamedPortsMap is a map of port number to the named port for that port.
type NamedPortsMap map[int64]*compute.NamedPort

// BackendServicesMap is a map from the kubernetes service name to the corresponding GCE backend service.
type BackendServicesMap map[string]*compute.BackendService

// SyncerInterface is an interface to manage GCP backend services.
type SyncerInterface interface {
	// EnsureBackendService ensures that the required backend services
	// exist. forceUpdate must be true in order to modify an existing
	// BackendService.
	// Returns a map of the ensured backend services.
	// In case of no error, the map will contain services for all the given array of ports.
	EnsureBackendService(lbName string, ports []ingressbe.ServicePort, hcMap healthcheck.HealthChecksMap, npMap NamedPortsMap, igLinks []string, forceUpdate bool) (BackendServicesMap, error)
	// DeleteBackendServices deletes all backend services that would have been created by EnsureBackendService.
	DeleteBackendServices(ports []ingressbe.ServicePort) error
	// RemoveFromClusters removes the clusters corresponding to the given removeIGLinks from the existing backend services corresponding to the given ports.
	RemoveFromClusters(ports []ingressbe.ServicePort, removeIGLinks []string) error
}
