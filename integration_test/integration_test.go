// Copyright (c) 2018 Palantir Technologies Inc. All rights reserved.
// Use of this source code is governed by the Apache License, Version 2.0
// that can be found in the LICENSE file.

package integration_test

import (
	"bytes"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"strings"
	"testing"

	"github.com/nmiyake/pkg/dirs"
	"github.com/nmiyake/pkg/gofiles"
	"github.com/palantir/godel/v2/framework/pluginapitester"
	"github.com/palantir/godel/v2/pkg/products"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	godelYML = `
exclude:
  names:
    - "\\..+"
    - "vendor"
  paths:
    - "godel"
`
)

func TestMod(t *testing.T) {
	restoreEnvVars := setEnvVars(map[string]string{
		"GO111MODULE": "",
		"GOFLAGS":     "",
	})
	defer restoreEnvVars()

	pluginPath, err := products.Bin("mod-plugin")
	require.NoError(t, err)

	projectDir, cleanup, err := dirs.TempDir("", "")
	require.NoError(t, err)
	defer cleanup()

	origWd, err := os.Getwd()
	require.NoError(t, err)
	defer func() {
		err = os.Chdir(origWd)
		require.NoError(t, err)
	}()
	err = os.Chdir(projectDir)
	require.NoError(t, err)

	err = os.MkdirAll(path.Join(projectDir, "godel", "config"), 0755)
	require.NoError(t, err)
	err = ioutil.WriteFile(path.Join(projectDir, "godel", "config", "godel.yml"), []byte(godelYML), 0644)
	require.NoError(t, err)

	goModInitOutput, err := exec.Command("go", "mod", "init", "github.com/mod/test").CombinedOutput()
	require.NoError(t, err, "go mod init failed. Output: %s", string(goModInitOutput))

	_, err = os.Stat("vendor")
	require.True(t, os.IsNotExist(err))

	specs := []gofiles.GoFileSpec{
		{
			RelPath: "foo.go",
			Src:     `package foo; import _ "github.com/pkg/errors";`,
		},
	}

	_, err = gofiles.Write(projectDir, specs)
	require.NoError(t, err)

	restoreEnvVars = setEnvVars(map[string]string{
		"GO111MODULE": "on",
	})
	defer restoreEnvVars()

	outputBuf := &bytes.Buffer{}
	runPluginCleanup, err := pluginapitester.RunPlugin(pluginapitester.NewPluginProvider(pluginPath), nil, "mod", nil, projectDir, false, outputBuf)
	defer runPluginCleanup()
	require.NoError(t, err, "Output: %s", outputBuf.String())

	_, err = os.Stat("vendor")
	require.True(t, os.IsNotExist(err))
}

func TestModWithVendor(t *testing.T) {
	restoreEnvVars := setEnvVars(map[string]string{
		"GO111MODULE": "",
		"GOFLAGS":     "",
	})
	defer restoreEnvVars()

	pluginPath, err := products.Bin("mod-plugin")
	require.NoError(t, err)

	projectDir, cleanup, err := dirs.TempDir("", "")
	require.NoError(t, err)
	defer cleanup()

	origWd, err := os.Getwd()
	require.NoError(t, err)
	defer func() {
		err = os.Chdir(origWd)
		require.NoError(t, err)
	}()
	err = os.Chdir(projectDir)
	require.NoError(t, err)

	err = os.MkdirAll(path.Join(projectDir, "godel", "config"), 0755)
	require.NoError(t, err)
	err = ioutil.WriteFile(path.Join(projectDir, "godel", "config", "godel.yml"), []byte(godelYML), 0644)
	require.NoError(t, err)

	goModInitOutput, err := exec.Command("go", "mod", "init", "github.com/mod/test").CombinedOutput()
	require.NoError(t, err, "go mod init failed. Output: %s", string(goModInitOutput))

	_, err = os.Stat("vendor")
	require.True(t, os.IsNotExist(err))

	specs := []gofiles.GoFileSpec{
		{
			RelPath: "foo.go",
			Src:     `package foo; import _ "github.com/pkg/errors";`,
		},
	}

	_, err = gofiles.Write(projectDir, specs)
	require.NoError(t, err)

	restoreEnvVars = setEnvVars(map[string]string{
		"GO111MODULE": "on",
		"GOFLAGS":     "-mod=vendor",
	})
	defer restoreEnvVars()

	outputBuf := &bytes.Buffer{}
	runPluginCleanup, err := pluginapitester.RunPlugin(pluginapitester.NewPluginProvider(pluginPath), nil, "mod", nil, projectDir, false, outputBuf)
	defer runPluginCleanup()
	require.NoError(t, err, "Output: %s", outputBuf.String())

	_, err = os.Stat("vendor/github.com/pkg/errors")
	assert.NoError(t, err, "Output: %s", outputBuf.String())
}

