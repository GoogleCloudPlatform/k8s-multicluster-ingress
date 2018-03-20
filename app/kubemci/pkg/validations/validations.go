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

	kubeclient "k8s.io/client-go/kubernetes"

	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/kubeutils"
	"github.com/golang/glog"
	multierror "github.com/hashicorp/go-multierror"
	"k8s.io/api/extensions/v1beta1"
)

func Validate(clients map[string]kubeclient.Interface, ing *v1beta1.Ingress) error {
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
				fmt.Errorf("nodePort validation error for default backend service '%s/%s': %s", *ing.Spec.Backend, ing.Namespace, err))
		}
		glog.V(1).Infof("Default backend's nodeports passed validation.")
	} else {
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
			return fmt.Errorf("Could not get service NodePort in cluster %s: %s", clientName, err)
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
