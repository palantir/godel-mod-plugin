// Copyright (c) 2026 Palantir Technologies Inc. All rights reserved.
// Use of this source code is governed by the Apache License, Version 2.0
// that can be found in the LICENSE file.

package gomod

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSyncDir(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  map[string]string
		dst  map[string]string
		// wantDiff is the report-only diff, forward-slashed; a trailing slash means a directory and its contents.
		wantDiff []string
	}{
		{
			name: "identical trees report no differences",
			src:  map[string]string{"a.go": "a", "sub/b.go": "b"},
			dst:  map[string]string{"a.go": "a", "sub/b.go": "b"},
		},
		{
			name:     "missing file is added",
			src:      map[string]string{"a.go": "a", "sub/b.go": "b"},
			dst:      map[string]string{"a.go": "a"},
			wantDiff: []string{"+ sub/b.go"},
		},
		{
			name:     "missing directory is added",
			src:      map[string]string{"a.go": "a", "sub/deep/b.go": "b"},
			dst:      map[string]string{"a.go": "a"},
			wantDiff: []string{"+ sub/deep/b.go"},
		},
		{
			name:     "differing content of differing length is modified",
			src:      map[string]string{"a.go": "new content"},
			dst:      map[string]string{"a.go": "old"},
			wantDiff: []string{"M a.go"},
		},
		{
			name:     "differing content of identical length is modified",
			src:      map[string]string{"a.go": "abc"},
			dst:      map[string]string{"a.go": "abd"},
			wantDiff: []string{"M a.go"},
		},
		{
			name:     "extraneous file is removed",
			src:      map[string]string{"a.go": "a"},
			dst:      map[string]string{"a.go": "a", "stale.go": "stale"},
			wantDiff: []string{"- stale.go"},
		},
		{
			name:     "extraneous directory is removed as a whole rather than file by file",
			src:      map[string]string{"keep/a.go": "a"},
			dst:      map[string]string{"keep/a.go": "a", "gone/b.go": "b", "gone/deep/c.go": "c"},
			wantDiff: []string{"- gone/"},
		},
		{
			name:     "directory left empty by a removal is itself removed",
			src:      map[string]string{"org/keep/a.go": "a"},
			dst:      map[string]string{"org/keep/a.go": "a", "org/drop/b.go": "b"},
			wantDiff: []string{"- org/drop/"},
		},
		{
			name:     "empty directory that the source does not have is removed",
			src:      map[string]string{"a.go": "a"},
			dst:      map[string]string{"a.go": "a", "empty/": ""},
			wantDiff: []string{"- empty/"},
		},
		{
			name:     "leftover temporary file from an interrupted run is removed",
			src:      map[string]string{"a.go": "a"},
			dst:      map[string]string{"a.go": "a", tmpFilePrefix + "987654": "partial"},
			wantDiff: []string{"- " + tmpFilePrefix + "987654"},
		},
		{
			name: "several differences are reported together",
			src:  map[string]string{"same.go": "s", "changed.go": "new", "added.go": "a"},
			dst:  map[string]string{"same.go": "s", "changed.go": "old", "removed.go": "r"},
			// removals are walked before additions and modifications
			wantDiff: []string{"- removed.go", "+ added.go", "M changed.go"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srcDir, dstDir := filepath.Join(t.TempDir(), "src"), filepath.Join(t.TempDir(), "dst")
			writeTree(t, srcDir, tc.src)
			writeTree(t, dstDir, tc.dst)
			dstBefore := readTree(t, dstDir)

			diff, err := syncDir(srcDir, dstDir, false)
			require.NoError(t, err)
			assert.Equal(t, tc.wantDiff, diffLines(diff))
			assert.Equal(t, len(tc.wantDiff), diff.count)
			assert.Equal(t, len(tc.wantDiff) == 0, diff.empty())
			assert.Equal(t, dstBefore, readTree(t, dstDir), "report-only mode modified the destination")

			diff, err = syncDir(srcDir, dstDir, true)
			require.NoError(t, err)
			assert.Equal(t, len(tc.wantDiff), diff.count)
			assert.Equal(t, readTree(t, srcDir), readTree(t, dstDir))

			// applying converges: a second pass finds nothing
			diff, err = syncDir(srcDir, dstDir, false)
			require.NoError(t, err)
			assert.True(t, diff.empty(), "destination still differs after being synced: %s", diff)
		})
	}
}

