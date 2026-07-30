// Copyright (c) 2018 Palantir Technologies Inc. All rights reserved.
// Use of this source code is governed by the Apache License, Version 2.0
// that can be found in the LICENSE file.

package integration_test

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"testing"
	"time"

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
	project := setupModProject(t, "", "github.com/pkg/errors")

	_, err := os.Stat("vendor")
	require.True(t, os.IsNotExist(err))

	output, err := project.run(t)
	require.NoError(t, err, "Output: %s", output)

	_, err = os.Stat("vendor")
	require.True(t, os.IsNotExist(err))
}

func TestModWithVendor(t *testing.T) {
	project := setupModProject(t, "-mod=vendor", "github.com/pkg/errors")

	_, err := os.Stat("vendor")
	require.True(t, os.IsNotExist(err))

	output, err := project.run(t)
	require.NoError(t, err, "Output: %s", output)

	_, err = os.Stat("vendor/github.com/pkg/errors")
	assert.NoError(t, err, "Output: %s", output)
}

func TestModVerifyWithEmptySumSucceeds(t *testing.T) {
	project := setupModProject(t, "")

	output, err := project.run(t)
	require.NoError(t, err, "Output: %s", output)

	_, err = os.Stat("vendor")
	require.True(t, os.IsNotExist(err))

	output, err = project.run(t, "--verify")
	require.NoError(t, err, "Output: %s", output)
}

func TestModVerifyApplyFalseFails(t *testing.T) {
	project := setupModProject(t, "", "github.com/pkg/errors")

	output, err := project.run(t)
	require.NoError(t, err, "Output: %s", output)

	_, err = os.Stat("vendor")
	require.True(t, os.IsNotExist(err))

	writeSourceFile(t, project.dir, "github.com/pkg/errors", "github.com/pkg/math")

	output, err = project.run(t, "--verify")
	require.Error(t, err)
	// "go mod tidy -diff" reports what tidying would change, for both files rather than only the first
	assert.Contains(t, output, "diff current/go.mod tidy/go.mod", output)
	assert.Contains(t, output, "diff current/go.sum tidy/go.sum", output)
	assert.Contains(t, output, "+\tgithub.com/pkg/math ", output)
}

func TestModVerifyApplyFalseFailsWithVendor(t *testing.T) {
	project := setupModProject(t, "-mod=vendor", "github.com/pkg/errors")

	output, err := project.run(t)
	require.NoError(t, err, "Output: %s", output)

	_, err = os.Stat("vendor/github.com/pkg/errors")
	assert.NoError(t, err, "Output: %s", output)

	writeSourceFile(t, project.dir, "github.com/pkg/errors", "github.com/pkg/math")

	output, err = project.run(t, "--verify")
	require.Error(t, err)
	assert.Contains(t, output, "diff current/go.mod tidy/go.mod", output)
}

func TestModVerifyApplyFalseWithVendorSucceedsWithNoModDependencies(t *testing.T) {
	project := setupModProject(t, "-mod=vendor")

	output, err := project.run(t)
	require.NoError(t, err, "Output: %s", output)

	_, err = os.Stat("vendor")
	require.True(t, os.IsNotExist(err))

	output, err = project.run(t, "--verify")
	require.NoError(t, err, "Output: %s", output)
}

// TestModVendorOnlyRewritesChangedFiles verifies that a second run leaves the files that did not change alone.
func TestModVendorOnlyRewritesChangedFiles(t *testing.T) {
	project := setupModProject(t, "-mod=vendor", "github.com/pkg/errors")

	output, err := project.run(t)
	require.NoError(t, err, "Output: %s", output)

	vendorDir := path.Join(project.dir, "vendor")
	tamperedRelPath := path.Join("github.com", "pkg", "errors", "errors.go")
	tamperedPath := path.Join(vendorDir, tamperedRelPath)
	origContent, err := os.ReadFile(tamperedPath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(tamperedPath, append(origContent, []byte("\n// modified\n")...), 0644))

	backdated := backdateDir(t, vendorDir)

	output, err = project.run(t)
	require.NoError(t, err, "Output: %s", output)

	restoredContent, err := os.ReadFile(tamperedPath)
	require.NoError(t, err)
	assert.Equal(t, string(origContent), string(restoredContent), "modified vendored file was not restored")

	assertNotRewritten(t, vendorDir, backdated, tamperedRelPath)
	tamperedInfo, err := os.Stat(tamperedPath)
	require.NoError(t, err)
	assert.True(t, tamperedInfo.ModTime().After(backdated), "%s was not rewritten", tamperedRelPath)
}

