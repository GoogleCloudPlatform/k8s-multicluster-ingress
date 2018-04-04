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
	"github.com/golang/glog"
)

func TestValidateListArgs(t *testing.T) {
	// To start with, there's no gcloud GCP project set:
	mockProject := ""
	kubeutils.ExecuteCommand = func(args []string) (string, error) {
		if strings.Join(args, " ") == "gcloud config get-value project" {
			glog.V(2).Infof("Returning proj: [%s]", mockProject)
			return mockProject, nil
		}
		return "", nil
	}

	// It should return an error with extra args.
	options := listOptions{}
	if err := validateListArgs(&options, []string{"arg1"}); err == nil {
		t.Errorf("Expected error for non-empty args")
	}

	// It should return an error without project being set anywhere.
	options = listOptions{}
	if err := validateListArgs(&options, []string{}); err == nil {
		t.Errorf("Expected error for missing explicit project")
	}

	// validateListArgs should succeed with project flag and empty args.
	options = listOptions{
		GCPProject: "myGcpProject",
	}
	if err := validateListArgs(&options, []string{}); err != nil {
		t.Errorf("unexpected error from validateListArgs with explicit project: %s", err)
	}
	if options.GCPProject != "myGcpProject" {
		t.Errorf("unexpected project. Want: %s. Got:%s", "myGcpProject", options.GCPProject)
	}

	// validateListArgs should succeed with a project from gcloud and empty args.
	mockProject = "gcloud-project"
	options = listOptions{}
	if err := validateListArgs(&options, []string{}); err != nil {
		t.Errorf("unexpected error from validateListArgs with gcloud project: %s", err)
	}
	if options.GCPProject != mockProject {
		t.Errorf("unexpected project. Want: %s. Got:%s", mockProject, options.GCPProject)
	}

}
