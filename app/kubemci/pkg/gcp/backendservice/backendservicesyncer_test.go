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
	"testing"

	compute "google.golang.org/api/compute/v1"

	"k8s.io/apimachinery/pkg/types"
	ingressbe "k8s.io/ingress-gce/pkg/backends"

	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/healthcheck"
	utilsnamer "github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/namer"
	"github.com/golang/glog"
)

func TestEnsureBackendService(t *testing.T) {
	lbName := "lb-name"
	port := int64(32211)
	portName := "portName"
	igLink := "igLink"
	kubeSvcName := "ingress-svc"
	// Should create the backend service as expected.
	bsp := ingressbe.NewFakeBackendServices(func(op int, be *compute.BackendService) error { return nil })
	namer := utilsnamer.NewNamer("mci1", lbName)
	beName := namer.BeServiceName(port)
	bss := NewBackendServiceSyncer(namer, bsp)
	// GET should return NotFound.
	if _, err := bsp.GetGlobalBackendService(beName); err == nil {
		t.Fatalf("expected NotFound error, actual: nil")
	}

	testCases := []struct {
		desc string
		// In's
		hcLink      string
		forceUpdate bool
		// Out's
		ensureErr bool
	}{
		{
			desc:        "Initial BE Creation",
			hcLink:      "hcLink",
			forceUpdate: false,
			ensureErr:   false,
		},
		{
			desc:        "ensureBackend: no work to do",
			hcLink:      "hcLink",
			forceUpdate: false,
			ensureErr:   false,
		},
		{
			desc:        "Change backend (force=true)",
			hcLink:      "different hcLink",
			forceUpdate: true,
			ensureErr:   false,
		},
		{
			desc:        "Attempt backend change with (force=false) fails",
			hcLink:      "hcLink the 3rd",
			forceUpdate: false,
			ensureErr:   true,
		},
	}

	for _, c := range testCases {
		glog.Infof("test case starting:%v", c.desc)
		beMap, err := bss.EnsureBackendService(lbName, []ingressbe.ServicePort{
			{
				Port:     port,
				Protocol: "HTTP",
				SvcName:  types.NamespacedName{Name: kubeSvcName},
			},
		}, healthcheck.HealthChecksMap{
			port: &compute.HealthCheck{
				SelfLink: c.hcLink,
			},
		}, NamedPortsMap{
			port: &compute.NamedPort{
				Port: port,
				Name: portName,
			},
		}, []string{igLink}, c.forceUpdate)
		if (err != nil) != c.ensureErr {
			t.Errorf("expected (error != nil) = %v, in ensuring backend service, got error: %v", c.ensureErr, err)
		}
		if c.ensureErr {
			// Can't do validation if we expected an error.
			continue
		}
		_, err = bsp.GetGlobalBackendService(beName)
		if err != nil {
			t.Errorf("expected nil GetGlobalBackendSvc error, actual: %v", err)
			continue
		}
		if len(beMap) != 1 || beMap[kubeSvcName] == nil {
			t.Errorf("unexpected backend service map: %v. Expected it to contain only the backend service for kube service %s", beMap, kubeSvcName)
			continue
		}
		be := beMap[kubeSvcName]
		if len(be.HealthChecks) != 1 || be.HealthChecks[0] != c.hcLink {
			t.Errorf("unexpected health check in backend service. expected: %s, got: %v", c.hcLink, be.HealthChecks)
		}
		if be.Port != port || be.PortName != portName {
			t.Errorf("unexpected port and port name, expected: %s/%d, got: %s/%d", portName, port, be.PortName, be.Port)
		}
		if len(be.Backends) != 1 || be.Backends[0].Group != igLink {
			t.Errorf("unexpected backends in backend service. expected one backend for %s, got: %v", igLink, be.Backends)
		}
	}
}

func TestDeleteBackendService(t *testing.T) {
	lbName := "lb-name"
	port := int64(32211)
	portName := "portName"
	igLink := "igLink"
	hcLink := "hcLink"
	kubeSvcName := "ingress-svc"
	// Should create the backend service as expected.
	bsp := ingressbe.NewFakeBackendServices(func(op int, be *compute.BackendService) error { return nil })
	namer := utilsnamer.NewNamer("mci1", lbName)
	beName := namer.BeServiceName(port)
	bss := NewBackendServiceSyncer(namer, bsp)
	ports := []ingressbe.ServicePort{
		{
			Port:     port,
			Protocol: "HTTP",
			SvcName:  types.NamespacedName{Name: kubeSvcName},
		},
	}
	if _, err := bss.EnsureBackendService(lbName, ports, healthcheck.HealthChecksMap{
		port: &compute.HealthCheck{
			SelfLink: hcLink,
		},
	}, NamedPortsMap{
		port: &compute.NamedPort{
			Port: port,
			Name: portName,
		},
	}, []string{igLink}, false /*forceUpdate*/); err != nil {
		t.Fatalf("expected no error in ensuring backend service, actual: %v", err)
	}
	if _, err := bsp.GetGlobalBackendService(beName); err != nil {
		t.Errorf("expected nil error, actual: %v", err)
	}
	// Verify that GET fails after DELETE.
	if err := bss.DeleteBackendServices(ports); err != nil {
		t.Fatalf("unexpected error in deleting backend services: %s", err)
	}
	if _, err := bsp.GetGlobalBackendService(beName); err == nil {
		t.Errorf("unexpected nil error, expected NotFound")
	}
}