// TestSyncDirTypeChange covers an entry whose type differs between the two trees.
func TestSyncDirTypeChange(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  map[string]string
		dst  map[string]string
	}{
		{
			name: "destination file is replaced by a directory",
			src:  map[string]string{"x/a.go": "a"},
			dst:  map[string]string{"x": "x was a file"},
		},
		{
			name: "destination directory is replaced by a file",
			src:  map[string]string{"x": "x is now a file"},
			dst:  map[string]string{"x/a.go": "a"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srcDir, dstDir := filepath.Join(t.TempDir(), "src"), filepath.Join(t.TempDir(), "dst")
			writeTree(t, srcDir, tc.src)
			writeTree(t, dstDir, tc.dst)

			diff, err := syncDir(srcDir, dstDir, false)
			require.NoError(t, err)
			assert.False(t, diff.empty(), "type change was not reported as a difference")

			_, err = syncDir(srcDir, dstDir, true)
			require.NoError(t, err)
			assert.Equal(t, readTree(t, srcDir), readTree(t, dstDir))

			diff, err = syncDir(srcDir, dstDir, false)
			require.NoError(t, err)
			assert.True(t, diff.empty(), "destination still differs after being synced: %s", diff)
		})
	}
}

func TestSyncDirWhenSourceIsMissing(t *testing.T) {
	t.Run("destination is removed entirely", func(t *testing.T) {
		srcDir := filepath.Join(t.TempDir(), "does-not-exist")
		dstDir := filepath.Join(t.TempDir(), "dst")
		writeTree(t, dstDir, map[string]string{"a.go": "a", "sub/b.go": "b"})

		diff, err := syncDir(srcDir, dstDir, false)
		require.NoError(t, err)
		assert.False(t, diff.empty(), "a populated destination with no source was not reported as a difference")
		assert.DirExists(t, dstDir, "report-only mode removed the destination")

		_, err = syncDir(srcDir, dstDir, true)
		require.NoError(t, err)
		assert.NoDirExists(t, dstDir)
	})

	t.Run("a destination that does not exist either is not a difference", func(t *testing.T) {
		tmpDir := t.TempDir()
		srcDir, dstDir := filepath.Join(tmpDir, "no-src"), filepath.Join(tmpDir, "no-dst")

		for _, apply := range []bool{false, true} {
			diff, err := syncDir(srcDir, dstDir, apply)
			require.NoError(t, err)
			assert.True(t, diff.empty())
			assert.NoDirExists(t, dstDir)
		}
	})
}

// TestSyncDirDoesNotRewriteUnchangedFiles covers the property the sync exists for, seen through modification times.
func TestSyncDirDoesNotRewriteUnchangedFiles(t *testing.T) {
	srcDir, dstDir := filepath.Join(t.TempDir(), "src"), filepath.Join(t.TempDir(), "dst")
	tree := map[string]string{"a.go": "a", "sub/b.go": "b", "sub/c.go": "c"}
	writeTree(t, srcDir, tree)
	writeTree(t, dstDir, tree)

	backdated := backdateTree(t, dstDir)
	diff, err := syncDir(srcDir, dstDir, true)
	require.NoError(t, err)
	require.True(t, diff.empty())
	for relPath, modTime := range modTimes(t, dstDir) {
		assert.True(t, modTime.Equal(backdated), "%s was rewritten but its content did not change", relPath)
	}

	// only the file whose content changed may be rewritten
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "sub", "b.go"), []byte("b changed"), 0644))
	_, err = syncDir(srcDir, dstDir, true)
	require.NoError(t, err)
	for relPath, modTime := range modTimes(t, dstDir) {
		if relPath == "sub/b.go" {
			assert.True(t, modTime.After(backdated), "%s was not rewritten", relPath)
			continue
		}
		assert.True(t, modTime.Equal(backdated), "%s was rewritten but its content did not change", relPath)
	}
}

