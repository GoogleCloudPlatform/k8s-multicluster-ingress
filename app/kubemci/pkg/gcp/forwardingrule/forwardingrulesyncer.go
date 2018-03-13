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
	"net/http"
	"reflect"
	"sort"
	"strings"

	compute "google.golang.org/api/compute/v1"
	"google.golang.org/api/googleapi"

	"github.com/golang/glog"
	multierror "github.com/hashicorp/go-multierror"
	"k8s.io/apimachinery/pkg/util/diff"
	ingresslb "k8s.io/ingress-gce/pkg/loadbalancers"
	"k8s.io/ingress-gce/pkg/utils"

	utilsnamer "github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/namer"
	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/status"
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
// Stores the given list of clusters in the description field of forwarding rule to use it to generate status later.
func (s *ForwardingRuleSyncer) EnsureHttpForwardingRule(lbName, ipAddress, targetProxyLink string, clusters []string, forceUpdate bool) error {
	s.namer.HttpForwardingRuleName()
	return s.ensureForwardingRule(lbName, ipAddress, targetProxyLink, httpDefaultPortRange, "http", s.namer.HttpForwardingRuleName(), clusters, forceUpdate)
}

// EnsureHttpsForwardingRule ensures that the required https forwarding rule exists.
// Does nothing if it exists already, else creates a new one.
// Stores the given list of clusters in the description field of forwarding rule to use it to generate status later.
func (s *ForwardingRuleSyncer) EnsureHttpsForwardingRule(lbName, ipAddress, targetProxyLink string, clusters []string, forceUpdate bool) error {
	return s.ensureForwardingRule(lbName, ipAddress, targetProxyLink, httpsDefaultPortRange, "https", s.namer.HttpsForwardingRuleName(), clusters, forceUpdate)
}

// ensureForwardingRule ensures a forwarding rule exists as per the given input parameters.
func (s *ForwardingRuleSyncer) ensureForwardingRule(lbName, ipAddress, targetProxyLink, portRange, httpProtocol, name string, clusters []string, forceUpdate bool) error {
	fmt.Println("Ensuring", httpProtocol, "forwarding rule")
	desiredFR, err := s.desiredForwardingRule(lbName, ipAddress, targetProxyLink, portRange, httpProtocol, name, clusters)
	if err != nil {
		fmt.Println("Error getting desired forwarding rule:", err)
		return err
	}
	// Check if forwarding rule already exists.
	existingFR, err := s.frp.GetGlobalForwardingRule(name)
	if err == nil {
		fmt.Println("forwarding rule", name, "exists already. Checking if it matches our desired forwarding rule", name)
		glog.V(5).Infof("Existing forwarding rule:\n%+v\nDesired forwarding rule:\n%+v", existingFR, desiredFR)
		// Forwarding Rule with that name exists already. Check if it matches what we want.
		if forwardingRuleMatches(desiredFR, existingFR) {
			// Nothing to do. Desired forwarding rule exists already.
			fmt.Println("Desired forwarding rule exists already")
			return nil
		}
		if forceUpdate {
			return s.updateForwardingRule(existingFR, desiredFR)
		} else {
			fmt.Println("Will not overwrite this differing Forwarding Rule without the --force flag")
			return fmt.Errorf("Will not overwrite Forwarding Rule without --force")
		}
	}
	glog.V(2).Infof("Got error %s while trying to get existing forwarding rule %s. Will try to create new one", err, name)
	// TODO: Handle non NotFound errors. We should create only if the error is NotFound.
	// Create the forwarding rule.
	return s.createForwardingRule(desiredFR)
}

func (s *ForwardingRuleSyncer) DeleteForwardingRules() error {
	var err error
	name := s.namer.HttpForwardingRuleName()
	fmt.Println("Deleting HTTP forwarding rule", name)
	httpErr := s.frp.DeleteGlobalForwardingRule(name)
	if httpErr != nil {
		if utils.IsHTTPErrorCode(httpErr, http.StatusNotFound) {
			fmt.Println("HTTP forwarding rule", name, "does not exist. Nothing to delete")
		} else {
			httpErr := fmt.Errorf("error %s in deleting HTTP forwarding rule %s", httpErr, name)
			fmt.Println(httpErr)
			err = multierror.Append(err, httpErr)
		}
	} else {
		fmt.Println("HTTP forwarding rule", name, "deleted successfully")
	}
	name = s.namer.HttpsForwardingRuleName()
	fmt.Println("Deleting HTTPS forwarding rule", name)
	httpsErr := s.frp.DeleteGlobalForwardingRule(name)
	if httpsErr != nil {
		if utils.IsHTTPErrorCode(httpsErr, http.StatusNotFound) {
			fmt.Println("HTTPS forwarding rule", name, "does not exist. Nothing to delete")
		} else {
			httpsErr := fmt.Errorf("error %s in deleting HTTPS forwarding rule %s", httpsErr, name)
			fmt.Println(httpsErr)
			err = multierror.Append(err, httpsErr)
		}
	} else {
		fmt.Println("HTTPS forwarding rule", name, "deleted successfully")
	}
	return err
}

