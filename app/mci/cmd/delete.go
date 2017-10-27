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
	"strings"

	"github.com/golang/glog"
	"github.com/hashicorp/go-multierror"
	"github.com/spf13/cobra"
	"k8s.io/api/extensions/v1beta1"

	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/mci/pkg/gcp/cloudinterface"
	gcplb "github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/mci/pkg/gcp/loadbalancer"
)

var (
	deleteShortDescription = "Delete a multicluster ingress."
	deleteLongDescription  = `Delete a multicluster ingress.

	Takes an ingress spec and a list of clusters and deletes the multicluster ingress targetting those clusters.
	`
)

type DeleteOptions struct {
	// Name of the YAML file containing ingress spec.
	IngressFilename string
	// Path to kubeconfig.
	Kubeconfig string
	// Name of the load balancer.
	// Required.
	LBName string
	// Name of the GCP project in which the load balancer should be configured.
	// Required
	// TODO(nikhiljindal): This should be optional. Figure it out from gcloud settings.
	GCPProject string
}

func NewCmdDelete(out, err io.Writer) *cobra.Command {
	var options DeleteOptions

	cmd := &cobra.Command{
		Use:   "delete",
		Short: deleteShortDescription,
		Long:  deleteLongDescription,
		// TODO(nikhiljindal): Add an example.
		Run: func(cmd *cobra.Command, args []string) {
			if err := ValidateDeleteArgs(&options, args); err != nil {
				fmt.Println(err)
				return
			}
			if err := RunDelete(&options, args); err != nil {
				fmt.Println("Error in deleting load balancer:", err)
			}
		},
	}
	AddDeleteFlags(cmd, &options)
	return cmd
}

func AddDeleteFlags(cmd *cobra.Command, options *DeleteOptions) error {
	cmd.Flags().StringVarP(&options.IngressFilename, "ingress", "i", options.IngressFilename, "filename containing ingress spec")
	cmd.Flags().StringVarP(&options.Kubeconfig, "kubeconfig", "k", options.Kubeconfig, "path to kubeconfig")
	cmd.Flags().StringVarP(&options.GCPProject, "gcp-project", "", options.GCPProject, "name of the gcp project")
	// TODO Add a verbose flag that turns on glog logging.
	return nil
}

func ValidateDeleteArgs(options *DeleteOptions, args []string) error {
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

func RunDelete(options *DeleteOptions, args []string) error {
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
		return fmt.Errorf("error in deleting cloud interface: %s", err)
	}

	// Delete ingress in all clusters.
	if delErr := deleteIngress(options.Kubeconfig, options.IngressFilename); delErr != nil {
		err = multierror.Append(err, delErr)
	}

	lbs := gcplb.NewLoadBalancerSyncer(options.LBName, clientset, cloudInterface)
	if delErr := lbs.DeleteLoadBalancer(&ing); delErr != nil {
		err = multierror.Append(err, delErr)
	}
	return err
}

// Extracts the contexts from the given kubeconfig and deletes ingress in those context clusters.
func deleteIngress(kubeconfig, ingressFilename string) error {
	// TODO(nikhiljindal): Allow users to specify the list of clusters to delete the ingress in
	// rather than assuming all contexts in kubeconfig.
	clusters, err := getClusters(kubeconfig)
	if err != nil {
		return err
	}
	return deleteIngressInClusters(kubeconfig, ingressFilename, clusters)
}

// Deletes the given ingress in the given list of clusters.
func deleteIngressInClusters(kubeconfig, ingressFilename string, clusters []string) error {
	kubectlArgs := []string{"kubectl"}
	if kubeconfig != "" {
		kubectlArgs = append(kubectlArgs, fmt.Sprintf("--kubeconfig=%s", kubeconfig))
	}
	deleteArgs := append(kubectlArgs, []string{"delete", fmt.Sprintf("--filename=%s", ingressFilename)}...)
	for _, c := range clusters {
		fmt.Println("Deleting ingress in context:", c)
		contextArgs := append(deleteArgs, fmt.Sprintf("--context=%s", c))
		output, err := runCommand(contextArgs)
		if err != nil {
			// TODO(nikhiljindal): Continue if this is an ingress does not exist error.
			glog.V(2).Infof("error in running command: %s", strings.Join(contextArgs, " "))
			return fmt.Errorf("error in deleting ingress in cluster %s: %s, output: %s", c, err, output)
		}
	}
	return nil
}
