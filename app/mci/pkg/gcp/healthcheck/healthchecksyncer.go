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
	"fmt"
	"time"

	compute "google.golang.org/api/compute/v1"

	utilsnamer "k8s-multi-cluster-ingress/app/mci/pkg/gcp/namer"
	sp "k8s-multi-cluster-ingress/app/mci/pkg/serviceport"
)

const (
	// TODO: Share them with kubernetes/ingress.
	// These values set a low health threshold and a high failure threshold.
	// We're just trying to detect if the node networking is
	// borked, service level outages will get detected sooner
	// by kube-proxy.
	// DefaultHealthCheckInterval defines how frequently a probe runs
	DefaultHealthCheckInterval = 1 * time.Minute
	// DefaultHealthyThreshold defines the threshold of success probes that declare a backend "healthy"
	DefaultHealthyThreshold = 1
	// DefaultUnhealthyThreshold defines the threshold of failure probes that declare a backend "unhealthy"
	DefaultUnhealthyThreshold = 10
	// DefaultTimeout defines the timeout of each probe
	DefaultTimeout = 1 * time.Minute
)

type HealthCheckSyncer struct {
	namer *utilsnamer.Namer
}

func NewHealthCheckSyncer(namer *utilsnamer.Namer) *HealthCheckSyncer {
	return &HealthCheckSyncer{
		namer: namer,
	}
}

// Ensures that the required health check exists.
// Does nothing if it exists already, else creates a new one.
func (h *HealthCheckSyncer) EnsureHealthCheck(lbName string, ports []sp.ServicePort) error {
	// TODO: Check if the health check already exists and is as expected.
	// Create a new health check.
	for _, p := range ports {
		hc := &compute.HealthCheck{
			Name:        h.namer.HealthCheckName(p.Port),
			Description: fmt.Sprintf("Kubernetes L7 Loadbalancing health check for multi cluster ingress %s.", lbName),
			// How often to health check.
			CheckIntervalSec: int64(DefaultHealthCheckInterval.Seconds()),
			// How long to wait before claiming failure of a health check.
			TimeoutSec: int64(DefaultTimeout.Seconds()),
			// Number of healthchecks to pass for a vm to be deemed healthy.
			HealthyThreshold: DefaultHealthyThreshold,
			// Number of healthchecks to fail before the vm is deemed unhealthy.
			UnhealthyThreshold: DefaultUnhealthyThreshold,
			Type:               p.Protocol,
		}
		switch p.Protocol {
		case "HTTP":
			hc.HttpHealthCheck = &compute.HTTPHealthCheck{
				Port:        p.Port,
				RequestPath: "/healthz", // TODO: Allow customization.
			}
			break
		case "HTTPS":
			hc.HttpsHealthCheck = &compute.HTTPSHealthCheck{
				Port:        p.Port,     // TODO: Allow customization.
				RequestPath: "/healthz", // TODO: Allow customization.
			}
			break
		default:
			return fmt.Errorf("Unexpected port protocol: %s", p.Protocol)

		}
		// TODO: Create health check.
		fmt.Printf("Creating health check: %v\n", hc)
	}
	return nil
}
