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
	"net/http"
	"reflect"

	"github.com/golang/glog"
	multierror "github.com/hashicorp/go-multierror"
	compute "google.golang.org/api/compute/v1"
	"google.golang.org/api/googleapi"
	"k8s.io/apimachinery/pkg/util/diff"
	ingresslb "k8s.io/ingress-gce/pkg/loadbalancers"
	"k8s.io/ingress-gce/pkg/utils"

	utilsnamer "github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/namer"
)

// Syncer manages GCP target proxies for multicluster GCP L7 load balancers.
type Syncer struct {
	namer *utilsnamer.Namer
	// Target proxies provider to call GCP APIs to manipulate target proxies.
	tpp ingresslb.LoadBalancers
}

// NewTargetProxySyncer returns a new instance of syncer.
func NewTargetProxySyncer(namer *utilsnamer.Namer, tpp ingresslb.LoadBalancers) SyncerInterface {
	return &Syncer{
		namer: namer,
		tpp:   tpp,
	}
}

// Ensure this implements SyncerInterface.
var _ SyncerInterface = &Syncer{}

// EnsureHTTPTargetProxy ensures that the required http target proxy exists for the given url map.
// Does nothing if it exists already, else creates a new one.
func (s *Syncer) EnsureHTTPTargetProxy(lbName, umLink string, forceUpdate bool) (string, error) {
	fmt.Println("Ensuring http target proxy.")
	var err error
	tpLink, httpProxyErr := s.ensureHTTPProxy(lbName, umLink, forceUpdate)
	if httpProxyErr != nil {
		httpProxyErr = fmt.Errorf("Error in ensuring http target proxy: %s", httpProxyErr)
		err = multierror.Append(err, httpProxyErr)
	}
	return tpLink, err
}

// EnsureHTTPSTargetProxy ensures that the required https target proxy exists for the given url map.
// Does nothing if it exists already, else creates a new one.
func (s *Syncer) EnsureHTTPSTargetProxy(lbName, umLink, certLink string, forceUpdate bool) (string, error) {
	fmt.Println("Ensuring http target proxy.")
	var err error
	tpLink, proxyErr := s.ensureHTTPSProxy(lbName, umLink, certLink, forceUpdate)
	if proxyErr != nil {
		proxyErr = fmt.Errorf("Error in ensuring https target proxy: %s", proxyErr)
		err = multierror.Append(err, proxyErr)
	}
	return tpLink, err
}

// DeleteTargetProxies deletes the target proxies that EnsureHTTPTargetProxy and EnsureHTTPSTargetProxy would have created.
// See interface comments for details.
func (s *Syncer) DeleteTargetProxies() error {
	var err error
	httpName := s.namer.TargetHttpProxyName()
	fmt.Println("Deleting target HTTP proxy", httpName)
	httpErr := s.tpp.DeleteTargetHttpProxy(httpName)
	if httpErr != nil {
		if utils.IsHTTPErrorCode(httpErr, http.StatusNotFound) {
			fmt.Println("Target HTTP proxy", httpName, "does not exist. Nothing to delete")
		} else {
			httpErr = fmt.Errorf("error in deleting target HTTP proxy %s: %s", httpName, httpErr)
			fmt.Println(httpErr)
			err = multierror.Append(err, httpErr)
		}
	} else {
		fmt.Println("Target HTTP proxy", httpName, "deleted successfully")
	}

	httpsName := s.namer.TargetHttpsProxyName()
	fmt.Println("Deleting target HTTPS proxy", httpsName)
	httpsErr := s.tpp.DeleteTargetHttpsProxy(httpsName)
	if httpsErr != nil {
		if utils.IsHTTPErrorCode(httpsErr, http.StatusNotFound) {
			fmt.Println("Target HTTPS proxy", httpsName, "does not exist. Nothing to delete")
		} else {
			httpsErr = fmt.Errorf("error in deleting target HTTPS proxy %s: %s", httpsName, httpsErr)
			fmt.Println(httpsErr)
			err = multierror.Append(err, httpsErr)
		}
	} else {
		fmt.Println("Target HTTPS proxy", httpsName, "deleted successfully")
	}
	return err
}

