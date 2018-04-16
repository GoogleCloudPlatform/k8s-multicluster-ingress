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

	"github.com/spf13/cobra"
)

var (
	versionShortDescription = "Print the version of this tool."
	versionLongDescription  = `Print the version of this tool.

	Release builds have versions of the form x.y.z.
	version x.y.z+ indicates that the client has some changes over the x.y.z release.
	`
)

const (
	// String indicating the client version.
	// TODO(nikhiljindal): Add more useful information such as build date, git commit, etc similar to kubectl.
	clientVersion = "0.4.0"
)

func newCmdGetVersion(out, err io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: versionShortDescription,
		Long:  versionLongDescription,
		// TODO(nikhiljindal): Add an example.
		Run: func(cmd *cobra.Command, args []string) {
			if err := validateVersionArgs(args); err != nil {
				fmt.Println(err)
				return
			}
			if err := runVersion(args); err != nil {
				fmt.Println("Error in printing version:", err)
			}
		},
	}
	return cmd
}

func validateVersionArgs(args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("unexpected args: %v. Expected no arguments", args)
	}
	return nil
}

func runVersion(args []string) error {
	fmt.Println("Client version:", clientVersion)
	return nil
}
