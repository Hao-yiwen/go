// 版权所有 2021 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package fs_test

import (
	"fmt"
	"io/fs"
	"log"
	"os"
	"testing/fstest"
)

func ExampleGlob() {
	fsys := fstest.MapFS{
		"file.txt":        {},
		"file.go":         {},
		"dir/file.txt":    {},
		"dir/file.go":     {},
		"dir/subdir/x.go": {},
	}

	patterns := []string{
		"*.txt",
		"*.go",
		"dir/*.go",
		"dir/*/x.go",
	}

	for _, pattern := range patterns {
		matches, err := fs.Glob(fsys, pattern)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%q matches: %v\n", pattern, matches)
	}

	// Output:
	// "*.txt" matches: [file.txt]
	// "*.go" matches: [file.go]
	// "dir/*.go" matches: [dir/file.go]
	// "dir/*/x.go" matches: [dir/subdir/x.go]
}

func ExampleReadFile() {
	fsys := fstest.MapFS{
		"hello.txt": {
			Data: []byte("Hello, World!\n"),
		},
	}

	data, err := fs.ReadFile(fsys, "hello.txt")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Print(string(data))

	// Output:
	// Hello, World!
}

func ExampleValidPath() {
	paths := []string{
		".",
		"x",
		"x/y/z",
		"",
		"..",
		"/x",
		"x/",
		"x//y",
		"x/./y",
		"x/../y",
	}

	for _, path := range paths {
		fmt.Printf("ValidPath(%q) = %t\n", path, fs.ValidPath(path))
	}

	// Output:
	// ValidPath(".") = true
	// ValidPath("x") = true
	// ValidPath("x/y/z") = true
	// ValidPath("") = false
	// ValidPath("..") = false
	// ValidPath("/x") = false
	// ValidPath("x/") = false
	// ValidPath("x//y") = false
	// ValidPath("x/./y") = false
	// ValidPath("x/../y") = false
}

func ExampleWalkDir() {
	root := "/usr/local/go/bin"
	fileSystem := os.DirFS(root)

	fs.WalkDir(fileSystem, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(path)
		return nil
	})
}
