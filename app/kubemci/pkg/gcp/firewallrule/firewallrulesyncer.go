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

package firewallrule

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/golang/glog"
	"google.golang.org/api/compute/v1"
	ingressbe "k8s.io/ingress-gce/pkg/backends"
	ingressfw "k8s.io/ingress-gce/pkg/firewalls"

	utilsnamer "github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/namer"
	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/networktags"
)

// Src ranges from which the GCE L7 performs health checks.
var l7SrcRanges = []string{"130.211.0.0/22", "35.191.0.0/16"}

// FirewallRuleSyncer manages GCP firewall rules for multicluster GCP L7 load balancers.
type FirewallRuleSyncer struct {
	namer *utilsnamer.Namer
	// Firewall rules provider to call GCE APIs to manipulate firewall rules.
	fwp ingressfw.Firewall
	// NetworkTagsGetterInterface to fetch network tags from instances.
	ntg networktags.NetworkTagsGetterInterface
}

func NewFirewallRuleSyncer(namer *utilsnamer.Namer, fwp ingressfw.Firewall, ntg networktags.NetworkTagsGetterInterface) FirewallRuleSyncerInterface {
	return &FirewallRuleSyncer{
		namer: namer,
		fwp:   fwp,
		ntg:   ntg,
	}
}

// Ensure this implements FirewallRuleSyncerInterface.
var _ FirewallRuleSyncerInterface = &FirewallRuleSyncer{}

// EnsureFirewallRule ensures that the required firewall rules exist for the given ports.
// Does nothing if they exist already, else creates new ones.
func (s *FirewallRuleSyncer) EnsureFirewallRule(lbName string, ports []ingressbe.ServicePort, igLinks map[string][]string) error {
	fmt.Println("Ensuring firewall rule")
	glog.V(5).Infof("Received ports: %v", ports)
	glog.V(5).Infof("Received instance groups: %v", igLinks)
	err := s.ensureFirewallRule(lbName, ports, igLinks)
	if err != nil {
		return fmt.Errorf("Error %s in ensuring firewall rule", err)
	}
	return nil
}

func (s *FirewallRuleSyncer) DeleteFirewallRules() error {
	name := s.namer.FirewallRuleName()
	fmt.Println("Deleting firewall rule", name)
	if err := s.fwp.DeleteFirewall(name); err != nil {
		fmt.Printf("Error in deleting firewall rule %s: %s", name, err)
		return err
	}
	fmt.Println("firewall rule", name, "deleted successfully")
	return nil
}

// ensureFirewallRule ensures that the required firewall rule exists for the given ports.
// Does nothing if it exists already, else creates a new one.
func (s *FirewallRuleSyncer) ensureFirewallRule(lbName string, ports []ingressbe.ServicePort, igLinks map[string][]string) error {
	desiredFW, err := s.desiredFirewallRule(lbName, ports, igLinks)
	if err != nil {
		return err
	}
	name := desiredFW.Name
	// Check if firewall rule already exists.
	existingFW, err := s.fwp.GetFirewall(name)
	if err == nil {
		fmt.Println("Firewall rule", name, "exists already. Checking if it matches our desired firewall rule")
		glog.V(5).Infof("Existing firewall rule: %v\n, desired firewall rule: %v", existingFW, *desiredFW)
		// Firewall rule with that name exists already. Check if it matches what we want.
		if firewallRuleMatches(desiredFW, existingFW) {
			// Nothing to do. Desired firewall rule exists already.
			fmt.Println("Desired firewall rule exists already")
			return nil
		}
		// TODO (nikhiljindal): Require explicit permission from user before doing this.
		return s.updateFirewallRule(desiredFW)
	}
	glog.V(5).Infof("Got error %s while trying to get existing firewall rule %s", err, name)
	// TODO(nikhiljindal): Handle non NotFound errors. We should create only if the error is NotFound.
	// Create the firewall rule.
	return s.createFirewallRule(desiredFW)
}

// updateFirewallRule updates the firewall rule and returns the updated firewall rule.
func (s *FirewallRuleSyncer) updateFirewallRule(desiredFR *compute.Firewall) error {
	name := desiredFR.Name
	fmt.Println("Updating existing firewall rule", name, "to match the desired state")
	err := s.fwp.UpdateFirewall(desiredFR)
	if err != nil {
		return err
	}
	fmt.Println("Firewall rule", name, "updated successfully")
	return nil
}

// createFirewallRule creates the firewall rule and returns the created firewall rule.
func (s *FirewallRuleSyncer) createFirewallRule(desiredFR *compute.Firewall) error {
	name := desiredFR.Name
	fmt.Println("Creating firewall rule", name)
	glog.V(5).Infof("Creating firewall rule %v", desiredFR)
	err := s.fwp.CreateFirewall(desiredFR)
	if err != nil {
		return err
	}
	fmt.Println("Firewall rule", name, "created successfully")
	return nil
}

func firewallRuleMatches(desiredFR, existingFR *compute.Firewall) bool {
	// TODO(nikhiljindal): Add proper logic to figure out if the 2 firewall rules match.
	// Need to add the --force flag for user to consent overwriting before this method can be updated to return false.
	// Also need to take special care of target tags. There can be multiple target tags, all of which can be "correct".
	return true
}

func (s *FirewallRuleSyncer) desiredFirewallRule(lbName string, ports []ingressbe.ServicePort, igLinks map[string][]string) (*compute.Firewall, error) {
	// Compute the desired firewall rule.
	targetTags, err := s.getTargetTags(igLinks)
	if err != nil {
		return nil, err
	}
	fwPorts := make([]string, len(ports))
	for i := range ports {
		fwPorts[i] = strconv.Itoa(int(ports[i].Port))
	}
	// Sort the ports and tags to have a deterministic order.
	sort.Strings(fwPorts)
	sort.Strings(targetTags)
	return &compute.Firewall{
		Name:         s.namer.FirewallRuleName(),
		Description:  fmt.Sprintf("Firewall rule for kubernetes multicluster loadbalancer %s", lbName),
		SourceRanges: l7SrcRanges,
		Allowed: []*compute.FirewallAllowed{
			{
				IPProtocol: "tcp",
				Ports:      fwPorts,
			},
		},
		TargetTags: targetTags,
		// TODO(nikhiljindal): Set the `Network` field for non-default networks.
	}, nil
}

func (s *FirewallRuleSyncer) getTargetTags(igLinks map[string][]string) ([]string, error) {
	// We assume that all instances in a cluster have the same target tags.
	// This is true for default GKE and GCE clusters (brought up using kube-up).
	// So we return the first target tag from the first instance in first instance group of each cluster.
	// TODO(nikhiljindal): Make this more resilient. If we fail to fetch tag from one instance group, try the next.
	var tags []string
	for _, v := range igLinks {
		items, err := s.ntg.GetNetworkTags(v[0])
		if err != nil {
			return nil, err
		}
		tags = append(tags, items[0])
	}
	return tags, nil
}
