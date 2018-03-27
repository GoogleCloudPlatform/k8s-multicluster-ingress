// Copyright 2018 Google Inc.
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

// TestValidateRemoveClustersArgs verifies that validateRemoveClustersArgs validates required args as expected.
// This tests all flags except `--gcp-project`, which is tested by the next test.
func TestValidateRemoveClustersArgs(t *testing.T) {
	// validateRemoveClustersArgs should return an error with empty options.
	options := removeClustersOptions{}
	if err := validateRemoveClustersArgs(&options, []string{}); err == nil {
		t.Errorf("Expected error for emtpy options")
	}

	// validateRemoveClustersArgs should return an error with missing load balancer name.
	options = removeClustersOptions{
		IngressFilename:    "ingress.yaml",
		GCPProject:         "gcp-project",
		KubeconfigFilename: "kubeconfig",
	}
	if err := validateRemoveClustersArgs(&options, []string{}); err == nil {
		t.Errorf("Expected error for missing load balancer name")
	}

	// validateRemoveClustersArgs should return an error with missing ingress.
	options = removeClustersOptions{
		GCPProject:         "gcp-project",
		KubeconfigFilename: "kubeconfig",
	}
	if err := validateRemoveClustersArgs(&options, []string{"lbname"}); err == nil {
		t.Errorf("Expected error for missing ingress")
	}

	// validateRemoveClustersArgs should return an error with missing kubeconfig.
	options = removeClustersOptions{
		IngressFilename: "ingress.yaml",
		GCPProject:      "gcp-project",
	}
	if err := validateRemoveClustersArgs(&options, []string{"lbname"}); err == nil {
		t.Errorf("Expected error for missing kubeconfig")
	}

	// validateRemoveClustersArgs should succeed when all arguments are passed as expected.
	options = removeClustersOptions{
		IngressFilename:    "ingress.yaml",
		GCPProject:         "gcp-project",
		KubeconfigFilename: "kubeconfig",
	}
	if err := validateRemoveClustersArgs(&options, []string{"lbname"}); err != nil {
		t.Errorf("unexpected error from validateRemoveClustersArgs: %s", err)
	}
}

// TestValidateRemoveClustersArgs verifies that validateRemoveClustersArgs validates --gcp-project flag as expected.
// This is tested separately than other flags since this requires extra logic to mock gcloud commands execution.
func TestValidateRemoveClustersWithGCPProject(t *testing.T) {
	mockProject := ""
	kubeutils.ExecuteCommand = func(args []string) (string, error) {
		if strings.Join(args, " ") == "gcloud config get-value project" {
			return mockProject, nil
		}
		return "", nil
	}

	// validateRemoveClustersArgs should return an error with missing gcp project.
	options := removeClustersOptions{
		IngressFilename:    "ingress.yaml",
		KubeconfigFilename: "kubeconfig",
	}
	if err := validateRemoveClustersArgs(&options, []string{"lbname"}); err == nil {
		t.Errorf("Expected error for missing gcp project")
	}

	// validateRemoveClustersArgs should succeed when all arguments are passed as expected.
	options = removeClustersOptions{
		IngressFilename:    "ingress.yaml",
		GCPProject:         "gcp-project",
		KubeconfigFilename: "kubeconfig",
	}
	if err := validateRemoveClustersArgs(&options, []string{"lbname"}); err != nil {
		t.Errorf("unexpected error from validateRemoveClustersArgs: %s", err)
	}

	// validateRemoveClustersArgs should succeed when gcp project is passed via gcloud.
	mockProject = "mock-project"
	options = removeClustersOptions{
		IngressFilename:    "ingress.yaml",
		KubeconfigFilename: "kubeconfig",
	}
	if err := validateRemoveClustersArgs(&options, []string{"lbname"}); err != nil {
		t.Errorf("unexpected error from validateRemoveClustersArgs: %s", err)
	}
}