func TestModVerifyWithEmptySumSucceeds(t *testing.T) {
	restoreEnvVars := setEnvVars(map[string]string{
		"GO111MODULE": "",
		"GOFLAGS":     "",
	})
	defer restoreEnvVars()

	pluginPath, err := products.Bin("mod-plugin")
	require.NoError(t, err)

	projectDir, cleanup, err := dirs.TempDir("", "")
	require.NoError(t, err)
	defer cleanup()

	origWd, err := os.Getwd()
	require.NoError(t, err)
	defer func() {
		err = os.Chdir(origWd)
		require.NoError(t, err)
	}()
	err = os.Chdir(projectDir)
	require.NoError(t, err)

	err = os.MkdirAll(path.Join(projectDir, "godel", "config"), 0755)
	require.NoError(t, err)
	err = ioutil.WriteFile(path.Join(projectDir, "godel", "config", "godel.yml"), []byte(godelYML), 0644)
	require.NoError(t, err)

	goModInitOutput, err := exec.Command("go", "mod", "init", "github.com/mod/test").CombinedOutput()
	require.NoError(t, err, "go mod init failed. Output: %s", string(goModInitOutput))

	_, err = os.Stat("vendor")
	require.True(t, os.IsNotExist(err))

	specs := []gofiles.GoFileSpec{
		{
			RelPath: "foo.go",
			Src:     `package foo`,
		},
	}
	_, err = gofiles.Write(projectDir, specs)
	require.NoError(t, err)

	restoreEnvVars = setEnvVars(map[string]string{
		"GO111MODULE": "on",
	})
	defer restoreEnvVars()

	outputBuf := &bytes.Buffer{}
	runPluginCleanup, err := pluginapitester.RunPlugin(pluginapitester.NewPluginProvider(pluginPath), nil, "mod", nil, projectDir, false, outputBuf)
	defer runPluginCleanup()
	require.NoError(t, err, "Output: %s", outputBuf.String())

	_, err = os.Stat("vendor")
	require.True(t, os.IsNotExist(err))

	outputBuf = &bytes.Buffer{}
	runPluginCleanup, err = pluginapitester.RunPlugin(pluginapitester.NewPluginProvider(pluginPath), nil, "mod", []string{"--verify"}, projectDir, false, outputBuf)
	defer runPluginCleanup()
	require.NoError(t, err, "Output: %s", outputBuf.String())
}

