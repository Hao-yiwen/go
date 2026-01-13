// 版权所有 2021 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package types

import (
	"fmt"
	"go/version"
	"internal/goversion"
)

// 一个goVersion 是一个 Go language version string of the form "go1.%d"
// where d 是 minor version number. goVersion strings don't
// contain release numbers ("go1.20.1" is not a valid goVersion).
type goVersion string

// asGoVersion 返回v as a goVersion (e.g., "go1.20.1" becomes "go1.20").
// If v is not a valid Go version, the result 是 empty string.
func asGoVersion(v string) goVersion {
	return goVersion(version.Lang(v))
}

// isValid 报告whether v 是一个 valid Go version.
func (v goVersion) isValid() bool {
	return v != ""
}

// cmp 返回-1, 0, or +1 depending on whether x < y, x == y, or x > y,
// interpreted as Go versions.
func (x goVersion) cmp(y goVersion) int {
	return version.Compare(string(x), string(y))
}

var (
	// Go versions that introduced language changes
	go1_9  = asGoVersion("go1.9")
	go1_13 = asGoVersion("go1.13")
	go1_14 = asGoVersion("go1.14")
	go1_17 = asGoVersion("go1.17")
	go1_18 = asGoVersion("go1.18")
	go1_20 = asGoVersion("go1.20")
	go1_21 = asGoVersion("go1.21")
	go1_22 = asGoVersion("go1.22")
	go1_23 = asGoVersion("go1.23")
	go1_26 = asGoVersion("go1.26")

	// current (deployed) Go version
	go_current = asGoVersion(fmt.Sprintf("go1.%d", goversion.Version))
)

// allowVersion 报告whether the current effective Go version
// (which may vary from one file to another) 是一个llowed to use the
// feature version (want).
func (check *Checker) allowVersion(want goVersion) bool {
	return !check.version.isValid() || check.version.cmp(want) >= 0
}

// verifyVersionf is like allowVersion but also accepts a format string and arguments
// which are used to report a version error if allowVersion returns false.
func (check *Checker) verifyVersionf(at positioner, v goVersion, format string, args ...any) bool {
	if !check.allowVersion(v) {
		check.versionErrorf(at, v, format, args...)
		return false
	}
	return true
}
