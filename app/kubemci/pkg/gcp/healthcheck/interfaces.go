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

package healthcheck

import (
	compute "google.golang.org/api/compute/v1"
	"k8s.io/client-go/kubernetes"
	ingressbe "k8s.io/ingress-gce/pkg/backends"
)

// HealthChecksMap is a map of port number to the health check for that port.
type HealthChecksMap map[int64]*compute.HealthCheck

// SyncerInterface is an interface to manage GCP health checks.
type SyncerInterface interface {
	// EnsureHealthCheck ensures that the required health checks exist.
	// Returns a map of port number to the health check for that port. The map contains all the ports for which  it was successfully able to ensure a health check.
	// In case of no error, the map will contain all the ports from the given array of ports.
	EnsureHealthCheck(lbName string, ports []ingressbe.ServicePort, clients map[string]kubernetes.Interface, forceUpdate bool) (HealthChecksMap, error)
	// DeleteHealthChecks deletes all the health checks that EnsureHealthCheck would have created.
	DeleteHealthChecks(ports []ingressbe.ServicePort) error
}
