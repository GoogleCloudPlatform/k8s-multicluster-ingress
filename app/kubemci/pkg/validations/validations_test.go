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

package validations

import (
	"testing"

	"github.com/golang/glog"
	"k8s.io/apimachinery/pkg/version"
	fakediscovery "k8s.io/client-go/discovery/fake"
	kubeclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
)

func TestParseVersion(t *testing.T) {
	var parseTests = []struct {
		in    string
		isErr bool
		major uint64
		minor uint64
		patch uint64
	}{
		{"v1.2.3", false, 1, 2, 3},
		{"v10.200.3000", false, 10, 200, 3000},
		{"v0.8.a", true, 0, 0, 0},
		{"v1.-2.3", true, 1, 2, 3},
	}
	for _, tt := range parseTests {
		major, minor, patch, err := parseVersion(tt.in)
		if tt.isErr != (err != nil) {
			t.Errorf("parseVersion(%s): Want err:%v. Got:%v", tt.in, tt.isErr, err)
		}
		if tt.isErr {
			continue
		}
		if major != tt.major {
			t.Errorf("parseVersion(%s): Major: Want:%v. Got:%v", tt.in, tt.major, major)
		}
		if minor != tt.minor {
			t.Errorf("parseVersion(%s): Minor: Want:%v. Got:%v", tt.in, tt.minor, minor)
		}
		if patch != tt.patch {
			t.Errorf("parseVersion(%s): Patch: Want:%v. Got:%v", tt.in, tt.patch, patch)
		}

	}
}

func TestServerVersionNewEnough(t *testing.T) {
	var versionTests = []struct {
		major     uint64
		minor     uint64
		patch     uint64
		newEnough bool
	}{
		// Test major
		{0, 9, 10, false},
		{2, 0, 0, true},
		// Test minor
		{1, 7, 0, false},
		{1, 7, 14, false},
		{1, 9, 0, true},
		// Test patch.
		{1, 8, 0, false},
		{1, 8, 1, true},

		// 1.10.0 was buggy and doesn't work.
		{1, 10, 0, false},
	}
	for _, tt := range versionTests {
		ok, err := serverVersionNewEnough(tt.major, tt.minor, tt.patch)
		if err != nil {
			t.Errorf("ServerVerNewEnough(%d, %d, %d): Expected no err. Got err %v", tt.major, tt.minor, tt.patch, err)
		}
		if tt.newEnough != ok {
			t.Errorf("ServerVerNewEnough(%d, %d, %d): Expected newEnough? %v. Got newEnough:%v", tt.major, tt.minor, tt.patch, tt.newEnough, ok)
		}
	}
}

func TestVersionsAcrossClusters(t *testing.T) {

	var versionTests = []struct {
		version      string
		isErr        bool
		allNewEnough bool
	}{
		{"v1.8.1", false, true},
		{"v1.7.0", false, false},
		{"v1.9.3-gke.0", false, true}, // Test a GKE version string.
		{"v1.bad.data.0", true, false},
	}

	for _, tt := range versionTests {
		glog.Infof("start")
		clients := make(map[string]kubeclient.Interface)
		clients["cluster1"] = fake.NewSimpleClientset()

		fakeclientDiscovery, ok := clients["cluster1"].Discovery().(*fakediscovery.FakeDiscovery)
		if !ok {
			glog.Errorf("couldn't set fake discovery's server version")
			return
		}
		glog.Infof("fakeclient.discovery: %+v", fakeclientDiscovery)
		var verInfo version.Info
		verInfo.GitVersion = tt.version
		fakeclientDiscovery.FakedServerVersion = &verInfo

		newEnough, err := ServerVersionsNewEnough(clients)
		glog.Errorf("newEnough? %v err: %v", newEnough, err)
		if tt.isErr != (err != nil) {
			t.Errorf("error testing version. Expected err? %v Err:%v", tt.isErr, err)
		}
		if tt.allNewEnough != newEnough {
			t.Errorf("Error with versions. Expected all new enough? %v Got:%v", tt.allNewEnough, newEnough)
		}
	}
}
