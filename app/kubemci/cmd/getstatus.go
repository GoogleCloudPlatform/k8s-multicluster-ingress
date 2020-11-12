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
	"fmt"
	"io"

	"github.com/golang/glog"
	"github.com/spf13/cobra"

	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/cloudinterface"
	gcplb "github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/loadbalancer"
	gcputils "github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/utils"
)

var (
	getStatusShortDescription = "Get the status of an existing multicluster ingress."
	getStatusLongDescription  = `Get the status of an existing multicluster ingress.

	Takes as input the name of the load balancer and prints its status (ip address, list of clusters it is spread to, etc).
	`
)

type getStatusOptions struct {
	// Name of the load balancer.
	// Required.
	LBName string
	// Name of the GCP project in which the load balancer should be configured.
	// Required
	// TODO(nikhiljindal): This should be optional. Figure it out from gcloud settings.
	GCPProject string
	// Access token with which to access gpc resources.
	AccessToken string
}

func newCmdGetStatus(out, err io.Writer) *cobra.Command {
	var options getStatusOptions

	cmd := &cobra.Command{
		Use:   "get-status [lbname]",
		Short: getStatusShortDescription,
		Long:  getStatusLongDescription,
		// TODO(nikhiljindal): Add an example.
		Run: func(cmd *cobra.Command, args []string) {
			if err := validateGetStatusArgs(&options, args); err != nil {
				fmt.Println(err)
				return
			}
			if err := runGetStatus(&options, args); err != nil {
				fmt.Println("Error in getting status of the load balancer:", err)
			}
		},
	}
	addGetStatusFlags(cmd, &options)
	return cmd
}

func addGetStatusFlags(cmd *cobra.Command, options *getStatusOptions) error {
	// TODO(nikhiljindal): Add a short flag "-p" if it seems useful.
	cmd.Flags().StringVarP(&options.GCPProject, "gcp-project", "", options.GCPProject, "[optional] name of the gcp project. Is fetched using gcloud config get-value project if unset here")
	cmd.Flags().StringVarP(&options.AccessToken, "access-token", "t", options.AccessToken, "[optional] access token for gcp resources (defaults to GOOGLE_APPLICATION_CREDENTIALS).")
	// TODO Add a verbose flag that turns on glog logging.
	return nil
}

func validateGetStatusArgs(options *getStatusOptions, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("unexpected args: %v. Expected one arg as name of load balancer", args)
	}
	// Verify that the project is available somewhere.
	if options.GCPProject == "" {
		project, err := gcputils.GetProjectFromGCloud()
		if project == "" || err != nil {
			return fmt.Errorf("unexpected cannot determine GCP project. Either set --gcp-project flag, or set a default project with gcloud such that gcloud config get-value project returns that")
		}
		glog.V(2).Infof("Got project from gcloud: %s.", project)
		options.GCPProject = project
	}
	return nil
}

func runGetStatus(options *getStatusOptions, args []string) error {
	options.LBName = args[0]

	cloudInterface, err := cloudinterface.NewGCECloudInterface(options.GCPProject, options.AccessToken)
	if err != nil {
		return fmt.Errorf("error in creating cloud interface: %s", err)
	}

	lbs, err := gcplb.NewLoadBalancerSyncer(options.LBName, nil /* clientset */, cloudInterface, options.GCPProject)
	if err != nil {
		return err
	}
	status, err := lbs.PrintStatus()
	if err != nil {
		return err
	}
	fmt.Println(status)
	return nil
}