func (s *ForwardingRuleSyncer) GetLoadBalancerStatus(lbName string) (*status.LoadBalancerStatus, error) {
	// Fetch the http forwarding rule.
	httpName := s.namer.HttpForwardingRuleName()
	httpFr, httpErr := s.frp.GetGlobalForwardingRule(httpName)
	if httpErr == nil {
		return getStatus(httpFr)
	}
	// Try fetching https forwarding rule.
	// Ingresses with http-only annotation will not have a http forwarding rule.
	httpsName := s.namer.HttpsForwardingRuleName()
	httpsFr, httpsErr := s.frp.GetGlobalForwardingRule(httpsName)
	if httpsErr == nil {
		return getStatus(httpsFr)
	}
	if utils.IsHTTPErrorCode(httpErr, http.StatusNotFound) && utils.IsHTTPErrorCode(httpsErr, http.StatusNotFound) {
		// We assume the load balancer does not exist until the forwarding rule exists.
		return nil, fmt.Errorf("Load balancer %s does not exist", lbName)
	}
	return nil, fmt.Errorf("error in fetching http forwarding rule: %s, error in fetching https forwarding rule: %s. Cannot determine status without forwarding rule", httpErr, httpsErr)
}

func getStatus(fr *compute.ForwardingRule) (*status.LoadBalancerStatus, error) {
	status, err := status.FromString(fr.Description)
	if err != nil {
		return nil, fmt.Errorf("error in parsing forwarding rule description %s. Cannot determine status without it.", err)
	}
	return status, nil
}

func (s *ForwardingRuleSyncer) ListLoadBalancerStatuses() ([]status.LoadBalancerStatus, error) {
	var rules *compute.ForwardingRuleList
	var err error
	result := []status.LoadBalancerStatus{}
	if rules, err = s.frp.ListGlobalForwardingRules(); err != nil {
		err = fmt.Errorf("Error getting global forwarding rules: %s", err)
		fmt.Println(err)
		return result, err
	}
	if rules == nil {
		// This is being defensive.
		err = fmt.Errorf("Unexpected nil from ListGlobalForwardingRules.")
		fmt.Println(err)
		return result, err
	}
	glog.V(5).Infof("rules: %+v", rules)
	lbsSeen := map[string]bool{}
	for _, item := range rules.Items {
		if strings.HasPrefix(item.Name, "mci1") {
			if lbStatus, decodeErr := status.FromString(item.Description); decodeErr != nil {
				decodeErr = fmt.Errorf("Error decoding load balancer status: %s", decodeErr)
				fmt.Println(decodeErr)
				err = multierror.Append(err, decodeErr)
			} else {
				if lbsSeen[lbStatus.LoadBalancerName] {
					// Single load balancer can have multiple forwarding rules
					// (for ex: one for http and one for https).
					// Skip the load balancer to not list it multiple times.
					continue
				}
				lbsSeen[lbStatus.LoadBalancerName] = true
				result = append(result, *lbStatus)
			}
		}
	}
	return result, err
}

// See interface for comment.
func (s *ForwardingRuleSyncer) RemoveClustersFromStatus(clusters []string) error {
	// Update HTTPS status first and then HTTP.
	// This ensures that get-status returns the correct status even if HTTPS is updated but updating HTTP fails.
	// This is because get-status reads from HTTP if it exists.
	// Also note that it continues silently if either HTTP or HTTPS forwarding rules do not exist.
	if err := s.removeClustersFromStatus("https", s.namer.HttpsForwardingRuleName(), clusters); err != nil {
		return fmt.Errorf("unexpected error in updating the https forwarding rule status: %s", err)
	}
	return s.removeClustersFromStatus("http", s.namer.HttpForwardingRuleName(), clusters)
}

