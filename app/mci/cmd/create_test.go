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
	"reflect"
	"strings"
	"testing"

	kubeclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
)

type ExpectedCommand struct {
	Args   []string
	Output string
	Err    error
}

func run(expectedCmds []ExpectedCommand, runFn func() error) error {
	getClientset = func(kubeconfigPath string) (kubeclient.Interface, error) {
		return &fake.Clientset{}, nil
	}

	i := 0
	executeCommand = func(args []string) (string, error) {
		if i >= len(expectedCmds) {
			return "", fmt.Errorf("unexpected command: %s", strings.Join(args, " "))
		}
		if !reflect.DeepEqual(args, expectedCmds[i].Args) {
			return "", fmt.Errorf("unexpected command: %s, was expecting   : %s", strings.Join(args, " "), strings.Join(expectedCmds[i].Args, " "))
		}
		output, err := expectedCmds[i].Output, expectedCmds[i].Err
		i++
		return output, err
	}
	err := runFn()
	if err != nil {
		return err
	}
	if i != len(expectedCmds) {
		return fmt.Errorf("expected commands not called: %s", expectedCmds[i:])
	}
	return nil
}

func TestValidateArgs(t *testing.T) {
	// ValidateArgs should return an error with empty options.
	options := CreateOptions{}
	if err := ValidateArgs(&options, []string{}); err == nil {
		t.Errorf("Expected error for emtpy options")
	}

	// ValidateArgs should return an error with missing load balancer name.
	options = CreateOptions{
		IngressFilename: "ingress.yaml",
		GCPProject:      "gcp-project",
	}
	if err := ValidateArgs(&options, []string{}); err == nil {
		t.Errorf("Expected error for missing load balancer name")
	}

	// ValidateArgs should return an error with missing ingress.
	options = CreateOptions{
		GCPProject: "gcp-project",
	}
	if err := ValidateArgs(&options, []string{"lbname"}); err == nil {
		t.Errorf("Expected error for missing ingress")
	}

	// ValidateArgs should return an error with missing gcp project.
	options = CreateOptions{
		IngressFilename: "ingress.yaml",
	}
	if err := ValidateArgs(&options, []string{"lbname"}); err == nil {
		t.Errorf("Expected error for missing gcp project")
	}

	// ValidateArgs should succeed when all arguments are passed as expected.
	options = CreateOptions{
		IngressFilename: "ingress.yaml",
		GCPProject:      "gcp-project",
	}
	if err := ValidateArgs(&options, []string{"lbname"}); err != nil {
		t.Errorf("unexpected error from ValidateArgs: %s", err)
	}
}

func TestCreateIngress(t *testing.T) {
	runFn := func() error {
		return createIngress("kubeconfig", "../../../testdata/ingress.yaml")
	}
	expectedCommands := []ExpectedCommand{
		{
			Args:   []string{"kubectl", "--kubeconfig=kubeconfig", "config", "get-contexts", "-o=name"},
			Output: "context-1\ncontext-2",
			Err:    nil,
		},
		{
			Args:   []string{"kubectl", "--kubeconfig=kubeconfig", "create", "--filename=../../../testdata/ingress.yaml", "--context=context-1"},
			Output: "ingress created",
			Err:    nil,
		},
		{
			Args:   []string{"kubectl", "--kubeconfig=kubeconfig", "create", "--filename=../../../testdata/ingress.yaml", "--context=context-2"},
			Output: "ingress created",
			Err:    nil,
		},
	}
	if err := run(expectedCommands, runFn); err != nil {
		t.Errorf("%s", err)
	}
}
