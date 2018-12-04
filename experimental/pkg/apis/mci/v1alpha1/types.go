// Copyright 2019 Google Inc.
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

package v1alpha1

import (
	core "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// MultiClusterIngress is the API resource used to create an Ingress spanning
// multiple clusters.
// The API schema here is very similar to a single cluster Ingress resource
// with the primary difference being that the service name referenced in the
// path refers to a MultiClusterService resource rather than a Service object.
// +k8s:openapi-gen=true
type MultiClusterIngress struct {
	meta_v1.TypeMeta   `json:",inline"`
	meta_v1.ObjectMeta `json:"metadata,omitempty"`
	Spec               MultiClusterIngressSpec   `json:"spec"`
	Status             MultiClusterIngressStatus `json:"status,omitempty"`
}

// MultiClusterIngressSpec is the spec for a MultiClusterIngress.
type MultiClusterIngressSpec struct {
	// Template contains the ingress spec used to create a
	// MultiClusterIngress.
	Template IngressSpecTemplate `json:"template,omitempty"`
}

// MultiClusterIngressStatus is the status of a MultiClusterIngress.
type MultiClusterIngressStatus struct {
	// VIP is the virtual IP address of the load balancer.
	VIP string
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// MultiClusterIngressList is a list of MultiClusterIngress resources
type MultiClusterIngressList struct {
	meta_v1.TypeMeta `json:",inline"`
	meta_v1.ListMeta `json:"metadata"`
	Items            []MultiClusterIngress `json:"items"`
}

// IngressSpecTemplate contains the spec for MultiClusterIngress resource.
// +k8s:openapi-gen=true
type IngressSpecTemplate struct {
	// +optional
	meta_v1.ObjectMeta `json:"metadata,omitempty"`
	Spec               extensions.IngressSpec `json:"spec"`
}

// MultiClusterService is the API resource used to create services in multiple
// clusters.
// This is replicated across multiple clusters.
// By doing cluster selection with Service instead of with Ingress, we can have
// setups where different paths on the same MultiClusterIngress can route
// traffic to multiple clusters.
// For example, /foo can route traffic to clusters A and B and /bar can
// route traffic to clusters B and C for a single MultiClusterIngress.
// +k8s:openapi-gen=true
type MultiClusterService struct {
	meta_v1.TypeMeta   `json:",inline"`
	meta_v1.ObjectMeta `json:"metadata,omitempty"`
	Spec               MultiClusterServiceSpec   `json:"spec"`
	Status             MultiClusterServiceStatus `json:"status,omitempty"`
}

// MultiClusterServiceSpec is the spec for a MultiClusterService.
type MultiClusterServiceSpec struct {
	// Spec for the service that will be created in individual clusters.
	Template ServiceSpecTemplate `json:"template,omitempty"`
	// Clusters selects the list of target clusters where the Service resource
	// should be replicated.
	Clusters []MultiClusterDestination `json:"clusters,omitempty"`
}

// MultiClusterServiceStatus is the status of a MultiClusterService.
type MultiClusterServiceStatus struct{}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// MultiClusterServiceList is a list of MultiClusterService resources
type MultiClusterServiceList struct {
	meta_v1.TypeMeta `json:",inline"`
	meta_v1.ListMeta `json:"metadata"`
	Items            []MultiClusterService `json:"items"`
}

// ServiceSpecTemplate is the spec for the service that will be created in
// individual clusters.
// +k8s:openapi-gen=true
type ServiceSpecTemplate struct {
	// +optional
	meta_v1.ObjectMeta `json:"metadata,omitempty"`
	Spec               core.ServiceSpec `json:"spec"`
}

// MultiClusterDestination selects the list of destination clusters.
type MultiClusterDestination struct {
	// Selector selects which clusters are part of this list.
	Selector *meta_v1.LabelSelector `json:"selector"`
}
