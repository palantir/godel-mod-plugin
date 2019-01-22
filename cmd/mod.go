// Copyright (c) 2018 Palantir Technologies Inc. All rights reserved.
// Use of this source code is governed by the Apache License, Version 2.0
// that can be found in the LICENSE file.

package cmd

import (
	"github.com/palantir/godel-mod-plugin/gomod"
	"github.com/spf13/cobra"
)

var modCmd = &cobra.Command{
	Use:   "mod [flags] [args]",
	Short: "Ensures that the go module state for the project is up-to-date",
	Long: `Executes "go mod tidy" followed by "go mod vendor" to ensure that the module state for the repository is
up-to-date. When run in verification mode, fails if either operation resulted in project state being modified.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return gomod.Run(projectDirFlagVal, verifyFlagVal, cmd.OutOrStdout())
	},
}

func init() {
	modCmd.Flags().BoolVar(&verifyFlagVal, "verify", false, "verify that go module state is up-to-date")
	rootCmd.AddCommand(modCmd)
}
