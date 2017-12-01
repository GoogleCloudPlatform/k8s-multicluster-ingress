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
	"io/ioutil"
	"math/rand"
	"net/http"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/golang/glog"

	"k8s.io/apimachinery/pkg/util/wait"
)

const (
	Letters = "abcdefghijklmnopqrstuvwxyz"

	// On average it takes ~6 minutes for a single backend to come online in GCE.
	LoadBalancerPollTimeout  = 12 * time.Minute
	LoadBalancerPollInterval = 30 * time.Second

	// IngressReqTimeout is the timeout on a single http request.
	IngressReqTimeout = 10 * time.Second
)

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

func runCommand(args []string) (string, error) {
	cmd := exec.Command(args[0], args[1:]...)
	glog.Infof("Running command: %s", strings.Join(args, " "))
	// TODO(nikhiljindal): Figure out how to use CombinedOutput here to get error message in output.
	// We dont use it right now because then "gcloud get project" fails.
	output, err := cmd.Output()
	if err != nil {
		err = fmt.Errorf("unexpected error in executing command %s: error: %s, output: %s", strings.Join(args, " "), err, output)
		glog.Errorf("%s", err)
	}
	return strings.TrimSuffix(string(output), "\n"), err
}

// Returns the first IPv4 address found in the given string.
func findIPv4(input string) string {
	numBlock := "(25[0-5]|2[0-4][0-9]|1[0-9][0-9]|[1-9]?[0-9])"
	regexPattern := numBlock + "\\." + numBlock + "\\." + numBlock + "\\." + numBlock

	regEx := regexp.MustCompile(regexPattern)
	return regEx.FindString(input)
}

// Waits for the ingress paths to be reachable or returns an error if it times out.
func waitForIngress(ip string) error {
	timeoutClient := &http.Client{Timeout: IngressReqTimeout}
	// TODO(nikhiljindal): Add support for https and also verify all paths rather than just the root one.
	pollURL("http://"+ip, "" /* host */, LoadBalancerPollTimeout, LoadBalancerPollInterval, timeoutClient, false /* expectUnreachable */)
	return nil
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
			glog.Infof("host %s%s: %v unreachable", host, route, err)
			return expectUnreachable, nil
		}
		glog.Infof("Got success response from %s%s: %s", host, route, lastBody)
		return !expectUnreachable, nil
	})
	if pollErr != nil {
		return fmt.Errorf("Failed to execute a successful GET within %v, Last response body for %v, host %v:\n%v\n\n%v\n",
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
