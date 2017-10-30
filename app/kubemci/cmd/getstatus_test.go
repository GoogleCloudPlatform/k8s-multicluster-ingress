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

func TestValidateGetStatusArgs(t *testing.T) {
	// validateGetStatusArgs should return an error with empty options.
	options := GetStatusOptions{}
	if err := validateGetStatusArgs(&options, []string{}); err == nil {
		t.Errorf("Expected error for emtpy options")
	}

	// validateGetStatusArgs should return an error with missing load balancer name.
	options = GetStatusOptions{
		GCPProject: "gcp-project",
	}
	if err := validateGetStatusArgs(&options, []string{}); err == nil {
		t.Errorf("Expected error for missing load balancer name")
	}

	// validateGetStatusArgs should return an error with missing gcp project.
	options = GetStatusOptions{}
	if err := validateGetStatusArgs(&options, []string{"lbname"}); err == nil {
		t.Errorf("Expected error for missing gcp-project")
	}

	// validateGetStatusArgs should succeed when all arguments are passed as expected.
	options = GetStatusOptions{
		GCPProject: "gcp-project",
	}
	if err := validateGetStatusArgs(&options, []string{"lbname"}); err != nil {
		t.Errorf("unexpected error from validateGetStatusArgs: %s", err)
	}
}
