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
	"fmt"

	"github.com/golang/glog"
	"github.com/hashicorp/go-multierror"
	"google.golang.org/api/compute/v1"
	ingressbe "k8s.io/ingress-gce/pkg/backends"

	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/healthcheck"
	utilsnamer "github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/namer"
)

// BackendServiceSyncer manages GCP backend services for multicluster GCP L7 load balancers.
type BackendServiceSyncer struct {
	namer *utilsnamer.Namer
	// Backend services provider to call GCP APIs to manipulate backend services.
	bsp ingressbe.BackendServices
}

func NewBackendServiceSyncer(namer *utilsnamer.Namer, bsp ingressbe.BackendServices) BackendServiceSyncerInterface {
	return &BackendServiceSyncer{
		namer: namer,
		bsp:   bsp,
	}
}

// Ensure this implements BackendServiceSyncerInterface.
var _ BackendServiceSyncerInterface = &BackendServiceSyncer{}

// EnsureBackendService ensures that the required backend services exist for the given ports.
// Does nothing if they exist already, else creates new ones.
// Returns a map of the kubernetes service name to the corresponding GCE backend service.
func (b *BackendServiceSyncer) EnsureBackendService(lbName string, ports []ingressbe.ServicePort, hcMap healthcheck.HealthChecksMap, npMap NamedPortsMap, igLinks []string) (BackendServicesMap, error) {
	fmt.Println("Ensuring backend services")
	glog.V(5).Infof("Received health checks map: %v", hcMap)
	glog.V(5).Infof("Received named ports map: %v", npMap)
	glog.V(5).Infof("Received instance groups: %v", igLinks)
	var err error
	ensuredBackendServices := BackendServicesMap{}
	for _, p := range ports {
		be, beErr := b.ensureBackendService(lbName, p, hcMap[p.Port], npMap[p.Port], igLinks)
		if beErr != nil {
			beErr = fmt.Errorf("Error %s in ensuring backend service for port %v", beErr, p)
			// Try ensuring backend services for all ports and return all errors at once.
			err = multierror.Append(err, beErr)
			continue
		}
		ensuredBackendServices[p.SvcName.Name] = be
	}
	return ensuredBackendServices, err
}

func (b *BackendServiceSyncer) DeleteBackendServices(ports []ingressbe.ServicePort) error {
	fmt.Println("Deleting backend services")
	var err error
	for _, p := range ports {
		if beErr := b.deleteBackendService(p); beErr != nil {
			// Try deleting all backend services and return all errors at once.
			err = multierror.Append(err, beErr)
		}
	}
	if err != nil {
		fmt.Printf("Errors in deleting backend services: %s", err)
		return err
	}
	fmt.Println("Successfully deleted all backend services")
	return nil
}

func (b *BackendServiceSyncer) deleteBackendService(port ingressbe.ServicePort) error {
	name := b.namer.BeServiceName(port.Port)
	glog.V(2).Infof("Deleting backend service %s", name)
	if err := b.bsp.DeleteGlobalBackendService(name); err != nil {
		glog.V(2).Infof("Error in deleting backend service %s: %s", name, err)
		return err
	}
	glog.V(2).Infof("Successfully deleted backend service %s", name)
	return nil
}

// ensureBackendService ensures that the required backend service exists for the given port.
// Does nothing if it exists already, else creates a new one.
func (b *BackendServiceSyncer) ensureBackendService(lbName string, port ingressbe.ServicePort, hc *compute.HealthCheck, np *compute.NamedPort, igLinks []string) (*compute.BackendService, error) {
	fmt.Println("Ensuring backend service for port:", port)
	if hc == nil {
		return nil, fmt.Errorf("missing health check probably due to an error in creating health check. Cannot create backend service without health chech link")
	}
	if hc.SelfLink == "" {
		glog.V(2).Infof("Unexpected: empty self link in hc: %v", hc)
		return nil, fmt.Errorf("missing self link in health check %s", hc.Name)
	}
	if np == nil {
		return nil, fmt.Errorf("missing corresponding named port on the instance group")
	}
	desiredBE := b.desiredBackendService(lbName, port, hc.SelfLink, np.Name, igLinks)
	name := desiredBE.Name
	// Check if backend service already exists.
	existingBE, err := b.bsp.GetGlobalBackendService(name)
	if err == nil {
		fmt.Println("Backend service", name, "exists already. Checking if it matches our desired backend service")
		glog.V(5).Infof("Existing backend service: %v\n, desired backend service: %v", existingBE, *desiredBE)
		// Backend service with that name exists already. Check if it matches what we want.
		if backendServiceMatches(desiredBE, existingBE) {
			// Nothing to do. Desired backend service exists already.
			fmt.Println("Desired backend service exists already")
			return existingBE, nil
		}
		// TODO (nikhiljindal): Require explicit permission from user before doing this.
		return b.updateBackendService(desiredBE)
	}
	glog.V(5).Infof("Got error %s while trying to get existing backend service %s", err, name)
	// TODO(nikhiljindal): Handle non NotFound errors. We should create only if the error is NotFound.
	// Create the backend service.
	return b.createBackendService(desiredBE)
}

// updateBackendService updates the backend service and returns the updated backend service.
func (b *BackendServiceSyncer) updateBackendService(desiredBE *compute.BackendService) (*compute.BackendService, error) {
	name := desiredBE.Name
	fmt.Println("Updating existing backend service", name, "to match the desired state")
	err := b.bsp.UpdateGlobalBackendService(desiredBE)
	if err != nil {
		return nil, err
	}
	fmt.Println("Backend service", name, "updated successfully")
	return b.bsp.GetGlobalBackendService(name)
}

// createBackendService creates the backend service and returns the created backend service.
func (b *BackendServiceSyncer) createBackendService(desiredBE *compute.BackendService) (*compute.BackendService, error) {
	name := desiredBE.Name
	fmt.Println("Creating backend service", name)
	glog.V(5).Infof("Creating backend service %v", desiredBE)
	err := b.bsp.CreateGlobalBackendService(desiredBE)
	if err != nil {
		return nil, err
	}
	fmt.Println("Backend service", name, "created successfully")
	return b.bsp.GetGlobalBackendService(name)
}

func backendServiceMatches(desiredBE, existingBE *compute.BackendService) bool {
	// TODO(nikhiljindal): Add proper logic to figure out if the 2 backend services match.
	// Need to add the --force flag for user to consent overritting before this method can be updated to return false.
	return true
}

func (b *BackendServiceSyncer) desiredBackendService(lbName string, port ingressbe.ServicePort, hcLink, portName string, igLinks []string) *compute.BackendService {
	// Compute the desired backend service.
	return &compute.BackendService{
		Name:         b.namer.BeServiceName(port.Port),
		Description:  fmt.Sprintf("Backend service for service %s as part of kubernetes multicluster loadbalancer %s", port.Description(), lbName),
		Protocol:     string(port.Protocol),
		HealthChecks: []string{hcLink},
		Port:         port.Port,
		PortName:     portName,
		Backends:     desiredBackends(igLinks),
	}
}

func desiredBackends(igLinks []string) []*compute.Backend {
	var backends []*compute.Backend
	for _, ig := range igLinks {
		backends = append(backends, &compute.Backend{
			Group: ig,
			// We create the backend service with RATE balancing mode and set max rate
			// per instance to max value so that all requests in a region are sent to
			// instances in that region.
			// Setting rps to 1, for example, would round robin requests amongst all
			// instances.
			BalancingMode:      "RATE",
			MaxRatePerInstance: 1e14,
		})
	}
	return backends
}
