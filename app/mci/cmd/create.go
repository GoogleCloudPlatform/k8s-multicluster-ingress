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
	"os/exec"
	"strings"

	"github.com/golang/glog"
	"github.com/spf13/cobra"
)

var (
	createShortDescription = "Create a multi-cluster ingress."
	createLongDescription  = `Create a multi-cluster ingress.

	Takes an ingress spec and a list of clusters and creates a multi-cluster ingress targetting those clusters.
	`
)

// Exported here to allow overriding in tests.
var ExecuteCommand = func(args []string) (string, error) {
	output, err := exec.Command(args[0], args[1:]...).CombinedOutput()
	return strings.TrimSuffix(string(output), "\n"), err
}

type CreateOptions struct {
	// Name of the YAML file containing ingress spec.
	IngressFilename string
	// Path to kubeconfig.
	Kubeconfig string
}

func NewCmdCreate(out, err io.Writer) *cobra.Command {
	var options CreateOptions

	cmd := &cobra.Command{
		Use:   "create",
		Short: createShortDescription,
		Long:  createLongDescription,
		// TODO: Add an example.
		Run: func(cmd *cobra.Command, args []string) {
			if err := ValidateArgs(&options, args); err != nil {
				fmt.Printf("%s\n", err)
			}
			if err := RunCreate(&options); err != nil {
				fmt.Printf("Error in creating ingress: %s\n", err)
			}
		},
	}
	AddFlags(cmd, &options)
	return cmd
}

func AddFlags(cmd *cobra.Command, options *CreateOptions) error {
	cmd.Flags().StringVarP(&options.IngressFilename, "ingress", "i", options.IngressFilename, "filename containing ingress spec")
	cmd.Flags().StringVarP(&options.Kubeconfig, "kubeconfig", "k", options.Kubeconfig, "path to kubeconfig")
	// TODO Add a verbose flag that turns on glog logging.
	return nil
}

func ValidateArgs(options *CreateOptions, args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("Unexpected args: %v", args)
	}
	// Verify that the required params are not missing.
	if options.IngressFilename == "" {
		return fmt.Errorf("unexpected missing argument ingress")
	}
	return nil
}

func RunCreate(options *CreateOptions) error {
	// Create ingress in all clusters.
	kubectlArgs := []string{"kubectl"}
	if options.Kubeconfig != "" {
		kubectlArgs = append(kubectlArgs, fmt.Sprintf("--kubeconfig=%s", options.Kubeconfig))
	}
	contextArgs := append(kubectlArgs, []string{"config", "get-contexts", "-o=name"}...)
	output, err := runCommand(contextArgs)
	if err != nil {
		return fmt.Errorf("error in getting contexts from kubeconfig: %s", err)
	}
	contexts := strings.Split(output, "\n")
	createArgs := append(kubectlArgs, []string{"create", fmt.Sprintf("--filename=%s", options.IngressFilename)}...)
	for _, c := range contexts {
		fmt.Printf("Creating ingress in context: %s\n", c)
		contextArgs := append(createArgs, fmt.Sprintf("--context=%s", c))
		output, err = runCommand(contextArgs)
		if err != nil {
			return fmt.Errorf("error in creating ingress in cluster %s: %s", c, output)
		}
	}
	return nil
}

func runCommand(args []string) (string, error) {
	glog.V(3).Infof("Running command: %s\n", strings.Join(args, " "))
	output, err := ExecuteCommand(args)
	if err != nil {
		glog.V(3).Infof("%s", output)
	}
	return output, err
}
