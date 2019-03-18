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
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"sort"

	compute "google.golang.org/api/compute/v1"

	"github.com/golang/glog"
	multierror "github.com/hashicorp/go-multierror"
	"k8s.io/apimachinery/pkg/util/diff"
	ingressbe "k8s.io/ingress-gce/pkg/backends"
	"k8s.io/ingress-gce/pkg/utils"

	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/healthcheck"
	utilsnamer "github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/namer"
)

// Syncer manages GCP backend services for multicluster GCP L7 load balancers.
type Syncer struct {
	namer *utilsnamer.Namer
	// Backend services provider to call GCP APIs to manipulate backend services.
	bsp ingressbe.BackendServices
}

// NewBackendServiceSyncer returns a new instance of syncer.
func NewBackendServiceSyncer(namer *utilsnamer.Namer, bsp ingressbe.BackendServices) SyncerInterface {
	return &Syncer{
		namer: namer,
		bsp:   bsp,
	}
}

// Ensure this implements SyncerInterface.
var _ SyncerInterface = &Syncer{}

// EnsureBackendService ensures that the required backend services exist.
// See interface for more details.
func (b *Syncer) EnsureBackendService(lbName string, ports []ingressbe.ServicePort, hcMap healthcheck.HealthChecksMap, npMap NamedPortsMap, igLinks []string, forceUpdate bool) (BackendServicesMap, error) {
	fmt.Println("Ensuring backend services")
	glog.V(5).Infof("Received health checks map: %v", hcMap)
	glog.V(5).Infof("Received named ports map: %v", npMap)
	glog.V(5).Infof("Received instance groups: %v", igLinks)

	// Get existing balancing modes for the current backends
	balancingModeMap := map[string]string{}
	bss, beErr := b.bsp.ListGlobalBackendServices()
	if beErr != nil {
		return nil, beErr
	}
	fmt.Printf("Listing current BalancingModes\n")
	for _, bs := range bss {
		for _, backend := range bs.Backends {
			fmt.Printf("Group %s: mode is %s\n", backend.Group, backend.BalancingMode)
			if backend.BalancingMode == "RATE" || backend.BalancingMode == "UTILIZATION" {
				balancingModeMap[backend.Group] = backend.BalancingMode
				// BalancingMode "CONNECTION" is ignored: that is the same as RATE but for TCP instead of HTTP
			}
		}
	}

	// Create any non-existing backend services
	var err error
	ensuredBackendServices := BackendServicesMap{}
	for _, p := range ports {
		be, beErr := b.ensureBackendService(lbName, p, hcMap[p.NodePort], npMap[p.NodePort], igLinks, balancingModeMap, forceUpdate)
		if beErr != nil {
			beErr = fmt.Errorf("Error %s in ensuring backend service for port %v", beErr, p)
			fmt.Printf("Error ensuring backend service for port %v: %v. Continuing.\n", p, beErr)
			// Try ensuring backend services for all ports and return all errors at once.
			err = multierror.Append(err, beErr)
			continue
		}
		ensuredBackendServices[p.SvcName.Name] = be
	}
	return ensuredBackendServices, err
}

