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

package urlmap

import (
	"k8s.io/api/extensions/v1beta1"

	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/mci/pkg/gcp/backendservice"
)

const (
	FakeUrlSelfLink = "selfLink"
)

type FakeURLMap struct {
	LBName  string
	Ingress *v1beta1.Ingress
	BeMap   backendservice.BackendServicesMap
}

type FakeURLMapSyncer struct {
	// List of url maps that this has been asked to ensure.
	EnsuredURLMaps []FakeURLMap
}

// Fake url map syncer to be used for tests.
func NewFakeURLMapSyncer() URLMapSyncerInterface {
	return &FakeURLMapSyncer{}
}

// Ensure this implements URLMapSyncerInterface.
var _ URLMapSyncerInterface = &FakeURLMapSyncer{}

func (f *FakeURLMapSyncer) EnsureURLMap(lbName string, ing *v1beta1.Ingress, beMap backendservice.BackendServicesMap) (string, error) {
	f.EnsuredURLMaps = append(f.EnsuredURLMaps, FakeURLMap{
		LBName:  lbName,
		Ingress: ing,
		BeMap:   beMap,
	})
	return FakeUrlSelfLink, nil
}
