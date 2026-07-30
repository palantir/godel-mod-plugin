// Copyright (c) 2018 Palantir Technologies Inc. All rights reserved.
// Use of this source code is governed by the Apache License, Version 2.0
// that can be found in the LICENSE file.

package gomod

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

func Run(projectDir string, verify bool, stdout io.Writer) error {
	if err := runTidy(verify, stdout); err != nil {
		return err
	}
	// if vendor mode is not set, do not perform vendor operations
	if !modVendorGoFlagsSet() {
		return nil
	}
	return runVendor(projectDir, verify, stdout)
}

// runTidy runs "go mod tidy". Verification adds "-diff", which prints what tidying would change and exits non-zero
// without touching go.mod or go.sum.
func runTidy(verify bool, stdout io.Writer) error {
	if verify {
		return run(stdout, "tidy", "-diff")
	}
	return run(stdout, "tidy")
}

// runVendor updates the project's vendor directory. "go mod vendor" resets the whole tree, rewriting every file whether
// or not it changed, so it is generated into a temporary directory and only the differences are written back.
func runVendor(projectDir string, verify bool, stdout io.Writer) error {
	tmpDir, err := os.MkdirTemp("", "godel-mod-plugin-")
	if err != nil {
		return fmt.Errorf("failed to create temporary directory: %w", err)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	// absolute because run does not set the command's working directory. "go mod vendor" removes this directory
	// rather than creating it when there is nothing to vendor.
	generatedDir := filepath.Join(tmpDir, "vendor")
	if err := run(stdout, "vendor", "-o", generatedDir); err != nil {
		return err
	}

	diff, err := syncDir(generatedDir, filepath.Join(projectDir, "vendor"), !verify)
	if err != nil {
		return err
	}
	if verify && !diff.empty() {
		return fmt.Errorf("vendor directory out of date:\n%s", diff)
	}
	return nil
}

// modVendorGoFlagsSet returns true if the GOFLAGS environment variable contains the value "-mod=vendor".
func modVendorGoFlagsSet() bool {
	return slices.Contains(strings.Fields(os.Getenv("GOFLAGS")), "-mod=vendor")
}

func run(stdout io.Writer, args ...string) error {
	cmd := exec.Command("go", append([]string{"mod"}, args...)...)
	cmd.Stdout = stdout
	cmd.Stderr = stdout
	if err := cmd.Run(); err != nil {
		var exitError *exec.ExitError
		if !errors.As(err, &exitError) {
			// if error is not an exit error, wrap it
			return fmt.Errorf("failed to execute command %v: %w", cmd.Args, err)
		}
		// otherwise, return empty error
		return fmt.Errorf("")
	}
	return nil
}
