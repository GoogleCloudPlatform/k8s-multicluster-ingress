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

	"github.com/golang/glog"
	"google.golang.org/api/compute/v1"
	"k8s.io/api/extensions/v1beta1"
	ingresslb "k8s.io/ingress-gce/pkg/loadbalancers"
	"k8s.io/ingress-gce/pkg/utils"

	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/mci/pkg/gcp/backendservice"
	utilsnamer "github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/mci/pkg/gcp/namer"
)

const (
	// The gce api uses the name of a path rule to match a host rule.
	// TODO(nikhiljindal): Refactor to share with kubernetes/ingress-gce
	hostRulePrefix = "host"
)

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

// EnsureURLMap ensures that the required url map exists.
// Does nothing if it exists already, else creates a new one.
func (h *URLMapSyncer) EnsureURLMap(lbName string, ing *v1beta1.Ingress, beMap backendservice.BackendServicesMap) error {
	fmt.Println("Ensuring url map")
	var err error
	desiredUM, err := h.desiredURLMap(lbName, ing, beMap)
	if err != nil {
		return fmt.Errorf("error %s in computing desired url map", err)
	}
	name := desiredUM.Name
	// Check if url map already exists.
	existingUM, err := h.ump.GetUrlMap(name)
	if err == nil {
		fmt.Println("url map", name, "exists already. Checking if it matches our desired url map", name)
		glog.V(5).Infof("Existing url map: %v\n, desired url map: %v", existingUM, desiredUM)
		// URL Map with that name exists already. Check if it matches what we want.
		if urlMapMatches(desiredUM, existingUM) {
			// Nothing to do. Desired url map exists already.
			fmt.Println("Desired url map exists already")
			return nil
		}
		// TODO: Require explicit permission from user before doing this.
		return h.updateURLMap(desiredUM)
	}
	glog.V(5).Infof("Got error %s while trying to get existing url map %s", err, name)
	// TODO: Handle non NotFound errors. We should create only if the error is NotFound.
	// Create the url map.
	return h.createURLMap(desiredUM)
}

func (h *URLMapSyncer) updateURLMap(desiredUM *compute.UrlMap) error {
	name := desiredUM.Name
	fmt.Println("Updating existing url map", name, "to match the desired state")
	err := h.ump.UpdateUrlMap(desiredUM)
	if err != nil {
		return err
	}
	fmt.Println("URL Map", name, "updated successfully")
	return nil
}

func (h *URLMapSyncer) createURLMap(desiredUM *compute.UrlMap) error {
	name := desiredUM.Name
	fmt.Println("Creating url map", name)
	glog.V(5).Infof("Creating url map %v", desiredUM)
	err := h.ump.CreateUrlMap(desiredUM)
	if err != nil {
		return err
	}
	fmt.Println("URL Map", name, "created successfully")
	return nil
}

func urlMapMatches(desiredUM, existingUM *compute.UrlMap) bool {
	// TODO: Add proper logic to figure out if the 2 url maps match.
	// Need to add the --force flag for user to consent overritting before this method can be updated to return false.
	return true
}

func (h *URLMapSyncer) desiredURLMap(lbName string, ing *v1beta1.Ingress, beMap backendservice.BackendServicesMap) (*compute.UrlMap, error) {
	// Compute the desired url map.
	um := &compute.UrlMap{
		Name:        h.namer.URLMapName(),
		Description: fmt.Sprintf("URL map for kubernetes multicluster loadbalancer %s", lbName),
	}
	gceMap, err := h.ingToURLMap(ing, beMap)
	if err != nil {
		return nil, err
	}
	um.DefaultService = gceMap.GetDefaultBackend().SelfLink
	um.HostRules = []*compute.HostRule{}
	um.PathMatchers = []*compute.PathMatcher{}

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

// ingToURLMap converts an ingress to a map of subdomain: url-regex: gce backend.
// TODO: Copied from kubernetes/ingress-gce with minor changes to print errors
// instead of generating events. Refactor it to make it reusable.
func (h *URLMapSyncer) ingToURLMap(ing *v1beta1.Ingress, beMap backendservice.BackendServicesMap) (utils.GCEURLMap, error) {
	hostPathBackend := utils.GCEURLMap{}
	for _, rule := range ing.Spec.Rules {
		if rule.HTTP == nil {
			fmt.Println("Ignoring non http ingress rule", rule)
			continue
		}
		pathToBackend := map[string]*compute.BackendService{}
		for _, p := range rule.HTTP.Paths {
			backend, err := getBackendService(&p.Backend, ing.Namespace, beMap)
			if err != nil {
				fmt.Println("Skipping path", p.Backend, "due to error", err)
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
		return nil, fmt.Errorf("unexpected: ing.spec.backend is nil. Multicluster ingress needs a user specified default backend")
	}
	defaultBackend, err := getBackendService(ing.Spec.Backend, ing.Namespace, beMap)
	if err != nil {
		return nil, err
	}
	hostPathBackend.PutDefaultBackend(defaultBackend)
	return hostPathBackend, nil
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
// TODO: Copied from kubernetes/ingress-gce. Make it a public method there so that it can be reused.
func getNameForPathMatcher(hostRule string) string {
	hasher := md5.New()
	hasher.Write([]byte(hostRule))
	return fmt.Sprintf("%v%v", hostRulePrefix, hex.EncodeToString(hasher.Sum(nil)))
}