// TestSyncDirModeOnlyDifference pins the decision that mode is not part of the comparison: under a different umask,
// counting it would rewrite every file and fail verification on a byte-for-byte correct tree.
func TestSyncDirModeOnlyDifference(t *testing.T) {
	srcDir, dstDir := filepath.Join(t.TempDir(), "src"), filepath.Join(t.TempDir(), "dst")
	writeTree(t, srcDir, map[string]string{"a.go": "a"})
	writeTree(t, dstDir, map[string]string{"a.go": "a"})
	srcPath, dstPath := filepath.Join(srcDir, "a.go"), filepath.Join(dstDir, "a.go")
	require.NoError(t, os.Chmod(srcPath, 0600))
	require.NoError(t, os.Chmod(dstPath, 0644))

	backdated := backdateTree(t, dstDir)
	diff, err := syncDir(srcDir, dstDir, false)
	require.NoError(t, err)
	assert.True(t, diff.empty(), "a mode-only difference was reported as a difference")

	diff, err = syncDir(srcDir, dstDir, true)
	require.NoError(t, err)
	assert.True(t, diff.empty())

	dstInfo, err := os.Stat(dstPath)
	require.NoError(t, err)
	assert.Equal(t, fs.FileMode(0600), dstInfo.Mode().Perm(), "mode was not brought in line")
	assert.True(t, dstInfo.ModTime().Equal(backdated), "content was rewritten to change the mode")
}

func TestSyncDirCopiesModeOfNewFiles(t *testing.T) {
	srcDir, dstDir := filepath.Join(t.TempDir(), "src"), filepath.Join(t.TempDir(), "dst")
	writeTree(t, srcDir, map[string]string{"a.go": "a"})
	require.NoError(t, os.Chmod(filepath.Join(srcDir, "a.go"), 0640))

	_, err := syncDir(srcDir, dstDir, true)
	require.NoError(t, err)

	dstInfo, err := os.Stat(filepath.Join(dstDir, "a.go"))
	require.NoError(t, err)
	assert.Equal(t, fs.FileMode(0640), dstInfo.Mode().Perm())
}

func TestSyncDirLeavesNoTemporaryFiles(t *testing.T) {
	srcDir, dstDir := filepath.Join(t.TempDir(), "src"), filepath.Join(t.TempDir(), "dst")
	writeTree(t, srcDir, map[string]string{"a.go": "a", "sub/b.go": "b"})
	writeTree(t, dstDir, map[string]string{"a.go": "differs"})

	_, err := syncDir(srcDir, dstDir, true)
	require.NoError(t, err)

	for relPath := range readTree(t, dstDir) {
		assert.False(t, strings.HasPrefix(filepath.Base(relPath), tmpFilePrefix), "temporary file %s was left behind", relPath)
	}
}

