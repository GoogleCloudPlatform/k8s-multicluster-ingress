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
	"reflect"
	"sort"
	"strconv"

	"github.com/golang/glog"
	compute "google.golang.org/api/compute/v1"
	"google.golang.org/api/googleapi"
	"k8s.io/apimachinery/pkg/util/diff"
	ingressbe "k8s.io/ingress-gce/pkg/backends"
	ingressfw "k8s.io/ingress-gce/pkg/firewalls"
	"k8s.io/ingress-gce/pkg/utils"

	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/instances"
	utilsnamer "github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/namer"
)

// Src ranges from which the GCE L7 performs health checks.
var l7SrcRanges = []string{"130.211.0.0/22", "35.191.0.0/16"}

// FirewallRuleSyncer manages GCP firewall rules for multicluster GCP L7 load balancers.
type FirewallRuleSyncer struct {
	namer *utilsnamer.Namer
	// Firewall rules provider to call GCE APIs to manipulate firewall rules.
	fwp ingressfw.Firewall
	// InstanceGetterInterface to fetch instances.
	ig instances.InstanceGetterInterface
}

func NewFirewallRuleSyncer(namer *utilsnamer.Namer, fwp ingressfw.Firewall, ig instances.InstanceGetterInterface) FirewallRuleSyncerInterface {
	return &FirewallRuleSyncer{
		namer: namer,
		fwp:   fwp,
		ig:    ig,
	}
}

// Ensure this implements FirewallRuleSyncerInterface.
var _ FirewallRuleSyncerInterface = &FirewallRuleSyncer{}

// EnsureFirewallRule ensures that the required firewall rules exist for the given ports.
// Does nothing if they exist already, else creates new ones.
func (s *FirewallRuleSyncer) EnsureFirewallRule(lbName string, ports []ingressbe.ServicePort, igLinks map[string][]string, forceUpdate bool) error {
	fmt.Println("Ensuring firewall rule")
	glog.V(5).Infof("Received ports: %v", ports)
	glog.V(5).Infof("Received instance groups: %v", igLinks)
	err := s.ensureFirewallRule(lbName, ports, igLinks, forceUpdate)
	if err != nil {
		return fmt.Errorf("Error %s in ensuring firewall rule", err)
	}
	return nil
}

func (s *FirewallRuleSyncer) DeleteFirewallRules() error {
	name := s.namer.FirewallRuleName()
	fmt.Println("Deleting firewall rule", name)
	if err := utils.IgnoreHTTPNotFound(s.fwp.DeleteFirewall(name)); err != nil {
		fmt.Printf("Error in deleting firewall rule %s: %s", name, err)
		return err
	}
	fmt.Println("Firewall rule", name, "deleted successfully")
	return nil
}

// ensureFirewallRule ensures that the required firewall rule exists for the given ports.
// Does nothing if it exists already, else creates a new one.
func (s *FirewallRuleSyncer) ensureFirewallRule(lbName string, ports []ingressbe.ServicePort, igLinks map[string][]string, forceUpdate bool) error {
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
		// Use the existing network.
		desiredFW.Network = existingFW.Network
		// Firewall rule with that name exists already. Check if it matches what we want.
		if firewallRuleMatches(desiredFW, existingFW) {
			// Nothing to do. Desired firewall rule exists already.
			fmt.Println("Desired firewall rule exists already.")
			return nil
		}
		if forceUpdate {
			return s.updateFirewallRule(desiredFW)
		} else {
			fmt.Println("Will not overwrite a differing firewall rule without the --force flag.")
			return fmt.Errorf("will not overwrite firewall rule without --force")
		}
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
		fmt.Println("Error updating firewall:", err)
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

// Note: mutates the existingFR by clearing fields we don't care about matching.
func firewallRuleMatches(desiredFR, existingFR *compute.Firewall) bool {
	// Also need to take special care of target tags. There can be multiple target tags, all of which can be "correct".

	// Clear output-only fields.
	existingFR.CreationTimestamp = ""
	existingFR.Id = 0
	existingFR.Kind = ""
	existingFR.SelfLink = ""
	// Check for and clear default Direction.
	// if existingFR.Direction == "INGRESS" {
	// 	existingFR.Direction = ""
	// }

	// It is fine if the Priority differs- The user may have legitimately changed it.
	existingFR.ServerResponse = googleapi.ServerResponse{}
	if existingFR.Priority == 1000 {
		existingFR.Priority = 0
	}

	glog.V(5).Infof("Desired firewall rule:\n%#v", desiredFR)
	glog.V(5).Infof("Existing firewall rule (ignoring some fields):\n%#v", existingFR)

	equal := reflect.DeepEqual(desiredFR, existingFR)
	if !equal {
		glog.V(5).Infof("%s", diff.ObjectDiff(desiredFR, existingFR))
	}
	return equal
}

func (s *FirewallRuleSyncer) desiredFirewallRule(lbName string, ports []ingressbe.ServicePort, igLinks map[string][]string) (*compute.Firewall, error) {
	// Compute the desired firewall rule.
	instances, err := s.getInstances(igLinks)
	if err != nil {
		return nil, err
	}
	targetTags, err := s.getTargetTags(instances)
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

	// We assume that all instances are in the same network, so we just fetch the network of the first instance.
	// TODO(nikhiljindal): Handle the case where different clusters in the same project are in different networks.
	network := s.getNetworkName(instances[0])

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
		Direction:  "INGRESS",
		Network:    network,
	}, nil
}

// Returns an array of instances, with an instance from each cluster.
func (s *FirewallRuleSyncer) getInstances(igLinks map[string][]string) ([]*compute.Instance, error) {
	var instances []*compute.Instance
	for _, v := range igLinks {
		// Return an instance from the first instance group of each cluster.
		// TODO(nikhiljindal): Make this more resilient. If we fail to fetch an instance from the first instance group, try the next group.
		instance, err := s.ig.GetInstance(v[0])
		if err != nil {
			return nil, err
		}
		instances = append(instances, instance)
	}
	return instances, nil
}

// getTargetTags returns the required network tags to target all instances.
// It assumes that the input contains an instance from each cluster.
func (s *FirewallRuleSyncer) getTargetTags(instances []*compute.Instance) ([]string, error) {
	// We assume that all instances in a cluster have the same target tags.
	// This is true for default GKE and GCE clusters (brought up using kube-up).
	// So we return the first target tag from an instance in each cluster.
	var tags []string
	for _, instance := range instances {
		items := instance.Tags.Items
		if len(items) == 0 {
			return nil, fmt.Errorf("no network tag found on instance %s/%s", instance.Zone, instance.Name)
		}
		tags = append(tags, items[0])
	}
	return tags, nil
}

// getNetworkName returns the full network URL of the given instance.
// Example network URL: https://www.googleapis.com/compute/v1/projects/myproject/global/networks/my-network
func (s *FirewallRuleSyncer) getNetworkName(instance *compute.Instance) string {
	if len(instance.NetworkInterfaces) > 0 {
		return instance.NetworkInterfaces[0].Network
	}
	return ""
}