func TestModVerifyApplyFalseFails(t *testing.T) {
	restoreEnvVars := setEnvVars(map[string]string{
		"GO111MODULE": "",
		"GOFLAGS":     "",
	})
	defer restoreEnvVars()

	pluginPath, err := products.Bin("mod-plugin")
	require.NoError(t, err)

	projectDir, cleanup, err := dirs.TempDir("", "")
	require.NoError(t, err)
	defer cleanup()

	origWd, err := os.Getwd()
	require.NoError(t, err)
	defer func() {
		err = os.Chdir(origWd)
		require.NoError(t, err)
	}()
	err = os.Chdir(projectDir)
	require.NoError(t, err)

	err = os.MkdirAll(path.Join(projectDir, "godel", "config"), 0755)
	require.NoError(t, err)
	err = ioutil.WriteFile(path.Join(projectDir, "godel", "config", "godel.yml"), []byte(godelYML), 0644)
	require.NoError(t, err)

	goModInitOutput, err := exec.Command("go", "mod", "init", "github.com/mod/test").CombinedOutput()
	require.NoError(t, err, "go mod init failed. Output: %s", string(goModInitOutput))

	_, err = os.Stat("vendor")
	require.True(t, os.IsNotExist(err))

	specs := []gofiles.GoFileSpec{
		{
			RelPath: "foo.go",
			Src:     `package foo; import _ "github.com/pkg/errors";`,
		},
	}
	_, err = gofiles.Write(projectDir, specs)
	require.NoError(t, err)

	restoreEnvVars = setEnvVars(map[string]string{
		"GO111MODULE": "on",
	})
	defer restoreEnvVars()

	outputBuf := &bytes.Buffer{}
	runPluginCleanup, err := pluginapitester.RunPlugin(pluginapitester.NewPluginProvider(pluginPath), nil, "mod", nil, projectDir, false, outputBuf)
	defer runPluginCleanup()
	require.NoError(t, err, "Output: %s", outputBuf.String())

	_, err = os.Stat("vendor")
	require.True(t, os.IsNotExist(err))

	specs = []gofiles.GoFileSpec{
		{
			RelPath: "foo.go",
			Src:     `package foo; import _ "github.com/pkg/errors"; import _ "github.com/pkg/math"`,
		},
	}
	_, err = gofiles.Write(projectDir, specs)
	require.NoError(t, err)

	outputBuf = &bytes.Buffer{}
	runPluginCleanup, err = pluginapitester.RunPlugin(pluginapitester.NewPluginProvider(pluginPath), nil, "mod", []string{"--verify"}, projectDir, false, outputBuf)
	defer runPluginCleanup()
	require.Error(t, err)

	output := outputBuf.String()
	assert.True(t, strings.HasSuffix(output, "Error: go.mod modified\n"), output)
}

func TestModVerifyApplyFalseFailsWithVendor(t *testing.T) {
	restoreEnvVars := setEnvVars(map[string]string{
		"GO111MODULE": "",
		"GOFLAGS":     "",
	})
	defer restoreEnvVars()

	pluginPath, err := products.Bin("mod-plugin")
	require.NoError(t, err)

	projectDir, cleanup, err := dirs.TempDir("", "")
	require.NoError(t, err)
	defer cleanup()

	origWd, err := os.Getwd()
	require.NoError(t, err)
	defer func() {
		err = os.Chdir(origWd)
		require.NoError(t, err)
	}()
	err = os.Chdir(projectDir)
	require.NoError(t, err)

	err = os.MkdirAll(path.Join(projectDir, "godel", "config"), 0755)
	require.NoError(t, err)
	err = ioutil.WriteFile(path.Join(projectDir, "godel", "config", "godel.yml"), []byte(godelYML), 0644)
	require.NoError(t, err)

	goModInitOutput, err := exec.Command("go", "mod", "init", "github.com/mod/test").CombinedOutput()
	require.NoError(t, err, "go mod init failed. Output: %s", string(goModInitOutput))

	_, err = os.Stat("vendor")
	require.True(t, os.IsNotExist(err))

	specs := []gofiles.GoFileSpec{
		{
			RelPath: "foo.go",
			Src:     `package foo; import _ "github.com/pkg/errors";`,
		},
	}
	_, err = gofiles.Write(projectDir, specs)
	require.NoError(t, err)

	restoreEnvVars = setEnvVars(map[string]string{
		"GO111MODULE": "on",
		"GOFLAGS":     "-mod=vendor",
	})
	defer restoreEnvVars()

	outputBuf := &bytes.Buffer{}
	runPluginCleanup, err := pluginapitester.RunPlugin(pluginapitester.NewPluginProvider(pluginPath), nil, "mod", nil, projectDir, false, outputBuf)
	defer runPluginCleanup()
	require.NoError(t, err, "Output: %s", outputBuf.String())

	_, err = os.Stat("vendor/github.com/pkg/errors")
	assert.NoError(t, err, "Output: %s", outputBuf.String())

	specs = []gofiles.GoFileSpec{
		{
			RelPath: "foo.go",
			Src:     `package foo; import _ "github.com/pkg/errors"; import _ "github.com/pkg/math"`,
		},
	}
	_, err = gofiles.Write(projectDir, specs)
	require.NoError(t, err)

	outputBuf = &bytes.Buffer{}
	runPluginCleanup, err = pluginapitester.RunPlugin(pluginapitester.NewPluginProvider(pluginPath), nil, "mod", []string{"--verify"}, projectDir, false, outputBuf)
	defer runPluginCleanup()
	require.Error(t, err)

	output := outputBuf.String()
	assert.True(t, strings.HasSuffix(output, "Error: go.mod modified\n"), output)
}

