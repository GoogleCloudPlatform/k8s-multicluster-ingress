// Copyright 2018 Google Inc.
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

package validations

import (
	"fmt"
	"regexp"
	"strconv"

	kubeclient "k8s.io/client-go/kubernetes"

	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/kubeutils"
	"github.com/golang/glog"
	multierror "github.com/hashicorp/go-multierror"
	"k8s.io/api/extensions/v1beta1"
)

// validator is a concrete implementation of ValidatorInterface.
type validator struct {
}

var _ ValidatorInterface = &validator{}

// NewValidator returns a new Validator.
func NewValidator() ValidatorInterface {
	return &validator{}
}

// Validate performs pre-flight checks on the clusters and services.
func (v *validator) Validate(clients map[string]kubeclient.Interface, ing *v1beta1.Ingress) error {
	if err := serverVersionsNewEnough(clients); err != nil {
		return err
	}
	return servicesNodePortsSame(clients, ing)
}

// servicesNodePortsSame checks that for each backend/service, the services are
// all listening on the same node port in all clusters.
func servicesNodePortsSame(clients map[string]kubeclient.Interface, ing *v1beta1.Ingress) error {
	var multiErr error
	for _, rule := range ing.Spec.Rules {
		if rule.HTTP == nil {
			glog.Warningf("ignoring non http Ingress rule: %v", rule)
			continue
		}
		for _, path := range rule.HTTP.Paths {
			glog.V(3).Infof("Validating path: %s", path.Path)
			if err := nodePortSameInAllClusters(path.Backend, ing.Namespace, clients); err != nil {
				multierror.Append(multiErr, fmt.Errorf("nodePort validation error for service '%s/%s': %s", ing.Namespace, path.Backend.ServiceName, err))
			} else {
				glog.V(1).Infof("path: %s: passed validation", path.Path)
			}
		}
	}
	glog.V(2).Infof("Checking default backend's nodeports.")

	if ing.Spec.Backend != nil {
		if err := nodePortSameInAllClusters(*ing.Spec.Backend, ing.Namespace, clients); err != nil {
			multiErr = multierror.Append(multiErr,
				fmt.Errorf("nodePort validation error for default backend service '%s/%v': %s", ing.Namespace, *ing.Spec.Backend, err))
		}
		glog.V(1).Infof("Default backend's nodeports passed validation.")
	} else {
		glog.V(1).Infof("Default backend is missing from Ingress Spec:\n%+v", ing.Spec)
		multiErr = multierror.Append(multiErr, fmt.Errorf("unexpected: ing.spec.backend is nil. Multicluster ingress needs a user-specified default backend"))
	}
	return multiErr
}

// nodePortSameInAllClusters checks that the given backend's service is running
// on the same NodePort in all clusters (defined by clients).
func nodePortSameInAllClusters(backend v1beta1.IngressBackend, namespace string, clients map[string]kubeclient.Interface) error {
	nodePort := int64(-1)
	var firstClusterName string
	// TODO(G-Harmon): We could provide the list of all clusters that are mismatching instead of just 1.
	for clientName, client := range clients {
		glog.V(2).Infof("Checking cluster: %s", clientName)

		servicePort, err := kubeutils.GetServiceNodePort(backend, namespace, client)
		if err != nil {
			return fmt.Errorf("could not get service NodePort in cluster '%s': %s", clientName, err)
		}
		glog.V(2).Infof("cluster %s: Service's servicePort: %+v", clientName, servicePort)
		clusterNodePort := servicePort.NodePort

		if nodePort == -1 {
			nodePort = clusterNodePort
			firstClusterName = clientName
			continue
		}
		if clusterNodePort != nodePort {
			return fmt.Errorf("some instances of the '%s/%s' Service (e.g. in '%s') are on NodePort %v, but '%s' is on %v. All clusters must use same NodePort",
				namespace, backend.ServiceName, firstClusterName, nodePort, clientName, clusterNodePort)
		}
	}
	return nil
}

// serverVersionsNewEnough returns an error if the version of any cluster is not supported.
func serverVersionsNewEnough(clients map[string]kubeclient.Interface) error {
	for key := range clients {
		glog.Infof("Checking client %s", key)
		discoveryClient := clients[key].Discovery()
		if discoveryClient == nil {
			return fmt.Errorf("no discovery client in %s client: %+v", key, clients[key])
		}
		ver, err := discoveryClient.ServerVersion()
		if err != nil {
			return fmt.Errorf("could not get discovery client to lookup server version: %s", err)
		}
		glog.Infof("ServerVersion: %+v", ver)
		major, minor, patch, err := parseVersion(ver.GitVersion)
		if err != nil {
			return err
		}
		if newEnough := serverVersionNewEnough(major, minor, patch); !newEnough {
			return fmt.Errorf("cluster %s (ver %d.%d.%d) is not running a supported kubernetes version. Need >= 1.8.1 and not 1.10.0",
				key, major, minor, patch)
		}

	}
	return nil
}

func serverVersionNewEnough(major, minor, patch uint64) bool {
	// 1.10.0 was buggy and doesn't work: kubernetes/ingress-gce#182.
	if major == 1 && minor == 10 && patch == 0 {
		return false
	}

	// 1.8.1 was the first supported release.
	if major > 1 {
		return true
	} else if major < 1 {
		return false
	}
	// Minor version
	if minor > 8 {
		return true
	} else if minor < 8 {
		return false
	}
	// Patch version
	if patch >= 1 {
		return true
	}
	return false
}

func parseVersion(version string) (uint64, uint64, uint64, error) {
	// Example string we're matching: v1.9.3-gke.0
	re := regexp.MustCompile("^v([0-9]*).([0-9]*).([0-9]*)")
	matches := re.FindStringSubmatch(version)
	glog.V(4).Infof("version string matches: %v\n", matches)

	if len(matches) < 3 {
		return 0, 0, 0, fmt.Errorf("did not find 3 components to version number %s", version)
	}
	// Major version
	major, err := strconv.ParseUint(matches[1], 10 /*base*/, 32 /*bitSize*/)
	if err != nil {
		return 0, 0, 0, err
	}
	// Minor version
	minor, err := strconv.ParseUint(matches[2], 10 /*base*/, 32 /*bitSize*/)
	if err != nil {
		return 0, 0, 0, err
	}
	// Patch version
	patch, err := strconv.ParseUint(matches[3], 10 /*bases*/, 32 /*bitSize*/)
	if err != nil {
		return 0, 0, 0, err
	}
	glog.V(2).Infof("Got version: major: %d, minor: %d, patch: %d\n", major, minor, patch)
	return major, minor, patch, nil
}
