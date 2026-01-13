// 版权所有 2020 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

//go:build goexperiment.jsonv2

package json

import (
	"unicode"
	"unicode/utf8"
)

// foldName 返回a folded string such that foldName(x) == foldName(y)
// is similar to strings.EqualFold(x, y), but ignores underscore and dashes.
// This 允许foldName to match common naming conventions.
func foldName(in []byte) []byte {
	// This is inlinable to take advantage of "function outlining".
	// See https://blog.filippo.io/efficient-go-apis-with-the-inliner/
	var arr [32]byte // large enough for most JSON names
	return appendFoldedName(arr[:0], in)
}
func appendFoldedName(out, in []byte) []byte {
	for i := 0; i < len(in); {
		// Handle single-byte ASCII.
		if c := in[i]; c < utf8.RuneSelf {
			if c != '_' && c != '-' {
				if 'a' <= c && c <= 'z' {
					c -= 'a' - 'A'
				}
				out = append(out, c)
			}
			i++
			continue
		}
		// Handle multi-byte Unicode.
		r, n := utf8.DecodeRune(in[i:])
		out = utf8.AppendRune(out, foldRune(r))
		i += n
	}
	return out
}

// foldRune 是一个 variation on unicode.SimpleFold that 返回 same rune
// for all runes in the same fold set.
//
// Invariant:
//
//	foldRune(x) == foldRune(y) ⇔ strings.EqualFold(string(x), string(y))
func foldRune(r rune) rune {
	for {
		r2 := unicode.SimpleFold(r)
		if r2 <= r {
			return r2 // smallest character in the fold set
		}
		r = r2
	}
}
