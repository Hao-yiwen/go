// 版权所有 2024 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package strings

import (
	"iter"
	"unicode"
	"unicode/utf8"
)

// Lines 返回一个迭代器，遍历字符串 s 中以换行符结尾的行。
// 迭代器产生的行包含其终止换行符。
// 如果 s 为空，迭代器不会产生任何行。
// 如果 s 不以换行符结尾，最后产生的行将不以换行符结尾。
// 它返回一个一次性使用的迭代器。
func Lines(s string) iter.Seq[string] {
	return func(yield func(string) bool) {
		for len(s) > 0 {
			var line string
			if i := IndexByte(s, '\n'); i >= 0 {
				line, s = s[:i+1], s[i+1:]
			} else {
				line, s = s, ""
			}
			if !yield(line) {
				return
			}
		}
	}
}

// splitSeq 是 SplitSeq 或 SplitAfterSeq，通过配置结果中包含
// sep 的多少字节（无或全部）来区分。
func splitSeq(s, sep string, sepSave int) iter.Seq[string] {
	return func(yield func(string) bool) {
		if len(sep) == 0 {
			for len(s) > 0 {
				_, size := utf8.DecodeRuneInString(s)
				if !yield(s[:size]) {
					return
				}
				s = s[size:]
			}
			return
		}
		for {
			i := Index(s, sep)
			if i < 0 {
				break
			}
			frag := s[:i+sepSave]
			if !yield(frag) {
				return
			}
			s = s[i+len(sep):]
		}
		yield(s)
	}
}

// SplitSeq 返回一个迭代器，遍历 s 中由 sep 分隔的所有子字符串。
// 迭代器产生的字符串与 [Split](s, sep) 返回的相同，
// 但不构造切片。
// 它返回一个一次性使用的迭代器。
func SplitSeq(s, sep string) iter.Seq[string] {
	return splitSeq(s, sep, 0)
}

// SplitAfterSeq 返回一个迭代器，遍历在每个 sep 实例之后分割的 s 的子字符串。
// 迭代器产生的字符串与 [SplitAfter](s, sep) 返回的相同，
// 但不构造切片。
// 它返回一个一次性使用的迭代器。
func SplitAfterSeq(s, sep string) iter.Seq[string] {
	return splitSeq(s, sep, len(sep))
}

// FieldsSeq 返回一个迭代器，遍历围绕连续空白字符分割的 s 的子字符串，
// 空白字符由 [unicode.IsSpace] 定义。
// 迭代器产生的字符串与 [Fields](s) 返回的相同，
// 但不构造切片。
func FieldsSeq(s string) iter.Seq[string] {
	return func(yield func(string) bool) {
		start := -1
		for i := 0; i < len(s); {
			size := 1
			r := rune(s[i])
			isSpace := asciiSpace[s[i]] != 0
			if r >= utf8.RuneSelf {
				r, size = utf8.DecodeRuneInString(s[i:])
				isSpace = unicode.IsSpace(r)
			}
			if isSpace {
				if start >= 0 {
					if !yield(s[start:i]) {
						return
					}
					start = -1
				}
			} else if start < 0 {
				start = i
			}
			i += size
		}
		if start >= 0 {
			yield(s[start:])
		}
	}
}

// FieldsFuncSeq 返回一个迭代器，遍历围绕满足 f(c) 的连续
// Unicode 码点分割的 s 的子字符串。
// 迭代器产生的字符串与 [FieldsFunc](s) 返回的相同，
// 但不构造切片。
func FieldsFuncSeq(s string, f func(rune) bool) iter.Seq[string] {
	return func(yield func(string) bool) {
		start := -1
		for i := 0; i < len(s); {
			r, size := utf8.DecodeRuneInString(s[i:])
			if f(r) {
				if start >= 0 {
					if !yield(s[start:i]) {
						return
					}
					start = -1
				}
			} else if start < 0 {
				start = i
			}
			i += size
		}
		if start >= 0 {
			yield(s[start:])
		}
	}
}
