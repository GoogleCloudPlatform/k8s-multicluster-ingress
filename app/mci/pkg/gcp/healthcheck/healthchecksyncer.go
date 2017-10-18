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

	"github.com/golang/glog"
	"github.com/hashicorp/go-multierror"
	compute "google.golang.org/api/compute/v1"
	ingresshc "k8s.io/ingress-gce/pkg/healthchecks"

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
	hcp   ingresshc.HealthCheckProvider
}

func NewHealthCheckSyncer(namer *utilsnamer.Namer, hcp ingresshc.HealthCheckProvider) HealthCheckSyncerInterface {
	return &HealthCheckSyncer{
		namer: namer,
		hcp:   hcp,
	}
}

// Ensure this implements HealthCheckSyncerInterface.
var _ HealthCheckSyncerInterface = &HealthCheckSyncer{}

// EnsureHealthCheck ensures that the required health check exists.
// Does nothing if it exists already, else creates a new one.
func (h *HealthCheckSyncer) EnsureHealthCheck(lbName string, ports []sp.ServicePort) error {
	fmt.Println("Ensuring health checks")
	var err error
	for _, p := range ports {
		if hcErr := h.ensureHealthCheck(lbName, p); hcErr != nil {
			hcErr = fmt.Errorf("Error %s in ensuring health check for port %v", hcErr, p)
			// Try ensuring health checks for all ports and return all errors at once.
			err = multierror.Append(err, hcErr)
		}
	}
	return err
}

func (h *HealthCheckSyncer) ensureHealthCheck(lbName string, port sp.ServicePort) error {
	fmt.Println("Ensuring health check for port:", port)
	desiredHC, err := h.desiredHealthCheck(lbName, port)
	if err != nil {
		return fmt.Errorf("error %s in computing desired health check", err)
	}
	name := desiredHC.Name
	// Check if hc already exists.
	existingHC, err := h.hcp.GetHealthCheck(name)
	if err == nil {
		fmt.Println("Health check", name, "exists already. Checking if it matches our desired health check", name)
		glog.V(5).Infof("Existing health check: %v\n, desired health check: %v", existingHC, desiredHC)
		// Health check with that name exists already. Check if it matches what we want.
		if healthCheckMatches(&desiredHC, existingHC) {
			// Nothing to do. Desired health check exists already.
			fmt.Println("Desired health check exists already")
			return nil
		}
		// TODO: Require explicit permission from user before doing this.
		fmt.Println("Updating existing health check", name, "to match the desired state")
		return h.hcp.UpdateHealthCheck(&desiredHC)
	}
	glog.V(5).Infof("Got error %s while trying to get existing health check %s", err, name)
	// TODO: Handle non NotFound errors. We should create only if the error is NotFound.
	// Create the health check.
	fmt.Println("Creating health check", name)
	glog.V(5).Infof("Creating health check %v", desiredHC)
	return h.hcp.CreateHealthCheck(&desiredHC)
}

func healthCheckMatches(desiredHC, existingHC *compute.HealthCheck) bool {
	// TODO: Add proper logic to figure out if the 2 health checks match.
	// Need to add the --force flag for user to consent overritting before this method can be updated to return false.
	return true
}

func (h *HealthCheckSyncer) desiredHealthCheck(lbName string, port sp.ServicePort) (compute.HealthCheck, error) {
	// Compute the desired health check.
	hc := compute.HealthCheck{
		Name:        h.namer.HealthCheckName(port.Port),
		Description: fmt.Sprintf("Kubernetes L7 Loadbalancing health check for multi cluster ingress %s.", lbName),
		// How often to health check.
		CheckIntervalSec: int64(DefaultHealthCheckInterval.Seconds()),
		// How long to wait before claiming failure of a health check.
		TimeoutSec: int64(DefaultTimeout.Seconds()),
		// Number of healthchecks to pass for a vm to be deemed healthy.
		HealthyThreshold: DefaultHealthyThreshold,
		// Number of healthchecks to fail before the vm is deemed unhealthy.
		UnhealthyThreshold: DefaultUnhealthyThreshold,
		Type:               port.Protocol,
	}
	switch port.Protocol {
	case "HTTP":
		hc.HttpHealthCheck = &compute.HTTPHealthCheck{
			Port:        port.Port,
			RequestPath: "/healthz", // TODO: Allow customization.
		}
		break
	case "HTTPS":
		hc.HttpsHealthCheck = &compute.HTTPSHealthCheck{
			Port:        port.Port,  // TODO: Allow customization.
			RequestPath: "/healthz", // TODO: Allow customization.
		}
		break
	default:
		return compute.HealthCheck{}, fmt.Errorf("Unexpected port protocol: %s", port.Protocol)

	}
	return hc, nil
}