// TestModVerifyDoesNotModifyProject verifies that verification of an up-to-date project is read-only.
func TestModVerifyDoesNotModifyProject(t *testing.T) {
	project := setupModProject(t, "-mod=vendor", "github.com/pkg/errors")

	output, err := project.run(t)
	require.NoError(t, err, "Output: %s", output)

	vendorDir := path.Join(project.dir, "vendor")
	goModPath, goSumPath := path.Join(project.dir, "go.mod"), path.Join(project.dir, "go.sum")
	backdated := backdateDir(t, vendorDir, goModPath, goSumPath)

	contentsBefore := contentsForDir(t, vendorDir)
	moduleFilesBefore := make(map[string]string)
	for _, fpath := range []string{goModPath, goSumPath} {
		content, err := os.ReadFile(fpath)
		require.NoError(t, err)
		moduleFilesBefore[fpath] = string(content)
	}

	output, err = project.run(t, "--verify")
	require.NoError(t, err, "Output: %s", output)

	assert.Equal(t, contentsBefore, contentsForDir(t, vendorDir), "verify modified the content of the vendor directory")
	assertNotRewritten(t, vendorDir, backdated)
	for fpath, before := range moduleFilesBefore {
		after, err := os.ReadFile(fpath)
		require.NoError(t, err)
		assert.Equal(t, before, string(after), "verify modified the content of %s", path.Base(fpath))
		info, err := os.Stat(fpath)
		require.NoError(t, err)
		assert.True(t, info.ModTime().Equal(backdated), "verify rewrote %s", path.Base(fpath))
	}
}

// TestModVerifyFailsAndDoesNotModifyStaleVendorDir verifies that a stale vendor directory is reported, not repaired.
func TestModVerifyFailsAndDoesNotModifyStaleVendorDir(t *testing.T) {
	project := setupModProject(t, "-mod=vendor", "github.com/pkg/errors")

	output, err := project.run(t)
	require.NoError(t, err, "Output: %s", output)

	vendorDir := path.Join(project.dir, "vendor")
	stalePath := path.Join(vendorDir, "github.com", "pkg", "errors", "errors.go")
	origContent, err := os.ReadFile(stalePath)
	require.NoError(t, err)
	staleContent := append(origContent, []byte("\n// stale\n")...)
	require.NoError(t, os.WriteFile(stalePath, staleContent, 0644))

	backdated := backdateDir(t, vendorDir)
	contentsBefore := contentsForDir(t, vendorDir)

	output, err = project.run(t, "--verify")
	require.Error(t, err)
	assert.Contains(t, output, "vendor directory out of date", output)
	assert.Contains(t, output, path.Join("github.com", "pkg", "errors", "errors.go"), output)

	assert.Equal(t, contentsBefore, contentsForDir(t, vendorDir), "verify modified the stale vendor directory")
	assertNotRewritten(t, vendorDir, backdated)
}

// TestModVendorRemovesDependencyDroppedFromModuleGraph verifies that a dropped dependency is removed along with the
// directories it leaves empty.
func TestModVendorRemovesDependencyDroppedFromModuleGraph(t *testing.T) {
	project := setupModProject(t, "-mod=vendor", "github.com/pkg/errors", "github.com/inconshreveable/mousetrap")

	output, err := project.run(t)
	require.NoError(t, err, "Output: %s", output)

	vendorDir := path.Join(project.dir, "vendor")
	require.DirExists(t, path.Join(vendorDir, "github.com", "inconshreveable", "mousetrap"))
	require.DirExists(t, path.Join(vendorDir, "github.com", "pkg", "errors"))

	writeSourceFile(t, project.dir, "github.com/pkg/errors")
	backdated := backdateDir(t, vendorDir)

	output, err = project.run(t)
	require.NoError(t, err, "Output: %s", output)

	assert.NoDirExists(t, path.Join(vendorDir, "github.com", "inconshreveable"), "directory left empty by the removed dependency was not removed")
	assert.DirExists(t, path.Join(vendorDir, "github.com", "pkg", "errors"))

	// the surviving dependency must not have been rewritten; vendor/modules.txt legitimately changes
	assertNotRewritten(t, path.Join(vendorDir, "github.com", "pkg", "errors"), backdated)
}

// TestModVendorCreatesVendorDirWhenAbsent verifies that the vendor directory is populated when it does not exist.
func TestModVendorCreatesVendorDirWhenAbsent(t *testing.T) {
	project := setupModProject(t, "-mod=vendor", "github.com/pkg/errors")

	output, err := project.run(t)
	require.NoError(t, err, "Output: %s", output)

	vendorDir := path.Join(project.dir, "vendor")
	contentsBefore := contentsForDir(t, vendorDir)
	require.NotEmpty(t, contentsBefore)
	require.NoError(t, os.RemoveAll(vendorDir))

	output, err = project.run(t)
	require.NoError(t, err, "Output: %s", output)

	assert.Equal(t, contentsBefore, contentsForDir(t, vendorDir))
}

