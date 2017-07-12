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
	createShortDescription = "Create a multi-cluster ingress."
	createLongDescription  = `Create a multi-cluster ingress.

	Takes an ingress spec and a list of clusters and creates a multi-cluster ingress targetting those clusters.
	`
)

func NewCmdCreate(out, err io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "create",
		Short: createShortDescription,
		Long:  createLongDescription,
		// TODO: Add an example.
		Run: func(cmd *cobra.Command, args []string) {
			RunCreate(cmd, out, err)
		},
	}
}

func RunCreate(cmd *cobra.Command, out, err io.Writer) {
	// TODO: Add logic.
	fmt.Println("Create called")
}
