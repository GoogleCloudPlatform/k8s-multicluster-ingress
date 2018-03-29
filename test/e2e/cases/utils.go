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
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"regexp"
	"time"

	"github.com/golang/glog"

	gcputils "github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/utils"
	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/kubeutils"
	"k8s.io/apimachinery/pkg/util/wait"
	kubeclient "k8s.io/client-go/kubernetes"
)

const (
	// Letters is the letters of the alphabet.
	Letters = "abcdefghijklmnopqrstuvwxyz"

	// LoadBalancerPollTimeout is how long we wait for a the LB to be
	// ready. On average it takes ~6 minutes for a single backend to come
	// online in GCE.
	LoadBalancerPollTimeout = 12 * time.Minute
	// LoadBalancerPollInterval is the time between checking for the LB to be ready.
	LoadBalancerPollInterval = 10 * time.Second

	// IngressReqTimeout is the timeout on a single http request.
	IngressReqTimeout = 10 * time.Second
)

// kubemci is expected to be in PATH.
var kubemci = "kubemci"
var gnuSed = ""

func init() {
	rand.Seed(time.Now().UnixNano())
}

// Returns a random string of the given length.
func randString(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = Letters[rand.Intn(len(Letters))]
	}
	return string(b)
}

// Returns the first IPv4 address found in the given string.
func findIPv4(input string) string {
	numBlock := "(25[0-5]|2[0-4][0-9]|1[0-9][0-9]|[1-9]?[0-9])"
	regexPattern := numBlock + "\\." + numBlock + "\\." + numBlock + "\\." + numBlock

	regEx := regexp.MustCompile(regexPattern)
	return regEx.FindString(input)
}

// createIngress runs the kubemci tool to create a multi-cluster ingress.
// It returns a delete function that MUST be run once the test is over.
func createIngress(project, kubeConfigPath, lbName, ingressPath string) (func(), error) {
	kubemciArgs := []string{kubemci, fmt.Sprintf("--ingress=%s", ingressPath), fmt.Sprintf("--gcp-project=%s", project), fmt.Sprintf("--kubeconfig=%s", kubeConfigPath)}
	createArgs := append(kubemciArgs, []string{"create", lbName}...)
	glog.Infof("running: %v", createArgs)
	output, err := kubeutils.ExecuteCommand(createArgs)
	if err != nil {
		glog.Fatalf("Error running kubemci create: %s", err)
	}
	glog.Infof("kubemci output:\n%s", output)

	deleteFn := func() {
		glog.Infof("Deleting ingress %s", ingressPath)
		deleteArgs := append(kubemciArgs, []string{"delete", lbName}...)
		kubeutils.ExecuteCommand(deleteArgs)
	}

	return deleteFn, nil
}

// createTLSSecrets generates crt and key and puts them as secrets in every cluster
// and returns a delete function that MUST be called after the test is done
func createTLSSecrets(kubectlArgs []string, clients map[string]kubeclient.Interface) func() {
	// Generate the crt and key.
	// TODO(nikhiljindal): Generate valid certs instead of using self signed
	// certs so that we do not need InsecureSkipVerify.
	certGenArgs := []string{"openssl", "req", "-x509", "-nodes", "-days", "365", "-newkey", "rsa:2048", "-keyout", "tls.key", "-out", "tls.crt", "-subj", "/CN=zoneprintersvc/O=zoneprintersv"}
	kubeutils.ExecuteCommand(certGenArgs)
	// Create the secret in all clusters.
	for k := range clients {
		createSecretArgs := append(kubectlArgs, []string{"create", "secret", "tls", "tls-secret", "--key", "tls.key", "--cert", "tls.crt", fmt.Sprintf("--context=%s", k)}...)
		// create secret may fail if this was setup in a previous run.
		kubeutils.ExecuteCommand(createSecretArgs)
	}

	deleteFn := func() {
		// Delete the secret from all clusters.
		for k := range clients {
			deleteSecretArgs := append(kubectlArgs, []string{"delete", "secret", "tls-secret", fmt.Sprintf("--context=%s", k)}...)
			kubeutils.ExecuteCommand(deleteSecretArgs)
		}
	}
	return deleteFn
}

