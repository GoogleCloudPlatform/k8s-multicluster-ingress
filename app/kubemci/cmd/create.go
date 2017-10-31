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

package cmd

import (
	"fmt"
	"io"
	"io/ioutil"
	"os/exec"
	"strings"

	"github.com/ghodss/yaml"
	"github.com/golang/glog"
	"github.com/hashicorp/go-multierror"
	"github.com/spf13/cobra"
	"k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	kubeclient "k8s.io/client-go/kubernetes"
	// gcp is needed for GKE cluster auth to work.
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/cloudinterface"
	gcplb "github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/loadbalancer"
)

var (
	createShortDescription = "Create a multicluster ingress."
	createLongDescription  = `Create a multicluster ingress.

	Takes an ingress spec and a list of clusters and creates a multicluster ingress targetting those clusters.
	`
)

// Extracted out here to allow overriding in tests.
var executeCommand = func(args []string) (string, error) {
	output, err := exec.Command(args[0], args[1:]...).CombinedOutput()
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

type CreateOptions struct {
	// Name of the YAML file containing ingress spec.
	IngressFilename string
	// Path to kubeconfig file.
	KubeconfigFilename string
	// Name of the load balancer.
	// Required.
	LBName string
	// Name of the GCP project in which the load balancer should be configured.
	// Required
	// TODO(nikhiljindal): This should be optional. Figure it out from gcloud settings.
	GCPProject string
	// Overwrite values when they differ from what's requested. If
	// the resource does not exist, or is already the correct
	// value, then 'force' is a no-op.
	ForceUpdate bool
}

func NewCmdCreate(out, err io.Writer) *cobra.Command {
	var options CreateOptions

	cmd := &cobra.Command{
		Use:   "create",
		Short: createShortDescription,
		Long:  createLongDescription,
		// TODO(nikhiljindal): Add an example.
		Run: func(cmd *cobra.Command, args []string) {
			if err := validateCreateArgs(&options, args); err != nil {
				fmt.Println(err)
				return
			}
			if err := runCreate(&options, args); err != nil {
				fmt.Println("Error in creating load balancer:", err)
			}
		},
	}
	addCreateFlags(cmd, &options)
	return cmd
}

func addCreateFlags(cmd *cobra.Command, options *CreateOptions) error {
	cmd.Flags().StringVarP(&options.IngressFilename, "ingress", "i", options.IngressFilename, "filename containing ingress spec")
	cmd.Flags().StringVarP(&options.KubeconfigFilename, "kubeconfig", "k", options.KubeconfigFilename, "path to kubeconfig file")
	// TODO(nikhiljindal): Add a short flag "-p" if it seems useful.
	cmd.Flags().StringVarP(&options.GCPProject, "gcp-project", "", options.GCPProject, "name of the gcp project")
	cmd.Flags().BoolVarP(&options.ForceUpdate, "force", "f", options.ForceUpdate, "overwrite existing settings if they are different.")
	// TODO Add a verbose flag that turns on glog logging, or figure out how
	// to accept glog flags in addition to the cobra flags.
	return nil
}

func validateCreateArgs(options *CreateOptions, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("unexpected args: %v. Expected one arg as name of load balancer.", args)
	}
	// Verify that the required params are not missing.
	if options.IngressFilename == "" {
		return fmt.Errorf("unexpected missing argument ingress.")
	}
	if options.GCPProject == "" {
		return fmt.Errorf("unexpected missing argument gcp-project.")
	}
	return nil
}

func runCreate(options *CreateOptions, args []string) error {
	options.LBName = args[0]

	// Unmarshal the YAML into ingress struct.
	var ing v1beta1.Ingress
	if err := unmarshall(options.IngressFilename, &ing); err != nil {
		return fmt.Errorf("error in unmarshalling the yaml file %s, err: %s", options.IngressFilename, err)
	}
	cloudInterface, err := cloudinterface.NewGCECloudInterface(options.GCPProject)
	if err != nil {
		return fmt.Errorf("error in creating cloud interface: %s", err)
	}

	// Create ingress in all clusters.
	clusters, clients, err := createIngress(options.KubeconfigFilename, options.IngressFilename)
	if err != nil {
		return err
	}

	lbs := gcplb.NewLoadBalancerSyncer(options.LBName, clients, cloudInterface)
	return lbs.CreateLoadBalancer(&ing, options.ForceUpdate, clusters)
}

