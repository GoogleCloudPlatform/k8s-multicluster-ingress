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
)

func TestValidateDeleteArgs(t *testing.T) {
	// ValidateDeleteArgs should return an error with empty options.
	options := DeleteOptions{}
	if err := ValidateDeleteArgs(&options, []string{}); err == nil {
		t.Errorf("Expected error for emtpy options")
	}

	// ValidateDeleteArgs should return an error with missing load balancer name.
	options = DeleteOptions{
		IngressFilename: "ingress.yaml",
		GCPProject:      "gcp-project",
	}
	if err := ValidateDeleteArgs(&options, []string{}); err == nil {
		t.Errorf("Expected error for missing load balancer name")
	}

	// ValidateDeleteArgs should return an error with missing ingress.
	options = DeleteOptions{
		GCPProject: "gcp-project",
	}
	if err := ValidateDeleteArgs(&options, []string{"lbname"}); err == nil {
		t.Errorf("Expected error for missing ingress")
	}

	// ValidateDeleteArgs should return an error with missing gcp project.
	options = DeleteOptions{
		IngressFilename: "ingress.yaml",
	}
	if err := ValidateDeleteArgs(&options, []string{"lbname"}); err == nil {
		t.Errorf("Expected error for missing gcp project")
	}

	// ValidateDeleteArgs should succeed when all arguments are passed as expected.
	options = DeleteOptions{
		IngressFilename: "ingress.yaml",
		GCPProject:      "gcp-project",
	}
	if err := ValidateDeleteArgs(&options, []string{"lbname"}); err != nil {
		t.Errorf("unexpected error from ValidateDeleteArgs: %s", err)
	}
}

func TestDeleteIngress(t *testing.T) {
	runFn := func() error {
		return deleteIngress("kubeconfig", "../../../testdata/ingress.yaml")
	}
	expectedCommands := []ExpectedCommand{
		{
			Args:   []string{"kubectl", "--kubeconfig=kubeconfig", "config", "get-contexts", "-o=name"},
			Output: "context-1\ncontext-2",
			Err:    nil,
		},
		{
			Args:   []string{"kubectl", "--kubeconfig=kubeconfig", "delete", "--filename=../../../testdata/ingress.yaml", "--context=context-1"},
			Output: "ingress deleted",
			Err:    nil,
		},
		{
			Args:   []string{"kubectl", "--kubeconfig=kubeconfig", "delete", "--filename=../../../testdata/ingress.yaml", "--context=context-2"},
			Output: "ingress deleted",
			Err:    nil,
		},
	}
	if err := run(expectedCommands, runFn); err != nil {
		t.Errorf("%s", err)
	}
}