// ensureForwardingRule ensures a forwarding rule exists as per the given input parameters.
func (s *ForwardingRuleSyncer) removeClustersFromStatus(httpProtocol, name string, clusters []string) error {
	fmt.Println("Removing clusters", clusters, "from", httpProtocol, "forwarding rule")
	existingFR, err := s.frp.GetGlobalForwardingRule(name)
	if err != nil {
		// Ignore NotFound error.
		if utils.IsHTTPErrorCode(err, http.StatusNotFound) {
			fmt.Println(httpProtocol, "forwarding rule not found. Nothing to update. Continuing")
			return nil
		}
		err := fmt.Errorf("error in fetching existing forwarding rule %s: %s", name, err)
		fmt.Println(err)
		return err
	}
	// Remove clusters from the forwardingrule
	desiredFR, err := s.desiredForwardingRuleWithoutClusters(existingFR, clusters)
	if err != nil {
		fmt.Println("Error getting desired forwarding rule:", err)
		return err
	}
	glog.V(5).Infof("Existing forwarding rule: %v\n, desired forwarding rule: %v\n", *existingFR, *desiredFR)
	return s.updateForwardingRule(existingFR, desiredFR)
}

func (s *ForwardingRuleSyncer) updateForwardingRule(existingFR, desiredFR *compute.ForwardingRule) error {
	name := desiredFR.Name
	fmt.Println("Updating existing forwarding rule", name, "to match the desired state")
	// We do not have an UpdateForwardingRule method.
	// If target proxy link is the only thing that is different, then we can call SetProxyForGlobalForwardingRule.
	// Else, we need to delete the existing rule and create a new one.
	if existingFR.IPAddress != desiredFR.IPAddress || existingFR.PortRange != desiredFR.PortRange ||
		existingFR.IPProtocol != desiredFR.IPProtocol || existingFR.Description != desiredFR.Description {
		fmt.Println("Deleting the existing forwarding rule", name, "and will create a new one")
		if err := utils.IgnoreHTTPNotFound(s.frp.DeleteGlobalForwardingRule(name)); err != nil {
			fmt.Println("Error deleting global forwarding rule:", err)
			return fmt.Errorf("error in deleting existing forwarding rule %s: %s", name, err)
		}
		return s.createForwardingRule(desiredFR)
	}
	// Update target proxy link in forwarding rule.
	err := s.frp.SetProxyForGlobalForwardingRule(name, desiredFR.Target)
	if err != nil {
		fmt.Println("Error setting proxy for forwarding rule. Target:", desiredFR.Target, "Error:", err)
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
	existingFR.CreationTimestamp = ""
	existingFR.Id = 0
	existingFR.Kind = ""
	existingFR.SelfLink = ""
	existingFR.ServerResponse = googleapi.ServerResponse{}

	equal := reflect.DeepEqual(existingFR, desiredFR)
	if !equal {
		glog.V(5).Infof("Forwarding Rules differ:\n%v", diff.ObjectDiff(desiredFR, existingFR))
	} else {
		glog.V(2).Infof("Rules match.")
	}

	return equal
}

func (s *ForwardingRuleSyncer) desiredForwardingRule(lbName, ipAddress, targetProxyLink, portRange, httpProtocol, name string, clusters []string) (*compute.ForwardingRule, error) {
	// Sort the clusters so we get a deterministic order.
	sort.Strings(clusters)
	status := status.LoadBalancerStatus{
		Description:      fmt.Sprintf("%s forwarding rule for kubernetes multicluster loadbalancer %s", httpProtocol, lbName),
		LoadBalancerName: lbName,
		Clusters:         clusters,
		IPAddress:        ipAddress,
	}
	desc, err := status.ToString()
	if err != nil {
		return nil, fmt.Errorf("unexpected error in generating the description for forwarding rule: %s", err)
	}
	// Compute the desired forwarding rule.
	return &compute.ForwardingRule{
		Name:                name,
		Description:         desc,
		IPAddress:           ipAddress,
		Target:              targetProxyLink,
		PortRange:           portRange,
		IPProtocol:          "TCP",
		LoadBalancingScheme: "EXTERNAL",
	}, nil
}

func (s *ForwardingRuleSyncer) desiredForwardingRuleWithoutClusters(existingFR *compute.ForwardingRule, removeClusters []string) (*compute.ForwardingRule, error) {
	statusStr := existingFR.Description
	status, err := status.FromString(statusStr)
	if err != nil {
		return nil, fmt.Errorf("error in parsing string %s: %s", statusStr, err)
	}
	removeMap := map[string]bool{}
	for _, v := range removeClusters {
		removeMap[v] = true
	}
	var newClusters []string
	for _, v := range status.Clusters {
		if !removeMap[v] {
			newClusters = append(newClusters, v)
		}
	}
	status.Clusters = newClusters
	newDesc, err := status.ToString()
	if err != nil {
		return nil, fmt.Errorf("unexpected error in generating the description for forwarding rule: %s", err)
	}
	desiredFR := existingFR
	desiredFR.Description = newDesc
	return desiredFR, nil
}
