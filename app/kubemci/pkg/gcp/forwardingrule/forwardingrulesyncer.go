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

// Syncer manages GCP forwarding rules for multicluster GCP L7 load balancers.
type Syncer struct {
	namer *utilsnamer.Namer
	// Instance of ForwardingRuleProvider interface for calling GCE ForwardingRule APIs.
	// There is no separate ForwardingRuleProvider interface, so we use the bigger LoadBalancers interface here.
	frp ingresslb.LoadBalancers
}

// NewForwardingRuleSyncer returns a new instance of syncer.
func NewForwardingRuleSyncer(namer *utilsnamer.Namer, frp ingresslb.LoadBalancers) SyncerInterface {
	return &Syncer{
		namer: namer,
		frp:   frp,
	}
}

// Ensure this implements SyncerInterface.
var _ SyncerInterface = &Syncer{}

// EnsureHTTPForwardingRule ensures that the required http forwarding rule exists.
// Does nothing if it exists already, else creates a new one.
// See interface for more details.
func (s *Syncer) EnsureHTTPForwardingRule(lbName, ipAddress, targetProxyLink string, forceUpdate bool) error {
	s.namer.HttpForwardingRuleName()
	return s.ensureForwardingRule(lbName, ipAddress, targetProxyLink, httpDefaultPortRange, "http", s.namer.HttpForwardingRuleName(), forceUpdate)
}

// EnsureHTTPSForwardingRule ensures that the required https forwarding rule exists.
// Does nothing if it exists already, else creates a new one.
// See interface for more details.
func (s *Syncer) EnsureHTTPSForwardingRule(lbName, ipAddress, targetProxyLink string, forceUpdate bool) error {
	return s.ensureForwardingRule(lbName, ipAddress, targetProxyLink, httpsDefaultPortRange, "https", s.namer.HttpsForwardingRuleName(), forceUpdate)
}

// ensureForwardingRule ensures a forwarding rule exists as per the given input parameters.
func (s *Syncer) ensureForwardingRule(lbName, ipAddress, targetProxyLink, portRange, httpProtocol, name string, forceUpdate bool) error {
	fmt.Println("Ensuring", httpProtocol, "forwarding rule")
	desiredFR, err := s.desiredForwardingRule(lbName, ipAddress, targetProxyLink, portRange, httpProtocol, name)
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
		}
		fmt.Println("Will not overwrite this differing Forwarding Rule without the --force flag")
		return fmt.Errorf("Will not overwrite Forwarding Rule without --force")
	}
	glog.V(2).Infof("Got error %s while trying to get existing forwarding rule %s. Will try to create new one", err, name)
	// TODO: Handle non NotFound errors. We should create only if the error is NotFound.
	// Create the forwarding rule.
	return s.createForwardingRule(desiredFR)
}

// DeleteForwardingRules deletes the forwarding rules that EnsureHTTPSForwardingRule and EnsureHTTPForwardingRule would have created.
// See interface for more details.
func (s *Syncer) DeleteForwardingRules() error {
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

// GetLoadBalancerStatus returns the status for the given load balancer.
// See interface for more details.
func (s *Syncer) GetLoadBalancerStatus(lbName string) (*status.LoadBalancerStatus, error) {
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
		// Preserve StatusNotFound and return the error as is.
		return nil, httpErr
	}
	return nil, fmt.Errorf("error in fetching http forwarding rule: %s, error in fetching https forwarding rule: %s. Cannot determine status without forwarding rule", httpErr, httpsErr)
}

func getStatus(fr *compute.ForwardingRule) (*status.LoadBalancerStatus, error) {
	status, err := status.FromString(fr.Description)
	if err != nil {
		return nil, fmt.Errorf("error in parsing forwarding rule description %s. Cannot determine status without it", err)
	}
	return status, nil
}