func TestDirDiff(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		diff := &dirDiff{}
		assert.True(t, diff.empty())
		assert.Empty(t, diff.String())
	})

	t.Run("records each marker", func(t *testing.T) {
		diff := &dirDiff{}
		diff.record("+", "added.go")
		diff.record("M", "changed.go")
		diff.record("-", "removed.go")
		assert.False(t, diff.empty())
		assert.Equal(t, 3, diff.count)
		assert.Equal(t, "+ added.go\nM changed.go\n- removed.go", diff.String())
	})

	t.Run("retains at most maxDiffLines paths but counts them all", func(t *testing.T) {
		diff := &dirDiff{}
		const recorded = maxDiffLines + 7
		for range recorded {
			diff.record("+", "file")
		}
		assert.Equal(t, recorded, diff.count)
		assert.Len(t, diff.lines, maxDiffLines, "more paths were retained than are ever rendered")

		lines := strings.Split(diff.String(), "\n")
		assert.Len(t, lines, maxDiffLines+1)
		assert.Equal(t, "...and 7 more", lines[len(lines)-1])
	})

	t.Run("renders exactly maxDiffLines without a summary line", func(t *testing.T) {
		diff := &dirDiff{}
		for range maxDiffLines {
			diff.record("+", "file")
		}
		lines := strings.Split(diff.String(), "\n")
		assert.Len(t, lines, maxDiffLines)
		assert.NotContains(t, diff.String(), "more")
	})
}

func TestFileComparerEqual(t *testing.T) {
	for _, tc := range []struct {
		name      string
		a, b      string
		wantEqual bool
	}{
		{name: "both empty", a: "", b: "", wantEqual: true},
		{name: "one empty", a: "", b: "a"},
		{name: "identical short", a: "hello", b: "hello", wantEqual: true},
		{name: "differ at first byte", a: "aaa", b: "baa"},
		{name: "differ at last byte", a: "aaa", b: "aab"},
		{name: "differ in length", a: "aa", b: "aaa"},
		{
			name:      "identical at exactly the buffer size",
			a:         strings.Repeat("a", compareBufSize),
			b:         strings.Repeat("a", compareBufSize),
			wantEqual: true,
		},
		{
			name:      "identical one byte past the buffer size",
			a:         strings.Repeat("a", compareBufSize+1),
			b:         strings.Repeat("a", compareBufSize+1),
			wantEqual: true,
		},
		{
			name:      "identical across several buffers",
			a:         strings.Repeat("ab", compareBufSize*2),
			b:         strings.Repeat("ab", compareBufSize*2),
			wantEqual: true,
		},
		{
			name: "differ only in a later buffer",
			a:    strings.Repeat("a", compareBufSize) + "x",
			b:    strings.Repeat("a", compareBufSize) + "y",
		},
		{
			name: "differ only in the final byte of a multi-buffer file",
			a:    strings.Repeat("a", compareBufSize*2+10),
			b:    strings.Repeat("a", compareBufSize*2+9) + "b",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			aPath, bPath := filepath.Join(dir, "a"), filepath.Join(dir, "b")
			require.NoError(t, os.WriteFile(aPath, []byte(tc.a), 0644))
			require.NoError(t, os.WriteFile(bPath, []byte(tc.b), 0644))

			equal, err := newFileComparer().equal(aPath, bPath)
			require.NoError(t, err)
			assert.Equal(t, tc.wantEqual, equal)
		})
	}
}

// TestFileComparerReusesBuffers covers that a comparison is not influenced by the previous one. The first returns early
// leaving different data in the two buffers, which a whole-buffer comparison would then see.
func TestFileComparerReusesBuffers(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) string {
		fpath := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(fpath, []byte(content), 0644))
		return fpath
	}
	longA := write("longA", strings.Repeat("a", compareBufSize*2))
	longB := write("longB", strings.Repeat("a", compareBufSize)+strings.Repeat("b", compareBufSize))
	shortA := write("shortA", "same")
	shortB := write("shortB", "same")

	comparer := newFileComparer()
	equal, err := comparer.equal(longA, longB)
	require.NoError(t, err)
	require.False(t, equal)

	equal, err = comparer.equal(shortA, shortB)
	require.NoError(t, err)
	assert.True(t, equal, "comparison was influenced by the previous one")
}

