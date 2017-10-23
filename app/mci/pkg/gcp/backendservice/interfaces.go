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
	"google.golang.org/api/compute/v1"
	ingressbe "k8s.io/ingress-gce/pkg/backends"

	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/mci/pkg/gcp/healthcheck"
)

// NamedPortsMap is a map of port number to the named port for that port.
type NamedPortsMap map[int64]*compute.NamedPort

// BackendServicesMap is a map from the kubernetes service name to the corresponding GCE backend service.
type BackendServicesMap map[string]*compute.BackendService

// BackendServiceSyncerInterface is an interface to manage GCP backend services.
type BackendServiceSyncerInterface interface {
	// EnsureBackendService ensures that the required backend services exist.
	EnsureBackendService(lbName string, ports []ingressbe.ServicePort, hcMap healthcheck.HealthChecksMap, npMap NamedPortsMap, igLinks []string) (BackendServicesMap, error)
}
