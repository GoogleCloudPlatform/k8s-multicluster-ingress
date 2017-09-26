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

package loadbalancer

import (
	"encoding/json"
	"fmt"

	"github.com/golang/glog"

	"k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	kubeclient "k8s.io/client-go/kubernetes"

	"k8s-multi-cluster-ingress/app/mci/pkg/gcp/healthcheck"
	utilsnamer "k8s-multi-cluster-ingress/app/mci/pkg/gcp/namer"
	sp "k8s-multi-cluster-ingress/app/mci/pkg/serviceport"
)

const (

	// TODO: refactor code in kubernetes/ingress to reuse this.
	// serviceApplicationProtocolKey is a stringified JSON map of port names to
	// protocol names. Possible values are HTTP, HTTPS.
	// Example annotation:
	// {
	//   "service.alpha.kubernetes.io/app-protocols" : '{"my-https-port":"HTTPS","my-http-port":"HTTP"}'
	// }
	serviceApplicationProtocolKey = "service.alpha.kubernetes.io/app-protocols"

	// Prefix used by the namer to generate names.
	// This is used to identify resources created by this code.
	mciPrefix = "mci-v1"
)

type LoadBalancerSyncer struct {
	lbName string
	hcs    *healthcheck.HealthCheckSyncer
	client kubeclient.Interface
}

func NewLoadBalancerSyncer(lbName string, client kubeclient.Interface) *LoadBalancerSyncer {
	namer := utilsnamer.NewNamer(mciPrefix, lbName)
	return &LoadBalancerSyncer{
		lbName: lbName,
		hcs:    healthcheck.NewHealthCheckSyncer(namer),
		client: client,
	}
}

func (l *LoadBalancerSyncer) CreateLoadBalancer(ing *v1beta1.Ingress) error {
	ports := l.ingToNodePorts(ing)
	// Create health check to be used by the backend service.
	if err := l.hcs.EnsureHealthCheck(l.lbName, ports); err != nil {
		return err
	}
	return nil
}

func (l *LoadBalancerSyncer) ingToNodePorts(ing *v1beta1.Ingress) []sp.ServicePort {
	var knownPorts []sp.ServicePort
	defaultBackend := ing.Spec.Backend
	if defaultBackend != nil {
		port, err := l.getServiceNodePort(*defaultBackend, ing.Namespace)
		if err != nil {
			glog.Errorf("%v", err)
		} else {
			knownPorts = append(knownPorts, port)
		}
	}
	for _, rule := range ing.Spec.Rules {
		if rule.HTTP == nil {
			glog.Errorf("ignoring non http Ingress rule")
			continue
		}
		for _, path := range rule.HTTP.Paths {
			port, err := l.getServiceNodePort(path.Backend, ing.Namespace)
			if err != nil {
				glog.Errorf("%v", err)
				continue
			}
			knownPorts = append(knownPorts, port)
		}
	}
	return knownPorts
}

func svcProtocolsFromAnnotation(annotations map[string]string) (map[string]string, error) {
	val, ok := annotations[serviceApplicationProtocolKey]
	if !ok {
		return map[string]string{}, nil
	}

	var svcProtocols map[string]string
	if err := json.Unmarshal([]byte(val), &svcProtocols); err != nil {
		return nil, fmt.Errorf("error in parsing annotation with key %s, error: %v", serviceApplicationProtocolKey, err)
	}
	return svcProtocols, nil
}

func (l *LoadBalancerSyncer) getServiceNodePort(be v1beta1.IngressBackend, namespace string) (sp.ServicePort, error) {
	svc, err := l.getSvc(be.ServiceName, namespace)
	// Refactor this code to get serviceport from a given service and share it with kubernetes/ingress.
	appProtocols, err := svcProtocolsFromAnnotation(svc.GetAnnotations())
	if err != nil {
		return sp.ServicePort{}, err
	}

	var port *v1.ServicePort
PortLoop:
	for _, p := range svc.Spec.Ports {
		np := p
		switch be.ServicePort.Type {
		case intstr.Int:
			if p.Port == be.ServicePort.IntVal {
				port = &np
				break PortLoop
			}
		default:
			if p.Name == be.ServicePort.StrVal {
				port = &np
				break PortLoop
			}
		}
	}

	if port == nil {
		return sp.ServicePort{}, fmt.Errorf("could not find matching nodeport for backend %v and service %s/%s", be, be.ServiceName, namespace)
	}

	proto := "HTTP"
	if protoStr, exists := appProtocols[port.Name]; exists {
		proto = protoStr
	}

	p := sp.ServicePort{
		Port:     int64(port.NodePort),
		Protocol: proto,
	}
	return p, nil
}

func (l *LoadBalancerSyncer) getSvc(svcName, nsName string) (*v1.Service, error) {
	return l.client.CoreV1().Services(nsName).Get(svcName, metav1.GetOptions{})
}
