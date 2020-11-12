// Copyright 2017 Google, Inc.
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
	"io"

	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/cloudinterface"
	gcplb "github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/loadbalancer"
	gcputils "github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/utils"
	"github.com/golang/glog"
	"github.com/spf13/cobra"
)

var (
	listShortDesc = "List multicluster ingresses."
	listLongDesc  = `List multicluster ingresses.

	List multicluster ingresses found in the given GCP project. Searches for MCIs based on naming convention.
`
)

type listOptions struct {
	// Name of the GCP project in which the load balancer should be configured.
	// Required
	// TODO(nikhiljindal): This should be optional. Figure it out from gcloud settings.
	GCPProject string
	// Access token with which to access gpc resources.
	AccessToken string
}

func newCmdList(out, err io.Writer) *cobra.Command {
	var options listOptions
	cmd := &cobra.Command{
		Use:   "list",
		Short: listShortDesc,
		Long:  listLongDesc,
		Run: func(cmd *cobra.Command, args []string) {
			if err := validateListArgs(&options, args); err != nil {
				fmt.Println(err)
				return
			}
			if err := runList(&options, args); err != nil {
				fmt.Println("Error listing load balancers:", err)
				return
			}
		},
	}
	addListFlags(cmd, &options)
	return cmd
}

func addListFlags(cmd *cobra.Command, options *listOptions) error {
	cmd.Flags().StringVarP(&options.GCPProject, "gcp-project", "", options.GCPProject, "[optional] name of the gcp project. Is fetched using 'gcloud config get-value project' if unset here")
	cmd.Flags().StringVarP(&options.AccessToken, "access-token", "t", options.AccessToken, "[optional] access token for gcp resources (defaults to GOOGLE_APPLICATION_CREDENTIALS).")
	return nil
}

func validateListArgs(options *listOptions, args []string) error {
	// Verify that the required flag is not missing.
	if options.GCPProject == "" {
		project, err := gcputils.GetProjectFromGCloud()
		if project == "" || err != nil {
			return fmt.Errorf("unexpected cannot determine GCP project. Either set --gcp-project flag, or set a default project with gcloud such that 'gcloud config get-value project' returns that")
		}
		glog.V(2).Infof("Got project from gcloud: %s.", project)
		options.GCPProject = project
	}
	if len(args) != 0 {
		return fmt.Errorf("unexpected args: %v. Expected 0 arguments", args)
	}
	return nil
}

// runList prints a list of all mcis that we've created.
func runList(options *listOptions, args []string) error {
	cloudInterface, err := cloudinterface.NewGCECloudInterface(options.GCPProject, options.AccessToken)
	if err != nil {
		fmt.Println("error creating cloud interface:", err)
		return fmt.Errorf("error in creating cloud interface: %s", err)
	}

	lbs, err := gcplb.NewLoadBalancerSyncer("" /*lbName*/, nil /* clientset */, cloudInterface, options.GCPProject)
	if err != nil {
		fmt.Println("Error creating load balancer syncer:", err)
		return err
	}
	balancers, err := lbs.ListLoadBalancers()
	if err != nil {
		fmt.Println("Error getting list of load balancers:", err)
		return err
	}
	fmt.Println(balancers)
	return nil
}
