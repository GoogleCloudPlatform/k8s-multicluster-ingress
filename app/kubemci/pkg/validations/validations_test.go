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

	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/ingress"
	"github.com/golang/glog"

	"k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/version"
	fakediscovery "k8s.io/client-go/discovery/fake"
	kubeclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	coretesting "k8s.io/client-go/testing"
)

func addServiceReactor(client *fake.Clientset, nodePort int64) {
	client.AddReactor("get", "services", func(action coretesting.Action) (handled bool, ret runtime.Object, err error) {
		glog.V(2).Infof("fake.Client.Get.Services.")
		ret = &v1.Service{
			Spec: v1.ServiceSpec{
				Ports: []v1.ServicePort{
					{
						Port:     80,
						NodePort: int32(nodePort),
					},
				},
			},
		}
		return true, ret, nil
	})
}

func TestServicesNodePortsSameFails(t *testing.T) {
	ing := v1beta1.Ingress{}
	if err := ingress.UnmarshallAndApplyDefaults("../../../../testdata/ingress.yaml", "" /*namespace*/, &ing); err != nil {
		t.Fatalf("%s", err)
	}

	client1 := &fake.Clientset{}
	client2 := &fake.Clientset{}
	addServiceReactor(client1, 31000)
	addServiceReactor(client2, 31000)

	clients := map[string]kubeclient.Interface{
		"cluster1": client1,
		"cluster2": client2,
	}

	// Test where validation passes.
	err := servicesNodePortsSame(clients, &ing)
	glog.Infof("servicesNodePortsSame = %+v", err)
	if err != nil {
		t.Errorf("Validation should pass. Got err: %v", err)
	}

	// Test where validation fails
	client3 := &fake.Clientset{}
	addServiceReactor(client3, 31005)
	clients = map[string]kubeclient.Interface{
		"cluster2": client2,
		"cluster3": client3,
	}
	err = servicesNodePortsSame(clients, &ing)
	glog.Infof("servicesNodePortsSame = %+v", err)
	if err == nil {
		t.Errorf("Validation should pass. Got err: %v", err)
	}

}

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
		{"v1.9.3-gke.0", false, 1, 9, 3},
		// Error cases:
		// Doesn't have 3 numbers in dotted version:
		{"v0.8.a", true, 0, 0, 0},
		// negative number in version:
		{"v1.-2.3", true, 0, 0, 0},
		// some prefix to the version:
		{"blah v1.9.1", true, 0, 0, 0},
		// no leading "v":
		{"1.10.1", true, 0, 0, 0},
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

		// 1.10.0 was buggy and doesn't work: kubernetes/ingress-gce#182.
		{1, 10, 0, false},
	}
	for _, tt := range versionTests {
		if newEnough := serverVersionNewEnough(tt.major, tt.minor, tt.patch); tt.newEnough != newEnough {
			t.Errorf("ServerVerNewEnough(%d, %d, %d): Expected newEnough? %v. Got newEnough:%v",
				tt.major, tt.minor, tt.patch, tt.newEnough, newEnough)
		}
	}
}

func TestVersionsAcrossClusters(t *testing.T) {
	var versionTests = []struct {
		version string
		isErr   bool
	}{
		{"v1.7.0", true},
		{"v1.8.1", false},
		{"v1.9.3-gke.0", false}, // Test a GKE version string.
		// bad input string:
		{"v1.bad.data.0", true},
	}

	for _, tt := range versionTests {
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

		err := ServerVersionsNewEnough(clients)
		if tt.isErr != (err != nil) {
			t.Errorf("error testing version. Expected err? %v Err:%v", tt.isErr, err)
		}
	}
}
