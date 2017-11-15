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
	"strings"

	"github.com/golang/glog"
	multierror "github.com/hashicorp/go-multierror"
	kubeclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// GetClients returns a map of cluster name to kubeclient for each cluster context.
// Uses all contexts from the given kubeconfig if kubeContexts is empty.
func GetClients(kubeconfig string, kubeContexts []string) (map[string]kubeclient.Interface, error) {
	// Pass the contexts list through getClusterContexts even if we already
	// know the contexts to verify that they are valid.
	contexts, err := getClusterContexts(kubeconfig, kubeContexts)
	if err != nil {
		return nil, err
	}
	return getClientsForContexts(kubeconfig, contexts)
}

// Extracts and returns the list of contexts from the given kubeconfig.
// Returns the passed kubeContexts if they are all valid. Returns an error otherwise.
func getClusterContexts(kubeconfig string, kubeContexts []string) ([]string, error) {
	kubectlArgs := []string{"kubectl"}
	if kubeconfig != "" {
		kubectlArgs = append(kubectlArgs, fmt.Sprintf("--kubeconfig=%s", kubeconfig))
	}
	contextArgs := append(kubectlArgs, []string{"config", "get-contexts", "-o=name"}...)
	contextArgs = append(contextArgs, kubeContexts...)
	output, err := executeCommand(contextArgs)
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
			err = multierror.Append(err, fmt.Errorf("Error getting kubectl client interface for context %s:", c, clientErr))
			continue
		}
		clients[c] = client
	}
	return clients, err
}

// Extracted out here to allow overriding in tests.
var executeCommand = func(args []string) (string, error) {
	glog.V(3).Infof("Running command: %s\n", strings.Join(args, " "))
	output, err := exec.Command(args[0], args[1:]...).CombinedOutput()
	if err != nil {
		glog.V(3).Infof("%s", output)
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
