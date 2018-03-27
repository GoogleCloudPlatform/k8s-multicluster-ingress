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
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/golang/glog"
	multierror "github.com/hashicorp/go-multierror"
	compute "google.golang.org/api/compute/v1"
	"google.golang.org/api/googleapi"
	"k8s.io/apimachinery/pkg/util/diff"
	"k8s.io/client-go/kubernetes"
	ingressbe "k8s.io/ingress-gce/pkg/backends"
	ingresshc "k8s.io/ingress-gce/pkg/healthchecks"
	"k8s.io/ingress-gce/pkg/utils"

	utilsnamer "github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/namer"
	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/kubeutils"
)

const (
	// TODO(nikhiljindal): Share them with kubernetes/ingress.
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

// HealthCheckSyncer manages GCP health checks for multicluster GCP L7 load balancers.
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
// Returns a map of the ensured health checks keyed by the corresponding port.
func (h *HealthCheckSyncer) EnsureHealthCheck(lbName string, ports []ingressbe.ServicePort, clients map[string]kubernetes.Interface, forceUpdate bool) (HealthChecksMap, error) {
	fmt.Println("Ensuring health checks")
	var err error
	ensuredHealthChecks := HealthChecksMap{}
	for _, p := range ports {
		path, pathErr := getHealthCheckPath(p, clients)
		if pathErr != nil {
			err = multierror.Append(err, pathErr)
			continue
		}
		fmt.Println("Path for healthcheck is", path)
		hc, hcErr := h.ensureHealthCheck(lbName, p, path, forceUpdate)
		if hcErr != nil {
			hcErr = fmt.Errorf("Error %s in ensuring health check for port %v", hcErr, p)
			// Try ensuring health checks for all ports and return all errors at once.
			err = multierror.Append(err, hcErr)
			continue
		}
		ensuredHealthChecks[p.NodePort] = hc
	}
	return ensuredHealthChecks, err
}

func getHealthCheckPath(port ingressbe.ServicePort, clients map[string]kubernetes.Interface) (string, error) {
	var err error
	path := ""
	for cName, c := range clients {
		probe, prErr := kubeutils.GetProbe(c, port)
		if prErr != nil {
			prErr = fmt.Errorf("Error %s when getting readiness probe from pods for service %s", prErr, port.SvcName)
			err = multierror.Append(err, prErr)
			continue
		}
		if probe != nil && probe.HTTPGet != nil {
			if path != "" {
				// Ensure that all probes have the same health check path
				if path != probe.HTTPGet.Path {
					pathErr := fmt.Errorf("readiness probe path '%s' from pod spec in cluster '%s' does not match already extracted health check path '%s'", probe.HTTPGet.Path, cName, path)
					err = multierror.Append(err, pathErr)
					return path, err
				}
			} else {
				// TODO share code with `applyProbeSettingsToHC` method from ingress-gce/backend to get probe settings
				path = probe.HTTPGet.Path
			}
		}
	}

	// GCE requires a leading "/" for health check urls.
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return path, err
}

func (h *HealthCheckSyncer) DeleteHealthChecks(ports []ingressbe.ServicePort) error {
	fmt.Println("Deleting health checks")
	var err error
	for _, p := range ports {
		if hcErr := h.deleteHealthCheck(p); hcErr != nil {
			err = multierror.Append(err, hcErr)
		}
	}
	if err != nil {
		fmt.Println("Errors in deleting health checks:", err)
		return err
	}
	fmt.Println("Successfully deleted all health checks")
	return nil
}

func (h *HealthCheckSyncer) deleteHealthCheck(port ingressbe.ServicePort) error {
	name := h.namer.HealthCheckName(port.NodePort)
	glog.V(2).Infof("Deleting health check %s", name)
	err := h.hcp.DeleteHealthCheck(name)
	if err != nil {
		if utils.IsHTTPErrorCode(err, http.StatusNotFound) {
			fmt.Println("Health check", name, "does not exist. Nothing to delete")
			return nil
		} else {
			fmt.Println("Error in deleting health check", name, ":", err)
			return err
		}
	} else {
		glog.V(2).Infof("Successfully deleted health check %s", name)
		return nil
	}
}

func getJsonIgnoreErr(v interface{}) string {
	output, err := json.Marshal(v)
	if err != nil {
		glog.Warningf("Marshalling error: %v", err)
	}
	return string(output)
}

func (h *HealthCheckSyncer) ensureHealthCheck(lbName string, port ingressbe.ServicePort, path string, forceUpdate bool) (*compute.HealthCheck, error) {
	fmt.Println("Ensuring health check for port:", port)
	desiredHC, err := h.desiredHealthCheck(lbName, port, path)
	if err != nil {
		return nil, fmt.Errorf("error %s in computing desired health check", err)
	}
	name := desiredHC.Name
	// Check if hc already exists.
	existingHC, err := h.hcp.GetHealthCheck(name)
	if err == nil {
		fmt.Println("Health check", name, "exists already. Checking if it matches our desired health check")
		glog.V(5).Infof("Existing health check:\n%v\nDesired health check:\n%v\n", getJsonIgnoreErr(existingHC), getJsonIgnoreErr(desiredHC))
		// Health check with that name exists already. Check if it matches what we want.
		if healthCheckMatches(desiredHC, *existingHC) {
			// Nothing to do. Desired health check exists already.
			fmt.Println("Desired health check exists already")
			return existingHC, nil
		}
		if forceUpdate {
			return h.updateHealthCheck(&desiredHC)
		} else {
			// TODO(G-Harmon): prompt yes/no for overwriting.
			fmt.Println("Will not overwrite this differing health check without the --force flag.")
			return nil, fmt.Errorf("will not overwrite healthcheck without --force")
		}
	}
	glog.V(5).Infof("Got error %s while trying to get existing health check %s", err, name)
	// TODO(nikhiljindal): Handle non NotFound errors. We should create only if the error is NotFound.
	// Create the health check.
	return h.createHealthCheck(&desiredHC)
}

// updateHealthCheck updates the health check and returns the updated health check.
func (h *HealthCheckSyncer) updateHealthCheck(desiredHC *compute.HealthCheck) (*compute.HealthCheck, error) {
	name := desiredHC.Name
	fmt.Println("Updating existing health check", name, "to match the desired state")
	err := h.hcp.UpdateHealthCheck(desiredHC)
	if err != nil {
		return nil, err
	}
	fmt.Println("Health check", name, "updated successfully")
	return h.hcp.GetHealthCheck(name)
}

// createHealthCheck creates the health check and returns the created health check.
func (h *HealthCheckSyncer) createHealthCheck(desiredHC *compute.HealthCheck) (*compute.HealthCheck, error) {
	name := desiredHC.Name
	fmt.Println("Creating health check", name)
	glog.V(5).Infof("Creating health check %v", desiredHC)
	err := h.hcp.CreateHealthCheck(desiredHC)
	if err != nil {
		return nil, err
	}
	fmt.Println("Health check", name, "created successfully")
	return h.hcp.GetHealthCheck(name)
}

func healthCheckMatches(desiredHC, existingHC compute.HealthCheck) bool {
	// Clear fields we don't care about to make the printout easier to read.
	existingHC.CreationTimestamp = ""
	existingHC.Id = 0
	existingHC.SelfLink = ""
	existingHC.ServerResponse = googleapi.ServerResponse{}

	equal := reflect.DeepEqual(existingHC, desiredHC)
	if !equal {
		glog.Infof("Diff:\n%v", diff.ObjectDiff(desiredHC, existingHC))
	}
	return equal
}

func (h *HealthCheckSyncer) desiredHealthCheck(lbName string, port ingressbe.ServicePort, path string) (compute.HealthCheck, error) {
	// Compute the desired health check.
	hc := compute.HealthCheck{
		Name:        h.namer.HealthCheckName(port.NodePort),
		Description: fmt.Sprintf("Health check for service %s as part of kubernetes multicluster loadbalancer %s", port.Description(), lbName),
		// How often to health check.
		CheckIntervalSec: int64(DefaultHealthCheckInterval.Seconds()),
		// How long to wait before claiming failure of a health check.
		TimeoutSec: int64(DefaultTimeout.Seconds()),
		// Number of healthchecks to pass for a vm to be deemed healthy.
		HealthyThreshold: DefaultHealthyThreshold,
		// Number of healthchecks to fail before the vm is deemed unhealthy.
		UnhealthyThreshold: DefaultUnhealthyThreshold,
		Type:               string(port.Protocol),
		Kind:               "compute#healthCheck",
	}
	switch port.Protocol {
	case "HTTP":
		hc.HttpHealthCheck = &compute.HTTPHealthCheck{
			Port:        port.NodePort,
			RequestPath: path,
			ProxyHeader: "NONE",
		}
		break
	case "HTTPS":
		hc.HttpsHealthCheck = &compute.HTTPSHealthCheck{
			Port:        port.NodePort,
			RequestPath: path,
			ProxyHeader: "NONE",
		}
		break
	default:
		return compute.HealthCheck{}, fmt.Errorf("Unexpected port protocol: %s", port.Protocol)

	}
	return hc, nil
}
