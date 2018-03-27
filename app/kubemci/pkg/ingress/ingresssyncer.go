package ingress

import (
	"fmt"
	"io/ioutil"

	"github.com/ghodss/yaml"
	"github.com/golang/glog"
	multierror "github.com/hashicorp/go-multierror"
	"k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/diff"
	kubeclient "k8s.io/client-go/kubernetes"
	"k8s.io/ingress-gce/pkg/annotations"

	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/kubeutils"
)

var (
	defaultIngressNamespace    = "default"
	instanceGroupAnnotationKey = "ingress.gcp.kubernetes.io/instance-groups"
)

type IngressSyncer struct {
}

func NewIngressSyncer() IngressSyncerInterface {
	return &IngressSyncer{}
}

// EnsureIngress ensures that the ingress exists in all clusters given.
// Does nothing if it already exists in a cluster, else creates the resource.
// Returns a list of clusters where the ingress resource has been ensured.
func (i *IngressSyncer) EnsureIngress(ing *v1beta1.Ingress, clients map[string]kubeclient.Interface, forceUpdate bool) ([]string, error) {
	var err error
	var ensuredClusters []string
	for cluster, client := range clients {
		glog.V(4).Infof("Creating Ingress in cluster: %v...", cluster)
		ensuredClusters = append(ensuredClusters, cluster)
		glog.V(3).Infof("Using namespace %s for ingress %s", ing.Namespace, ing.Name)
		existingIng, getErr := client.ExtensionsV1beta1().Ingresses(ing.Namespace).Get(ing.Name, metav1.GetOptions{})
		if getErr != nil {
			if errors.IsNotFound(getErr) {
				actualIng, createErr := client.ExtensionsV1beta1().Ingresses(ing.Namespace).Create(ing)
				glog.V(2).Infof("Ingress Create returned: err:%v. Actual Ingress:%+v\n", createErr, actualIng)
				if createErr != nil {
					err = multierror.Append(err, fmt.Errorf("Error in creating ingress in cluster %s: %s", cluster, createErr))
				}
			} else {
				err = multierror.Append(err, fmt.Errorf("Error in checking for existing ingress in cluster %s: %s", cluster, getErr))
			}
			fmt.Println("Created Ingress in cluster:", cluster)
		} else {
			// Ignore instance group annotation while comparing ingresses.
			// Its added by the in-cluster ingress-gce controller, so the existing ingress will have it while the user provided will not.
			ignoreAnnotations := map[string]string{
				instanceGroupAnnotationKey: "",
			}
			if !kubeutils.ObjectMetaAndSpecEquivalent(ing, existingIng, ignoreAnnotations) {
				fmt.Printf("Found existing ingress resource which differs from the proposed one\n")
				glog.V(2).Infof("Diff: %v\n", diff.ObjectDiff(ing, existingIng))
				if forceUpdate {
					fmt.Println("Updating existing ingress resource to match the desired state (since --force specified)")
					updatedIng, updateErr := client.ExtensionsV1beta1().Ingresses(ing.Namespace).Update(ing)
					glog.V(2).Infof("Ingress Update returned: err:%v, Actual Ingress:%+v\n", updateErr, updatedIng)
					if updateErr != nil {
						err = multierror.Append(err, fmt.Errorf("Error in updating ingress in cluster %s: %s", cluster, updateErr))
					} else {
						fmt.Printf("Updated ingress for cluster: %v\n", cluster)
					}
				} else {
					fmt.Println("Will not overwrite the differing Ingress resource without the --force flag.")
					err = multierror.Append(fmt.Errorf("will not overwrite Ingress resource in cluster '%v' without --force", cluster))
				}
			} else {
				fmt.Println("Ingress already exists and matches; moving on.")
			}
		}
	}
	return ensuredClusters, err
}

// DeleteIngress deletes the ingress resource from all clusters.
func (i *IngressSyncer) DeleteIngress(ing *v1beta1.Ingress, clients map[string]kubeclient.Interface) error {
	var err error
	for cluster, client := range clients {
		fmt.Printf("Deleting Ingress from cluster: %v...\n", cluster)
		glog.V(3).Infof("Using namespace %s for ingress %s", ing.Namespace, ing.Name)
		deleteErr := client.ExtensionsV1beta1().Ingresses(ing.Namespace).Delete(ing.Name, &metav1.DeleteOptions{})
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

func UnmarshallAndApplyDefaults(filename, namespace string, ing *v1beta1.Ingress) error {
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
	ingAnnotations := annotations.FromIngress(ing)
	class := ingAnnotations.IngressClass()
	if class == "" {
		addAnnotation(ing, annotations.IngressClassKey, annotations.GceMultiIngressClass)
		glog.V(3).Infof("Adding class annotation to be of type %v\n", annotations.GceMultiIngressClass)
	} else if class != annotations.GceMultiIngressClass {
		return fmt.Errorf("ingress class is %v, must be %v", class, annotations.GceMultiIngressClass)
	}

	return nil
}

func ApplyStaticIP(staticIPName string, ing *v1beta1.Ingress) error {
	ingAnnotations := annotations.FromIngress(ing)

	if ingAnnotations == nil || ingAnnotations.StaticIPName() == "" {
		if staticIPName == "" {
			return fmt.Errorf("ingress spec must provide a Global Static IP through annotation of %q (alternatively, use --static-ip flag)", annotations.StaticIPNameKey)
		}
		addAnnotation(ing, annotations.StaticIPNameKey, staticIPName)
	} else if staticIPName != "" && ingAnnotations.StaticIPName() != staticIPName {
		return fmt.Errorf("the %q from the provided ingress %q does not match --static-ip=%v. You must pass '--static-ip=%v' to perform this operation.", annotations.StaticIPNameKey, ingAnnotations.StaticIPName(), staticIPName, ingAnnotations.StaticIPName())
	}

	return nil
}

// TODO Move to ingress-gce/pkg/annotations
func addAnnotation(ing *v1beta1.Ingress, key, val string) {
	if ing.Annotations == nil {
		ing.Annotations = map[string]string{}
	}
	ing.Annotations[key] = val
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
