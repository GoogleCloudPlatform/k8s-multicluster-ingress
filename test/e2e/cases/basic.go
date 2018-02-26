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

package e2e

import (
	"fmt"
	"strings"

	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/kubeutils"
	"github.com/golang/glog"
	kubeclient "k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
)

// Creates a basic multicluster ingress and verifies that it works as expected.
// TODO(nikhiljindal): Use ginkgo and gomega and possibly reuse k/k e2e framework.
func RunBasicCreateTest() {
	project, kubeConfigPath, lbName, ipName, clients := initDeps()
	kubectlArgs := []string{"kubectl", fmt.Sprintf("--kubeconfig=%s", kubeConfigPath)}
	setupBasic(kubectlArgs, ipName, clients)
	defer func() {
		cleanupBasic(kubectlArgs, lbName, ipName, clients)
		cleanupDeps(ipName)
	}()
	testHTTPIngress(project, kubeConfigPath, lbName)
	testHTTPSIngress(project, kubeConfigPath, lbName, kubectlArgs, clients)
}

func setupBasic(kubectlArgs []string, ipName string, clients map[string]kubeclient.Interface) {
	// Create the zone-printer app in all contexts.
	deployApp(kubectlArgs, clients, "examples/zone-printer/app/")
	// Update the ingress YAML specs to replace $ZP_KUBEMCI_IP with ip name.
	replaceVariable("examples/zone-printer/ingress/nginx.yaml", "\\$ZP_KUBEMCI_IP", ipName)
	replaceVariable("examples/zone-printer/ingress/https-ingress.yaml", "\\$ZP_KUBEMCI_IP", ipName)
}

func cleanupBasic(kubectlArgs []string, lbName, ipName string, clients map[string]kubeclient.Interface) {
	// Delete the zone-printer app from all contexts.
	cleanupApp(kubectlArgs, clients, "examples/zone-printer/app/")
	// Update the ingress YAML spec to put $ZP_KUBEMCI_IP back.
	replaceVariable("examples/zone-printer/ingress/nginx.yaml", ipName, "\\$ZP_KUBEMCI_IP")
	replaceVariable("examples/zone-printer/ingress/https-ingress.yaml", ipName, "\\$ZP_KUBEMCI_IP")
}

func testHTTPIngress(project, kubeConfigPath, lbName string) {
	glog.Infof("Testing HTTP ingress")
	deleteFn, err := createIngress(project, kubeConfigPath, lbName, "examples/zone-printer/ingress/nginx.yaml")
	if err != nil {
		glog.Fatalf("error creating ingress: %+v", err)
	}
	defer deleteFn()

	// Tests
	ipAddress := getIpAddress(project, lbName)
	// Ensure that the IP address eventually returns 202.
	if err := waitForIngress("http", ipAddress, "/"); err != nil {
		glog.Errorf("error in waiting for ingress: %s", err)
	}
	fmt.Println("PASS: got 200 from ingress url")
	testList(project, ipAddress, lbName)

	// Running create again should not return any error.
	_, err = createIngress(project, kubeConfigPath, lbName, "examples/zone-printer/ingress/nginx.yaml")
	if err != nil {
		glog.Fatalf("Unexpected error in re-creating ingress: %+v", err)
	}

	// TODO(nikhiljindal): Ensure that the ingress is created and deleted in all
	// clusters as expected.
}

func testHTTPSIngress(project, kubeConfigPath, lbName string, kubectlArgs []string, clients map[string]kubeclient.Interface) {
	glog.Infof("Testing HTTPS ingress")
	// Creates secrets with TLS crt and key in every cluster
	deleteTLSFn := createTLSSecrets(kubectlArgs, clients)
	defer deleteTLSFn()

	// Run kubemci create command.
	deleteFn, err := createIngress(project, kubeConfigPath, lbName, "examples/zone-printer/ingress/https-ingress.yaml")
	if err != nil {
		glog.Fatalf("error creating ingress: %+v", err)
	}
	defer deleteFn()

	// Tests
	ipAddress := getIpAddress(project, lbName)
	// Ensure that the IP address eventually returns 202.
	if err := waitForIngress("http", ipAddress, "/"); err != nil {
		glog.Errorf("error in GET %s: %s", ipAddress, err)
	}
	// Ensure that the IP address returns 202 for https as well.
	if err := waitForIngress("https", ipAddress, "/"); err != nil {
		glog.Errorf("error in GET %s: %s", ipAddress, err)
	}
	fmt.Println("PASS: got 200 from ingress url")
	testList(project, ipAddress, lbName)

	// Running create again should not return any error.
	_, err = createIngress(project, kubeConfigPath, lbName, "examples/zone-printer/ingress/https-ingress.yaml")
	if err != nil {
		// TODO(nikhiljindal): Change this to unexpected fatal error once
		// https://github.com/GoogleCloudPlatform/k8s-multicluster-ingress/issues/125 is fixed.
		glog.Infof("Expected error in re-creating https ingress: %+v", err)
	}

	// TODO(nikhiljindal): Ensure that the ingress is created and deleted in all
	// clusters as expected.
}

// testList tests that the list command returns a load balancer with the given name and ip address.
func testList(project, ipAddress, lbName string) {
	listArgs := []string{kubemci, "list", fmt.Sprintf("--gcp-project=%s", project)}
	listStr, err := kubeutils.ExecuteCommand(listArgs)
	if err != nil {
		glog.Fatalf("Error listing MCIs: %v", err)
	}
	glog.Infof("kubemci list returned:\n%s", listStr)
	if !strings.Contains(listStr, lbName) || !strings.Contains(listStr, ipAddress) {
		glog.Fatalf("Status does not contain lb name (%s) and IP (%s): %s", lbName, ipAddress, listStr)
	}
	fmt.Println("PASS: found loadbalancer name and IP in 'list' command.")
}