func TestModVerifyApplyFalseWithVendorSucceedsWithNoModDependencies(t *testing.T) {
	restoreEnvVars := setEnvVars(map[string]string{
		"GO111MODULE": "",
		"GOFLAGS":     "",
	})
	defer restoreEnvVars()

	pluginPath, err := products.Bin("mod-plugin")
	require.NoError(t, err)

	projectDir, cleanup, err := dirs.TempDir("", "")
	require.NoError(t, err)
	defer cleanup()

	origWd, err := os.Getwd()
	require.NoError(t, err)
	defer func() {
		err = os.Chdir(origWd)
		require.NoError(t, err)
	}()
	err = os.Chdir(projectDir)
	require.NoError(t, err)

	err = os.MkdirAll(path.Join(projectDir, "godel", "config"), 0755)
	require.NoError(t, err)
	err = ioutil.WriteFile(path.Join(projectDir, "godel", "config", "godel.yml"), []byte(godelYML), 0644)
	require.NoError(t, err)

	goModInitOutput, err := exec.Command("go", "mod", "init", "github.com/mod/test").CombinedOutput()
	require.NoError(t, err, "go mod init failed. Output: %s", string(goModInitOutput))

	_, err = os.Stat("vendor")
	require.True(t, os.IsNotExist(err))

	specs := []gofiles.GoFileSpec{
		{
			RelPath: "foo.go",
			Src:     `package foo`,
		},
	}
	_, err = gofiles.Write(projectDir, specs)
	require.NoError(t, err)

	restoreEnvVars = setEnvVars(map[string]string{
		"GO111MODULE": "on",
		"GOFLAGS":     "-mod=vendor",
	})
	defer restoreEnvVars()

	outputBuf := &bytes.Buffer{}
	runPluginCleanup, err := pluginapitester.RunPlugin(pluginapitester.NewPluginProvider(pluginPath), nil, "mod", nil, projectDir, false, outputBuf)
	defer runPluginCleanup()
	require.NoError(t, err, "Output: %s", outputBuf.String())

	_, err = os.Stat("vendor")
	require.True(t, os.IsNotExist(err))

	outputBuf = &bytes.Buffer{}
	runPluginCleanup, err = pluginapitester.RunPlugin(pluginapitester.NewPluginProvider(pluginPath), nil, "mod", []string{"--verify"}, projectDir, false, outputBuf)
	defer runPluginCleanup()
	require.NoError(t, err, "Output: %s", outputBuf.String())
}

func setEnvVars(envVars map[string]string) func() {
	origVars := make(map[string]string)
	var unsetVars []string
	for k := range envVars {
		val, ok := os.LookupEnv(k)
		if !ok {
			unsetVars = append(unsetVars, k)
			continue
		}
		origVars[k] = val
	}

	for k, v := range envVars {
		_ = os.Setenv(k, v)
	}

	return func() {
		for _, k := range unsetVars {
			_ = os.Unsetenv(k)
		}
		for k, v := range origVars {
			_ = os.Setenv(k, v)
		}
	}
}