// getIPAddress gets the allocated IP address from a loadbalancer
func getIPAddress(project, lbName string) string {
	// TODO(nikhiljindal): Figure out why is sleep required? get-status should work immediately after create is successful.
	time.Sleep(5 * time.Second)
	// Ensure that get-status returns the expected output.
	getStatusArgs := []string{kubemci, "get-status", lbName, fmt.Sprintf("--gcp-project=%s", project)}
	output, _ := kubeutils.ExecuteCommand(getStatusArgs)
	glog.Infof("Output from get-status: %s", output)
	ipAddress := findIPv4(output)
	glog.Infof("IP Address: %s", ipAddress)
	return ipAddress
}

// Waits for the ingress paths to be reachable or returns an error if it times out.
func waitForIngress(httpProtocol, ipAddress, path string) error {
	tr := &http.Transport{
		// Skip verify so that self signed certs work.
		// TODO(nikhiljindal): Generate valid certs instead of using self signed
		// certs so that we do not need InsecureSkipVerify.
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	timeoutClient := &http.Client{
		Timeout:   IngressReqTimeout,
		Transport: tr,
	}
	// TODO(nikhiljindal): Verify all paths rather than just the root one.
	return pollURL(httpProtocol+"://"+ipAddress+path, "" /* host */, LoadBalancerPollTimeout, LoadBalancerPollInterval, timeoutClient, false /* expectUnreachable */)
}

// Polls the given URL until it gets a success response.
// Copied from /kubernetes/test/e2e/framework/util.go.PollURL.
// Using it directly imports huge dependencies (all cloud providers, all apimachinery code, etc)
// TODO(nikhiljindal) Refactor to reuse.
func pollURL(route, host string, timeout time.Duration, interval time.Duration, httpClient *http.Client, expectUnreachable bool) error {
	var lastBody string
	pollErr := wait.PollImmediate(interval, timeout, func() (bool, error) {
		var err error
		lastBody, err = simpleGET(httpClient, route, host)
		if err != nil {
			glog.Infof("host %s%s: error:'%v' unreachable", host, route, err)
			return expectUnreachable, nil
		}
		glog.Infof("Got success response from %s%s:\n%s", host, route, lastBody)
		return !expectUnreachable, nil
	})
	if pollErr != nil {
		return fmt.Errorf("failed to execute a successful GET within %v, Last response body for %v, host %v:\n%v\n\n%v",
			timeout, route, host, lastBody, pollErr)
	}
	return nil
}

// simpleGET executes a get on the given url, returns error if non-200 returned.
// Copied from /kubernetes/test/e2e/framework/util.go.simpleGET.
// Using it directly imports huge dependencies (all cloud providers, all apimachinery code, etc)
// TODO(nikhiljindal) Refactor to reuse.
func simpleGET(c *http.Client, url, host string) (string, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Host = host
	res, err := c.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	rawBody, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return "", err
	}
	body := string(rawBody)
	if res.StatusCode != http.StatusOK {
		err = fmt.Errorf(
			"GET returned http error %v", res.StatusCode)
	}
	return body, err
}

// ensureGnuSed tries to find a GNU sed executable and return the path to it or fail if not found
func ensureGnuSed() string {
	if gnuSed == "" {
		// From https://github.com/kubernetes/kubernetes/blob/f5f6f3e715cb8dfbd9657a4229c77ec6a5eab135/hack/lib/util.sh#L790
		ensureGnuSedScript :=
			"if LANG=C sed --help 2>&1 | grep -q GNU;" +
				"then which sed;" +
				"elif which gsed &>/dev/null;" +
				"then which gsed;" +
				"else return 1;" +
				"fi"
		out, err := kubeutils.ExecuteCommand([]string{"bash", "-c", ensureGnuSedScript})
		if err != nil {
			glog.Fatalf("Failed to find GNU sed as sed or gsed. If you are on Mac: brew install gnu-sed.")
		}
		glog.Infof("Found GNU sed: %s", out)
		gnuSed = out
	}
	return gnuSed
}