func TestFileComparerEqualMissingFile(t *testing.T) {
	dir := t.TempDir()
	present := filepath.Join(dir, "present")
	require.NoError(t, os.WriteFile(present, []byte("a"), 0644))
	missing := filepath.Join(dir, "missing")

	// an unreadable file must be an error, not differing content, and must name the file
	_, err := newFileComparer().equal(missing, present)
	assert.ErrorIs(t, err, fs.ErrNotExist)
	assert.ErrorContains(t, err, missing)

	_, err = newFileComparer().equal(present, missing)
	assert.ErrorIs(t, err, fs.ErrNotExist)
	assert.ErrorContains(t, err, missing)
}

func TestCopyFile(t *testing.T) {
	t.Run("copies content and mode", func(t *testing.T) {
		dir := t.TempDir()
		srcPath, dstPath := filepath.Join(dir, "src"), filepath.Join(dir, "dst")
		require.NoError(t, os.WriteFile(srcPath, []byte("content"), 0644))

		require.NoError(t, copyFile(srcPath, dstPath, 0640))

		content, err := os.ReadFile(dstPath)
		require.NoError(t, err)
		assert.Equal(t, "content", string(content))
		dstInfo, err := os.Stat(dstPath)
		require.NoError(t, err)
		assert.Equal(t, fs.FileMode(0640), dstInfo.Mode().Perm())
	})

	t.Run("replaces an existing file", func(t *testing.T) {
		dir := t.TempDir()
		srcPath, dstPath := filepath.Join(dir, "src"), filepath.Join(dir, "dst")
		require.NoError(t, os.WriteFile(srcPath, []byte("new"), 0644))
		require.NoError(t, os.WriteFile(dstPath, []byte("old and longer"), 0644))

		require.NoError(t, copyFile(srcPath, dstPath, 0644))

		content, err := os.ReadFile(dstPath)
		require.NoError(t, err)
		assert.Equal(t, "new", string(content))
	})

	t.Run("creates missing parent directories", func(t *testing.T) {
		dir := t.TempDir()
		srcPath := filepath.Join(dir, "src")
		dstPath := filepath.Join(dir, "a", "b", "c", "dst")
		require.NoError(t, os.WriteFile(srcPath, []byte("content"), 0644))

		require.NoError(t, copyFile(srcPath, dstPath, 0644))
		assert.FileExists(t, dstPath)
	})

	t.Run("leaves no temporary file behind", func(t *testing.T) {
		dir := t.TempDir()
		srcPath, dstPath := filepath.Join(dir, "src"), filepath.Join(dir, "dst")
		require.NoError(t, os.WriteFile(srcPath, []byte("content"), 0644))

		require.NoError(t, copyFile(srcPath, dstPath, 0644))

		entries, err := os.ReadDir(dir)
		require.NoError(t, err)
		assert.Len(t, entries, 2, "expected only the source and destination in %s", dir)
	})

	t.Run("reports a missing source without creating the destination", func(t *testing.T) {
		dir := t.TempDir()
		dstPath := filepath.Join(dir, "dst")
		srcPath := filepath.Join(dir, "missing")

		err := copyFile(srcPath, dstPath, 0644)
		assert.ErrorIs(t, err, fs.ErrNotExist)
		assert.ErrorContains(t, err, srcPath)
		assert.NoFileExists(t, dstPath)
	})
}

func TestChmodIfDifferent(t *testing.T) {
	dir := t.TempDir()
	fpath := filepath.Join(dir, "f")
	require.NoError(t, os.WriteFile(fpath, []byte("content"), 0644))

	// a mode that already matches leaves the file alone
	require.NoError(t, chmodIfDifferent(fpath, 0644, 0644))
	info, err := os.Stat(fpath)
	require.NoError(t, err)
	assert.Equal(t, fs.FileMode(0644), info.Mode().Perm())

	require.NoError(t, chmodIfDifferent(fpath, 0644, 0600))
	info, err = os.Stat(fpath)
	require.NoError(t, err)
	assert.Equal(t, fs.FileMode(0600), info.Mode().Perm())

	// a file that does not exist is only an error when a change is actually needed
	missing := filepath.Join(dir, "missing")
	require.NoError(t, chmodIfDifferent(missing, 0644, 0644))
	err = chmodIfDifferent(missing, 0644, 0600)
	assert.ErrorIs(t, err, fs.ErrNotExist)
	assert.ErrorContains(t, err, missing)
}

