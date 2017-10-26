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

import (
	"fmt"

	"github.com/golang/glog"
	"github.com/hashicorp/go-multierror"
	"google.golang.org/api/compute/v1"
	ingresslb "k8s.io/ingress-gce/pkg/loadbalancers"

	utilsnamer "github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/mci/pkg/gcp/namer"
)

type TargetProxySyncer struct {
	namer *utilsnamer.Namer
	// Target proxies provider to call GCP APIs to manipulate target proxies.
	tpp ingresslb.LoadBalancers
}

func NewTargetProxySyncer(namer *utilsnamer.Namer, tpp ingresslb.LoadBalancers) TargetProxySyncerInterface {
	return &TargetProxySyncer{
		namer: namer,
		tpp:   tpp,
	}
}

// Ensure this implements TargetProxySyncerInterface.
var _ TargetProxySyncerInterface = &TargetProxySyncer{}

// EnsureTargetProxy ensures that the required target proxies exist for the given url map.
// Does nothing if it exists already, else creates a new one.
func (s *TargetProxySyncer) EnsureTargetProxy(lbName, umLink string) (string, error) {
	fmt.Println("Ensuring target proxies")
	// TODO(nikhiljindal): Support creating https proxies.
	fmt.Println("Warning: We create http proxies only, even if https was requested.")
	var err error
	tpLink, httpProxyErr := s.ensureHttpProxy(lbName, umLink)
	if httpProxyErr != nil {
		httpProxyErr = fmt.Errorf("Error in ensuring http target proxy: %s", httpProxyErr)
		// Try ensuring both http and https target proxies and return all errors at once.
		err = multierror.Append(err, httpProxyErr)
	}
	return tpLink, err
}

// ensureHttpProxy ensures that the required target proxy exists for the given port.
// Does nothing if it exists already, else creates a new one.
// Returns the self link for the ensured http proxy.
func (s *TargetProxySyncer) ensureHttpProxy(lbName, umLink string) (string, error) {
	fmt.Println("Ensuring target http proxy")
	desiredHttpProxy := s.desiredHttpTargetProxy(lbName, umLink)
	name := desiredHttpProxy.Name
	// Check if target proxy already exists.
	existingHttpProxy, err := s.tpp.GetTargetHttpProxy(name)
	if err == nil {
		fmt.Println("Target proxy", name, "exists already. Checking if it matches our desired target proxy")
		glog.V(5).Infof("Existing target proxy: %v\n, desired target proxy: %v", existingHttpProxy, *desiredHttpProxy)
		// Target proxy with that name exists already. Check if it matches what we want.
		if targetHttpProxyMatches(desiredHttpProxy, existingHttpProxy) {
			// Nothing to do. Desired target proxy exists already.
			fmt.Println("Desired target proxy exists already")
			return existingHttpProxy.SelfLink, nil
		}
		// TODO (nikhiljindal): Require explicit permission from user before doing this.
		fmt.Println("Updating existing target proxy", name, "to match the desired state")
		return s.updateHttpTargetProxy(desiredHttpProxy)
	}
	glog.V(5).Infof("Got error %s while trying to get existing target proxy %s", err, name)
	// TODO(nikhiljindal): Handle non NotFound errors. We should create only if the error is NotFound.
	// Create the target proxy.
	fmt.Println("Creating target proxy", name)
	glog.V(5).Infof("Creating target proxy %v", *desiredHttpProxy)
	return s.createHttpTargetProxy(desiredHttpProxy)
}

func (s *TargetProxySyncer) updateHttpTargetProxy(desiredHttpProxy *compute.TargetHttpProxy) (string, error) {
	name := desiredHttpProxy.Name
	fmt.Println("Updating existing target http proxy", name, "to match the desired state")
	// There is no UpdateTargetHttpProxy method.
	// Apart from name, UrlMap is the only field that can be different. We update that field directly.
	err := s.tpp.SetUrlMapForTargetHttpProxy(desiredHttpProxy, &compute.UrlMap{SelfLink: desiredHttpProxy.UrlMap})
	if err != nil {
		return "", err
	}
	fmt.Println("Target http proxy", name, "updated successfully")
	existing, err := s.tpp.GetTargetHttpProxy(name)
	if err != nil {
		return "", err
	}
	return existing.SelfLink, nil
}

func (s *TargetProxySyncer) createHttpTargetProxy(desiredHttpProxy *compute.TargetHttpProxy) (string, error) {
	name := desiredHttpProxy.Name
	fmt.Println("Creating target http proxy", name)
	glog.V(5).Infof("Creating target http proxy %v", desiredHttpProxy)
	err := s.tpp.CreateTargetHttpProxy(desiredHttpProxy)
	if err != nil {
		return "", err
	}
	fmt.Println("Target http proxy", name, "created successfully")
	existing, err := s.tpp.GetTargetHttpProxy(name)
	if err != nil {
		return "", err
	}
	return existing.SelfLink, nil
}

func targetHttpProxyMatches(desiredHttpProxy, existingHttpProxy *compute.TargetHttpProxy) bool {
	// TODO(nikhiljindal): Add proper logic to figure out if the 2 target proxies match.
	// Need to add the --force flag for user to consent overritting before this method can be updated to return false.
	return true
}

func (s *TargetProxySyncer) desiredHttpTargetProxy(lbName, umLink string) *compute.TargetHttpProxy {
	// Compute the desired target http proxy.
	return &compute.TargetHttpProxy{
		Name:        s.namer.TargetHttpProxyName(),
		Description: fmt.Sprintf("Target http proxy for kubernetes multicluster loadbalancer %s", lbName),
		UrlMap:      umLink,
	}
}