// Extracts the contexts from the given kubeconfig and creates ingress in those context clusters.
// Returns the list of clusters in which it created the ingress and a map of clients for each of those clusters.
func createIngress(kubeconfig, ingressFilename string) ([]string, map[string]kubeclient.Interface, error) {
	// TODO(nikhiljindal): Allow users to specify the list of clusters to create the ingress in
	// rather than assuming all contexts in kubeconfig.
	clusters, err := getClusters(kubeconfig)
	if err != nil {
		return nil, nil, err
	}
	clients, createErr := createIngressInClusters(kubeconfig, ingressFilename, clusters)
	return clusters, clients, createErr
}

// Extracts the list of contexts from the given kubeconfig.
func getClusters(kubeconfig string) ([]string, error) {
	kubectlArgs := []string{"kubectl"}
	if kubeconfig != "" {
		kubectlArgs = append(kubectlArgs, fmt.Sprintf("--kubeconfig=%s", kubeconfig))
	}
	contextArgs := append(kubectlArgs, []string{"config", "get-contexts", "-o=name"}...)
	output, err := runCommand(contextArgs)
	if err != nil {
		return nil, fmt.Errorf("error in getting contexts from kubeconfig: %s", err)
	}
	glog.V(5).Infof("Contexts found:\n%v", output)
	return strings.Split(output, "\n"), nil
}

// Creates the given ingress in the given list of clusters.
func createIngressInClusters(kubeconfig, ingressFilename string, clusters []string) (map[string]kubeclient.Interface, error) {
	clients := map[string]kubeclient.Interface{}
	var ing v1beta1.Ingress
	if err := unmarshall(ingressFilename, &ing); err != nil {
		return clients, fmt.Errorf("error in parsing the yaml file %s, err: %s", ingressFilename, err)
	}
	glog.V(5).Infof("Unmarshaled this ingress:\n%+v", ing)

	// TODO(nikhiljindal): Validate and optionally add the gce-multi-cluster class annotation to the ingress YAML spec.
	var err error
	for _, c := range clusters {
		fmt.Println("\nHandling context:", c)
		client, clientErr := getClientset(kubeconfig, c)
		if clientErr != nil {
			err = multierror.Append(err, fmt.Errorf("Error getting kubectl client interface for context %s:", c, clientErr))
			continue
		}
		clients[c] = client
		glog.V(3).Infof("Using this namespace for ingress: %v", ing.Namespace)
		actualIng, createErr := client.Extensions().Ingresses(ing.Namespace).Create(&ing)
		glog.V(2).Infof("Ingress Create returned: err:%v. Actual Ingress:%+v", err, actualIng)
		if createErr != nil {
			if errors.IsAlreadyExists(createErr) {
				fmt.Println("Ingress already exists; moving on.")
				continue
			} else {
				err = multierror.Append(err, fmt.Errorf("Error in creating ingress in cluster %s: %s", c, createErr))
			}
		}
	}
	return clients, err
}

func runCommand(args []string) (string, error) {
	glog.V(3).Infof("Running command: %s\n", strings.Join(args, " "))
	output, err := executeCommand(args)
	if err != nil {
		glog.V(3).Infof("%s", output)
	}
	return output, err
}

func unmarshall(filename string, ing *v1beta1.Ingress) error {
	// Read the file
	bytes, err := ioutil.ReadFile(filename)
	if err != nil {
		return err
	}
	// Unmarshall into ingress struct.
	if err := yaml.Unmarshal(bytes, ing); err != nil {
		return err
	}
	return nil
}
