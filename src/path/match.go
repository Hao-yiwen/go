// 版权所有 2010 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package path

import (
	"errors"
	"internal/bytealg"
	"unicode/utf8"
)

// ErrBadPattern 表示模式格式不正确。
var ErrBadPattern = errors.New("syntax error in pattern")

// Match 报告 name 是否匹配 shell 模式。
// 模式语法如下：
//
//	pattern:
//		{ term }
//	term:
//		'*'         匹配任意非 / 字符序列
//		'?'         匹配任意单个非 / 字符
//		'[' [ '^' ] { character-range } ']'
//		            字符类（必须非空）
//		c           匹配字符 c（c != '*', '?', '\\', '['）
//		'\\' c      匹配字符 c
//
//	character-range:
//		c           匹配字符 c（c != '\\', '-', ']'）
//		'\\' c      匹配字符 c
//		lo '-' hi   匹配满足 lo <= c <= hi 的字符 c
//
// Match 要求模式匹配整个 name，而不仅仅是子字符串。
// 唯一可能返回的错误是 [ErrBadPattern]，当模式格式不正确时返回。
func Match(pattern, name string) (matched bool, err error) {
Pattern:
	for len(pattern) > 0 {
		var star bool
		var chunk string
		star, chunk, pattern = scanChunk(pattern)
		if star && chunk == "" {
			// 尾部的 * 匹配字符串的其余部分，除非它包含 /。
			return bytealg.IndexByteString(name, '/') < 0, nil
		}
		// 在当前位置查找匹配。
		t, ok, err := matchChunk(chunk, name)
		// 如果我们是最后一个块，确保已经用尽 name
		// 否则即使我们仍然可以使用 star 匹配，也会得到错误的结果
		if ok && (len(t) == 0 || len(pattern) > 0) {
			name = t
			continue
		}
		if err != nil {
			return false, err
		}
		if star {
			// 跳过 i+1 个字节查找匹配。
			// 不能跳过 /。
			for i := 0; i < len(name) && name[i] != '/'; i++ {
				t, ok, err := matchChunk(chunk, name[i+1:])
				if ok {
					// 如果我们是最后一个块，确保已经用尽 name
					if len(pattern) == 0 && len(t) > 0 {
						continue
					}
					name = t
					continue Pattern
				}
				if err != nil {
					return false, err
				}
			}
		}
		// 在返回 false 且无错误之前，
		// 检查模式的剩余部分在语法上是否有效。
		for len(pattern) > 0 {
			_, chunk, pattern = scanChunk(pattern)
			if _, _, err := matchChunk(chunk, ""); err != nil {
				return false, err
			}
		}
		return false, nil
	}
	return len(name) == 0, nil
}

// scanChunk 获取模式的下一个片段，它是一个非星号字符串，
// 可能以星号开头。
func scanChunk(pattern string) (star bool, chunk, rest string) {
	for len(pattern) > 0 && pattern[0] == '*' {
		pattern = pattern[1:]
		star = true
	}
	inrange := false
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '\\':
			// 错误检查在 matchChunk 中处理：错误的模式。
			if i+1 < len(pattern) {
				i++
			}
		case '[':
			inrange = true
		case ']':
			inrange = false
		case '*':
			if !inrange {
				return star, pattern[:i], pattern[i:]
			}
		}
	}
	return star, pattern, ""
}

// matchChunk 检查 chunk 是否匹配 s 的开头。
// 如果匹配，它返回 s 的剩余部分（匹配之后的部分）。
// Chunk 全是单字符操作符：字面量、字符类和 ?。
func matchChunk(chunk, s string) (rest string, ok bool, err error) {
	// failed 记录匹配是否已失败。
	// 匹配失败后，循环继续处理 chunk，
	// 检查模式是否格式正确，但不再读取 s。
	failed := false
	for len(chunk) > 0 {
		failed = failed || len(s) == 0
		switch chunk[0] {
		case '[':
			// 字符类
			var r rune
			if !failed {
				var n int
				r, n = utf8.DecodeRuneInString(s)
				s = s[n:]
			}
			chunk = chunk[1:]
			// 可能是取反的
			negated := false
			if len(chunk) > 0 && chunk[0] == '^' {
				negated = true
				chunk = chunk[1:]
			}
			// 解析所有范围
			match := false
			nrange := 0
			for {
				if len(chunk) > 0 && chunk[0] == ']' && nrange > 0 {
					chunk = chunk[1:]
					break
				}
				var lo, hi rune
				if lo, chunk, err = getEsc(chunk); err != nil {
					return "", false, err
				}
				hi = lo
				if chunk[0] == '-' {
					if hi, chunk, err = getEsc(chunk[1:]); err != nil {
						return "", false, err
					}
				}
				match = match || lo <= r && r <= hi
				nrange++
			}
			failed = failed || match == negated

		case '?':
			if !failed {
				failed = s[0] == '/'
				_, n := utf8.DecodeRuneInString(s)
				s = s[n:]
			}
			chunk = chunk[1:]

		case '\\':
			chunk = chunk[1:]
			if len(chunk) == 0 {
				return "", false, ErrBadPattern
			}
			fallthrough

		default:
			if !failed {
				failed = chunk[0] != s[0]
				s = s[1:]
			}
			chunk = chunk[1:]
		}
	}
	if failed {
		return "", false, nil
	}
	return s, true, nil
}

// getEsc 从 chunk 中获取可能转义的字符，用于字符类。
func getEsc(chunk string) (r rune, nchunk string, err error) {
	if len(chunk) == 0 || chunk[0] == '-' || chunk[0] == ']' {
		err = ErrBadPattern
		return
	}
	if chunk[0] == '\\' {
		chunk = chunk[1:]
		if len(chunk) == 0 {
			err = ErrBadPattern
			return
		}
	}
	r, n := utf8.DecodeRuneInString(chunk)
	if r == utf8.RuneError && n == 1 {
		err = ErrBadPattern
	}
	nchunk = chunk[n:]
	if len(nchunk) == 0 {
		err = ErrBadPattern
	}
	return
}
