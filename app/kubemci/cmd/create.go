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

	"github.com/spf13/cobra"
	"k8s.io/api/extensions/v1beta1"
	kubeclient "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/pkg/cloudprovider/providers/gce"

	// gcp is needed for GKE cluster auth to work.
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"

	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/cloudinterface"
	gcplb "github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/loadbalancer"
	gcputils "github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/utils"
	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/ingress"
	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/kubeutils"
	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/validations"
)

var (
	createShortDescription = "Create a multicluster ingress."
	createLongDescription  = `Create a multicluster ingress.

	Takes an ingress spec and a list of clusters and creates a multicluster ingress targetting those clusters.
	`
)

type createOptions struct {
	// Name of the YAML file containing ingress spec.
	IngressFilename string
	// Path to kubeconfig file.
	KubeconfigFilename string
	// Names of the contexts to use from the kubeconfig file.
	KubeContexts []string
	// Access token with which to access gpc resources.
	AccessToken string
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
	// Validation of Ingress spec and Kubernetes Services setup.
	Validate bool
	// Name of the namespace for the ingress when none is provided (mismatch of option with spec causes an error).
	// Optional.
	Namespace string
	// Static IP Name for the ingress when none is provided (mismatch of option with spec causes an error).
	// Optional.
	StaticIPName string
}

func newCmdCreate(out, err io.Writer) *cobra.Command {
	var options createOptions
	options.Validate = true

	cmd := &cobra.Command{
		Use:   "create [lbname]",
		Short: createShortDescription,
		Long:  createLongDescription,
		// TODO(nikhiljindal): Add an example.
		Run: func(cmd *cobra.Command, args []string) {
			if err := validateCreateArgs(&options, args); err != nil {
				fmt.Println(err)
				return
			}
			if err := runCreate(&options, args); err != nil {
				fmt.Println("Error: Error in creating load balancer:", err)
			} else {
				fmt.Println("\nSuccess.")
			}
		},
	}
	addCreateFlags(cmd, &options)
	return cmd
}

func addCreateFlags(cmd *cobra.Command, options *createOptions) error {
	cmd.Flags().StringVarP(&options.IngressFilename, "ingress", "i", options.IngressFilename, "[required] filename containing ingress spec")
	cmd.Flags().StringVarP(&options.KubeconfigFilename, "kubeconfig", "k", options.KubeconfigFilename, "[required] path to kubeconfig file")
	cmd.Flags().StringSliceVar(&options.KubeContexts, "kubecontexts", options.KubeContexts, "[optional] contexts in the kubeconfig file to install the ingress into")
	cmd.Flags().StringVarP(&options.AccessToken, "access-token", "t", options.AccessToken, "[optional] access token for gcp resources (defaults to GOOGLE_APPLICATION_CREDENTIALS).")

	// TODO(nikhiljindal): Add a short flag "-p" if it seems useful.
	cmd.Flags().StringVarP(&options.GCPProject, "gcp-project", "", options.GCPProject, "[optional] name of the gcp project. Is fetched using gcloud config get-value project if unset here")
	cmd.Flags().BoolVarP(&options.ForceUpdate, "force", "f", options.ForceUpdate, "[optional] overwrite existing settings if they are different")
	cmd.Flags().BoolVarP(&options.Validate, "validate", "", options.Validate, "[optional] If enabled (default), do some validation checks and potentially return an error, before creating load balancer")
	cmd.Flags().StringVarP(&options.Namespace, "namespace", "n", options.Namespace, "[optional] namespace for the ingress only if left unspecified by ingress spec")
	cmd.Flags().StringVarP(&options.StaticIPName, "static-ip", "", options.StaticIPName, "[optional] Global Static IP name to use only if left unspecified by ingress spec")
	return nil
}

func validateCreateArgs(options *createOptions, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("unexpected args: %v. Expected one arg as name of load balancer", args)
	}
	// Verify that the required params are not missing.
	if options.IngressFilename == "" {
		return fmt.Errorf("unexpected missing argument ingress")
	}
	if options.GCPProject == "" {
		project, err := gcputils.GetProjectFromGCloud()
		if project == "" || err != nil {
			return fmt.Errorf("unexpected cannot determine GCP project. Either set --gcp-project flag, or set a default project with gcloud such that gcloud config get-value project returns that")
		}
		options.GCPProject = project
	}
	if options.KubeconfigFilename == "" {
		return fmt.Errorf("unexpected missing argument kubeconfig")
	}
	return nil
}

func runCreate(options *createOptions, args []string) error {
	options.LBName = args[0]

	// Unmarshal the YAML into ingress struct.
	var ing v1beta1.Ingress
	if err := ingress.UnmarshallAndApplyDefaults(options.IngressFilename, options.Namespace, &ing); err != nil {
		return fmt.Errorf("error in unmarshalling the yaml file %s, err: %s", options.IngressFilename, err)
	}
	if err := ingress.ApplyStaticIP(options.StaticIPName, &ing); err != nil {
		return fmt.Errorf("error in verifying static IP for ingress %s, err: %s", options.IngressFilename, err)
	}

	cloudInterface, err := cloudinterface.NewGCECloudInterface(options.GCPProject, options.AccessToken)
	if err != nil {
		return fmt.Errorf("error in creating cloud interface: %s", err)
	}

	// Get clients for all clusters
	clients, err := kubeutils.GetClients(options.KubeconfigFilename, options.KubeContexts)
	if err != nil {
		return err
	}

	return createIngressAndLoadBalancer(ing, clients, options, cloudInterface, validations.NewValidator())
}

func createIngressAndLoadBalancer(ing v1beta1.Ingress, clients map[string]kubeclient.Interface, options *createOptions, cloudInterface *gce.GCECloud, validator validations.ValidatorInterface) error {
	if options.Validate {
		if err := validator.Validate(clients, &ing); err != nil {
			return err
		}
	}

	// Ensure that the ingress resource is deployed to the clusters
	clusters, err := ingress.NewIngressSyncer().EnsureIngress(&ing, clients, options.ForceUpdate)
	if err != nil {
		return err
	}

	lbs, err := gcplb.NewLoadBalancerSyncer(options.LBName, clients, cloudInterface, options.GCPProject)
	if err != nil {
		return err
	}
	return lbs.CreateLoadBalancer(&ing, options.ForceUpdate, options.Validate, clusters)
}
