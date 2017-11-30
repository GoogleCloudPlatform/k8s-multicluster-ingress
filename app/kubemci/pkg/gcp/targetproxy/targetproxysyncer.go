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
	"reflect"

	"github.com/golang/glog"
	multierror "github.com/hashicorp/go-multierror"
	compute "google.golang.org/api/compute/v1"
	"google.golang.org/api/googleapi"
	"k8s.io/apimachinery/pkg/util/diff"
	ingresslb "k8s.io/ingress-gce/pkg/loadbalancers"

	utilsnamer "github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/namer"
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

// EnsureHttpTargetProxy ensures that the required http target proxy exists for the given url map.
// Does nothing if it exists already, else creates a new one.
func (s *TargetProxySyncer) EnsureHttpTargetProxy(lbName, umLink string, forceUpdate bool) (string, error) {
	fmt.Println("Ensuring http target proxy.")
	var err error
	tpLink, httpProxyErr := s.ensureHttpProxy(lbName, umLink, forceUpdate)
	if httpProxyErr != nil {
		httpProxyErr = fmt.Errorf("Error in ensuring http target proxy: %s", httpProxyErr)
		// Try ensuring both http and https target proxies and return all errors at once.
		err = multierror.Append(err, httpProxyErr)
	}
	return tpLink, err
}

func (s *TargetProxySyncer) DeleteTargetProxies() error {
	// TODO(nikhiljindal): Also delete the https target proxy when we start creating it.
	name := s.namer.TargetHttpProxyName()
	fmt.Println("Deleting target HTTP proxy", name)
	err := s.tpp.DeleteTargetHttpProxy(name)
	if err != nil {
		fmt.Println("error", err, "in deleting target HTTP proxy", name)
		return err
	}
	fmt.Println("target HTTP proxy", name, "deleted successfully")
	return nil
}

// ensureHttpProxy ensures that the required target proxy exists for the given port.
// Does nothing if it exists already, else creates a new one.
// Returns the self link for the ensured http proxy.
func (s *TargetProxySyncer) ensureHttpProxy(lbName, umLink string, forceUpdate bool) (string, error) {
	fmt.Println("Ensuring target http proxy. UrlMap:", umLink)
	desiredHttpProxy := s.desiredHttpTargetProxy(lbName, umLink)
	glog.Infof("desiredHttpTargetProxy:%+v", desiredHttpProxy)
	name := desiredHttpProxy.Name
	// Check if target proxy already exists.
	existingHttpProxy, err := s.tpp.GetTargetHttpProxy(name)
	if err == nil {
		fmt.Println("Target HTTP proxy", name, "exists already. Checking if it matches our desired target proxy")
		glog.V(5).Infof("Existing target HTTP proxy: %+v", existingHttpProxy)
		// Target proxy with that name exists already. Check if it matches what we want.
		if targetHttpProxyMatches(*desiredHttpProxy, *existingHttpProxy) {
			// Nothing to do. Desired target proxy exists already.
			fmt.Println("Desired target HTTP proxy", name, "exists already.")
			return existingHttpProxy.SelfLink, nil
		}
		if forceUpdate {
			fmt.Println("Updating existing target HTTP proxy", name, "to match the desired state")
			return s.updateHttpTargetProxy(desiredHttpProxy)
		} else {
			fmt.Println("Will not overwrite this differing Target HTTP Proxy without the --force flag")
			return "", fmt.Errorf("Will not overwrite Target HTTP Proxy without --force")
		}
	}
	glog.V(5).Infof("Got error %s while trying to get existing target HTTP proxy %s", err, name)
	// TODO(nikhiljindal): Handle non NotFound errors. We should create only if the error is NotFound.
	// Create the target proxy.
	fmt.Println("Creating target HTTP proxy", name)
	glog.V(5).Infof("Creating target HTTP proxy %v", *desiredHttpProxy)
	return s.createHttpTargetProxy(desiredHttpProxy)
}

func (s *TargetProxySyncer) updateHttpTargetProxy(desiredHttpProxy *compute.TargetHttpProxy) (string, error) {
	name := desiredHttpProxy.Name
	fmt.Println("Updating existing target http proxy", name, "to match the desired state")
	// There is no UpdateTargetHttpProxy method.
	// Apart from name, UrlMap is the only field that can be different. We update that field directly.
	urlMap := &compute.UrlMap{SelfLink: desiredHttpProxy.UrlMap}
	glog.Infof("Setting URL Map to:%+v", urlMap)
	err := s.tpp.SetUrlMapForTargetHttpProxy(desiredHttpProxy, urlMap)
	if err != nil {
		fmt.Println("Error setting URL Map:", err)
		return "", err
	}
	fmt.Println("Target http proxy", name, "updated successfully")
	existing, err := s.tpp.GetTargetHttpProxy(name)
	if err != nil {
		fmt.Println("Error getting target HTTP Proxy:", err)
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

func targetHttpProxyMatches(desiredHttpProxy, existingHttpProxy compute.TargetHttpProxy) bool {
	existingHttpProxy.CreationTimestamp = ""
	existingHttpProxy.Id = 0
	existingHttpProxy.Kind = ""
	existingHttpProxy.SelfLink = ""
	existingHttpProxy.ServerResponse = googleapi.ServerResponse{}

	equal := reflect.DeepEqual(existingHttpProxy, desiredHttpProxy)
	if !equal {
		glog.V(2).Infof("TargetHttpProxies differ.")
		glog.V(0).Infof("Diff: %v", diff.ObjectDiff(desiredHttpProxy, existingHttpProxy))
	} else {
		glog.V(2).Infof("TargetHttpProxies match.")
	}
	return equal
}

func (s *TargetProxySyncer) desiredHttpTargetProxy(lbName, umLink string) *compute.TargetHttpProxy {
	// Compute the desired target http proxy.
	return &compute.TargetHttpProxy{
		Name:        s.namer.TargetHttpProxyName(),
		Description: fmt.Sprintf("Target http proxy for kubernetes multicluster loadbalancer %s", lbName),
		UrlMap:      umLink,
	}
}
