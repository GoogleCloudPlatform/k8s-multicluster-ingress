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
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/kubeutils"
)

// Test to verify validate.
// This tests all flags except `--gcp-project`, which is tested by the next test.
func TestValidateCreateArgs(t *testing.T) {
	// validateCreateArgs should return an error with empty options.
	options := createOptions{}
	if err := validateCreateArgs(&options, []string{}); err == nil {
		t.Errorf("Expected error for emtpy options")
	}

	// validateCreateArgs should return an error with missing load balancer name.
	options = createOptions{
		IngressFilename:    "ingress.yaml",
		GCPProject:         "gcp-project",
		KubeconfigFilename: "kubeconfig",
	}
	if err := validateCreateArgs(&options, []string{}); err == nil {
		t.Errorf("Expected error for missing load balancer name")
	}

	// validateCreateArgs should return an error with missing ingress.
	options = createOptions{
		GCPProject:         "gcp-project",
		KubeconfigFilename: "kubeconfig",
	}
	if err := validateCreateArgs(&options, []string{"lbname"}); err == nil {
		t.Errorf("Expected error for missing ingress")
	}

	// validateCreateArgs should return an error with missing kubeconfig.
	options = createOptions{
		IngressFilename: "ingress.yaml",
		GCPProject:      "gcp-project",
	}
	if err := validateCreateArgs(&options, []string{"lbname"}); err == nil {
		t.Errorf("Expected error for missing kubeconfig")
	}

	// validateCreateArgs should succeed when all arguments are passed as expected.
	options = createOptions{
		IngressFilename:    "ingress.yaml",
		GCPProject:         "gcp-project",
		KubeconfigFilename: "kubeconfig",
	}
	if err := validateCreateArgs(&options, []string{"lbname"}); err != nil {
		t.Errorf("unexpected error from validateCreateArgs: %s", err)
	}
}

// Test to verify validate for `--gcp-project` flag.
// This is tested separately since this requires extra logic to mock gcloud commands execution.
func TestValidateCreateWithGCPProject(t *testing.T) {
	mockProject := ""
	kubeutils.ExecuteCommand = func(args []string) (string, error) {
		if strings.Join(args, " ") == "gcloud config get-value project" {
			return mockProject, nil
		}
		return "", nil
	}

	// validateCreateArgs should return an error with missing gcp project.
	options := createOptions{
		IngressFilename:    "ingress.yaml",
		KubeconfigFilename: "kubeconfig",
	}
	if err := validateCreateArgs(&options, []string{"lbname"}); err == nil {
		t.Errorf("Expected error for missing gcp project")
	}

	// validateCreateArgs should succeed when all arguments are passed as expected.
	options = createOptions{
		IngressFilename:    "ingress.yaml",
		GCPProject:         "gcp-project",
		KubeconfigFilename: "kubeconfig",
	}
	if err := validateCreateArgs(&options, []string{"lbname"}); err != nil {
		t.Errorf("unexpected error from validateCreateArgs: %s", err)
	}

	// validateCreateArgs should succeed when gcp project is passed via gcloud.
	mockProject = "mock-project"
	options = createOptions{
		IngressFilename:    "ingress.yaml",
		KubeconfigFilename: "kubeconfig",
	}
	if err := validateCreateArgs(&options, []string{"lbname"}); err != nil {
		t.Errorf("unexpected error from validateCreateArgs: %s", err)
	}
}
