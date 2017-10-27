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

package forwardingrule

import (
	"fmt"

	"github.com/golang/glog"
	"google.golang.org/api/compute/v1"
	ingresslb "k8s.io/ingress-gce/pkg/loadbalancers"
	"k8s.io/ingress-gce/pkg/utils"

	utilsnamer "github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/mci/pkg/gcp/namer"
)

const (
	httpDefaultPortRange  = "80-80"
	httpsDefaultPortRange = "443-443"
)

// ForwardingRuleSyncer manages GCP forwarding rules for multicluster GCP L7 load balancers.
type ForwardingRuleSyncer struct {
	namer *utilsnamer.Namer
	// Instance of ForwardingRuleProvider interface for calling GCE ForwardingRule APIs.
	// There is no separate ForwardingRuleProvider interface, so we use the bigger LoadBalancers interface here.
	frp ingresslb.LoadBalancers
}

func NewForwardingRuleSyncer(namer *utilsnamer.Namer, frp ingresslb.LoadBalancers) ForwardingRuleSyncerInterface {
	return &ForwardingRuleSyncer{
		namer: namer,
		frp:   frp,
	}
}

// Ensure this implements ForwardingRuleSyncerInterface.
var _ ForwardingRuleSyncerInterface = &ForwardingRuleSyncer{}

// EnsureHttpForwardingRule ensures that the required http forwarding rule exists.
// Does nothing if it exists already, else creates a new one.
func (s *ForwardingRuleSyncer) EnsureHttpForwardingRule(lbName, ipAddress, targetProxyLink string) error {
	fmt.Println("Ensuring http forwarding rule")
	desiredFR := s.desiredForwardingRule(lbName, ipAddress, targetProxyLink)
	name := desiredFR.Name
	// Check if forwarding rule already exists.
	existingFR, err := s.frp.GetGlobalForwardingRule(name)
	if err == nil {
		fmt.Println("forwarding rule", name, "exists already. Checking if it matches our desired forwarding rule", name)
		glog.V(5).Infof("Existing forwarding rule: %v\n, desired forwarding rule: %v", existingFR, desiredFR)
		// Forwarding Rule with that name exists already. Check if it matches what we want.
		if forwardingRuleMatches(desiredFR, existingFR) {
			// Nothing to do. Desired forwarding rule exists already.
			fmt.Println("Desired forwarding rule exists already")
			return nil
		}
		// TODO: Require explicit permission from user before doing this.
		return s.updateForwardingRule(existingFR, desiredFR)
	}
	glog.V(5).Infof("Got error %s while trying to get existing forwarding rule %s. Will try to create new one", err, name)
	// TODO: Handle non NotFound errors. We should create only if the error is NotFound.
	// Create the forwarding rule.
	return s.createForwardingRule(desiredFR)
}

func (s *ForwardingRuleSyncer) updateForwardingRule(existingFR, desiredFR *compute.ForwardingRule) error {
	name := desiredFR.Name
	fmt.Println("Updating existing forwarding rule", name, "to match the desired state")
	// We do not have an UpdateForwardingRule method.
	// If target proxy link is the only thing that is different, then we can call SetProxyForGlobalForwardingRule.
	// Else, we need to delete the existing rule and create a new one.
	if existingFR.IPAddress != desiredFR.IPAddress || existingFR.PortRange != desiredFR.PortRange || existingFR.IPProtocol != desiredFR.IPProtocol {
		fmt.Println("Deleting the existing forwarding rule", name, "and will create a new one")
		if err := utils.IgnoreHTTPNotFound(s.frp.DeleteGlobalForwardingRule(name)); err != nil {
			return fmt.Errorf("error in deleting existing forwarding rule %s: %s", name, err)
		}
		return s.createForwardingRule(desiredFR)
	}
	// Update target proxy link in forwarding rule.
	err := s.frp.SetProxyForGlobalForwardingRule(name, desiredFR.Target)
	if err != nil {
		return err
	}
	fmt.Println("Forwarding rule", name, "updated successfully")
	return nil
}

func (s *ForwardingRuleSyncer) createForwardingRule(desiredFR *compute.ForwardingRule) error {
	name := desiredFR.Name
	fmt.Println("Creating forwarding rule", name)
	glog.V(5).Infof("Creating forwarding rule %v", desiredFR)
	err := s.frp.CreateGlobalForwardingRule(desiredFR)
	if err != nil {
		return err
	}
	fmt.Println("Forwarding rule", name, "created successfully")
	return nil
}

func forwardingRuleMatches(desiredFR, existingFR *compute.ForwardingRule) bool {
	// TODO: Add proper logic to figure out if the 2 forwarding rules match.
	// Need to add the --force flag for user to consent overritting before this method can be updated to return false.
	return true
}

func (s *ForwardingRuleSyncer) desiredForwardingRule(lbName, ipAddress, targetProxyLink string) *compute.ForwardingRule {
	// Compute the desired forwarding rule.
	return &compute.ForwardingRule{
		Name:        s.namer.HttpForwardingRuleName(),
		Description: fmt.Sprintf("Http forwarding rule for kubernetes multicluster loadbalancer %s", lbName),
		IPAddress:   ipAddress,
		Target:      targetProxyLink,
		PortRange:   httpDefaultPortRange,
		IPProtocol:  "TCP",
	}
}
