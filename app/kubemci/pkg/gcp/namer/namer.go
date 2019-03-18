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
	hcPrefix                  = "hc"
	backendPrefix             = "be"
	urlMapPrefix              = "um"
	targetHTTPProxyPrefix     = "tp"
	targetHTTPSProxyPrefix    = "tps"
	httpForwardingRulePrefix  = "fw"
	httpsForwardingRulePrefix = "fws"
	firewallRulePrefix        = "fr"
	sslCertPrefix             = "ssl"

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

// Namer provides a naming schema for GCP resources
type Namer struct {
	prefix string
	lbName string
}

// NewNamer returns a new namer using the given prefix and load balancer name.
func NewNamer(prefix, lbName string) *Namer {
	return &Namer{
		prefix: prefix,
		lbName: lbName,
	}
}

// HealthCheckName returns the name for a health check on the given port.
func (n *Namer) HealthCheckName(port int64) string {
	return n.decorateName(fmt.Sprintf("%v-%v-%d", n.prefix, hcPrefix, port))
}

// BeServiceName returns the name for the backend service on the given port.
func (n *Namer) BeServiceName(port int64) string {
	// We append the load balancer name to the backend service name and
	// hence we don't share a backend service even if same kube service is
	// backing multiple load balancers across the same set of clusters.
	// This is different than single cluster ingress, where we reuse backend services.
	// This is difficult in multi-cluster ingress since we will need to keep track
	// of the set of clusters the load balancer is spread to and stop/start sharing
	// anytime that set changes.
	return n.decorateName(fmt.Sprintf("%v-%v-%d", n.prefix, backendPrefix, port))
}

// URLMapName returns the name for a url map.
func (n *Namer) URLMapName() string {
	return n.decorateName(fmt.Sprintf("%v-%v", n.prefix, urlMapPrefix))
}

// TargetHTTPProxyName returns a name for the http proxy.
func (n *Namer) TargetHTTPProxyName() string {
	return n.decorateName(fmt.Sprintf("%v-%v", n.prefix, targetHTTPProxyPrefix))
}

// TargetHTTPSProxyName returns a name for the https proxy.
func (n *Namer) TargetHTTPSProxyName() string {
	return n.decorateName(fmt.Sprintf("%v-%v", n.prefix, targetHTTPSProxyPrefix))
}

// HTTPSForwardingRuleName returns the forwarding rule name.
func (n *Namer) HTTPSForwardingRuleName() string {
	return n.decorateName(fmt.Sprintf("%v-%v", n.prefix, httpsForwardingRulePrefix))
}

// HTTPForwardingRuleName returns the forwarding rule name.
func (n *Namer) HTTPForwardingRuleName() string {
	return n.decorateName(fmt.Sprintf("%v-%v", n.prefix, httpForwardingRulePrefix))
}

// FirewallRuleName returns a name for a firewall.
func (n *Namer) FirewallRuleName(network string) string {
	return n.decorateName(fmt.Sprintf("%v-%v-%v", n.prefix, firewallRulePrefix, network))
}

// SSLCertName returns a name for SSL certificates.
func (n *Namer) SSLCertName() string {
	return n.decorateName(fmt.Sprintf("%v-%v", n.prefix, sslCertPrefix))
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