// ListLoadBalancerStatuses returns a list of load balancer status from load balancers that have the status stored on their forwarding rules.
// It ignores the load balancers that dont have status on their forwarding rule.
// Returns an error if listing forwarding rules fails.
func (s *Syncer) ListLoadBalancerStatuses() ([]status.LoadBalancerStatus, error) {
	var rules []*compute.ForwardingRule
	var err error
	result := []status.LoadBalancerStatus{}
	if rules, err = s.frp.ListGlobalForwardingRules(); err != nil {
		err = fmt.Errorf("Error getting global forwarding rules: %s", err)
		fmt.Println(err)
		return result, err
	}
	if rules == nil {
		// This is being defensive.
		err = fmt.Errorf("Unexpected nil from ListGlobalForwardingRules")
		fmt.Println(err)
		return result, err
	}
	glog.V(5).Infof("rules: %+v", rules)
	lbsSeen := map[string]bool{}
	for _, item := range rules {
		if strings.HasPrefix(item.Name, "mci1") {
			if lbStatus, decodeErr := status.FromString(item.Description); decodeErr != nil {
				// Assume that url map has the right status for this MCI.
				glog.V(3).Infof("Error decoding load balancer status on forwarding rule %s: %s\nAssuming status is stored on urlmap. Ignoring the error and continuing.", item.Name, decodeErr)
				continue
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

// RemoveClustersFromStatus removes the given clusters from the forwarding rule.
// See interface for more details.
func (s *Syncer) RemoveClustersFromStatus(clusters []string) error {
	httpsErr := s.removeClustersFromStatus("https", s.namer.HttpsForwardingRuleName(), clusters)
	httpErr := s.removeClustersFromStatus("http", s.namer.HttpForwardingRuleName(), clusters)
	// If both are not found, then return that.
	if utils.IsHTTPErrorCode(httpErr, http.StatusNotFound) && utils.IsHTTPErrorCode(httpsErr, http.StatusNotFound) {
		return httpErr
	}
	if canIgnoreStatusUpdateError(httpErr) && canIgnoreStatusUpdateError(httpsErr) {
		return nil
	}
	if canIgnoreStatusUpdateError(httpsErr) {
		return httpErr
	}
	if canIgnoreStatusUpdateError(httpErr) {
		return httpsErr
	}
	return fmt.Errorf("errors in updating both http and https forwarding rules, http rule error: %s, https rule error: %s", httpErr, httpsErr)
}

// canIgnoreStatusUpdateError returns true if and only if either of the following is true:
// * err is nil
// * err has http.StatusNotFound code
func canIgnoreStatusUpdateError(err error) bool {
	return (err == nil || utils.IsHTTPErrorCode(err, http.StatusNotFound))
}

// removeClustersFromStatus removes the given list of clusters from the existing forwarding rule with the given name.
func (s *Syncer) removeClustersFromStatus(httpProtocol, name string, clusters []string) error {
	fmt.Println("Removing clusters", clusters, "from", httpProtocol, "forwarding rule")
	existingFR, err := s.frp.GetGlobalForwardingRule(name)
	if err != nil {
		if utils.IsHTTPErrorCode(err, http.StatusNotFound) {
			// Return this error as is.
			return err
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

func (s *Syncer) updateForwardingRule(existingFR, desiredFR *compute.ForwardingRule) error {
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

func (s *Syncer) createForwardingRule(desiredFR *compute.ForwardingRule) error {
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

func (s *Syncer) desiredForwardingRule(lbName, ipAddress, targetProxyLink, portRange, httpProtocol, name string) (*compute.ForwardingRule, error) {
	// Compute the desired forwarding rule.
	return &compute.ForwardingRule{
		Name:                name,
		Description:         fmt.Sprintf("%s forwarding rule for kubernetes multicluster loadbalancer %s", httpProtocol, lbName),
		IPAddress:           ipAddress,
		Target:              targetProxyLink,
		PortRange:           portRange,
		IPProtocol:          "TCP",
		LoadBalancingScheme: "EXTERNAL",
	}, nil
}

func (s *Syncer) desiredForwardingRuleWithoutClusters(existingFR *compute.ForwardingRule, clustersToRemove []string) (*compute.ForwardingRule, error) {
	existingStatusStr := existingFR.Description
	newStatusStr, err := status.RemoveClusters(existingStatusStr, clustersToRemove)
	if err != nil {
		return nil, fmt.Errorf("unexpected error in updating status to remove clusters on forwarding rule %s: %s", existingFR.Name, err)
	}
	// Shallow copy is fine since we are only changing description.
	desiredFR := existingFR
	desiredFR.Description = newStatusStr
	return desiredFR, nil
}
