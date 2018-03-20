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

package kubeutils

import (
	"fmt"
	"os/exec"
	"reflect"
	"strings"

	"github.com/golang/glog"
	multierror "github.com/hashicorp/go-multierror"
	"k8s.io/api/core/v1"
	api_v1 "k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	kubeclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/ingress-gce/pkg/annotations"
	ingressbe "k8s.io/ingress-gce/pkg/backends"
)

// GetClients returns a map of cluster name to kubeclient for each cluster context.
// Uses all contexts from the given kubeconfig if kubeContexts is empty.
func GetClients(kubeconfig string, kubeContexts []string) (map[string]kubeclient.Interface, error) {
	// Pass the contexts list through GetClusterContexts even if we already
	// know the contexts to verify that they are valid.
	contexts, err := GetClusterContexts(kubeconfig, kubeContexts)
	if err != nil {
		return nil, err
	}
	return getClientsForContexts(kubeconfig, contexts)
}

// GetClusterContexts extracts and returns the list of contexts from the given kubeconfig.
// Returns the passed kubeContexts if they are all valid. Returns an error otherwise.
func GetClusterContexts(kubeconfig string, kubeContexts []string) ([]string, error) {
	kubectlArgs := []string{"kubectl"}
	if kubeconfig != "" {
		kubectlArgs = append(kubectlArgs, fmt.Sprintf("--kubeconfig=%s", kubeconfig))
	}
	contextArgs := append(kubectlArgs, []string{"config", "get-contexts", "-o=name"}...)
	contextArgs = append(contextArgs, kubeContexts...)
	output, err := ExecuteCommand(contextArgs)
	if err != nil {
		return nil, fmt.Errorf("error in getting contexts from kubeconfig: %s", err)
	}
	return strings.Split(output, "\n"), nil
}

func getClientsForContexts(kubeconfig string, kubeContexts []string) (map[string]kubeclient.Interface, error) {
	clients := map[string]kubeclient.Interface{}
	var err error
	for _, c := range kubeContexts {
		client, clientErr := getClientset(kubeconfig, c)
		if clientErr != nil {
			err = multierror.Append(err, fmt.Errorf("Error getting kubectl client interface for context %s: %s", c, clientErr))
			continue
		}
		clients[c] = client
	}
	return clients, err
}

// ExecuteCommand executes the command in 'args' and returns the output and error, if any.
// Extracted out here to allow overriding in tests.
var ExecuteCommand = func(args []string) (string, error) {
	glog.V(3).Infof("Running command: %s\n", strings.Join(args, " "))
	// TODO(nikhiljindal): Figure out how to use CombinedOutput here to get error message in output.
	// We dont use it right now because then "gcloud get project" fails.
	output, err := exec.Command(args[0], args[1:]...).Output()
	if err != nil {
		err = fmt.Errorf("unexpected error in executing command %q: error: %q, output: %q", strings.Join(args, " "), err, output)
		glog.V(3).Infof("%s", err)
	}
	return strings.TrimSuffix(string(output), "\n"), err
}

// Extracted out here to allow overriding in tests.
var getClientset = func(kubeconfigPath, contextName string) (kubeclient.Interface, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if kubeconfigPath != "" {
		loadingRules.ExplicitPath = kubeconfigPath
	}
	loader := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, &clientcmd.ConfigOverrides{CurrentContext: contextName})

	clientConfig, err := loader.ClientConfig()
	if err != nil {
		fmt.Println("getClientset: error getting Client Config:", err)
		return nil, err
	}

	return kubeclient.NewForConfigOrDie(clientConfig), nil
}

// TODO refactor `getHTTPProbe` in ingress-gce/controller/utils.go so we can share code
// GetProbe returns a probe that's used for the given nodeport
func GetProbe(client kubeclient.Interface, port ingressbe.ServicePort) (*api_v1.Probe, error) {
	svc, err := client.CoreV1().Services(port.SvcName.Namespace).Get(port.SvcName.Name, meta_v1.GetOptions{})
	if err != nil {
		fmt.Printf("Unable to find service %v in namespace %v\n", port.SvcName.Name, port.SvcName.Namespace)
		return nil, err
	}

	selector := labels.SelectorFromSet(svc.Spec.Selector)

	pl, err := client.CoreV1().Pods(port.SvcName.Namespace).List(meta_v1.ListOptions{
		LabelSelector: selector.String(),
	})
	if err != nil {
		fmt.Printf("Unable to find pods backing service %v in namespace %v\n", port.SvcName.Name, port.SvcName.Namespace)
		return nil, err
	}

	for _, pod := range pl.Items {
		logStr := fmt.Sprintf("Pod %v matching service selectors %v (targetport %+v)", pod.Name, selector.String(), port.SvcTargetPort)

		for _, c := range pod.Spec.Containers {
			if !isSimpleHTTPProbe(c.ReadinessProbe) || string(port.Protocol) != string(c.ReadinessProbe.HTTPGet.Scheme) {
				continue
			}

			for _, p := range c.Ports {
				if (port.SvcPort.Type == intstr.Int && port.SvcPort.IntVal == p.ContainerPort) ||
					(port.SvcPort.Type == intstr.String && port.SvcPort.StrVal == p.Name) {

					readinessProbePort := c.ReadinessProbe.Handler.HTTPGet.Port

					switch readinessProbePort.Type {
					case intstr.Int:
						if readinessProbePort.IntVal == p.ContainerPort {
							return c.ReadinessProbe, nil
						}
					case intstr.String:
						if readinessProbePort.StrVal == p.Name {
							return c.ReadinessProbe, nil
						}
					}

					fmt.Printf("%v: found matching targetPort on container %v, but not on readinessProbe (%+v)\n",
						logStr, c.Name, c.ReadinessProbe.Handler.HTTPGet.Port)
				}
			}
		}

		fmt.Printf("%v: lacks a matching HTTP probe for use in health checks.\n", logStr)
	}

	return nil, nil
}

