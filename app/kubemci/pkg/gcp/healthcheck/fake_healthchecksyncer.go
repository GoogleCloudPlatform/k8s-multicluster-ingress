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
	"google.golang.org/api/compute/v1"
	"k8s.io/client-go/kubernetes"
	ingressbe "k8s.io/ingress-gce/pkg/backends"
)

type FakeHealthCheck struct {
	LBName string
	Port   ingressbe.ServicePort
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

func (f *FakeHealthCheckSyncer) EnsureHealthCheck(lbName string, ports []ingressbe.ServicePort, clients map[string]kubernetes.Interface, force bool) (HealthChecksMap, error) {
	hcMap := HealthChecksMap{}
	for _, p := range ports {
		f.EnsuredHealthChecks = append(f.EnsuredHealthChecks, FakeHealthCheck{
			LBName: lbName,
			Port:   p,
		})
		hcMap[p.Port] = &compute.HealthCheck{}
	}
	return hcMap, nil
}

func (f *FakeHealthCheckSyncer) DeleteHealthChecks(ports []ingressbe.ServicePort) error {
	f.EnsuredHealthChecks = nil
	return nil
}
