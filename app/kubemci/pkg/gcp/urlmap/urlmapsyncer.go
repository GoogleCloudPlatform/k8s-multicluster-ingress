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

package urlmap

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"reflect"

	compute "google.golang.org/api/compute/v1"
	"google.golang.org/api/googleapi"

	"github.com/golang/glog"
	multierror "github.com/hashicorp/go-multierror"
	"k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/util/diff"
	ingresslb "k8s.io/ingress-gce/pkg/loadbalancers"
	"k8s.io/ingress-gce/pkg/utils"

	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/backendservice"
	utilsnamer "github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/namer"
)

const (
	// The gce api uses the name of a path rule to match a host rule.
	// TODO(nikhiljindal): Refactor to share with kubernetes/ingress-gce which has the same constant.
	hostRulePrefix = "host"
)

// URLMapSyncer manages GCP url maps for multicluster GCP L7 load balancers.
type URLMapSyncer struct {
	namer *utilsnamer.Namer
	// Instance of URLMapProvider interface for calling GCE URLMap APIs.
	// There is no separate URLMapProvider interface, so we use the bigger LoadBalancers interface here.
	ump ingresslb.LoadBalancers
}

func NewURLMapSyncer(namer *utilsnamer.Namer, ump ingresslb.LoadBalancers) URLMapSyncerInterface {
	return &URLMapSyncer{
		namer: namer,
		ump:   ump,
	}
}

// Ensure this implements URLMapSyncerInterface.
var _ URLMapSyncerInterface = &URLMapSyncer{}

// See interfaces.go comment.
func (s *URLMapSyncer) EnsureURLMap(lbName string, ing *v1beta1.Ingress, beMap backendservice.BackendServicesMap, forceUpdate bool) (string, error) {
	fmt.Println("Ensuring url map")
	var err error
	desiredUM, err := s.desiredURLMap(lbName, ing, beMap)
	if err != nil {
		return "", fmt.Errorf("error %s in computing desired url map", err)
	}
	name := desiredUM.Name
	// Check if url map already exists.
	existingUM, err := s.ump.GetUrlMap(name)
	if err == nil {
		fmt.Println("url map", name, "exists already. Checking if it matches our desired url map", name)
		glog.V(5).Infof("Existing url map: %v\n, desired url map: %v", existingUM, desiredUM)
		// Fingerprint is required (and we get an error if it's not set).
		// TODO(G-Harmon): Figure out how to properly calculate the
		// FP. Using Sha256 returned a googleapi error. We shouldn't use
		// the existing FP when we're changing the object (as it seems
		// like it's used for some oportunistic optimization on the
		// server side).
		desiredUM.Fingerprint = existingUM.Fingerprint
		// URL Map with that name exists already. Check if it matches what we want.
		if urlMapMatches(*desiredUM, *existingUM) {
			// Nothing to do. Desired url map exists already.
			fmt.Println("Desired url map exists already")
			return existingUM.SelfLink, nil
		}
		if forceUpdate {
			return s.updateURLMap(desiredUM)
		} else {
			fmt.Println("Will not overwrite this differing URL Map without the --force flag.")
			return "", fmt.Errorf("will not overwrite URL Map without --force")
		}
	}
	glog.V(5).Infof("Got error %s while trying to get existing url map %s. Will try to create new one", err, name)
	// TODO: Handle non NotFound errors. We should create only if the error is NotFound.
	// Create the url map.
	return s.createURLMap(desiredUM)
}

func (s *URLMapSyncer) DeleteURLMap() error {
	name := s.namer.URLMapName()
	fmt.Println("Deleting url map", name)
	if err := utils.IgnoreHTTPNotFound(s.ump.DeleteUrlMap(name)); err != nil {
		fmt.Println("Error", err, "in deleting url map", name)
		return err
	}
	fmt.Println("Url map", name, "deleted successfully")
	return nil
}

func (s *URLMapSyncer) updateURLMap(desiredUM *compute.UrlMap) (string, error) {
	name := desiredUM.Name
	fmt.Println("Updating existing url map", name, "to match the desired state")
	err := s.ump.UpdateUrlMap(desiredUM)
	if err != nil {
		return "", err
	}
	fmt.Println("URL Map", name, "updated successfully")
	um, err := s.ump.GetUrlMap(name)
	if err != nil {
		return "", err
	}
	return um.SelfLink, nil
}

func (s *URLMapSyncer) createURLMap(desiredUM *compute.UrlMap) (string, error) {
	name := desiredUM.Name
	fmt.Println("Creating url map", name)
	glog.V(5).Infof("Creating url map %v", desiredUM)
	err := s.ump.CreateUrlMap(desiredUM)
	if err != nil {
		return "", err
	}
	fmt.Println("URL Map", name, "created successfully")
	um, err := s.ump.GetUrlMap(name)
	if err != nil {
		return "", err
	}
	return um.SelfLink, nil
}

func urlMapMatches(desiredUM, existingUM compute.UrlMap) bool {
	// Clear output-only fields to do our comparison
	existingUM.CreationTimestamp = ""
	existingUM.Kind = ""
	existingUM.Id = 0
	existingUM.SelfLink = ""
	existingUM.ServerResponse = googleapi.ServerResponse{}

	glog.V(5).Infof("desired UM:\n%+v", desiredUM)
	glog.V(5).Infof("existing UM:\n%+v", existingUM)

	equal := reflect.DeepEqual(existingUM, desiredUM)
	if !equal {
		glog.V(1).Infof("Diff:\n%v", diff.ObjectDiff(desiredUM, existingUM))
	}
	return equal
}

