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
	"net/http"
	"testing"

	compute "google.golang.org/api/compute/v1"

	ingresshc "k8s.io/ingress-gce/pkg/healthchecks"
	"k8s.io/ingress-gce/pkg/utils"

	utilsnamer "github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/mci/pkg/gcp/namer"
	sp "github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/mci/pkg/serviceport"
)

func TestEnsureHealthCheck(t *testing.T) {
	lbName := "lb-name"
	port := int64(32211)
	// Should create the health check as expected.
	hcp := ingresshc.NewFakeHealthCheckProvider()
	namer := utilsnamer.NewNamer("mci", lbName)
	hcName := namer.HealthCheckName(port)
	hcs := NewHealthCheckSyncer(namer, hcp)
	// GET should return NotFound.
	if _, err := hcp.GetHealthCheck(hcName); !utils.IsHTTPErrorCode(err, http.StatusNotFound) {
		t.Fatalf("expected NotFound error, actual: %v", err)
	}
	force := true
	err := hcs.EnsureHealthCheck(lbName, []sp.ServicePort{
		{
			Port:     port,
			Protocol: "HTTP",
		},
	}, force)
	if err != nil {
		t.Fatalf("expected no error in ensuring health check (force=true), actual: %v", err)
	}
	// Verify that GET does not return NotFound.
	if _, err := hcp.GetHealthCheck(hcName); err != nil {
		t.Fatalf("expected nil error, actual: %v", err)
	}

	force = false
	err = hcs.EnsureHealthCheck(lbName, []sp.ServicePort{
		{
			Port:     port,
			Protocol: "HTTPS", /* a different protocol */
		},
	}, force)
	if err == nil {
		t.Errorf("expected error in ensuring health check (force=false), actual: %v", err)
	}
	if _, err := hcp.GetHealthCheck(hcName); err != nil {
		t.Fatalf("expected nil error, actual: %v", err)
	}

	force = true
	err = hcs.EnsureHealthCheck(lbName, []sp.ServicePort{
		{
			Port:     port,
			Protocol: "HTTPS", /* a different protocol */
		},
	}, force)
	if err != nil {
		t.Errorf("expected no error in ensuring health check (force=true), actual: %v", err)
	}
	if _, err := hcp.GetHealthCheck(hcName); err != nil {
		t.Fatalf("expected nil error, actual: %v", err)
	}

	// TODO: Test update existing health check.
	// TODO: Validate values in health check.
}

func TestHealthCheckMatches(t *testing.T) {
	var check compute.HealthCheck
	if !healthCheckMatches(&check, &check) {
		t.Errorf("Want healthCheckMatches(c, c) = true. got false.")
	}
	check2 := check
	check2.Description = "foo"
	if healthCheckMatches(&check, &check2) {
		t.Errorf("Want healthCheckMatches(c, c2) = false, c.description differs. got true.")
	}
	check.Description = "foo"
	if !healthCheckMatches(&check, &check2) {
		t.Errorf("fail")
	}
	check2.CreationTimestamp = "1234"
	if !healthCheckMatches(&check, &check2) {
		t.Errorf("fail")
	}
}
