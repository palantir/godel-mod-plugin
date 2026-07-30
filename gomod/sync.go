// Copyright (c) 2018 Palantir Technologies Inc. All rights reserved.
// Use of this source code is governed by the Apache License, Version 2.0
// that can be found in the LICENSE file.

package gomod

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
)

// maxDiffLines bounds the paths a dirDiff retains: an absent or entirely stale vendor directory differs by every file.
const maxDiffLines = 50

// compareBufSize is the chunk size used to compare file content without holding whole files in memory.
const compareBufSize = 64 * 1024

// tmpFilePrefix marks the temporary files copyFile renames into place. One left by an interrupted run is removed by the
// next sync, as an entry the source does not have.
const tmpFilePrefix = ".mod-plugin-tmp-"

// dirDiff records how a destination directory differs from a source directory. Only the first maxDiffLines are
// retained; count is the total.
type dirDiff struct {
	lines []string
	count int
}

func (d *dirDiff) empty() bool {
	return d.count == 0
}

// record notes a difference at relPath; a trailing separator means a directory and everything beneath it.
func (d *dirDiff) record(marker, relPath string) {
	d.count++
	if len(d.lines) < maxDiffLines {
		d.lines = append(d.lines, marker+" "+relPath)
	}
}

func (d *dirDiff) String() string {
	if omitted := d.count - len(d.lines); omitted > 0 {
		return strings.Join(append(slices.Clone(d.lines), fmt.Sprintf("...and %d more", omitted)), "\n")
	}
	return strings.Join(d.lines, "\n")
}

// syncDir determines how dstDir differs from srcDir and, if apply is true, makes dstDir match by writing only those
// differences. Content decides, not modification time. A srcDir that does not exist means dstDir is removed entirely.
func syncDir(srcDir, dstDir string, apply bool) (*dirDiff, error) {
	srcExists, err := dirExists(srcDir)
	if err != nil {
		return nil, err
	}
	dstExists, err := dirExists(dstDir)
	if err != nil {
		return nil, err
	}

	diff := &dirDiff{}
	if dstExists {
		// remove first so a type change (file to directory or the reverse) is out of the way before srcDir is written
		if err := diff.removeExtraneous(srcDir, dstDir, apply); err != nil {
			return nil, err
		}
	}
	if srcExists {
		if err := diff.writeMissingAndModified(srcDir, dstDir, apply); err != nil {
			return nil, err
		}
	} else if dstExists && apply {
		// nothing to vendor means no vendor directory at all
		if err := os.RemoveAll(dstDir); err != nil {
			return nil, err
		}
	}
	return diff, nil
}

// removeExtraneous records (and, if apply is true, deletes) every entry in dstDir that srcDir does not have. A missing
// directory goes as a whole, which is also what prunes directories that would be left empty.
func (d *dirDiff) removeExtraneous(srcDir, dstDir string, apply bool) error {
	return filepath.WalkDir(dstDir, func(dstPath string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(dstDir, dstPath)
		if err != nil {
			return err
		}
		if relPath == "." {
			return nil
		}

		srcPath := filepath.Join(srcDir, relPath)
		srcInfo, err := os.Lstat(srcPath)
		if err != nil && !isNotExist(err) {
			return err
		}
		if err == nil && srcInfo.IsDir() == entry.IsDir() {
			return nil
		}

		if !entry.IsDir() {
			d.record("-", relPath)
			if apply {
				if err := os.Remove(dstPath); err != nil {
					return err
				}
			}
			return nil
		}
		d.record("-", relPath+string(filepath.Separator))
		if apply {
			if err := os.RemoveAll(dstPath); err != nil {
				return err
			}
		}
		return fs.SkipDir
	})
}

