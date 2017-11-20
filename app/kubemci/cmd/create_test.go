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
	"reflect"
	"sort"
	"testing"

	"k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	kubeclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
)

func TestValidateCreateArgs(t *testing.T) {
	// validateCreateArgs should return an error with empty options.
	options := CreateOptions{}
	if err := validateCreateArgs(&options, []string{}); err == nil {
		t.Errorf("Expected error for emtpy options")
	}

	// validateCreateArgs should return an error with missing load balancer name.
	options = CreateOptions{
		IngressFilename:    "ingress.yaml",
		GCPProject:         "gcp-project",
		KubeconfigFilename: "kubeconfig",
	}
	if err := validateCreateArgs(&options, []string{}); err == nil {
		t.Errorf("Expected error for missing load balancer name")
	}

	// validateCreateArgs should return an error with missing ingress.
	options = CreateOptions{
		GCPProject:         "gcp-project",
		KubeconfigFilename: "kubeconfig",
	}
	if err := validateCreateArgs(&options, []string{"lbname"}); err == nil {
		t.Errorf("Expected error for missing ingress")
	}

	// validateCreateArgs should return an error with missing gcp project.
	options = CreateOptions{
		IngressFilename:    "ingress.yaml",
		KubeconfigFilename: "kubeconfig",
	}
	if err := validateCreateArgs(&options, []string{"lbname"}); err == nil {
		t.Errorf("Expected error for missing gcp project")
	}

	// validateCreateArgs should return an error with missing kubeconfig.
	options = CreateOptions{
		IngressFilename: "ingress.yaml",
		GCPProject:      "gcp-project",
	}
	if err := validateCreateArgs(&options, []string{"lbname"}); err == nil {
		t.Errorf("Expected error for missing kubeconfig")
	}

	// validateCreateArgs should succeed when all arguments are passed as expected.
	options = CreateOptions{
		IngressFilename:    "ingress.yaml",
		GCPProject:         "gcp-project",
		KubeconfigFilename: "kubeconfig",
	}
	if err := validateCreateArgs(&options, []string{"lbname"}); err != nil {
		t.Errorf("unexpected error from validateCreateArgs: %s", err)
	}
}

func TestCreateIngressInClusters(t *testing.T) {
	fakeClient := fake.Clientset{}
	fakeClient.AddReactor("create", "ingresses", func(action core.Action) (handled bool, ret runtime.Object, err error) {
		return true, action.(core.CreateAction).GetObject(), nil
	})
	clients := map[string]kubeclient.Interface{
		"cluster1": &fakeClient,
		"cluster2": &fakeClient,
	}
	var ing v1beta1.Ingress
	if err := unmarshallAndApplyDefaults("../../../testdata/ingress.yaml", "", &ing); err != nil {
		t.Fatalf("%s", err)
	}
	// Verify that on calling createIngressInClusters, fakeClient sees 2 actions to create the ingress.
	clusters, err := createIngressInClusters(&ing, clients)
	if err != nil {
		t.Fatalf("%s", err)
	}
	actions := fakeClient.Actions()
	if len(actions) != 2 {
		t.Errorf("Expected 2 actions: Create Ingress 1, Create Ingress 2. Got:%v", actions)
	}
	if !actions[0].Matches("create", "ingresses") {
		t.Errorf("Expected ingress creation.")
	}
	if !actions[1].Matches("create", "ingresses") {
		t.Errorf("Expected ingress creation.")
	}
	expectedClusters := []string{"cluster1", "cluster2"}
	sort.Strings(clusters)
	if !reflect.DeepEqual(clusters, expectedClusters) {
		t.Errorf("unexpected list of clusters, expected: %v, got: %v", expectedClusters, clusters)
	}
	// TODO(G-Harmon): Verify that the ingress matches testdata/ingress.yaml
	// TODO(nikhiljindal): Also add tests for error cases including one for already exists.
}
