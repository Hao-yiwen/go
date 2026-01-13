// 版权所有 2021 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package strings_test

import (
	"strings"
	"testing"
	"unsafe"
)

var emptyString string

func TestClone(t *testing.T) {
	var cloneTests = []string{
		"",
		strings.Clone(""),
		strings.Repeat("a", 42)[:0],
		"short",
		strings.Repeat("a", 42),
	}
	for _, input := range cloneTests {
		clone := strings.Clone(input)
		if clone != input {
			t.Errorf("Clone(%q) = %q; want %q", input, clone, input)
		}

		if len(input) != 0 && unsafe.StringData(clone) == unsafe.StringData(input) {
			t.Errorf("Clone(%q) return value should not reference inputs backing memory.", input)
		}

		if len(input) == 0 && unsafe.StringData(clone) != unsafe.StringData(emptyString) {
			t.Errorf("Clone(%#v) return value should be equal to empty string.", unsafe.StringData(input))
		}
	}
}

func BenchmarkClone(b *testing.B) {
	var str = strings.Repeat("a", 42)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		stringSink = strings.Clone(str)
	}
}
