// Copyright (c) 2018 Palantir Technologies Inc. All rights reserved.
// Use of this source code is governed by the Apache License, Version 2.0
// that can be found in the LICENSE file.

package gomod

import (
	"crypto/sha256"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"reflect"
	"strings"

	"github.com/palantir/godel/v2/pkg/dirchecksum"
	"github.com/pkg/errors"
)

func Run(projectDir string, verify bool, stdout io.Writer) error {
	var goModChecksumBefore, goSumChecksumBefore [32]byte
	if verify {
		var err error
		goModChecksumBefore, goSumChecksumBefore, err = goModChecksums(projectDir)
		if err != nil {
			return err
		}
	}
	if err := run(stdout, "tidy"); err != nil {
		return err
	}
	if verify {
		goModChecksumAfter, goSumChecksumAfter, err := goModChecksums(projectDir)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(goModChecksumBefore, goModChecksumAfter) {
			return errors.Errorf("go.mod modified")
		}
		if !reflect.DeepEqual(goSumChecksumBefore, goSumChecksumAfter) {
			return errors.Errorf("go.sum modified")
		}
	}

	// if vendor mode is not set, do not perform vendor operations
	if !modVendorGoFlagsSet() {
		return nil
	}

	vendorDirPath := path.Join(projectDir, "vendor")
	var vendorChecksumBefore dirchecksum.ChecksumSet
	if verify {
		var err error
		vendorChecksumBefore, err = dirchecksum.ChecksumsForMatchingPaths(vendorDirPath, nil)
		if err != nil {
			return errors.Wrapf(err, "failed to compute checksums for %s", vendorDirPath)
		}
	}
	if err := run(stdout, "vendor"); err != nil {
		return err
	}
	if verify {
		vendorChecksumsAfter, err := dirchecksum.ChecksumsForMatchingPaths(vendorDirPath, nil)
		if err != nil {
			return errors.Wrapf(err, "failed to compute checksums for %s", vendorDirPath)
		}
		checksumDiff := vendorChecksumBefore.Diff(vendorChecksumsAfter)
		if len(checksumDiff.Diffs) > 0 {
			return errors.Errorf("vendor directory modified:\n%s", checksumDiff.String())
		}
	}
	return nil
}

func goModChecksums(projectDir string) (goModChecksum, goSumChecksum [32]byte, err error) {
	goModChecksum, err = fileChecksum(path.Join(projectDir, "go.mod"))
	if err != nil {
		return goModChecksum, goSumChecksum, err
	}
	goSumChecksum, err = fileChecksum(path.Join(projectDir, "go.sum"))
	if err != nil {
		return goModChecksum, goSumChecksum, err
	}
	return goModChecksum, goSumChecksum, nil
}

func fileChecksum(fpath string) ([32]byte, error) {
	fBytes, err := ioutil.ReadFile(fpath)
	if err != nil {
		return [32]byte{}, errors.Wrapf(err, "failed to read %s", fpath)
	}
	return sha256.Sum256(fBytes), nil
}

// modVendorGoFlagsSet returns true if the GOFLAGS environment variable contains the value "-mod=vendor".
func modVendorGoFlagsSet() bool {
	for _, flagField := range strings.Fields(os.Getenv("GOFLAGS")) {
		if flagField == "-mod=vendor" {
			return true
		}
	}
	return false
}

func run(stdout io.Writer, args ...string) error {
	cmd := exec.Command("go", append([]string{"mod"}, args...)...)
	cmd.Stdout = stdout
	cmd.Stderr = stdout
	if err := cmd.Run(); err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			// if error is not an exit error, wrap it
			return errors.Wrapf(err, "failed to execute command %v", cmd.Args)
		}
		// otherwise, return empty error
		return fmt.Errorf("")
	}
	return nil
}
