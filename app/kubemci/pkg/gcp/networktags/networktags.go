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

package networktags

import (
	"context"
	"fmt"
	"net/http"

	"golang.org/x/oauth2/google"
	"google.golang.org/api/compute/v1"

	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/utils"
)

func NewNetworkTagsGetter(projectID string) (NetworkTagsGetterInterface, error) {
	ctx := context.Background()
	client, err := google.DefaultClient(ctx, compute.CloudPlatformScope)
	if err != nil {
		return nil, fmt.Errorf("error in instantiating GCP client: %s", err)
	}
	return &NetworkTagsGetter{
		client:       client,
		gcpProjectID: projectID,
	}, nil
}

type NetworkTagsGetter struct {
	gcpProjectID string
	client       *http.Client
}

// Ensure this implements NetworkTagsGetterInterface
var _ NetworkTagsGetterInterface = &NetworkTagsGetter{}

func (g *NetworkTagsGetter) GetNetworkTags(igUrl string) ([]string, error) {
	service, err := compute.New(g.client)
	if err != nil {
		return nil, fmt.Errorf("error in instantiating GCP client: %s", err)
	}
	// First get an instance from this instance group.
	igZone, igName, err := utils.GetZoneAndNameFromIGUrl(igUrl)
	if err != nil {
		return nil, err
	}
	res, err := service.InstanceGroups.ListInstances(g.gcpProjectID, igZone, igName, &compute.InstanceGroupsListInstancesRequest{}).Do()
	if err != nil {
		return nil, fmt.Errorf("error in fetching instances in instance group %s/%s: %s", igZone, igName, err)
	}
	if len(res.Items) == 0 {
		return nil, fmt.Errorf("no instance found in instance group %s/%s", igZone, igName)
	}
	// Fetch the instance to get its network tags.
	iZone, iName, err := utils.GetZoneAndNameFromInstanceUrl(res.Items[0].Instance)
	if err != nil {
		return nil, fmt.Errorf("error in parsing an instance Url %s: %s", res.Items[0].Instance, err)
	}
	instance, err := service.Instances.Get(g.gcpProjectID, iZone, iName).Do()
	if err != nil {
		return nil, fmt.Errorf("error in fetching instance %s/%s: %s", iZone, iName, err)
	}
	if len(instance.Tags.Items) == 0 {
		return nil, fmt.Errorf("no network tag found on instance %s/%s", iZone, iName)
	}
	return instance.Tags.Items, nil
}