// DeleteBackendServices deletes all backend services that would have been created by EnsureBackendService.
// See interface for more details.
func (b *Syncer) DeleteBackendServices(ports []ingressbe.ServicePort) error {
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

// RemoveFromClusters removes the clusters corresponding to the given removeIGLinks from the existing backend services corresponding to the given ports.
// See interface for more details.
func (b *Syncer) RemoveFromClusters(ports []ingressbe.ServicePort, removeIGLinks []string) error {
	fmt.Println("Removing backend services from clusters")
	var err error
	for _, p := range ports {
		if beErr := b.removeFromClusters(p, removeIGLinks); beErr != nil {
			beErr = fmt.Errorf("Error %s in removing backend service for port %v", beErr, p)
			fmt.Printf("Error in removing backend service for port %v: %v. Continuing.\n", p, beErr)
			// Try removing backend services for all ports and return all errors at once.
			err = multierror.Append(err, beErr)
			continue
		}
	}
	return err
}

func (b *Syncer) removeFromClusters(port ingressbe.ServicePort, removeIGLinks []string) error {
	name := b.namer.BeServiceName(port.NodePort)

	existingBE, err := b.bsp.GetGlobalBackendService(name)
	if err != nil {
		err := fmt.Errorf("error in fetching existing backend service %s: %s", name, err)
		fmt.Println(err)
		return err
	}
	// Remove clusters from the backend service.
	desiredBE := b.desiredBackendServiceWithoutClusters(existingBE, removeIGLinks)
	glog.V(5).Infof("Existing backend service: %v\n, desired: %v\n", *existingBE, *desiredBE)
	_, err = b.updateBackendService(desiredBE)
	return err
}

func (b *Syncer) deleteBackendService(port ingressbe.ServicePort) error {
	name := b.namer.BeServiceName(port.NodePort)
	glog.V(2).Infof("Deleting backend service %s", name)
	err := b.bsp.DeleteGlobalBackendService(name)
	if err != nil {
		if utils.IsHTTPErrorCode(err, http.StatusNotFound) {
			fmt.Println("Backend service", name, "does not exist. Nothing to delete")
			return nil
		}
		fmt.Println("Error in deleting backend service", name, ":", err)
		return err
	}
	glog.V(2).Infof("Successfully deleted backend service %s", name)
	return nil
}

// ensureBackendService ensures that the required backend service exists for the given port.
// If it doesn't exist, creates one. If it already exists and is up to date,
// does nothing. If it exists, it will only be updated if forceUpdate is true.
func (b *Syncer) ensureBackendService(lbName string, port ingressbe.ServicePort, hc *compute.HealthCheck, np *compute.NamedPort, igLinks []string, balancingModeMap map[string]string, forceUpdate bool) (*compute.BackendService, error) {
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
	desiredBE := b.desiredBackendService(lbName, port, hc.SelfLink, np.Name, igLinks, balancingModeMap)
	name := desiredBE.Name
	// Check if backend service already exists.
	existingBE, err := b.bsp.GetGlobalBackendService(name)
	if err == nil {
		fmt.Println("Backend service", name, "exists already. Checking if it matches our desired backend service")
		jsonExisting, mErr := json.Marshal(existingBE)
		if mErr != nil {
			glog.Infof("Error marshaling existing backend: %v", mErr)
		}
		jsonDesired, mErr := json.Marshal(desiredBE)
		if mErr != nil {
			glog.Infof("Error marshalling desired backend: %v", mErr)
		}
		glog.V(5).Infof("Existing backend service:\n%s\nDesired backend service:\n%s", jsonExisting, jsonDesired)
		// Backend service with that name exists already. Check if it matches what we want.
		if backendServiceMatches(desiredBE, existingBE) {
			// Nothing to do. Desired backend service exists already.
			fmt.Println("Desired backend service exists already. Nothing to do.")
			return existingBE, nil
		}
		if forceUpdate {
			// TODO(G-Harmon): Figure out how to properly calculate the fp. Using Sha256 returned a googleapi error.
			desiredBE.Fingerprint = existingBE.Fingerprint
			return b.updateBackendService(desiredBE)
		}
		// TODO(G-Harmon): Show diff to user and prompt yes/no for overwriting.
		fmt.Println("Will not overwrite a differing BackendService without the --force flag.")
		// TODO(G-Harmon): share json-formatting logic with code above.
		glog.V(5).Infof("Desired backend service:\n%+v\nExisting backend service:\n%+v",
			desiredBE, existingBE)
		return nil, fmt.Errorf("will not overwrite BackendService without --force")
	}
	glog.V(5).Infof("Got error %s while trying to get existing backend service %s", err, name)
	// TODO(nikhiljindal): Handle non NotFound errors. We should create only if the error is NotFound.
	// Create the backend service.
	return b.createBackendService(desiredBE)
}

// updateBackendService updates the backend service and returns the updated backend service.
func (b *Syncer) updateBackendService(desiredBE *compute.BackendService) (*compute.BackendService, error) {
	name := desiredBE.Name
	fmt.Println("Updating existing backend service", name, "to match the desired state")
	err := b.bsp.UpdateGlobalBackendService(desiredBE)
	if err != nil {
		// TODO(G-Harmon): Errors should probably go to STDERR.
		fmt.Printf("Error from UpdateGlobalBackendService: %s\n", err)
		return nil, err
	}
	fmt.Println("Backend service", name, "updated successfully")
	return b.bsp.GetGlobalBackendService(name)
}

// createBackendService creates the backend service and returns the created backend service.
func (b *Syncer) createBackendService(desiredBE *compute.BackendService) (*compute.BackendService, error) {
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
	// TODO(G-Harmon): Switch to ignoring specific fields, to be safer when fields are later added.
	equal := desiredBE.AffinityCookieTtlSec == existingBE.AffinityCookieTtlSec &&
		reflect.DeepEqual(desiredBE.Backends, existingBE.Backends) &&
		reflect.DeepEqual(desiredBE.CdnPolicy, existingBE.CdnPolicy) &&
		reflect.DeepEqual(desiredBE.ConnectionDraining, existingBE.ConnectionDraining) &&
		desiredBE.Description == existingBE.Description &&
		desiredBE.EnableCDN == existingBE.EnableCDN &&
		// TODO(G-Harmon): Check fingerprint? fp is not required for
		// insert but is required for update. Seems like we don't have
		// to check it.
		reflect.DeepEqual(desiredBE.HealthChecks, existingBE.HealthChecks) &&
		((desiredBE.Iap == nil && existingBE.Iap == nil) ||
			(desiredBE.Iap.Enabled == existingBE.Iap.Enabled &&
				desiredBE.Iap.Oauth2ClientId == existingBE.Iap.Oauth2ClientId &&
				desiredBE.Iap.Oauth2ClientSecret == existingBE.Iap.Oauth2ClientSecret)) &&
		desiredBE.LoadBalancingScheme == existingBE.LoadBalancingScheme &&
		desiredBE.Name == existingBE.Name &&
		desiredBE.Port == existingBE.Port &&
		desiredBE.PortName == existingBE.PortName &&
		desiredBE.Protocol == existingBE.Protocol &&
		desiredBE.SessionAffinity == existingBE.SessionAffinity &&
		desiredBE.TimeoutSec == existingBE.TimeoutSec
	if !equal {
		glog.Infof("Diff:\n%v", diff.ObjectDiff(desiredBE, existingBE))
	}
	return equal
}

func (b *Syncer) desiredBackendService(lbName string, port ingressbe.ServicePort, hcLink, portName string, igLinks []string, balancingModeMap map[string]string) *compute.BackendService {
	// Compute the desired backend service.
	return &compute.BackendService{
		Name:         b.namer.BeServiceName(port.NodePort),
		Description:  fmt.Sprintf("Backend service for service %s as part of kubernetes multicluster loadbalancer %s", port.Description(), lbName),
		Protocol:     string(port.Protocol),
		HealthChecks: []string{hcLink},
		Port:         port.NodePort,
		PortName:     portName,
		Backends:     desiredBackends(igLinks, balancingModeMap),
		// We have to fill in these fields so we can properly compare to what's returned to us.
		// TODO(G-Harmon): We should get these values from existing if it exists and skip them otherwise, rather than hardcoding them here.
		ConnectionDraining:  &compute.ConnectionDraining{},
		Kind:                "compute#backendService",
		LoadBalancingScheme: "EXTERNAL",
		SessionAffinity:     "NONE",
		TimeoutSec:          30,
	}
}

// desiredBackendServiceWithoutClusters returns the desired backend service after removing the given instance groups from the given existing backend service.
func (b *Syncer) desiredBackendServiceWithoutClusters(existingBE *compute.BackendService, removeIGLinks []string) *compute.BackendService {
	// Compute the backends to be removed.
	removeBackends := desiredBackends(removeIGLinks, nil)
	removeBackendsMap := map[string]bool{}
	for _, v := range removeBackends {
		removeBackendsMap[v.Group] = true
	}
	var newBackends []*compute.Backend
	for _, v := range existingBE.Backends {
		if !removeBackendsMap[v.Group] {
			newBackends = append(newBackends, v)
		}
	}
	desiredBE := existingBE
	desiredBE.Backends = newBackends
	return desiredBE
}

func desiredBackends(igLinks []string, balancingModeMap map[string]string) []*compute.Backend {
	// Sort the slice so we get determistic results.
	sort.Strings(igLinks)
	var backends []*compute.Backend
	for _, ig := range igLinks {
		desiredMode, hasMode := balancingModeMap[ig]
		if !hasMode {
			desiredMode = "RATE"
		}

		switch desiredMode {
		case "UTILIZATION":
			backends = append(backends, &compute.Backend{
				Group:          ig,
				BalancingMode:  "UTILIZATION",
				MaxUtilization: 1.0,
				CapacityScaler: 1,
			})
		default:
			backends = append(backends, &compute.Backend{
				Group: ig,
				// We create the backend service with RATE balancing mode and set max rate
				// per instance to max value so that all requests in a region are sent to
				// instances in that region.
				// Setting rps to 1, for example, would round robin requests amongst all
				// instances.
				BalancingMode:      "RATE",
				MaxRatePerInstance: 1e14,
				// We have to fill in these fields so we can properly compare to what's returned to us
				CapacityScaler: 1,
			})
		}
	}
	return backends
}
