// Copyright (c) 2018 Palantir Technologies Inc. All rights reserved.
// Use of this source code is governed by the Apache License, Version 2.0
// that can be found in the LICENSE file.

package cmd

import (
	"github.com/palantir/godel/v2/framework/pluginapi"
	"github.com/palantir/pkg/cobracli"
	"github.com/spf13/cobra"
)

var (
	debugFlagVal      bool
	projectDirFlagVal string
	verifyFlagVal     bool
)

var rootCmd = &cobra.Command{
	Use:   "mod-plugin [flags] [files]",
	Short: "Run go mod",
}

func Execute() int {
	return cobracli.ExecuteWithDebugVarAndDefaultParams(rootCmd, &debugFlagVal)
}

func init() {
	pluginapi.AddDebugPFlagPtr(rootCmd.PersistentFlags(), &debugFlagVal)
	pluginapi.AddProjectDirPFlagPtr(rootCmd.PersistentFlags(), &projectDirFlagVal)
}
