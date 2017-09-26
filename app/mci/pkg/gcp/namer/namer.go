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

package namer

import (
	"fmt"
)

const (
	hcPrefix      = "hc"
	backendPrefix = "be"

	// A delimiter used for clarity in naming GCE resources.
	lbNameDelimiter = "--"

	// Arbitrarily chosen alphanumeric character to use in constructing resource
	// names, eg: to avoid cases where we end up with a name ending in '-'.
	alphaNumericChar = "0"

	// Names longer than this are truncated, because of GCE restrictions.
	// Note that the GCE limit is 63 but we use 62 here since we always append a
	// trailing alphanumeric character due to GCE's naming rules.
	nameLenLimit = 62
)

type Namer struct {
	prefix string
	lbName string
}

func NewNamer(prefix, lbName string) *Namer {
	return &Namer{
		prefix: prefix,
		lbName: lbName,
	}
}

func (n *Namer) HealthCheckName(port int64) string {
	return n.decorateName(fmt.Sprintf("%v-%v-%d", n.prefix, hcPrefix, port))
}

func (n *Namer) BeServiceName(port int64) string {
	// We append the load balancer name to the backend service name and
	// hence we dont share a backend service even if same kube service is
	// backing multiple load balancers across the same set of clusters.
	// This is different than single cluster ingress, where we reuse backend services.
	// This is difficult in multi cluster ingres since we will need to keep track
	// of the set of clusters the load balancer is spread to and stop/start sharing
	// anytime that set changes.
	return n.decorateName(fmt.Sprintf("%v-%v-%d", n.prefix, backendPrefix, port))
}

func (n *Namer) decorateName(name string) string {
	lbName := n.lbName
	if lbName == "" {
		return name
	}
	return n.Truncate(fmt.Sprintf("%v%v%v", name, lbNameDelimiter, n.lbName))
}

// Truncate truncates the given key to a GCE length limit.
func (n *Namer) Truncate(key string) string {
	if len(key) > nameLenLimit {
		// GCE requires names to end with an alphanumeric, but allows characters
		// like '-', so make sure the trucated name ends legally.
		return fmt.Sprintf("%v%v", key[:nameLenLimit], alphaNumericChar)
	}
	return key
}
