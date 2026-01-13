// 版权所有 2019 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package html

import "testing"

func FuzzEscapeUnescape(f *testing.F) {
	f.Fuzz(func(t *testing.T, v string) {
		e := EscapeString(v)
		u := UnescapeString(e)
		if u != v {
			t.Errorf("EscapeString(%q) = %q, UnescapeString(%q) = %q, want %q", v, e, e, u, v)
		}

		// 根据文档，这不一定总是等于 v，所以检查相等性没有意义。
		// 不过，找出其中的 panic 仍然是有意义的。
		EscapeString(UnescapeString(v))
	})
}