// replaceVariable replaces a string in a file with the given value
func replaceVariable(filepath, variable, value string) {
	if _, err := kubeutils.ExecuteCommand([]string{ensureGnuSed(), "-i", "-e", fmt.Sprintf("s/%s/%s/", variable, value), filepath}); err != nil {
		glog.Fatalf("Error updating yaml '%s' with sed: %v", filepath, err)
	}
}

// deployApp deploys all the resources under the specified filepath to the clients specified.
// kubectlArgs should already contain the kubectl command and kubeconfig path arguments
func deployApp(kubectlArgs []string, clients map[string]kubeclient.Interface, filepath string) {
	// We use kubectl to create the app since it is easier to point it at a
	// directory with all resources instead of having to create all of them
	// explicitly with go-client.
	args := append(kubectlArgs, "-f", filepath)
	for k := range clients {
		glog.Infof("Creating app in cluster %s", k)
		createArgs := append(args, []string{"create", fmt.Sprintf("--context=%s", k)}...)
		// kubectl create may fail if this was setup in a previous run.
		kubeutils.ExecuteCommand(createArgs)
	}
}

// cleanupApp deletes any resources found under the filepath from the specified clients.
// kubectlArgs should already contain the kubectl command and kubeconfig path arguments
func cleanupApp(kubectlArgs []string, clients map[string]kubeclient.Interface, filepath string) {
	args := append(kubectlArgs, "-f", filepath)
	for k := range clients {
		glog.Infof("Deleting app from cluster %s", k)
		deleteArgs := append(args, []string{"delete", fmt.Sprintf("--context=%s", k)}...)
		kubeutils.ExecuteCommand(deleteArgs)
	}
}

// initDeps initializes dependencies for a test run such as
// getting k8s clients from the contexts defined in the kubeconfig,
// generating name suffixes for the resources spawned and
// provisioning a static IP address.
func initDeps() (project, kubeConfigPath, lbName, ipName string, clients map[string]kubeclient.Interface) {
	// Validate inputs and instantiate required objects first to catch errors early.
	// TODO(nikhiljindal): User should be able to pass kubeConfigPath.
	kubeConfigPath = "minKubeconfig"
	// TODO(nikhiljindal): User should be able to pass gcp project.
	project, err := gcputils.GetProjectFromGCloud()
	if err != nil {
		glog.Fatalf("Error getting default project: %v", err)
	} else if project == "" {
		glog.Fatalf("unexpected: no default gcp project found. Run gcloud config set project to set a default project.")
	}
	// Get clients for all contexts in the kubeconfig.
	clients, err = kubeutils.GetClients(kubeConfigPath, []string{})
	if err != nil {
		glog.Fatalf("unexpected error in instantiating clients for all kubeconfig contexts: %s", err)
	}

	// Generate random names to be able to run this multiple times.
	// TODO(nikhiljindal): Use a random namespace name.
	lbName = randString(10)
	ipName = randString(10)
	glog.Infof("Creating an mci named '%s' with ip address named '%s'", lbName, ipName)

	// Reserve the IP address.
	if _, err := kubeutils.ExecuteCommand([]string{"gcloud", "compute", "addresses", "create", "--global", ipName}); err != nil {
		glog.Fatalf("Error creating IP address: %v", err)
	}

	return project, kubeConfigPath, lbName, ipName, clients
}

// cleanupDeps cleans up any resources created in initDeps
func cleanupDeps(ipName string) {
	// Release the IP address.
	kubeutils.ExecuteCommand([]string{"gcloud", "compute", "addresses", "delete", "--global", ipName})
}
