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

// TargetProxySyncerInterface is an interface to manage GCP target proxies.
type TargetProxySyncerInterface interface {
	// EnsureHttpTargetProxy ensures that the required http target proxy exists for the given load balancer and url map link.
	// Returns the self link for the ensured proxy.
	EnsureHttpTargetProxy(lbName, urlMapLink string) (string, error)
	// DeleteTargetProxies deletes the target proxies that EnsureTargetProxy would have created.
	DeleteTargetProxies() error
}
