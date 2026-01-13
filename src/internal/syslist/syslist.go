// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package syslist 存储操作系统和体系结构名称表，这些名称是
// （或曾经是）可接受的构建目标。

package syslist

// 注意：此文件由 internal/goarch/gengoarch.go 和
// internal/goos/gengoos.go 读取。如果您修改此文件，请查看那些
// 文件。

// KnownOS 是过去、现在和未来已知 GOOS 值的列表。
// 不要从此列表中删除，因为它用于文件名匹配。
// 如果您向此列表添加条目，请查看下面的 UnixOS。
var KnownOS = map[string]bool{
	"aix":       true,
	"android":   true,
	"darwin":    true,
	"dragonfly": true,
	"freebsd":   true,
	"hurd":      true,
	"illumos":   true,
	"ios":       true,
	"js":        true,
	"linux":     true,
	"nacl":      true,
	"netbsd":    true,
	"openbsd":   true,
	"plan9":     true,
	"solaris":   true,
	"wasip1":    true,
	"windows":   true,
	"zos":       true,
}

// UnixOS 是与 "unix" 构建标签匹配的 GOOS 值集合。
// 这不用于文件名匹配。
// 此列表也显示在 cmd/dist/build.go 中。
var UnixOS = map[string]bool{
	"aix":       true,
	"android":   true,
	"darwin":    true,
	"dragonfly": true,
	"freebsd":   true,
	"hurd":      true,
	"illumos":   true,
	"ios":       true,
	"linux":     true,
	"netbsd":    true,
	"openbsd":   true,
	"solaris":   true,
}

// KnownArch 是过去、现在和未来已知 GOARCH 值的列表。
// 不要从此列表中删除，因为它用于文件名匹配。
var KnownArch = map[string]bool{
	"386":         true,
	"amd64":       true,
	"amd64p32":    true,
	"arm":         true,
	"armbe":       true,
	"arm64":       true,
	"arm64be":     true,
	"loong64":     true,
	"mips":        true,
	"mipsle":      true,
	"mips64":      true,
	"mips64le":    true,
	"mips64p32":   true,
	"mips64p32le": true,
	"ppc":         true,
	"ppc64":       true,
	"ppc64le":     true,
	"riscv":       true,
	"riscv64":     true,
	"s390":        true,
	"s390x":       true,
	"sparc":       true,
	"sparc64":     true,
	"wasm":        true,
}
