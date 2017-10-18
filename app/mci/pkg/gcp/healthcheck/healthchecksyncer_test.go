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

	ingresshc "k8s.io/ingress-gce/pkg/healthchecks"
	"k8s.io/ingress-gce/pkg/utils"

	utilsnamer "k8s-multi-cluster-ingress/app/mci/pkg/gcp/namer"
	sp "k8s-multi-cluster-ingress/app/mci/pkg/serviceport"
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
	err := hcs.EnsureHealthCheck(lbName, []sp.ServicePort{
		{
			Port:     port,
			Protocol: "HTTP",
		},
	})
	if err != nil {
		t.Fatalf("expected no error in ensuring health check, actual: %v", err)
	}
	// Verify that GET does not return NotFound.
	if _, err := hcp.GetHealthCheck(hcName); err != nil {
		t.Fatalf("expected nil error, actual: %v", err)
	}
	// TODO: Test update existing health check.
}
