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
	sp "k8s-multi-cluster-ingress/app/mci/pkg/serviceport"
)

type FakeHealthCheck struct {
	LBName string
	Port   sp.ServicePort
}

type FakeHealthCheckSyncer struct {
	// List of health checks that this has been asked to ensure.
	EnsuredHealthChecks []FakeHealthCheck
}

// Fake health check syncer to be used for tests.
func NewFakeHealthCheckSyncer() HealthCheckSyncerInterface {
	return &FakeHealthCheckSyncer{}
}

// Ensure this implements HealthCheckSyncerInterface.
var _ HealthCheckSyncerInterface = &FakeHealthCheckSyncer{}

func (h *FakeHealthCheckSyncer) EnsureHealthCheck(lbName string, ports []sp.ServicePort) error {
	for _, p := range ports {
		h.EnsuredHealthChecks = append(h.EnsuredHealthChecks, FakeHealthCheck{
			LBName: lbName,
			Port:   p,
		})
	}
	return nil
}
