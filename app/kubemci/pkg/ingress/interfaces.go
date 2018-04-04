package ingress

import (
	"k8s.io/api/extensions/v1beta1"
	"k8s.io/client-go/kubernetes"
)

// SyncerInterface is an interface to manage ingress resources in clusters.
type SyncerInterface interface {
	EnsureIngress(ing *v1beta1.Ingress, clients map[string]kubernetes.Interface, forceUpdate bool) ([]string, error)
	DeleteIngress(ing *v1beta1.Ingress, clients map[string]kubernetes.Interface) error
}
