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
	"flag"
	"io"

	"github.com/spf13/cobra"
)

var (
	shortDescription = "kubemci is used to configure an ingress across multiple kubernetes clusters."
	longDescription  = `kubemci is used to configure an ingress across multiple kubernetes clusters.
It assumes that there is a working gcloud and kubectl in PATH.`
)

// NewCommand instantiates a new cobra command instance for kubemci commands.
func NewCommand(in io.Reader, out, err io.Writer) *cobra.Command {
	// Parent command to which all subcommands are added.
	rootCmd := &cobra.Command{
		Use:   "kubemci",
		Short: shortDescription,
		Long:  longDescription,
	}
	rootCmd.AddCommand(
		newCmdCreate(out, err),
		newCmdDelete(out, err),
		newCmdGetStatus(out, err),
		newCmdGetVersion(out, err),
		newCmdList(out, err),
		newCmdRemoveClusters(out, err),
	)
	rootCmd.PersistentFlags().AddGoFlagSet(flag.CommandLine)
	return rootCmd
}
