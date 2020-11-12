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

	multierror "github.com/hashicorp/go-multierror"
	"github.com/spf13/cobra"
	"k8s.io/api/extensions/v1beta1"

	// gcp is needed for GKE cluster auth to work.
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"

	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/cloudinterface"
	gcplb "github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/loadbalancer"
	gcputils "github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/utils"
	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/ingress"
	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/kubeutils"
)

var (
	deleteShortDescription = "Delete a multicluster ingress."
	deleteLongDescription  = `Delete a multicluster ingress.

	Takes an ingress spec and a list of clusters and deletes the multicluster ingress targetting those clusters.
	`
)

type deleteOptions struct {
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
	// Delete whatever can be deleted in case of errors.
	ForceDelete bool
	// Name of the namespace for the ingress when none is provided (mismatch of option with spec causes an error).
	// Optional.
	Namespace string
}

func newCmdDelete(out, err io.Writer) *cobra.Command {
	var options deleteOptions

	cmd := &cobra.Command{
		Use:   "delete [lbname]",
		Short: deleteShortDescription,
		Long:  deleteLongDescription,
		// TODO(nikhiljindal): Add an example.
		Run: func(cmd *cobra.Command, args []string) {
			if err := validateDeleteArgs(&options, args); err != nil {
				fmt.Println(err)
				return
			}
			if err := runDelete(&options, args); err != nil {
				fmt.Println("Error in deleting load balancer:", err)
			}
		},
	}
	addDeleteFlags(cmd, &options)
	return cmd
}

func addDeleteFlags(cmd *cobra.Command, options *deleteOptions) error {
	cmd.Flags().StringVarP(&options.IngressFilename, "ingress", "i", options.IngressFilename, "[required] filename containing ingress spec")
	cmd.Flags().StringVarP(&options.KubeconfigFilename, "kubeconfig", "k", options.KubeconfigFilename, "[required] path to kubeconfig file")
	cmd.Flags().StringSliceVar(&options.KubeContexts, "kubecontexts", options.KubeContexts, "[optional] contexts in the kubeconfig file to delete the ingress from")
	cmd.Flags().StringVarP(&options.AccessToken, "access-token", "t", options.AccessToken, "[optional] access token for gcp resources (defaults to GOOGLE_APPLICATION_CREDENTIALS).")
	// TODO(nikhiljindal): Add a short flag "-p" if it seems useful.
	cmd.Flags().StringVarP(&options.GCPProject, "gcp-project", "", options.GCPProject, "[optional] name of the gcp project. Is fetched using gcloud config get-value project if unset here")
	cmd.Flags().BoolVarP(&options.ForceDelete, "force", "f", options.ForceDelete, "[optional] delete whatever can be deleted in case of errors. This should only be used in exceptional cases (for example: when the clusters are deleted before the ingress was deleted)")
	cmd.Flags().StringVarP(&options.Namespace, "namespace", "n", options.Namespace, "[optional] namespace for the ingress only if left unspecified by ingress spec")
	// TODO Add a verbose flag that turns on glog logging.
	return nil
}

func validateDeleteArgs(options *deleteOptions, args []string) error {
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

func runDelete(options *deleteOptions, args []string) error {
	options.LBName = args[0]
	var err error

	// Unmarshal the YAML into ingress struct.
	var ing v1beta1.Ingress
	if ingErr := ingress.UnmarshallAndApplyDefaults(options.IngressFilename, options.Namespace, &ing); ingErr != nil {
		return fmt.Errorf("error in unmarshalling the yaml file %s, err: %s", options.IngressFilename, ingErr)
	}
	cloudInterface, ciErr := cloudinterface.NewGCECloudInterface(options.GCPProject, options.AccessToken)
	if ciErr != nil {
		return fmt.Errorf("error in creating cloud interface: %s", ciErr)
	}

	// Get clients for all clusters
	clients, clientsErr := kubeutils.GetClients(options.KubeconfigFilename, options.KubeContexts)
	if clientsErr != nil {
		return clientsErr
	}

	lbs, lbErr := gcplb.NewLoadBalancerSyncer(options.LBName, clients, cloudInterface, options.GCPProject)
	if lbErr != nil {
		return lbErr
	}
	if delErr := lbs.DeleteLoadBalancer(&ing, options.ForceDelete); delErr != nil {
		if !options.ForceDelete {
			return delErr
		}
		fmt.Printf("%s. Continuing with force delete\n", delErr)
		err = multierror.Append(err, delErr)
	}

	// Delete ingress resource in clusters
	// Note: Delete ingress from clusters after deleting the GCP resources.
	// This is to ensure that the backend service is deleted when ingress-gce controller
	// observes ingress deletion and hence tries to delete instance groups.
	// https://github.com/kubernetes/ingress-gce/issues/186 has more details.
	isErr := ingress.NewIngressSyncer().DeleteIngress(&ing, clients)
	if isErr != nil {
		if !options.ForceDelete {
			return isErr
		}
		fmt.Printf("%s. Continuing with force delete\n", isErr)
		err = multierror.Append(err, isErr)
	}
	return err
}
