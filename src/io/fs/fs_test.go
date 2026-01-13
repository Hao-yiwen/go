// 版权所有 2020 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package fs_test

import (
	. "io/fs"
	"testing"
)

var isValidPathTests = []struct {
	name string
	ok   bool
}{
	{".", true},
	{"x", true},
	{"x/y", true},

	{"", false},
	{"..", false},
	{"/", false},
	{"x/", false},
	{"/x", false},
	{"x/y/", false},
	{"/x/y", false},
	{"./", false},
	{"./x", false},
	{"x/.", false},
	{"x/./y", false},
	{"../", false},
	{"../x", false},
	{"x/..", false},
	{"x/../y", false},
	{"x//y", false},
	{`x\`, true},
	{`x\y`, true},
	{`x:y`, true},
	{`\x`, true},
}

func TestValidPath(t *testing.T) {
	for _, tt := range isValidPathTests {
		ok := ValidPath(tt.name)
		if ok != tt.ok {
			t.Errorf("ValidPath(%q) = %v, want %v", tt.name, ok, tt.ok)
		}
	}
}
