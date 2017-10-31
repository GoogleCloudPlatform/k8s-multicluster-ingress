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
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
)

func TestValidateDeleteArgs(t *testing.T) {
	// validateDeleteArgs should return an error with empty options.
	options := DeleteOptions{}
	if err := validateDeleteArgs(&options, []string{}); err == nil {
		t.Errorf("Expected error for emtpy options")
	}

	// validateDeleteArgs should return an error with missing load balancer name.
	options = DeleteOptions{
		IngressFilename: "ingress.yaml",
		GCPProject:      "gcp-project",
	}
	if err := validateDeleteArgs(&options, []string{}); err == nil {
		t.Errorf("Expected error for missing load balancer name")
	}

	// validateDeleteArgs should return an error with missing ingress.
	options = DeleteOptions{
		GCPProject: "gcp-project",
	}
	if err := validateDeleteArgs(&options, []string{"lbname"}); err == nil {
		t.Errorf("Expected error for missing ingress")
	}

	// validateDeleteArgs should return an error with missing gcp project.
	options = DeleteOptions{
		IngressFilename: "ingress.yaml",
	}
	if err := validateDeleteArgs(&options, []string{"lbname"}); err == nil {
		t.Errorf("Expected error for missing gcp project")
	}

	// validateDeleteArgs should succeed when all arguments are passed as expected.
	options = DeleteOptions{
		IngressFilename: "ingress.yaml",
		GCPProject:      "gcp-project",
	}
	if err := validateDeleteArgs(&options, []string{"lbname"}); err != nil {
		t.Errorf("unexpected error from validateDeleteArgs: %s", err)
	}
}

func TestDeleteIngress(t *testing.T) {
	fakeClient := fake.Clientset{}
	fakeClient.AddReactor("delete", "ingresses", func(action core.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, nil
	})

	runFn := func() ([]string, error) {
		return []string{}, deleteIngress("kubeconfig", "../../../testdata/ingress.yaml")
	}
	expectedCommands := []ExpectedCommand{
		{
			Args:   []string{"kubectl", "--kubeconfig=kubeconfig", "config", "get-contexts", "-o=name"},
			Output: "context-1\ncontext-2",
			Err:    nil,
		},
	}
	if _, err := run(&fakeClient, expectedCommands, runFn); err != nil {
		t.Errorf("%s", err)
	}

	actions := fakeClient.Actions()
	if len(actions) != 2 {
		t.Errorf("Expected 2 actions: delete ingress 1, delete ingress 2. Got:%v", actions)
	}
	if !actions[0].Matches("delete", "ingresses") {
		t.Errorf("Expected ingress deletion.")
	}
	if !actions[1].Matches("delete", "ingresses") {
		t.Errorf("Expected ingress deletion.")
	}
	// TODO(nikhiljindal): Test a failure case.
}
