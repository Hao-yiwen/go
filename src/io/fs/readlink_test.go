// 版权所有 2023 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package fs_test

import (
	. "io/fs"
	"testing"
	"testing/fstest"
)

func TestReadLink(t *testing.T) {
	testFS := fstest.MapFS{
		"foo": {
			Data: []byte("bar"),
			Mode: ModeSymlink | 0o777,
		},
		"bar": {
			Data: []byte("Hello, World!\n"),
			Mode: 0o644,
		},

		"dir/parentlink": {
			Data: []byte("../bar"),
			Mode: ModeSymlink | 0o777,
		},
		"dir/link": {
			Data: []byte("file"),
			Mode: ModeSymlink | 0o777,
		},
		"dir/file": {
			Data: []byte("Hello, World!\n"),
			Mode: 0o644,
		},
	}

	check := func(fsys FS, name string, want string) {
		t.Helper()
		got, err := ReadLink(fsys, name)
		if got != want || err != nil {
			t.Errorf("ReadLink(%q) = %q, %v; want %q, <nil>", name, got, err, want)
		}
	}

	check(testFS, "foo", "bar")
	check(testFS, "dir/parentlink", "../bar")
	check(testFS, "dir/link", "file")

	// Test that ReadLink on Sub works.
	sub, err := Sub(testFS, "dir")
	if err != nil {
		t.Fatal(err)
	}

	check(sub, "link", "file")
	check(sub, "parentlink", "../bar")
}

func TestLstat(t *testing.T) {
	testFS := fstest.MapFS{
		"foo": {
			Data: []byte("bar"),
			Mode: ModeSymlink | 0o777,
		},
		"bar": {
			Data: []byte("Hello, World!\n"),
			Mode: 0o644,
		},

		"dir/parentlink": {
			Data: []byte("../bar"),
			Mode: ModeSymlink | 0o777,
		},
		"dir/link": {
			Data: []byte("file"),
			Mode: ModeSymlink | 0o777,
		},
		"dir/file": {
			Data: []byte("Hello, World!\n"),
			Mode: 0o644,
		},
	}

	check := func(fsys FS, name string, want FileMode) {
		t.Helper()
		info, err := Lstat(fsys, name)
		var got FileMode
		if err == nil {
			got = info.Mode()
		}
		if got != want || err != nil {
			t.Errorf("Lstat(%q) = %v, %v; want %v, <nil>", name, got, err, want)
		}
	}

	check(testFS, "foo", ModeSymlink|0o777)
	check(testFS, "bar", 0o644)

	// Test that Lstat on Sub works.
	sub, err := Sub(testFS, "dir")
	if err != nil {
		t.Fatal(err)
	}
	check(sub, "link", ModeSymlink|0o777)
}
