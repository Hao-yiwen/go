// 版权所有 2023 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// version 包提供 operations on [Go versions]
// in [Go toolchain name syntax]: strings like
// "go1.20", "go1.21.0", "go1.22rc2", and "go1.23.4-custom".
//
// [Go versions]: https://go.dev/doc/toolchain#version
// [Go toolchain name syntax]: https://go.dev/doc/toolchain#name
package version // import "go/version"

import (
	"internal/gover"
	"strings"
)

// stripGo converts from a "go1.21-custom" version to a "1.21" version.
// If v does not start with "go", stripGo 返回 empty string (a known invalid version).
func stripGo(v string) string {
	v, _, _ = strings.Cut(v, "-") // strip -custom suffix.
	if len(v) < 2 || v[:2] != "go" {
		return ""
	}
	return v[2:]
}

// Lang 返回the Go language version for version x.
// If x is not a valid version, Lang 返回 empty string.
// For example:
//
//	Lang("go1.21rc2") = "go1.21"
//	Lang("go1.21.2") = "go1.21"
//	Lang("go1.21") = "go1.21"
//	Lang("go1") = "go1"
//	Lang("bad") = ""
//	Lang("1.21") = ""
func Lang(x string) string {
	v := gover.Lang(stripGo(x))
	if v == "" {
		return ""
	}
	if strings.HasPrefix(x[2:], v) {
		return x[:2+len(v)] // "go"+v without allocation
	} else {
		return "go" + v
	}
}

// Compare 返回-1, 0, or +1 depending on whether
// x < y, x == y, or x > y, interpreted as Go versions.
// The versions x and y must begin with a "go" prefix: "go1.21" not "1.21".
// Invalid versions, including the empty string, compare less than
// valid versions and equal to each other.
// The language version "go1.21" compares less than the
// release candidate and eventual releases "go1.21rc1" and "go1.21.0".
func Compare(x, y string) int {
	return gover.Compare(stripGo(x), stripGo(y))
}

// IsValid 报告whether the version x is valid.
func IsValid(x string) bool {
	return gover.IsValid(stripGo(x))
}
