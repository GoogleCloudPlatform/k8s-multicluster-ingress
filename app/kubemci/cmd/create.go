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

	"github.com/ghodss/yaml"
	"github.com/golang/glog"
	multierror "github.com/hashicorp/go-multierror"
	"github.com/spf13/cobra"
	"k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	kubeclient "k8s.io/client-go/kubernetes"
	"k8s.io/ingress-gce/pkg/annotations"
	// gcp is needed for GKE cluster auth to work.
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"

	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/cloudinterface"
	gcplb "github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/loadbalancer"
	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/kubeutils"
)

var (
	createShortDescription = "Create a multicluster ingress."
	createLongDescription  = `Create a multicluster ingress.

	Takes an ingress spec and a list of clusters and creates a multicluster ingress targetting those clusters.
	`
	defaultIngressNamespace = "default"
)

type CreateOptions struct {
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
	// Overwrite values when they differ from what's requested. If
	// the resource does not exist, or is already the correct
	// value, then 'force' is a no-op.
	ForceUpdate bool
	// Name of the namespace for the ingress when none is provided (mismatch of option with spec causes an error).
	// Optional.
	Namespace string
	// Static IP Name for the ingress when none is provided (mismatch of option with spec causes an error).
	// Optional.
	StaticIPName string
}

func NewCmdCreate(out, err io.Writer) *cobra.Command {
	var options CreateOptions

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

func addCreateFlags(cmd *cobra.Command, options *CreateOptions) error {
	cmd.Flags().StringVarP(&options.IngressFilename, "ingress", "i", options.IngressFilename, "[required] filename containing ingress spec")
	cmd.Flags().StringVarP(&options.KubeconfigFilename, "kubeconfig", "k", options.KubeconfigFilename, "[required] path to kubeconfig file")
	cmd.Flags().StringSliceVar(&options.KubeContexts, "kubecontexts", options.KubeContexts, "[optional] contexts in the kubeconfig file to install the ingress into")
	// TODO(nikhiljindal): Add a short flag "-p" if it seems useful.
	cmd.Flags().StringVarP(&options.GCPProject, "gcp-project", "", options.GCPProject, "[required] name of the gcp project")
	cmd.Flags().BoolVarP(&options.ForceUpdate, "force", "f", options.ForceUpdate, "[optional] overwrite existing settings if they are different")
	cmd.Flags().StringVarP(&options.Namespace, "namespace", "n", options.Namespace, "[optional] namespace for the ingress only if left unspecified by ingress spec")
	cmd.Flags().StringVarP(&options.StaticIPName, "static-ip", "", options.StaticIPName, "[optional] Global Static IP name to use only if left unspecified by ingress spec")
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
	if options.KubeconfigFilename == "" {
		return fmt.Errorf("unexpected missing argument kubeconfig.")
	}
	return nil
}

func runCreate(options *CreateOptions, args []string) error {
	options.LBName = args[0]

	// Unmarshal the YAML into ingress struct.
	var ing v1beta1.Ingress
	if err := unmarshallAndApplyDefaults(options.IngressFilename, options.Namespace, &ing); err != nil {
		return fmt.Errorf("error in unmarshalling the yaml file %s, err: %s", options.IngressFilename, err)
	}
	ingAnnotations := annotations.IngAnnotations(ing.Annotations)
	if ingAnnotations == nil || ingAnnotations.StaticIPName() == "" {
		if options.StaticIPName == "" {
			return fmt.Errorf("ingress spec must provide a Global Static IP through annotation of %q (alternatively, use --static-ip flag)", annotations.StaticIPNameKey)
		}
		addAnnotation(&ing, annotations.StaticIPNameKey, options.StaticIPName)
	} else if options.StaticIPName != "" && ingAnnotations.StaticIPName() != options.StaticIPName {
		return fmt.Errorf("the StaticIPName from the provided ingress %q does not match --static-ip=%v. You must pass '--static-ip=%v' to perform this operation.", ingAnnotations.StaticIPName(), options.StaticIPName, ingAnnotations.StaticIPName())
	}

	cloudInterface, err := cloudinterface.NewGCECloudInterface(options.GCPProject)
	if err != nil {
		return fmt.Errorf("error in creating cloud interface: %s", err)
	}

	// Create ingress in all clusters.
	clusters, clients, err := createIngress(options.KubeconfigFilename, options.KubeContexts, &ing)
	if err != nil {
		return err
	}

	lbs, err := gcplb.NewLoadBalancerSyncer(options.LBName, clients, cloudInterface, options.GCPProject)
	if err != nil {
		return err
	}
	return lbs.CreateLoadBalancer(&ing, options.ForceUpdate, clusters)
}

// Extracts the contexts from the given kubeconfig and creates ingress in those context clusters.
// Returns the list of clusters in which it created the ingress and a map of clients for each of those clusters.
func createIngress(kubeconfig string, kubeContexts []string, ing *v1beta1.Ingress) ([]string, map[string]kubeclient.Interface, error) {
	clients, err := kubeutils.GetClients(kubeconfig, kubeContexts)
	if err != nil {
		return nil, nil, err
	}
	clusters, createErr := createIngressInClusters(ing, clients)
	return clusters, clients, createErr
}

// Creates the given ingress in all clusters corresponding to the given clients.
func createIngressInClusters(ing *v1beta1.Ingress, clients map[string]kubeclient.Interface) ([]string, error) {
	var err error
	var clusters []string
	for cluster, client := range clients {
		glog.V(4).Infof("Creating Ingress in cluster: %v...", cluster)
		clusters = append(clusters, cluster)
		glog.V(3).Infof("Using namespace %s for ingress %s", ing.Namespace, ing.Name)
		actualIng, createErr := client.Extensions().Ingresses(ing.Namespace).Create(ing)
		glog.V(2).Infof("Ingress Create returned: err:%v. Actual Ingress:%+v", createErr, actualIng)
		if createErr != nil {
			if errors.IsAlreadyExists(createErr) {
				fmt.Println("Ingress already exists; moving on.")
				continue
			} else {
				err = multierror.Append(err, fmt.Errorf("Error in creating ingress in cluster %s: %s", cluster, createErr))
			}
		}
		fmt.Println("Created Ingress for cluster:", cluster)
	}
	return clusters, err
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

// TODO Move to ingress-gce/pkg/annotations
func addAnnotation(ing *v1beta1.Ingress, key, val string) {
	if ing.Annotations == nil {
		ing.Annotations = annotations.IngAnnotations{}
	}
	ing.Annotations[key] = val
}

func unmarshallAndApplyDefaults(filename, namespace string, ing *v1beta1.Ingress) error {
	if err := unmarshall(filename, ing); err != nil {
		return err
	}
	if ing.Namespace == "" {
		if namespace == "" {
			ing.Namespace = defaultIngressNamespace
		} else {
			ing.Namespace = namespace
		}
	} else if namespace != "" && ing.Namespace != namespace {
		return fmt.Errorf("the namespace from the provided ingress %q does not match the namespace %q. You must pass '--namespace=%v' to perform this operation.", ing.Namespace, namespace, ing.Namespace)
	}
	ingAnnotations := annotations.IngAnnotations(ing.Annotations)
	class := ingAnnotations.IngressClass()
	if class == "" {
		addAnnotation(ing, annotations.IngressClassKey, annotations.GceMultiIngressClass)
		glog.V(3).Infof("Adding class annotation to be of type %v\n", annotations.GceMultiIngressClass)
	} else if class != annotations.GceMultiIngressClass {
		return fmt.Errorf("ingress class is %v, must be %v", class, annotations.GceMultiIngressClass)
	}
	return nil
}
