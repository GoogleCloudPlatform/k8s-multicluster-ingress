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

	"github.com/golang/glog"
	"github.com/hashicorp/go-multierror"
	"github.com/spf13/cobra"
	"k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeclient "k8s.io/client-go/kubernetes"

	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/cloudinterface"
	gcplb "github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/loadbalancer"
	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/kubeutils"
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
	// Path to kubeconfig file.
	KubeconfigFilename string
	// Names of the contexts to use from the kubeconfig file.
	KubeContexts []string
	// Name of the load balancer.
	// Required.
	LBName string
	// Name of the GCP project in which the load balancer should be configured.
	// Required
	// TODO(nikhiljindal): This should be optional. Figure it out from gcloud settings.
	GCPProject string
	// Name of the namespace for the ingress when none is provided (mismatch of option with spec causes an error).
	// Optional.
	Namespace string
}

func NewCmdDelete(out, err io.Writer) *cobra.Command {
	var options DeleteOptions

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

func addDeleteFlags(cmd *cobra.Command, options *DeleteOptions) error {
	cmd.Flags().StringVarP(&options.IngressFilename, "ingress", "i", options.IngressFilename, "[required] filename containing ingress spec")
	cmd.Flags().StringVarP(&options.KubeconfigFilename, "kubeconfig", "k", options.KubeconfigFilename, "[required] path to kubeconfig file")
	cmd.Flags().StringSliceVar(&options.KubeContexts, "kubecontexts", options.KubeContexts, "[optional] contexts in the kubeconfig file to delete the ingress from")
	// TODO(nikhiljindal): Add a short flag "-p" if it seems useful.
	cmd.Flags().StringVarP(&options.GCPProject, "gcp-project", "", options.GCPProject, "[required] name of the gcp project")
	cmd.Flags().StringVarP(&options.Namespace, "namespace", "n", options.Namespace, "[optional] namespace for the ingress only if left unspecified by ingress spec")
	// TODO Add a verbose flag that turns on glog logging.
	return nil
}

func validateDeleteArgs(options *DeleteOptions, args []string) error {
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
	if options.KubeconfigFilename == "" {
		return fmt.Errorf("unexpected missing argument kubeconfig.")
	}
	return nil
}

func runDelete(options *DeleteOptions, args []string) error {
	options.LBName = args[0]

	// Unmarshal the YAML into ingress struct.
	var ing v1beta1.Ingress
	if err := unmarshallAndApplyDefaults(options.IngressFilename, options.Namespace, &ing); err != nil {
		return fmt.Errorf("error in unmarshalling the yaml file %s, err: %s", options.IngressFilename, err)
	}
	cloudInterface, err := cloudinterface.NewGCECloudInterface(options.GCPProject)
	if err != nil {
		return fmt.Errorf("error in creating cloud interface: %s", err)
	}

	// Delete ingress from all clusters.
	clients, delErr := deleteIngress(options.KubeconfigFilename, options.KubeContexts, &ing)
	if delErr != nil {
		err = multierror.Append(err, delErr)
	}

	lbs, err := gcplb.NewLoadBalancerSyncer(options.LBName, clients, cloudInterface, options.GCPProject)
	if err != nil {
		return err
	}
	if delErr := lbs.DeleteLoadBalancer(&ing); delErr != nil {
		err = multierror.Append(err, delErr)
	}
	return err
}

// Extracts the contexts from the given kubeconfig and deletes ingress in those context clusters.
func deleteIngress(kubeconfig string, kubeContexts []string, ing *v1beta1.Ingress) (map[string]kubeclient.Interface, error) {
	clients, err := kubeutils.GetClients(kubeconfig, kubeContexts)
	if err != nil {
		return nil, err
	}
	return clients, deleteIngressInClusters(ing, clients)
}

// Deletes the given ingress from all clusters corresponding to the given clients.
func deleteIngressInClusters(ing *v1beta1.Ingress, clients map[string]kubeclient.Interface) error {
	var err error
	for cluster, client := range clients {
		glog.V(4).Infof("Deleting Ingress from cluster: %v...", cluster)
		glog.V(3).Infof("Using namespace %s for ingress %s", ing.Namespace, ing.Name)
		deleteErr := client.Extensions().Ingresses(ing.Namespace).Delete(ing.Name, &metav1.DeleteOptions{})
		glog.V(2).Infof("Error in deleting ingress %s: %s", ing.Name, deleteErr)
		if deleteErr != nil {
			if errors.IsNotFound(err) {
				fmt.Println("Ingress doesnt exist; moving on.")
				continue
			} else {
				err = multierror.Append(err, fmt.Errorf("Error in deleting ingress from cluster %s: %s", cluster, deleteErr))
			}
		}
	}
	return err
}