func TestDirExists(t *testing.T) {
	dir := t.TempDir()
	fpath := filepath.Join(dir, "f")
	require.NoError(t, os.WriteFile(fpath, []byte("content"), 0644))

	for _, tc := range []struct {
		name  string
		path  string
		want  bool
		isDir bool
	}{
		{name: "existing directory", path: dir, want: true},
		{name: "missing path", path: filepath.Join(dir, "missing")},
		{name: "existing file is not a directory", path: fpath},
	} {
		t.Run(tc.name, func(t *testing.T) {
			exists, err := dirExists(tc.path)
			require.NoError(t, err)
			assert.Equal(t, tc.want, exists)
		})
	}
}

// writeTree creates the described entries under dir. Keys are slash-separated; a trailing slash makes an empty dir.
func writeTree(t *testing.T, dir string, entries map[string]string) {
	t.Helper()

	require.NoError(t, os.MkdirAll(dir, 0755))
	for relPath, content := range entries {
		fpath := filepath.Join(dir, filepath.FromSlash(relPath))
		if strings.HasSuffix(relPath, "/") {
			require.NoError(t, os.MkdirAll(fpath, 0755))
			continue
		}
		require.NoError(t, os.MkdirAll(filepath.Dir(fpath), 0755))
		require.NoError(t, os.WriteFile(fpath, []byte(content), 0644))
	}
}

// readTree returns the content of every regular file under dir, keyed by relative slash-separated path. A missing dir
// yields no content rather than an error.
func readTree(t *testing.T, dir string) map[string]string {
	t.Helper()

	entries := make(map[string]string)
	walkTree(t, dir, func(relPath string, _ fs.DirEntry) {
		content, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(relPath)))
		require.NoError(t, err)
		entries[relPath] = string(content)
	})
	return entries
}

// modTimes returns the modification time of every regular file under dir, keyed by relative path.
func modTimes(t *testing.T, dir string) map[string]time.Time {
	t.Helper()

	times := make(map[string]time.Time)
	walkTree(t, dir, func(relPath string, entry fs.DirEntry) {
		info, err := entry.Info()
		require.NoError(t, err)
		times[relPath] = info.ModTime()
	})
	return times
}

// backdateTree pushes every file under dir into the past so that anything written afterwards stands out. Truncated to a
// whole second because not every filesystem records finer.
func backdateTree(t *testing.T, dir string) time.Time {
	t.Helper()

	backdated := time.Now().Add(-time.Hour).Truncate(time.Second)
	walkTree(t, dir, func(relPath string, _ fs.DirEntry) {
		require.NoError(t, os.Chtimes(filepath.Join(dir, filepath.FromSlash(relPath)), time.Time{}, backdated))
	})
	return backdated
}

func walkTree(t *testing.T, dir string, fn func(relPath string, entry fs.DirEntry)) {
	t.Helper()

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return
	}
	require.NoError(t, filepath.WalkDir(dir, func(fpath string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		relPath, err := filepath.Rel(dir, fpath)
		if err != nil {
			return err
		}
		fn(filepath.ToSlash(relPath), entry)
		return nil
	}))
}

// diffLines returns the recorded differences with paths normalized to forward slashes.
func diffLines(d *dirDiff) []string {
	if len(d.lines) == 0 {
		return nil
	}
	lines := make([]string, 0, len(d.lines))
	for _, line := range d.lines {
		lines = append(lines, filepath.ToSlash(line))
	}
	return lines
}