// TODO copied from ingress-gce/controller/utils.go, refactor so we can share code
// isSimpleHTTPProbe returns true if the given Probe is:
// - an HTTPGet probe, as opposed to a tcp or exec probe
// - has no special host or headers fields, except for possibly an HTTP Host header
func isSimpleHTTPProbe(probe *api_v1.Probe) bool {
	return (probe != nil && probe.Handler.HTTPGet != nil && probe.Handler.HTTPGet.Host == "" &&
		(len(probe.Handler.HTTPGet.HTTPHeaders) == 0 ||
			(len(probe.Handler.HTTPGet.HTTPHeaders) == 1 && probe.Handler.HTTPGet.HTTPHeaders[0].Name == "Host")))
}

// Returns a copy of the given annotations map such that it does not contain keys that are present in the ignore map.
func ignoreAnnotations(annotations, ignore map[string]string) map[string]string {
	result := map[string]string{}
	for k, v := range annotations {
		if _, exists := ignore[k]; !exists {
			result[k] = v
		}
	}
	return result
}

// Note: copied from https://github.com/kubernetes/federation/blob/7951a643cebc3abdcd903eaff90d1383b43928d1/pkg/federation-controller/util/meta.go#L61
// Checks if cluster-independent, user provided data in two given ObjectMeta are equal. If in
// the future the ObjectMeta structure is expanded then any field that is not populated
// by the api server should be included here.
// Ignores annotations with keys in ignoreAnnotationKeys.
func ObjectMetaEquivalent(a, b meta_v1.ObjectMeta, ignoreAnnotationKeys map[string]string) bool {
	if a.Name != b.Name {
		return false
	}
	if a.Namespace != b.Namespace {
		return false
	}
	if !reflect.DeepEqual(a.Labels, b.Labels) && (len(a.Labels) != 0 || len(b.Labels) != 0) {
		return false
	}
	// Remove ignoreAnnotationKeys from the list of annotations.
	aAnnotations := ignoreAnnotations(a.Annotations, ignoreAnnotationKeys)
	bAnnotations := ignoreAnnotations(b.Annotations, ignoreAnnotationKeys)
	if !reflect.DeepEqual(aAnnotations, bAnnotations) && (len(aAnnotations) != 0 || len(bAnnotations) != 0) {
		return false
	}
	return true
}

// Note: copied from https://github.com/kubernetes/federation/blob/7951a643cebc3abdcd903eaff90d1383b43928d1/pkg/federation-controller/util/meta.go#L79
// Checks if cluster-independent, user provided data in ObjectMeta and Spec in two given top
// level api objects are equivalent.
// Ignores annotations with keys in ignoreAnnotationKeys.
func ObjectMetaAndSpecEquivalent(a, b runtime.Object, ignoreAnnotationKeys map[string]string) bool {
	objectMetaA := reflect.ValueOf(a).Elem().FieldByName("ObjectMeta").Interface().(meta_v1.ObjectMeta)
	objectMetaB := reflect.ValueOf(b).Elem().FieldByName("ObjectMeta").Interface().(meta_v1.ObjectMeta)
	specA := reflect.ValueOf(a).Elem().FieldByName("Spec").Interface()
	specB := reflect.ValueOf(b).Elem().FieldByName("Spec").Interface()
	return ObjectMetaEquivalent(objectMetaA, objectMetaB, ignoreAnnotationKeys) && reflect.DeepEqual(specA, specB)
}

// GetServiceNodePort takes an IngressBackend and returns a corresponding ServicePort
func GetServiceNodePort(be v1beta1.IngressBackend, namespace string, client kubeclient.Interface) (ingressbe.ServicePort, error) {
	svc, err := getSvc(be.ServiceName, namespace, client)
	// Refactor this code to get serviceport from a given service and share it with kubernetes/ingress.
	appProtocols, err := annotations.FromService(svc).ApplicationProtocols()
	if err != nil {
		return ingressbe.ServicePort{}, err
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
		return ingressbe.ServicePort{}, fmt.Errorf("could not find matching nodeport for backend %+v and service %s/%s. Looking for port %+v in %v", be, namespace, be.ServiceName, be.ServicePort, svc.Spec.Ports)
	}

	proto := annotations.ProtocolHTTP
	if protoStr, exists := appProtocols[port.Name]; exists {
		glog.V(2).Infof("service %s/%s: using protocol to %q", namespace, be.ServiceName, protoStr)
		proto = annotations.AppProtocol(protoStr)
	}

	p := ingressbe.ServicePort{
		NodePort: int64(port.NodePort),
		Protocol: proto,
		SvcName:  types.NamespacedName{Namespace: namespace, Name: be.ServiceName},
		SvcPort:  be.ServicePort,
	}
	glog.Infof("Found ServicePort: %+v", p)
	return p, nil
}
func getSvc(svcName, nsName string, client kubeclient.Interface) (*v1.Service, error) {
	return client.CoreV1().Services(nsName).Get(svcName, meta_v1.GetOptions{})
}
