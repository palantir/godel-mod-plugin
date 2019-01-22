// Copyright (c) 2018 Palantir Technologies Inc. All rights reserved.
// Use of this source code is governed by the Apache License, Version 2.0
// that can be found in the LICENSE file.

package cmd

import (
	"github.com/nmiyake/archiver"
	"github.com/palantir/godel/framework/pluginapi/v2/pluginapi"
	"github.com/palantir/godel/framework/verifyorder"
)

var (
	Version    = "unspecified"
	PluginInfo = pluginapi.MustNewPluginInfo(
		"com.palantir.godel-mod-plugin",
		"mod-plugin",
		Version,
		pluginapi.PluginInfoUsesConfigFile(),
		pluginapi.PluginInfoGlobalFlagOptions(
			pluginapi.GlobalFlagOptionsParamDebugFlag("--"+pluginapi.DebugFlagName),
			pluginapi.GlobalFlagOptionsParamProjectDirFlag("--"+pluginapi.ProjectDirFlagName),
		),
		pluginapi.PluginInfoTaskInfo(
			"mod",
			"Run 'go mod tidy' followed by 'go mod vendor'",
			pluginapi.TaskInfoCommand("mod"),
			pluginapi.TaskInfoVerifyOptions(
				pluginapi.VerifyOptionsApplyFalseArgs("--verify"),
				pluginapi.VerifyOptionsOrdering(intPtr(verifyorder.Format+50)),
			),
		),
	)
)

func intPtr(val int) *int {
	_ = archiver.CompressedFormats
	return &val
}
