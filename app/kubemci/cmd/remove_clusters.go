// Copyright 2018 Google, Inc.
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
	removeClustersShortDesc = "Remove an existing multicluster ingress from some clusters."
	removeClustersLongDesc  = `Remove an existing multicluster ingress from some clusters.

	Takes a load balancer name and a list of clusters and removes the existing multicluster ingress from those clusters.
	If the clusters have already been deleted, you can run "kubemci create --force" with the updated cluster list to update the
	load balancer to be restricted to those clusters. That will however not delete the ingress from old clusters (which is fine
	if the clusters have been deleted already).
`
)

type removeClustersOptions struct {
	// Name of the YAML file containing ingress spec.
	// Required.
	IngressFilename string
	// Path to kubeconfig file.
	// Required.
	KubeconfigFilename string
	// Names of the contexts to use from the kubeconfig file.
	KubeContexts []string
	// Access token with which to access gpc resources.
	AccessToken string
	// Name of the load balancer.
	// Required.
	LBName string
	// Name of the GCP project in which the load balancer is configured.
	GCPProject string
	// Overwrite values when they differ from what's requested. If
	// the resource does not exist, or is already the correct
	// value, then 'force' is a no-op.
	ForceUpdate bool
	// Name of the namespace for the ingress when none is provided (mismatch of option with spec causes an error).
	Namespace string
}

func newCmdRemoveClusters(out, err io.Writer) *cobra.Command {
	var options removeClustersOptions
	cmd := &cobra.Command{
		Use:   "remove-clusters",
		Short: removeClustersShortDesc,
		Long:  removeClustersLongDesc,
		Run: func(cmd *cobra.Command, args []string) {
			if err := validateRemoveClustersArgs(&options, args); err != nil {
				fmt.Println(err)
				return
			}
			if err := runRemoveClusters(&options, args); err != nil {
				fmt.Println("Error removing clusters:", err)
				return
			}
		},
	}
	addRemoveClustersFlags(cmd, &options)
	return cmd
}

func addRemoveClustersFlags(cmd *cobra.Command, options *removeClustersOptions) error {
	cmd.Flags().StringVarP(&options.IngressFilename, "ingress", "i", options.IngressFilename, "[required] filename containing ingress spec")
	cmd.Flags().StringVarP(&options.KubeconfigFilename, "kubeconfig", "k", options.KubeconfigFilename, "[required] path to kubeconfig file")
	cmd.Flags().StringSliceVar(&options.KubeContexts, "kubecontexts", options.KubeContexts, "[optional] contexts in the kubeconfig file to remove the ingress from")
	cmd.Flags().StringVarP(&options.AccessToken, "access-token", "t", options.AccessToken, "[optional] access token for gcp resources (defaults to GOOGLE_APPLICATION_CREDENTIALS).")
	cmd.Flags().StringVarP(&options.GCPProject, "gcp-project", "", options.GCPProject, "[required] name of the gcp project")
	cmd.Flags().BoolVarP(&options.ForceUpdate, "force", "f", options.ForceUpdate, "[optional] overwrite existing settings if they are different")
	cmd.Flags().StringVarP(&options.Namespace, "namespace", "n", options.Namespace, "[optional] namespace for the ingress only if left unspecified by ingress spec")
	return nil
}

func validateRemoveClustersArgs(options *removeClustersOptions, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("unexpected args: %v. Expected one arg as name of load balancer", args)
	}
	// Verify that the required options are not missing.
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

// runRemoveClusters removes the given load balancer from the given list of clusters.
func runRemoveClusters(options *removeClustersOptions, args []string) error {
	options.LBName = args[0]

	// Unmarshal the YAML into ingress struct.
	var ing v1beta1.Ingress
	if err := ingress.UnmarshallAndApplyDefaults(options.IngressFilename, options.Namespace, &ing); err != nil {
		return fmt.Errorf("error in unmarshalling the yaml file %s, err: %s", options.IngressFilename, err)
	}
	cloudInterface, err := cloudinterface.NewGCECloudInterface(options.GCPProject, options.AccessToken)
	if err != nil {
		err := fmt.Errorf("error in creating cloud interface: %s", err)
		fmt.Println(err)
		return err
	}
	// Get clients for all clusters
	clients, err := kubeutils.GetClients(options.KubeconfigFilename, options.KubeContexts)
	if err != nil {
		return err
	}

	lbs, err := gcplb.NewLoadBalancerSyncer(options.LBName, clients, cloudInterface, options.GCPProject)
	if err != nil {
		return err
	}
	if delErr := lbs.RemoveFromClusters(&ing, clients, options.ForceUpdate); delErr != nil {
		if !options.ForceUpdate {
			return delErr
		}
		fmt.Printf("%s. Continuing with force remove\n", delErr)
		err = multierror.Append(err, delErr)
	}

	// Delete ingress resource from clusters
	is := ingress.NewIngressSyncer()
	if is == nil {
		err = multierror.Append(err, fmt.Errorf("unexpected ingress syncer is nil"))
		// No point in proceeding.
		return err
	}
	isErr := is.DeleteIngress(&ing, clients)
	if isErr != nil {
		err = multierror.Append(err, isErr)
	}
	return err
}
