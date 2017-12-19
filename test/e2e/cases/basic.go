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
	"time"

	"github.com/golang/glog"

	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/kubeutils"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
)

var kubemci_bin = "./kubemci"

// Creates a basic multicluster ingress and verifies that it works as expected.
// TODO(nikhiljindal): Use ginkgo and gomega and possibly reuse k/k e2e framework.
func RunBasicCreateTest() {
	// Validate inputs and instantiate required objects first to catch errors early.
	// TODO(nikhiljindal): User should be able to pass kubeConfigPath.
	kubeConfigPath := "minKubeconfig"
	// TODO(nikhiljindal): User should be able to pass gcp project.
	project, err := runCommand([]string{"gcloud", "config", "get-value", "project"})
	if err != nil {
		glog.Fatalf("Error getting default project:%v", err)
	} else if project == "" {
		glog.Fatalf("unexpected: no default gcp project found. Run gcloud config set project to set a default project.")
	}
	// Get clients for all contexts in the kubeconfig.
	clients, err := kubeutils.GetClients(kubeConfigPath, []string{})
	if err != nil {
		glog.Fatalf("unexpected error in instantiating clients for all kubeconfig contexts: %s", err)
	}

	// Generate random names to be able to run this multiple times.
	// TODO(nikhiljindal): Use a random namespace name.
	lbName := randString(10)
	ipName := randString(10)
	glog.Infof("Creating an mci named '%s' with ip address named '%s'", lbName, ipName)

	// Create the zone-printer app in all contexts.
	// We use kubectl to create the app since it is easier to point it at a
	// directory with all resources instead of having to create all of them
	// explicitly with go-client.
	args := []string{"kubectl", fmt.Sprintf("--kubeconfig=%s", kubeConfigPath), "-f", "examples/zone-printer/app/"}
	for k := range clients {
		glog.Infof("Creating app in cluster %s", k)
		createArgs := append(args, []string{"create", fmt.Sprintf("--context=%s", k)}...)
		// kubectl create may fail if this was setup in a previous run.
		runCommand(createArgs)
		defer func() {
			// Delete the app.
			glog.Infof("Deleting app from cluster %s", k)
			deleteArgs := append(args, []string{"delete", fmt.Sprintf("--context=%s", k)}...)
			runCommand(deleteArgs)
		}()
	}
	// Reserve the IP address.
	if _, err := runCommand([]string{"gcloud", "compute", "addresses", "create", "--global", ipName}); err != nil {
		glog.Fatalf("Error creating IP address:%v", err)
	}
	defer func() {
		runCommand([]string{"gcloud", "compute", "addresses", "delete", "--global", ipName})

	}()
	// Update the ingress YAML spec to replace $ZP_KUBEMCI_IP with ip name.
	if _, err := runCommand([]string{"sed", "-i", "-e", fmt.Sprintf("s/\\$ZP_KUBEMCI_IP/%s/", ipName), "examples/zone-printer/ingress/nginx.yaml"}); err != nil {
		glog.Fatalf("Error updating yaml with sed:%v", err)
	}
	defer func() {
		// Put $ZP_KUBEMCI_IP back once done.
		runCommand([]string{"sed", "-i", "-e", fmt.Sprintf("s/%s/\\$ZP_KUBEMCI_IP/", ipName), "examples/zone-printer/ingress/nginx.yaml"})
	}()

	// Setup done.
	// Run kubemci create command.
	kubemciArgs := []string{kubemci_bin, "--ingress=examples/zone-printer/ingress/nginx.yaml", fmt.Sprintf("--gcp-project=%s", project), fmt.Sprintf("--kubeconfig=%s", kubeConfigPath)}
	createArgs := append(kubemciArgs, []string{"create", lbName}...)
	if _, err := runCommand(createArgs); err != nil {
		glog.Fatalf("Error creating kubemci:%v", err)
		return
	}
	defer func() {
		// Delete the mci.
		deleteArgs := append(kubemciArgs, []string{"delete", lbName}...)
		runCommand(deleteArgs)
	}()

	// Tests
	// TODO(nikhiljindal): Figure out why is sleep required? get-status should work immediately after create is successful.
	time.Sleep(5 * time.Second)
	// Ensure that get-status returns the expected output.
	getStatusArgs := []string{kubemci_bin, "get-status", lbName, fmt.Sprintf("--gcp-project=%s", project)}
	output, _ := runCommand(getStatusArgs)
	glog.Infof("Output from get-status: %s", output)
	ipAddress := findIPv4(output)
	glog.Infof("IP Address: %s", ipAddress)
	// Ensure that the IP address eventually returns 202.
	if err := waitForIngress(ipAddress); err != nil {
		glog.Errorf("error in GET %s: %s", ipAddress, err)
	}
	fmt.Println("PASS: got 200 from ingress url")

	listArgs := []string{kubemci_bin, "list", fmt.Sprintf("--gcp-project=%s", project)}
	listStr, err := runCommand(listArgs)
	if err != nil {
		glog.Fatalf("Error listing MCIs: %v", err)
	}
	glog.Infof("kubemci list returned:\n%s", listStr)
	if !strings.Contains(listStr, lbName) || !strings.Contains(listStr, ipAddress) {
		glog.Fatalf("Status does not contain lb name (%s) and IP (%s): %s", lbName, ipAddress, listStr)
	}
	fmt.Println("PASS: found loadbalancer name and IP in 'list' command.")

	// TODO(nikhiljindal): Ensure that the ingress is created and deleted in all
	// clusters as expected.
}