// ensureHTTPProxy ensures that the required target proxy exists for the given port.
// Does nothing if it exists already, else creates a new one.
// Returns the self link for the ensured http proxy.
func (s *Syncer) ensureHTTPProxy(lbName, umLink string, forceUpdate bool) (string, error) {
	fmt.Println("Ensuring target http proxy. UrlMap:", umLink)
	desiredHTTPProxy := s.desiredHTTPTargetProxy(lbName, umLink)
	glog.Infof("desiredHTTPTargetProxy:%+v", desiredHTTPProxy)
	name := desiredHTTPProxy.Name
	// Check if target proxy already exists.
	existingHTTPProxy, err := s.tpp.GetTargetHttpProxy(name)
	if err == nil {
		fmt.Println("Target HTTP proxy", name, "exists already. Checking if it matches our desired target proxy")
		glog.V(5).Infof("Existing target HTTP proxy: %+v", existingHTTPProxy)
		// Target proxy with that name exists already. Check if it matches what we want.
		if targetHTTPProxyMatches(*desiredHTTPProxy, *existingHTTPProxy) {
			// Nothing to do. Desired target proxy exists already.
			fmt.Println("Desired target HTTP proxy", name, "exists already.")
			return existingHTTPProxy.SelfLink, nil
		}
		if forceUpdate {
			return s.updateHTTPTargetProxy(desiredHTTPProxy)
		}
		fmt.Println("Will not overwrite this differing Target HTTP Proxy without the --force flag")
		return "", fmt.Errorf("Will not overwrite Target HTTP Proxy without --force")
	}
	glog.V(5).Infof("Got error %s while trying to get existing target HTTP proxy %s", err, name)
	// TODO(nikhiljindal): Handle non NotFound errors. We should create only if the error is NotFound.
	// Create the target proxy.
	fmt.Println("Creating target HTTP proxy", name)
	glog.V(5).Infof("Creating target HTTP proxy %v", *desiredHTTPProxy)
	return s.createHTTPTargetProxy(desiredHTTPProxy)
}

