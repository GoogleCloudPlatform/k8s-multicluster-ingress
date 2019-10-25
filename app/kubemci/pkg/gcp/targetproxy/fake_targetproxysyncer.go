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
	// FakeTargetProxySelfLink is a fake self link for target proxies.
	FakeTargetProxySelfLink = "target/proxy/self/link"
)

type fakeTargetProxy struct {
	LBName string
	UmLink string
	// Link to the SSL certificate. This is present only for HTTPS target proxies.
	CertLinks []string
}

// FakeTargetProxySyncer is a fake implementation of SyncerInterface to be used in tests.
type FakeTargetProxySyncer struct {
	// List of target proxies that this has been asked to ensure.
	EnsuredTargetProxies []fakeTargetProxy
}

// NewFakeTargetProxySyncer returns a new instance of the fake syncer.
func NewFakeTargetProxySyncer() SyncerInterface {
	return &FakeTargetProxySyncer{}
}

// Ensure this implements SyncerInterface.
var _ SyncerInterface = &FakeTargetProxySyncer{}

// EnsureHTTPTargetProxy ensures that a http target proxy exists for the given load balancer.
// See interface comments for details.
func (f *FakeTargetProxySyncer) EnsureHTTPTargetProxy(lbName, umLink string, forceUpdate bool) (string, error) {
	f.EnsuredTargetProxies = append(f.EnsuredTargetProxies, fakeTargetProxy{
		LBName: lbName,
		UmLink: umLink,
	})
	return FakeTargetProxySelfLink, nil
}

// EnsureHTTPSTargetProxy ensures that a https target proxy exists for the given load balancer.
// See interface comments for details.
func (f *FakeTargetProxySyncer) EnsureHTTPSTargetProxy(lbName, umLink string, certLinks []string, forceUpdate bool) (string, error) {
	f.EnsuredTargetProxies = append(f.EnsuredTargetProxies, fakeTargetProxy{
		LBName:    lbName,
		UmLink:    umLink,
		CertLinks: certLinks,
	})
	return FakeTargetProxySelfLink, nil
}

// DeleteTargetProxies deletes the target proxies that EnsureHTTPTargetProxy and EnsureHTTPSTargetProxy would have created.
// See interface comments for details.
func (f *FakeTargetProxySyncer) DeleteTargetProxies() error {
	f.EnsuredTargetProxies = nil
	return nil
}
