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
	"github.com/spf13/cobra"
	"k8s.io/api/extensions/v1beta1"
	kubeclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/mci/pkg/gcp/cloudinterface"
	gcplb "github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/mci/pkg/gcp/loadbalancer"
)

var (
	createShortDescription = "Create a multi-cluster ingress."
	createLongDescription  = `Create a multi-cluster ingress.

	Takes an ingress spec and a list of clusters and creates a multi-cluster ingress targetting those clusters.
	`
)

// Extracted out here to allow overriding in tests.
var executeCommand = func(args []string) (string, error) {
	output, err := exec.Command(args[0], args[1:]...).CombinedOutput()
	return strings.TrimSuffix(string(output), "\n"), err
}

// Extracted out here to allow overriding in tests.
var getClientset = func(kubeconfigPath string) (kubeclient.Interface, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if kubeconfigPath != "" {
		loadingRules.ExplicitPath = kubeconfigPath
	}
	loader := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, &clientcmd.ConfigOverrides{})

	clientConfig, err := loader.ClientConfig()
	if err != nil {
		return nil, err
	}

	return kubeclient.NewForConfigOrDie(clientConfig), nil
}

type CreateOptions struct {
	// Name of the YAML file containing ingress spec.
	IngressFilename string
	// Path to kubeconfig.
	Kubeconfig string
	// Name of the load balancer.
	// Required.
	LBName string
	// Name of the GCP project in which the load balancer should be configured.
	// Required
	// TODO: This should be optional. Figure it out from gcloud settings.
	GCPProject string
	// Overwrite values when they differ from what's requested. If
	// the resource does not exist, or is already the correct
	// value, then 'force' is a no-op.
	Force bool
}

func NewCmdCreate(out, err io.Writer) *cobra.Command {
	var options CreateOptions

	cmd := &cobra.Command{
		Use:   "create",
		Short: createShortDescription,
		Long:  createLongDescription,
		// TODO: Add an example.
		Run: func(cmd *cobra.Command, args []string) {
			if err := ValidateArgs(&options, args); err != nil {
				fmt.Println(err)
				return
			}
			if err := RunCreate(&options, args); err != nil {
				fmt.Println("Error in creating load balancer:", err)
			}
		},
	}
	AddFlags(cmd, &options)
	return cmd
}

func AddFlags(cmd *cobra.Command, options *CreateOptions) error {
	cmd.Flags().StringVarP(&options.IngressFilename, "ingress", "i", options.IngressFilename, "filename containing ingress spec")
	cmd.Flags().StringVarP(&options.Kubeconfig, "kubeconfig", "k", options.Kubeconfig, "path to kubeconfig")
	cmd.Flags().StringVarP(&options.GCPProject, "gcp-project", "", options.GCPProject, "name of the gcp project")
	// TODO Add a verbose flag that turns on glog logging.
	cmd.Flags().BoolVarP(&options.Force, "Force", "f", options.Force, "overwrite existing settings if they are different.")
	return nil
}

func ValidateArgs(options *CreateOptions, args []string) error {
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

func RunCreate(options *CreateOptions, args []string) error {
	options.LBName = args[0]

	// Unmarshal the YAML into ingress struct.
	var ing v1beta1.Ingress
	if err := unmarshall(options.IngressFilename, &ing); err != nil {
		return fmt.Errorf("error in unmarshalling the yaml file %s, err: %s", options.IngressFilename, err)
	}
	clientset, err := getClientset(options.Kubeconfig)
	if err != nil {
		return fmt.Errorf("unexpected error in instantiating clientset: %v", err)
	}
	cloudInterface, err := cloudinterface.NewGCECloudInterface(options.GCPProject)
	if err != nil {
		return fmt.Errorf("error in creating cloud interface: %s", err)
	}

	// Create ingress in all clusters.
	if err := createIngress(options.Kubeconfig, options.IngressFilename); err != nil {
		return err
	}

	lbs := gcplb.NewLoadBalancerSyncer(options.LBName, clientset, cloudInterface)
	return lbs.CreateLoadBalancer(&ing, options.Force)
}

// Extracts the contexts from the given kubeconfig and creates ingress in those context clusters.
func createIngress(kubeconfig, ingressFilename string) error {
	// TODO: Allow users to specify the list of clusters to create the ingress in
	// rather than assuming all contexts in kubeconfig.
	clusters, err := getClusters(kubeconfig)
	if err != nil {
		return err
	}
	return createIngressInClusters(kubeconfig, ingressFilename, clusters)
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
	return strings.Split(output, "\n"), nil
}

// Creates the given ingress in the given list of clusters.
func createIngressInClusters(kubeconfig, ingressFilename string, clusters []string) error {
	kubectlArgs := []string{"kubectl"}
	if kubeconfig != "" {
		kubectlArgs = append(kubectlArgs, fmt.Sprintf("--kubeconfig=%s", kubeconfig))
	}
	// TODO: Validate and optionally add the gce-multi-cluster class annotation to the ingress YAML spec.
	createArgs := append(kubectlArgs, []string{"create", fmt.Sprintf("--filename=%s", ingressFilename)}...)
	for _, c := range clusters {
		fmt.Println("Creating ingress in context:", c)
		contextArgs := append(createArgs, fmt.Sprintf("--context=%s", c))
		output, err := runCommand(contextArgs)
		if err != nil {
			// TODO: Continue if this is an ingress already exists error.
			glog.V(2).Infof("error in running command: %s", strings.Join(contextArgs, " "))
			return fmt.Errorf("error in creating ingress in cluster %s: %s, output: %s", c, err, output)
		}
	}
	return nil
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
