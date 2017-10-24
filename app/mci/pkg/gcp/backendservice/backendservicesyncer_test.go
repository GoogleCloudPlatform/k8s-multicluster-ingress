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

	"google.golang.org/api/compute/v1"
	ingressbe "k8s.io/ingress-gce/pkg/backends"

	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/mci/pkg/gcp/healthcheck"
	utilsnamer "github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/mci/pkg/gcp/namer"
)

func TestEnsureBackendService(t *testing.T) {
	lbName := "lb-name"
	port := int64(32211)
	portName := "portName"
	igLink := "igLink"
	hcLink := "hcLink"
	// Should create the backend service as expected.
	bsp := ingressbe.NewFakeBackendServices(func(op int, be *compute.BackendService) error { return nil })
	namer := utilsnamer.NewNamer("mci", lbName)
	beName := namer.BeServiceName(port)
	bss := NewBackendServiceSyncer(namer, bsp)
	// GET should return NotFound.
	if _, err := bsp.GetGlobalBackendService(beName); err == nil {
		t.Fatalf("expected NotFound error, actual: nil")
	}
	err := bss.EnsureBackendService(lbName, []ingressbe.ServicePort{
		{
			Port:     port,
			Protocol: "HTTP",
		},
	}, healthcheck.HealthChecksMap{
		port: &compute.HealthCheck{
			SelfLink: hcLink,
		},
	}, NamedPortsMap{
		port: &compute.NamedPort{
			Port: port,
			Name: portName,
		},
	}, []string{igLink})
	if err != nil {
		t.Fatalf("expected no error in ensuring backend service, actual: %v", err)
	}
	// Verify that the created backend service is as expected.
	be, err := bsp.GetGlobalBackendService(beName)
	if err != nil {
		t.Fatalf("expected nil error, actual: %v", err)
	}
	if len(be.HealthChecks) != 1 || be.HealthChecks[0] != hcLink {
		t.Errorf("unexpected health check in backend service. expected: %s, got: %v", hcLink, be.HealthChecks)
	}
	if be.Port != port || be.PortName != portName {
		t.Errorf("unexpected port and port name, expected: %s/%s, got: %s/%s", portName, port, be.PortName, be.Port)
	}
	if len(be.Backends) != 1 || be.Backends[0].Group != igLink {
		t.Errorf("unexpected backends in backend service. expected one backend for %s, got: %v", igLink, be.Backends)
	}
	// TODO(nikhiljindal): Test update existing backend service.
}