func (s *URLMapSyncer) desiredURLMap(lbName string, ing *v1beta1.Ingress, beMap backendservice.BackendServicesMap) (*compute.UrlMap, error) {
	// Compute the desired url map.
	um := &compute.UrlMap{
		Name:        s.namer.URLMapName(),
		Description: fmt.Sprintf("URL map for kubernetes multicluster loadbalancer %s", lbName),
	}
	gceMap, err := s.ingToURLMap(ing, beMap)
	if err != nil {
		fmt.Println("Error getting URL map from Ingress:", err)
		return nil, err
	}
	um.DefaultService = gceMap.GetDefaultBackend().SelfLink
	if len(gceMap) > 0 {
		// Only create these slices if we have data; otherwise we get a
		// DeepEqual mismatch when comparing to what the server returns.
		um.HostRules = []*compute.HostRule{}
		um.PathMatchers = []*compute.PathMatcher{}
	}

	// Code taken from kubernetes/ingress-gce/L7s.UpdateUrlMap.
	// TODO: Refactor it to share code.
	for hostname, urlToBackend := range gceMap {
		// Create a host rule
		// Create a path matcher
		// Add all given endpoint:backends to pathRules in path matcher
		pmName := getNameForPathMatcher(hostname)
		um.HostRules = append(um.HostRules, &compute.HostRule{
			Hosts:       []string{hostname},
			PathMatcher: pmName,
		})

		pathMatcher := &compute.PathMatcher{
			Name:           pmName,
			DefaultService: um.DefaultService,
			PathRules:      []*compute.PathRule{},
		}

		for expr, be := range urlToBackend {
			pathMatcher.PathRules = append(
				pathMatcher.PathRules, &compute.PathRule{Paths: []string{expr}, Service: be.SelfLink})
		}
		um.PathMatchers = append(um.PathMatchers, pathMatcher)
	}
	return um, nil
}

// ingToURLMap converts an ingress to GCEURLMap (nested map of subdomain: url-regex: gce backend).
// TODO: Copied from kubernetes/ingress-gce with minor changes to print errors
// instead of generating events. Refactor it to make it reusable.
func (s *URLMapSyncer) ingToURLMap(ing *v1beta1.Ingress, beMap backendservice.BackendServicesMap) (utils.GCEURLMap, error) {
	hostPathBackend := utils.GCEURLMap{}
	var err error
	for _, rule := range ing.Spec.Rules {
		if rule.HTTP == nil {
			fmt.Println("Ignoring non http ingress rule", rule)
			continue
		}
		pathToBackend := map[string]*compute.BackendService{}
		for _, p := range rule.HTTP.Paths {
			backend, beErr := getBackendService(&p.Backend, ing.Namespace, beMap)
			if beErr != nil {
				fmt.Println("Skipping path", p.Backend, "due to error", beErr)
				err = multierror.Append(err, beErr)
				continue
			}
			// The Ingress spec defines empty path as catch-all, so if a user
			// asks for a single host and multiple empty paths, all traffic is
			// sent to one of the last backend in the rules list.
			path := p.Path
			if path == "" {
				path = ingresslb.DefaultPath
			}
			pathToBackend[path] = backend
		}
		// If multiple hostless rule sets are specified, last one wins
		host := rule.Host
		if host == "" {
			host = ingresslb.DefaultHost
		}
		hostPathBackend[host] = pathToBackend
	}
	var defaultBackend *compute.BackendService
	if ing.Spec.Backend == nil {
		// TODO(nikhiljindal): Be able to create a default backend service.
		// For now, we require users to specify it and generate an error if its nil.
		// We cant create a url map without a default service, so no point continuing.
		err = multierror.Append(err, fmt.Errorf("unexpected: ing.spec.backend is nil. Multicluster ingress needs a user specified default backend"))
		return nil, err
	}
	defaultBackend, beErr := getBackendService(ing.Spec.Backend, ing.Namespace, beMap)
	if beErr != nil {
		fmt.Printf("Error getting backend service %s: %v", ing.Spec.Backend.ServiceName, beErr)
		err = multierror.Append(err, beErr)
		return nil, err
	}
	hostPathBackend.PutDefaultBackend(defaultBackend)
	return hostPathBackend, err
}

func getBackendService(be *v1beta1.IngressBackend, ns string, beMap backendservice.BackendServicesMap) (*compute.BackendService, error) {
	if be == nil {
		return nil, fmt.Errorf("unexpected: received nil ingress backend")
	}
	backendService := beMap[be.ServiceName]
	if backendService == nil {
		return nil, fmt.Errorf("unexpected: No backend service found for service: %s, must have been an error in ensuring backend services", be.ServiceName)
	}
	return backendService, nil
}

// getNameForPathMatcher returns a name for a pathMatcher based on the given host rule.
// The host rule can be a regex, the path matcher name used to associate the 2 cannot.
// TODO(nikhiljindal): Copied from kubernetes/ingress-gce. Make it a public method there so that it can be reused.
func getNameForPathMatcher(hostRule string) string {
	hasher := md5.New()
	hasher.Write([]byte(hostRule))
	return fmt.Sprintf("%v%v", hostRulePrefix, hex.EncodeToString(hasher.Sum(nil)))
}