// writeMissingAndModified records (and, if apply is true, writes) every srcDir entry that dstDir lacks or differs on.
func (d *dirDiff) writeMissingAndModified(srcDir, dstDir string, apply bool) error {
	comparer := newFileComparer()
	return filepath.WalkDir(srcDir, func(srcPath string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(srcDir, srcPath)
		if err != nil {
			return err
		}
		// directories need no comparison, so report-only mode can skip the stat
		if entry.IsDir() && !apply {
			return nil
		}
		srcInfo, err := entry.Info()
		if err != nil {
			return err
		}
		dstPath := filepath.Join(dstDir, relPath)

		if entry.IsDir() {
			if err := os.MkdirAll(dstPath, srcInfo.Mode().Perm()); err != nil {
				return err
			}
			return nil
		}

		// only content is compared: "go mod vendor" creates files subject to the umask, so counting mode as a
		// difference would rewrite every file under a different umask. Mode is still fixed up below, without a rewrite.
		switch dstInfo, err := os.Lstat(dstPath); {
		case isNotExist(err):
			d.record("+", relPath)
		case err != nil:
			return err
		case dstInfo.Mode().IsRegular() && dstInfo.Size() == srcInfo.Size():
			equal, err := comparer.equal(srcPath, dstPath)
			if err != nil {
				return err
			}
			if equal {
				if !apply {
					return nil
				}
				return chmodIfDifferent(dstPath, dstInfo.Mode(), srcInfo.Mode())
			}
			d.record("M", relPath)
		default:
			d.record("M", relPath)
		}

		if !apply {
			return nil
		}
		return copyFile(srcPath, dstPath, srcInfo.Mode().Perm())
	})
}

// fileComparer compares file content with buffers reused across calls, so comparing an up-to-date tree does not
// allocate per file. Not safe for concurrent use.
type fileComparer struct {
	aBuf, bBuf []byte
}

func newFileComparer() *fileComparer {
	return &fileComparer{
		aBuf: make([]byte, compareBufSize),
		bBuf: make([]byte, compareBufSize),
	}
}

// equal reports whether the two files have identical content, reading in chunks rather than whole files.
func (c *fileComparer) equal(aPath, bPath string) (bool, error) {
	aFile, err := os.Open(aPath)
	if err != nil {
		return false, err
	}
	defer func() {
		_ = aFile.Close()
	}()
	bFile, err := os.Open(bPath)
	if err != nil {
		return false, err
	}
	defer func() {
		_ = bFile.Close()
	}()

	for {
		aRead, aErr := io.ReadFull(aFile, c.aBuf)
		bRead, bErr := io.ReadFull(bFile, c.bBuf)
		// a failed read must not pass as differing content, which would silently overwrite the destination
		if aErr != nil && !isEOF(aErr) {
			return false, aErr
		}
		if bErr != nil && !isEOF(bErr) {
			return false, bErr
		}
		if aRead != bRead || !bytes.Equal(c.aBuf[:aRead], c.bBuf[:bRead]) {
			return false, nil
		}
		// any error still set here reports the end of a file: the files are equal if both ended together
		if aErr != nil || bErr != nil {
			return aErr != nil && bErr != nil, nil
		}
	}
}

func chmodIfDifferent(dstPath string, dstMode, srcMode fs.FileMode) error {
	if dstMode.Perm() == srcMode.Perm() {
		return nil
	}
	return os.Chmod(dstPath, srcMode.Perm())
}

// isEOF reports whether err marks the end of a file: io.ReadFull returns io.EOF or io.ErrUnexpectedEOF.
func isEOF(err error) bool {
	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)
}

// isNotExist reports whether err means the path is absent, ENOTDIR included: report-only mode looks up paths beneath an
// entry it has recorded for removal but not removed.
func isNotExist(err error) bool {
	return errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR)
}

// copyFile copies srcPath to dstPath with the given mode, via a temporary file in the destination directory so an
// interrupted run cannot leave a truncated file behind.
func copyFile(srcPath, dstPath string, mode fs.FileMode) (rErr error) {
	srcFile, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer func() {
		_ = srcFile.Close()
	}()

	dstDir := filepath.Dir(dstPath)
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return err
	}
	tmpFile, err := os.CreateTemp(dstDir, tmpFilePrefix)
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()
	closed := false
	defer func() {
		if !closed {
			_ = tmpFile.Close()
		}
		if rErr != nil {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := io.Copy(tmpFile, srcFile); err != nil {
		return fmt.Errorf("failed to copy %s to %s: %w", srcPath, tmpPath, err)
	}
	if err := tmpFile.Chmod(mode); err != nil {
		return err
	}
	// the content must reach the file before it takes the place of the destination
	closed = true
	if err := tmpFile.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, dstPath); err != nil {
		return err
	}
	return nil
}

func dirExists(dirPath string) (bool, error) {
	info, err := os.Stat(dirPath)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to stat %s: %w", dirPath, err)
	}
	return info.IsDir(), nil
}