func (s *Syncer) updateHTTPTargetProxy(desiredHTTPProxy *compute.TargetHttpProxy) (string, error) {
	name := desiredHTTPProxy.Name
	fmt.Println("Updating existing target http proxy", name, "to match the desired state")
	// There is no UpdateTargetHTTPProxy method.
	// Apart from name, UrlMap is the only field that can be different. We update that field directly.
	// TODO(nikhiljindal): Handle description field differences:
	// https://github.com/GoogleCloudPlatform/k8s-multicluster-ingress/issues/94.
	urlMap := &compute.UrlMap{SelfLink: desiredHTTPProxy.UrlMap}
	glog.Infof("Setting URL Map to:%+v", urlMap)
	err := s.tpp.SetUrlMapForTargetHttpProxy(desiredHTTPProxy, urlMap)
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

func (s *Syncer) createHTTPTargetProxy(desiredHTTPProxy *compute.TargetHttpProxy) (string, error) {
	name := desiredHTTPProxy.Name
	fmt.Println("Creating target http proxy", name)
	glog.V(5).Infof("Creating target http proxy %v", desiredHTTPProxy)
	err := s.tpp.CreateTargetHttpProxy(desiredHTTPProxy)
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

func targetHTTPProxyMatches(desiredHTTPProxy, existingHTTPProxy compute.TargetHttpProxy) bool {
	existingHTTPProxy.CreationTimestamp = ""
	existingHTTPProxy.Id = 0
	existingHTTPProxy.Kind = ""
	existingHTTPProxy.SelfLink = ""
	existingHTTPProxy.ServerResponse = googleapi.ServerResponse{}

	equal := reflect.DeepEqual(existingHTTPProxy, desiredHTTPProxy)
	if !equal {
		glog.V(2).Infof("TargetHTTPProxies differ.")
		glog.V(0).Infof("Diff: %v", diff.ObjectDiff(desiredHTTPProxy, existingHTTPProxy))
	} else {
		glog.V(2).Infof("TargetHTTPProxies match.")
	}
	return equal
}

func (s *Syncer) desiredHTTPTargetProxy(lbName, umLink string) *compute.TargetHttpProxy {
	// Compute the desired target http proxy.
	return &compute.TargetHttpProxy{
		Name:        s.namer.TargetHttpProxyName(),
		Description: fmt.Sprintf("Target http proxy for kubernetes multicluster loadbalancer %s", lbName),
		UrlMap:      umLink,
	}
}

// ensureHTTPSProxy ensures that the required target proxy exists for the given port.
// Does nothing if it exists already, else creates a new one.
// Returns the self link for the ensured https proxy.
func (s *Syncer) ensureHTTPSProxy(lbName, umLink, certLink string, forceUpdate bool) (string, error) {
	fmt.Println("Ensuring target https proxy")
	desiredHTTPSProxy := s.desiredHTTPSTargetProxy(lbName, umLink, certLink)
	name := desiredHTTPSProxy.Name
	// Check if target proxy already exists.
	existingHTTPSProxy, err := s.tpp.GetTargetHttpsProxy(name)
	if err == nil {
		fmt.Println("Target HTTPS proxy", name, "exists already. Checking if it matches our desired target proxy")
		glog.V(5).Infof("Existing target HTTPS proxy: %+v", existingHTTPSProxy)
		// Target proxy with that name exists already. Check if it matches what we want.
		if targetHTTPSProxyMatches(*desiredHTTPSProxy, *existingHTTPSProxy) {
			// Nothing to do. Desired target proxy exists already.
			fmt.Println("Desired target HTTPS proxy", name, "exists already.")
			return existingHTTPSProxy.SelfLink, nil
		}
		if forceUpdate {
			fmt.Println("Updating existing target HTTPS proxy", name, "to match the desired state")
			return s.updateHTTPSTargetProxy(desiredHTTPSProxy)
		}
		fmt.Println("Will not overwrite this differing Target HTTPS Proxy without the --force flag")
		return "", fmt.Errorf("Will not overwrite Target HTTPS Proxy without --force")
	}
	glog.V(5).Infof("Got error %s while trying to get existing target HTTPS proxy %s", err, name)
	// TODO(nikhiljindal): Handle non NotFound errors. We should create only if the error is NotFound.
	// Create the target proxy.
	fmt.Println("Creating target HTTPS proxy", name)
	glog.V(5).Infof("Creating target HTTPS proxy %v", *desiredHTTPSProxy)
	return s.createHTTPSTargetProxy(desiredHTTPSProxy)
}

func (s *Syncer) updateHTTPSTargetProxy(desiredHTTPSProxy *compute.TargetHttpsProxy) (string, error) {
	name := desiredHTTPSProxy.Name
	fmt.Println("Updating existing target https proxy", name, "to match the desired state")
	// There is no UpdateTargetHTTPSProxy method.
	// Apart from name, UrlMap is the only field that can be different. We update that field directly.
	// TODO(nikhiljindal): Handle description field differences:
	// https://github.com/GoogleCloudPlatform/k8s-multicluster-ingress/issues/94.
	err := s.tpp.SetUrlMapForTargetHttpsProxy(desiredHTTPSProxy, &compute.UrlMap{SelfLink: desiredHTTPSProxy.UrlMap})
	if err != nil {
		return "", err
	}
	fmt.Println("Target https proxy", name, "updated successfully")
	existing, err := s.tpp.GetTargetHttpsProxy(name)
	if err != nil {
		return "", err
	}
	return existing.SelfLink, nil
}

func (s *Syncer) createHTTPSTargetProxy(desiredHTTPSProxy *compute.TargetHttpsProxy) (string, error) {
	name := desiredHTTPSProxy.Name
	fmt.Println("Creating target https proxy", name)
	glog.V(5).Infof("Creating target https proxy %v", desiredHTTPSProxy)
	err := s.tpp.CreateTargetHttpsProxy(desiredHTTPSProxy)
	if err != nil {
		return "", err
	}
	fmt.Println("Target https proxy", name, "created successfully")
	existing, err := s.tpp.GetTargetHttpsProxy(name)
	if err != nil {
		return "", err
	}
	return existing.SelfLink, nil
}

func targetHTTPSProxyMatches(desiredHTTPSProxy, existingHTTPSProxy compute.TargetHttpsProxy) bool {
	existingHTTPSProxy.CreationTimestamp = ""
	existingHTTPSProxy.Id = 0
	existingHTTPSProxy.Kind = ""
	existingHTTPSProxy.SelfLink = ""
	existingHTTPSProxy.ServerResponse = googleapi.ServerResponse{}

	equal := reflect.DeepEqual(existingHTTPSProxy, desiredHTTPSProxy)
	if !equal {
		glog.V(2).Infof("TargetHTTPSProxies differ.")
		glog.V(3).Infof("Diff: %v", diff.ObjectDiff(desiredHTTPSProxy, existingHTTPSProxy))
	} else {
		glog.V(2).Infof("TargetHTTPSProxies match.")
	}
	return equal
}
func (s *Syncer) desiredHTTPSTargetProxy(lbName, umLink, certLink string) *compute.TargetHttpsProxy {
	// Compute the desired target https proxy.
	return &compute.TargetHttpsProxy{
		Name:            s.namer.TargetHttpsProxyName(),
		Description:     fmt.Sprintf("Target https proxy for kubernetes multicluster loadbalancer %s", lbName),
		UrlMap:          umLink,
		SslCertificates: []string{certLink},
	}
}
