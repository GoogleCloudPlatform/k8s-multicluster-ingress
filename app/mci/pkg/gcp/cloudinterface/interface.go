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

package cloudinterface

import (
	"k8s.io/kubernetes/pkg/cloudprovider/providers/gce"
)

func NewGCECloudInterface(projectID string) (*gce.GCECloud, error) {
	config := getCloudConfig(projectID)
	return gce.CreateGCECloud(&config)
}

func getCloudConfig(projectID string) gce.CloudConfig {
	return gce.CloudConfig{
		ProjectID: projectID,
		// TODO(nikhiljindal): Set the following properties.
		// ApiEndpoint
		// NetworkProjectID: Is different than project ID for projects with XPN enabled.
		// Zone, Region
		// NetworkName, SubnetworkName
	}
}
