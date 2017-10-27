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

package targetproxy

const (
	FakeTargetProxySelfLink = "target/proxy/self/link"
)

type FakeTargetProxy struct {
	LBName string
	UmLink string
}

type FakeTargetProxySyncer struct {
	// List of target proxies that this has been asked to ensure.
	EnsuredTargetProxies []FakeTargetProxy
}

// Fake target proxy syncer to be used for tests.
func NewFakeTargetProxySyncer() TargetProxySyncerInterface {
	return &FakeTargetProxySyncer{}
}

// Ensure this implements TargetProxySyncerInterface.
var _ TargetProxySyncerInterface = &FakeTargetProxySyncer{}

func (f *FakeTargetProxySyncer) EnsureHttpTargetProxy(lbName, umLink string) (string, error) {
	f.EnsuredTargetProxies = append(f.EnsuredTargetProxies, FakeTargetProxy{
		LBName: lbName,
		UmLink: umLink,
	})
	return FakeTargetProxySelfLink, nil
}

func (f *FakeTargetProxySyncer) DeleteTargetProxies() error {
	f.EnsuredTargetProxies = nil
	return nil
}