// modProject is a module in a temporary directory that the mod plugin can be run against.
type modProject struct {
	dir        string
	pluginPath string
}

// setupModProject creates a module in a new temporary directory, makes it the working directory and writes a source file
// importing the provided packages.
func setupModProject(t *testing.T, goFlags string, imports ...string) *modProject {
	t.Helper()

	// created with the module environment cleared, operated on with it set, as the plugin is invoked
	t.Setenv("GO111MODULE", "")
	t.Setenv("GOFLAGS", "")

	// resolve the plugin before changing directory: doing so locates the repository that builds it
	pluginPath, err := products.Bin("mod-plugin")
	require.NoError(t, err)

	projectDir := t.TempDir()
	t.Chdir(projectDir)

	require.NoError(t, os.MkdirAll(path.Join(projectDir, "godel", "config"), 0755))
	require.NoError(t, os.WriteFile(path.Join(projectDir, "godel", "config", "godel.yml"), []byte(godelYML), 0644))

	goModInitOutput, err := exec.Command("go", "mod", "init", "github.com/mod/test").CombinedOutput()
	require.NoError(t, err, "go mod init failed. Output: %s", string(goModInitOutput))

	writeSourceFile(t, projectDir, imports...)

	t.Setenv("GO111MODULE", "on")
	t.Setenv("GOFLAGS", goFlags)

	return &modProject{dir: projectDir, pluginPath: pluginPath}
}

// writeSourceFile writes the project's single source file, importing the provided packages.
func writeSourceFile(t *testing.T, projectDir string, imports ...string) {
	t.Helper()

	src := "package foo"
	for _, importPath := range imports {
		src += fmt.Sprintf("; import _ %q", importPath)
	}
	_, err := gofiles.Write(projectDir, []gofiles.GoFileSpec{{RelPath: "foo.go", Src: src}})
	require.NoError(t, err)
}

func (p *modProject) run(t *testing.T, args ...string) (string, error) {
	t.Helper()

	outputBuf := &bytes.Buffer{}
	runPluginCleanup, err := pluginapitester.RunPlugin(pluginapitester.NewPluginProvider(p.pluginPath), nil, "mod", args, p.dir, false, outputBuf)
	t.Cleanup(runPluginCleanup)
	return outputBuf.String(), err
}

// modTimesForDir returns the modification time of every regular file in dir, keyed by relative path.
func modTimesForDir(t *testing.T, dir string) map[string]time.Time {
	t.Helper()

	modTimes := make(map[string]time.Time)
	walkFiles(t, dir, func(relPath string, info os.FileInfo) {
		modTimes[relPath] = info.ModTime()
	})
	return modTimes
}

// contentsForDir returns the content of every regular file in dir, keyed by relative path.
func contentsForDir(t *testing.T, dir string) map[string]string {
	t.Helper()

	contents := make(map[string]string)
	walkFiles(t, dir, func(relPath string, _ os.FileInfo) {
		content, err := os.ReadFile(path.Join(dir, relPath))
		require.NoError(t, err)
		contents[relPath] = string(content)
	})
	return contents
}

// backdateDir pushes every file in dir into the past so that anything rewritten afterwards stands out. Truncated to a
// whole second because not every filesystem records finer.
func backdateDir(t *testing.T, dir string, alsoBackdate ...string) time.Time {
	t.Helper()

	backdated := time.Now().Add(-time.Hour).Truncate(time.Second)
	walkFiles(t, dir, func(relPath string, _ os.FileInfo) {
		require.NoError(t, os.Chtimes(path.Join(dir, relPath), time.Time{}, backdated))
	})
	for _, fpath := range alsoBackdate {
		require.NoError(t, os.Chtimes(fpath, time.Time{}, backdated))
	}
	return backdated
}

// assertNotRewritten asserts that no file in dir except those given has been written since it was backdated.
func assertNotRewritten(t *testing.T, dir string, backdated time.Time, except ...string) {
	t.Helper()

	modTimes := modTimesForDir(t, dir)
	require.NotEmpty(t, modTimes, "no files found in %s, so nothing was actually asserted", dir)
	for relPath, modTime := range modTimes {
		if slices.Contains(except, relPath) {
			continue
		}
		assert.True(t, modTime.Equal(backdated), "%s was rewritten but its content did not change", relPath)
	}
}

func walkFiles(t *testing.T, dir string, fn func(relPath string, info os.FileInfo)) {
	t.Helper()

	require.NoError(t, filepath.Walk(dir, func(fpath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		relPath, err := filepath.Rel(dir, fpath)
		if err != nil {
			return err
		}
		fn(relPath, info)
		return nil
	}))
}
