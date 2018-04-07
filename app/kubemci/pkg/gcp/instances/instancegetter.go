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

package instances

import (
	"context"
	"fmt"
	"net/http"

	"golang.org/x/oauth2/google"
	compute "google.golang.org/api/compute/v1"

	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/utils"
)

// NewInstanceGetter returns an InstanceGetter implementation.
func NewInstanceGetter(projectID string) (InstanceGetterInterface, error) {
	ctx := context.Background()
	client, err := google.DefaultClient(ctx, compute.CloudPlatformScope)
	if err != nil {
		return nil, fmt.Errorf("error in instantiating GCP client: %s", err)
	}
	return &InstanceGetter{
		client:       client,
		gcpProjectID: projectID,
	}, nil
}

// InstanceGetter is an implementation of InstanceGetterInterface.
type InstanceGetter struct {
	gcpProjectID string
	client       *http.Client
}

// Ensure this implements InstanceGetterInterface
var _ InstanceGetterInterface = &InstanceGetter{}

// GetInstance gets an instance in the given instance group.
func (g *InstanceGetter) GetInstance(igURL string) (*compute.Instance, error) {
	service, err := compute.New(g.client)
	if err != nil {
		return nil, fmt.Errorf("error in instantiating GCP client: %s", err)
	}
	// First get an instance from this instance group.
	igZone, igName, err := utils.GetZoneAndNameFromIGURL(igURL)
	if err != nil {
		return nil, err
	}
	// TODO: Handle paging if we care about more than one instance.
	res, err := service.InstanceGroups.ListInstances(g.gcpProjectID, igZone, igName, &compute.InstanceGroupsListInstancesRequest{}).Do()
	if err != nil {
		return nil, fmt.Errorf("error in fetching instances in instance group %s/%s: %s", igZone, igName, err)
	}
	if len(res.Items) == 0 {
		return nil, fmt.Errorf("no instance found in instance group %s/%s", igZone, igName)
	}
	// Fetch the instance to get its network tags.
	iZone, iName, err := utils.GetZoneAndNameFromInstanceURL(res.Items[0].Instance)
	if err != nil {
		return nil, fmt.Errorf("error in parsing an instance Url %s: %s", res.Items[0].Instance, err)
	}
	instance, err := service.Instances.Get(g.gcpProjectID, iZone, iName).Do()
	if err != nil {
		return nil, fmt.Errorf("error in fetching instance %s/%s: %s", iZone, iName, err)
	}
	return instance, nil
}
