// Copyright 2018 Google Inc.
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

package validations

import (
	"github.com/golang/glog"
	"k8s.io/api/extensions/v1beta1"
	kubeclient "k8s.io/client-go/kubernetes"
)

// FakeValidator is a fake implementation of ValidatorInterface, for tests.
type FakeValidator struct {
	ValidatedClusterSets []map[string]kubeclient.Interface
}

// NewFakeValidator returns a new intance of the fake Validator.
func NewFakeValidator() ValidatorInterface {
	return &FakeValidator{}
}

// Ensure this implements BackendServiceSyncerInterface.
var _ ValidatorInterface = &FakeValidator{}

// Validate records the clusters that were validated, so tests can use that information.
func (v *FakeValidator) Validate(clients map[string]kubeclient.Interface, ing *v1beta1.Ingress) error {
	return v.serverVersionsNewEnough(clients)
	// TODO: Might be useful to also call serviceNodePortsSame.
}

func (v *FakeValidator) serverVersionsNewEnough(clients map[string]kubeclient.Interface) error {
	glog.V(3).Infof("Fake ServerVersionsNewEnough: %v", clients)
	v.ValidatedClusterSets = append(v.ValidatedClusterSets, clients)
	return nil
}
