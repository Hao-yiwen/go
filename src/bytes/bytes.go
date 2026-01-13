// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package bytes 实现了字节切片的操作函数。
// 它类似于 [strings] 包的功能。
package bytes

import (
	"internal/bytealg"
	"math/bits"
	"unicode"
	"unicode/utf8"
	_ "unsafe" // for linkname
)

// Equal 报告 a 和 b 是否
// 长度相同并包含相同的字节。
// nil 参数等同于空切片。
func Equal(a, b []byte) bool {
	// cmd/compile 和 gccgo 都不为这些字符串转换分配内存。
	return string(a) == string(b)
}

// Compare 返回一个整数，按字典序比较两个字节切片。
// 如果 a == b 结果为 0，a < b 结果为 -1，a > b 结果为 +1。
// nil 参数等同于空切片。
func Compare(a, b []byte) int {
	return bytealg.Compare(a, b)
}

// explode 将 s 拆分为 UTF-8 序列的切片，每个 Unicode 代码点一个（仍为字节切片），
// 最多为 n 个字节切片。无效的 UTF-8 序列被分解为单个字节。
func explode(s []byte, n int) [][]byte {
	if n <= 0 || n > len(s) {
		n = len(s)
	}
	a := make([][]byte, n)
	var size int
	na := 0
	for len(s) > 0 {
		if na+1 >= n {
			a[na] = s
			na++
			break
		}
		_, size = utf8.DecodeRune(s)
		a[na] = s[0:size:size]
		s = s[size:]
		na++
	}
	return a[0:na]
}

// Count 计算 s 中 sep 的非重叠实例数。
// 如果 sep 是空切片，Count 返回 1 + s 中 UTF-8 编码的代码点数。
func Count(s, sep []byte) int {
	// 特殊情况
	if len(sep) == 0 {
		return utf8.RuneCount(s) + 1
	}
	if len(sep) == 1 {
		return bytealg.Count(s, sep[0])
	}
	n := 0
	for {
		i := Index(s, sep)
		if i == -1 {
			return n
		}
		n++
		s = s[i+len(sep):]
	}
}

// Contains 报告 subslice 是否在 b 中。
func Contains(b, subslice []byte) bool {
	return Index(b, subslice) != -1
}

// ContainsAny 报告 chars 中是否有任何 UTF-8 编码的代码点在 b 中。
func ContainsAny(b []byte, chars string) bool {
	return IndexAny(b, chars) >= 0
}

// ContainsRune 报告该 rune 是否包含在 UTF-8 编码的字节切片 b 中。
func ContainsRune(b []byte, r rune) bool {
	return IndexRune(b, r) >= 0
}

// ContainsFunc 报告 b 中是否有任何 UTF-8 编码的代码点 r 满足 f(r)。
func ContainsFunc(b []byte, f func(rune) bool) bool {
	return IndexFunc(b, f) >= 0
}

// IndexByte 返回 c 在 b 中第一个实例的索引，如果 c 不在 b 中返回 -1。
func IndexByte(b []byte, c byte) int {
	return bytealg.IndexByte(b, c)
}

func indexBytePortable(s []byte, c byte) int {
	for i, b := range s {
		if b == c {
			return i
		}
	}
	return -1
}

// LastIndex 返回 sep 在 s 中最后一个实例的索引，如果 sep 不在 s 中返回 -1。
func LastIndex(s, sep []byte) int {
	n := len(sep)
	switch {
	case n == 0:
		return len(s)
	case n == 1:
		return bytealg.LastIndexByte(s, sep[0])
	case n == len(s):
		if Equal(s, sep) {
			return 0
		}
		return -1
	case n > len(s):
		return -1
	}
	return bytealg.LastIndexRabinKarp(s, sep)
}

// LastIndexByte 返回 c 在 s 中最后一个实例的索引，如果 c 不在 s 中返回 -1。
func LastIndexByte(s []byte, c byte) int {
	return bytealg.LastIndexByte(s, c)
}

// IndexRune 将 s 解释为 UTF-8 编码的代码点序列。
// 它返回 s 中给定 rune 第一次出现的字节索引。
// 如果 rune 不在 s 中返回 -1。
// 如果 r 是 [utf8.RuneError]，它返回任何
// 无效 UTF-8 字节序列的第一个实例。
func IndexRune(s []byte, r rune) int {
	const haveFastIndex = bytealg.MaxBruteForce > 0
	switch {
	case 0 <= r && r < utf8.RuneSelf:
		return IndexByte(s, byte(r))
	case r == utf8.RuneError:
		for i := 0; i < len(s); {
			r1, n := utf8.DecodeRune(s[i:])
			if r1 == utf8.RuneError {
				return i
			}
			i += n
		}
		return -1
	case !utf8.ValidRune(r):
		return -1
	default:
		// 使用 rune r 的 UTF-8 编码形式的最后一个字节来搜索。
		// 最后一个字节的分布与第一个字节相比更均匀，
		// 第一个字节有 78% 的概率是 [240, 243, 244]。
		var b [utf8.UTFMax]byte
		n := utf8.EncodeRune(b[:], r)
		last := n - 1
		i := last
		fails := 0
		for i < len(s) {
			if s[i] != b[last] {
				o := IndexByte(s[i+1:], b[last])
				if o < 0 {
					return -1
				}
				i += o + 1
			}
			// 向后逐字节比较。
			for j := 1; j < n; j++ {
				if s[i-j] != b[last-j] {
					goto next
				}
			}
			return i - last
		next:
			fails++
			i++
			if (haveFastIndex && fails > bytealg.Cutover(i)) && i < len(s) ||
				(!haveFastIndex && fails >= 4+i>>4 && i < len(s)) {
				goto fallback
			}
		}
		return -1

	fallback:
		// 当 IndexByte 返回太多假正检查时，切换到 bytealg.Index 或蛮力搜索。
		if haveFastIndex {
			if j := bytealg.Index(s[i-last:], b[:n]); j >= 0 {
				return i + j - last
			}
		} else {
			// 如果 bytealg.Index 不可用，蛮力搜索比
			// Rabin-Karp 快 ~1.5-3 倍，因为 n 很小。
			c0 := b[last]
			c1 := b[last-1] // 至少需要匹配 2 个字符
		loop:
			for ; i < len(s); i++ {
				if s[i] == c0 && s[i-1] == c1 {
					for k := 2; k < n; k++ {
						if s[i-k] != b[last-k] {
							continue loop
						}
					}
					return i - last
				}
			}
		}
		return -1
	}
}

// IndexAny 将 s 解释为 UTF-8 编码的 Unicode 代码点序列。
// 它返回 s 中 chars 中任何 Unicode 代码点第一次出现的字节索引。
// 如果 chars 为空或没有公共代码点，返回 -1。
func IndexAny(s []byte, chars string) int {
	if chars == "" {
		// 避免扫描所有 s。
		return -1
	}
	if len(s) == 1 {
		r := rune(s[0])
		if r >= utf8.RuneSelf {
			// 搜索 utf8.RuneError。
			for _, r = range chars {
				if r == utf8.RuneError {
					return 0
				}
			}
			return -1
		}
		if bytealg.IndexByteString(chars, s[0]) >= 0 {
			return 0
		}
		return -1
	}
	if len(chars) == 1 {
		r := rune(chars[0])
		if r >= utf8.RuneSelf {
			r = utf8.RuneError
		}
		return IndexRune(s, r)
	}
	if len(s) > 8 {
		if as, isASCII := makeASCIISet(chars); isASCII {
			for i, c := range s {
				if as.contains(c) {
					return i
				}
			}
			return -1
		}
	}
	var width int
	for i := 0; i < len(s); i += width {
		r := rune(s[i])
		if r < utf8.RuneSelf {
			if bytealg.IndexByteString(chars, s[i]) >= 0 {
				return i
			}
			width = 1
			continue
		}
		r, width = utf8.DecodeRune(s[i:])
		if r != utf8.RuneError {
			// r 是 2 到 4 个字节
			if len(chars) == width {
				if chars == string(r) {
					return i
				}
				continue
			}
			// 如果可用，使用 bytealg.IndexString 获得更好性能。
			if bytealg.MaxLen >= width {
				if bytealg.IndexString(chars, string(r)) >= 0 {
					return i
				}
				continue
			}
		}
		for _, ch := range chars {
			if r == ch {
				return i
			}
		}
	}
	return -1
}

// LastIndexAny 将 s 解释为 UTF-8 编码的 Unicode 代码点序列。
// 它返回 s 中 chars 中任何 Unicode 代码点最后一次出现的字节索引。
// 如果 chars 为空或没有公共代码点，返回 -1。
func LastIndexAny(s []byte, chars string) int {
	if chars == "" {
		// 避免扫描所有 s。
		return -1
	}
	if len(s) > 8 {
		if as, isASCII := makeASCIISet(chars); isASCII {
			for i := len(s) - 1; i >= 0; i-- {
				if as.contains(s[i]) {
					return i
				}
			}
			return -1
		}
	}
	if len(s) == 1 {
		r := rune(s[0])
		if r >= utf8.RuneSelf {
			for _, r = range chars {
				if r == utf8.RuneError {
					return 0
				}
			}
			return -1
		}
		if bytealg.IndexByteString(chars, s[0]) >= 0 {
			return 0
		}
		return -1
	}
	if len(chars) == 1 {
		cr := rune(chars[0])
		if cr >= utf8.RuneSelf {
			cr = utf8.RuneError
		}
		for i := len(s); i > 0; {
			r, size := utf8.DecodeLastRune(s[:i])
			i -= size
			if r == cr {
				return i
			}
		}
		return -1
	}
	for i := len(s); i > 0; {
		r := rune(s[i-1])
		if r < utf8.RuneSelf {
			if bytealg.IndexByteString(chars, s[i-1]) >= 0 {
				return i - 1
			}
			i--
			continue
		}
		r, size := utf8.DecodeLastRune(s[:i])
		i -= size
		if r != utf8.RuneError {
			// r 是 2 到 4 个字节
			if len(chars) == size {
				if chars == string(r) {
					return i
				}
				continue
			}
			// 如果可用，使用 bytealg.IndexString 获得更好性能。
			if bytealg.MaxLen >= size {
				if bytealg.IndexString(chars, string(r)) >= 0 {
					return i
				}
				continue
			}
		}
		for _, ch := range chars {
			if r == ch {
				return i
			}
		}
	}
	return -1
}

// 通用拆分：在 sep 的每个实例后拆分，
// 在子切片中包含 sepSave 字节的 sep。
func genSplit(s, sep []byte, sepSave, n int) [][]byte {
	if n == 0 {
		return nil
	}
	if len(sep) == 0 {
		return explode(s, n)
	}
	if n < 0 {
		n = Count(s, sep) + 1
	}
	if n > len(s)+1 {
		n = len(s) + 1
	}

	a := make([][]byte, n)
	n--
	i := 0
	for i < n {
		m := Index(s, sep)
		if m < 0 {
			break
		}
		a[i] = s[: m+sepSave : m+sepSave]
		s = s[m+len(sep):]
		i++
	}
	a[i] = s
	return a[:i+1]
}

// SplitN 将 s 拆分为由 sep 分隔的子切片，并返回这些分隔符之间的子切片。
// 如果 sep 为空，SplitN 在每个 UTF-8 序列后拆分。
// count 确定要返回的子切片数：
//   - n > 0: 最多 n 个子切片；最后一个子切片是未拆分的余数；
//   - n == 0: 结果为 nil（零个子切片）；
//   - n < 0: 所有子切片。
//
// 要围绕分隔符的第一个实例拆分，请参阅 [Cut]。
func SplitN(s, sep []byte, n int) [][]byte { return genSplit(s, sep, 0, n) }

// SplitAfterN 将 s 拆分为 sep 每个实例后的子切片，
// 并返回这些子切片的切片。
// 如果 sep 为空，SplitAfterN 在每个 UTF-8 序列后拆分。
// count 确定要返回的子切片数：
//   - n > 0: 最多 n 个子切片；最后一个子切片是未拆分的余数；
//   - n == 0: 结果为 nil（零个子切片）；
//   - n < 0: 所有子切片。
func SplitAfterN(s, sep []byte, n int) [][]byte {
	return genSplit(s, sep, len(sep), n)
}

// Split 将 s 拆分为所有由 sep 分隔的子切片，并返回这些分隔符之间的子切片。
// 如果 sep 为空，Split 在每个 UTF-8 序列后拆分。
// 它等价于计数为 -1 的 SplitN。
//
// 要围绕分隔符的第一个实例拆分，请参阅 [Cut]。
func Split(s, sep []byte) [][]byte { return genSplit(s, sep, 0, -1) }

// SplitAfter 将 s 拆分为 sep 每个实例后的所有子切片，
// 并返回这些子切片的切片。
// 如果 sep 为空，SplitAfter 在每个 UTF-8 序列后拆分。
// 它等价于计数为 -1 的 SplitAfterN。
func SplitAfter(s, sep []byte) [][]byte {
	return genSplit(s, sep, len(sep), -1)
}

var asciiSpace = [256]uint8{'\t': 1, '\n': 1, '\v': 1, '\f': 1, '\r': 1, ' ': 1}

// Fields 将 s 解释为 UTF-8 编码的代码点序列。
// 它在每个一个或多个连续空白字符实例周围拆分切片 s（由 [unicode.IsSpace] 定义），
// 返回 s 的子切片切片或如果 s 仅包含空白则返回空切片。返回的切片的每个元素都是
// 非空的。与 [Split] 不同，前导和尾部的空白字符运行被丢弃。
func Fields(s []byte) [][]byte {
	// 首先计数字段数。
	// 如果 s 是 ASCII，这是精确计数，否则是近似值。
	n := 0
	wasSpace := 1
	// setBits 用于跟踪 s 的字节中设置的位。
	setBits := uint8(0)
	for i := 0; i < len(s); i++ {
		r := s[i]
		setBits |= r
		isSpace := int(asciiSpace[r])
		n += wasSpace & ^isSpace
		wasSpace = isSpace
	}

	if setBits >= utf8.RuneSelf {
		// 输入切片中的某些 rune 不是 ASCII。
		return FieldsFunc(s, unicode.IsSpace)
	}

	// ASCII 快速路径
	a := make([][]byte, n)
	na := 0
	fieldStart := 0
	i := 0
	// 跳过输入前面的空格。
	for i < len(s) && asciiSpace[s[i]] != 0 {
		i++
	}
	fieldStart = i
	for i < len(s) {
		if asciiSpace[s[i]] == 0 {
			i++
			continue
		}
		a[na] = s[fieldStart:i:i]
		na++
		i++
		// 跳过字段之间的空格。
		for i < len(s) && asciiSpace[s[i]] != 0 {
			i++
		}
		fieldStart = i
	}
	if fieldStart < len(s) { // 最后一个字段可能在 EOF 处结束。
		a[na] = s[fieldStart:len(s):len(s)]
	}
	return a
}

// FieldsFunc 将 s 解释为 UTF-8 编码的代码点序列。
// 它在满足 f(c) 的每个代码点运行处拆分切片 s，
// 并返回 s 的子切片切片。如果 s 中的所有代码点都满足 f(c)，或
// len(s) == 0，则返回空切片。返回的切片的每个元素都是
// 非空的。与 [Split] 不同，前导和尾部满足 f(c) 的
// 代码点运行被丢弃。
//
// FieldsFunc 对调用 f(c) 的顺序不做任何保证，
// 并假设 f 对给定的 c 总是返回相同的值。
func FieldsFunc(s []byte, f func(rune) bool) [][]byte {
	// span 用于记录形式为 s[start:end] 的 s 的切片。
	// start 索引是包含的，end 索引是排他的。
	type span struct {
		start int
		end   int
	}
	spans := make([]span, 0, 32)

	// 找到字段的起始和结束索引。
	// 在单独的遍历中执行此操作（而不是对字符串 s 进行分片
	// 并立即收集结果子字符串）显著更高效，可能是由于缓存效应。
	start := -1 // 如果 >= 0，则为有效的 span 起始
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRune(s[i:])
		if f(r) {
			if start >= 0 {
				spans = append(spans, span{start, i})
				start = -1
			}
		} else {
			if start < 0 {
				start = i
			}
		}
		i += size
	}

	// 最后一个字段可能在 EOF 处结束。
	if start >= 0 {
		spans = append(spans, span{start, len(s)})
	}

	// 从记录的字段索引创建子切片。
	a := make([][]byte, len(spans))
	for i, span := range spans {
		a[i] = s[span.start:span.end:span.end]
	}

	return a
}

// Join 连接 s 的元素以创建新的字节切片。分隔符
// sep 被放在结果切片中元素之间。
func Join(s [][]byte, sep []byte) []byte {
	if len(s) == 0 {
		return []byte{}
	}
	if len(s) == 1 {
		// 只返回一个副本。
		return append([]byte(nil), s[0]...)
	}

	var n int
	if len(sep) > 0 {
		if len(sep) >= maxInt/(len(s)-1) {
			panic("bytes: Join output length overflow")
		}
		n += len(sep) * (len(s) - 1)
	}
	for _, v := range s {
		if len(v) > maxInt-n {
			panic("bytes: Join output length overflow")
		}
		n += len(v)
	}

	b := bytealg.MakeNoZero(n)[:n:n]
	bp := copy(b, s[0])
	for _, v := range s[1:] {
		bp += copy(b[bp:], sep)
		bp += copy(b[bp:], v)
	}
	return b
}

// HasPrefix 报告字节切片 s 是否以 prefix 开始。
func HasPrefix(s, prefix []byte) bool {
	return len(s) >= len(prefix) && Equal(s[:len(prefix)], prefix)
}

// HasSuffix 报告字节切片 s 是否以 suffix 结束。
func HasSuffix(s, suffix []byte) bool {
	return len(s) >= len(suffix) && Equal(s[len(s)-len(suffix):], suffix)
}

// Map 返回字节切片 s 的副本，其所有字符都根据
// 映射函数进行了修改。如果映射返回负值，该字符会
// 从字节切片中删除，不进行替换。s 中的字符和
// 输出被解释为 UTF-8 编码的代码点。
func Map(mapping func(r rune) rune, s []byte) []byte {
	// 在最坏的情况下，切片可能在映射时增长，这
	// 会很不爽。但这非常罕见，我们假设它是
	// 可以的。它也可能缩小，但这自然发生。
	b := make([]byte, 0, len(s))
	for i := 0; i < len(s); {
		r, wid := utf8.DecodeRune(s[i:])
		r = mapping(r)
		if r >= 0 {
			b = utf8.AppendRune(b, r)
		}
		i += wid
	}
	return b
}

// 尽管是导出的符号，
// Repeat 被广泛使用的包通过 linkname 链接。
// 著名的无耻之殿成员包括：
//   - gitee.com/quant1x/num
//
// 不要删除或改变类型签名。
// 参见 go.dev/issue/67401。
//
// 注意这个注释不是文档注释的一部分。
//
//go:linkname Repeat

// Repeat 返回一个新的字节切片，包含 b 的 count 个副本。
//
// 如果 count 为负或 (len(b) * count) 的结果
// 溢出，它会 panic。
func Repeat(b []byte, count int) []byte {
	if count == 0 {
		return []byte{}
	}

	// 由于我们不能在溢出时返回错误，
	// 如果重复将生成溢出，我们应该 panic。
	// 参见 golang.org/issue/16237。
	if count < 0 {
		panic("bytes: negative Repeat count")
	}
	hi, lo := bits.Mul(uint(len(b)), uint(count))
	if hi > 0 || lo > uint(maxInt) {
		panic("bytes: Repeat output length overflow")
	}
	n := int(lo) // lo = len(b) * count

	if len(b) == 0 {
		return []byte{}
	}

	// 超过某个块大小后，使用更大的块作为写入源是不划算的，
	// 因为当源太大时，我们基本上就是在破坏 CPU D-cache。
	// 所以如果结果长度大于经验上找到的限制（8KB），
	// 一旦达到限制，我们就停止增长源字符串，
	// 并继续重复使用相同的源字符串 - 它应该
	// 始终驻留在 L1 缓存中 - 直到我们
	// 完成结果的构造。
	// 在结果长度很大的情况下（大致超过 L2 缓存大小），
	// 这会产生显著的加速（最多 +100%）。
	const chunkLimit = 8 * 1024
	chunkMax := n
	if chunkMax > chunkLimit {
		chunkMax = chunkLimit / len(b) * len(b)
		if chunkMax == 0 {
			chunkMax = len(b)
		}
	}
	nb := bytealg.MakeNoZero(n)[:n:n]
	bp := copy(nb, b)
	for bp < n {
		chunk := min(bp, chunkMax)
		bp += copy(nb[bp:], nb[:chunk])
	}
	return nb
}

// ToUpper 返回字节切片 s 的副本，所有 Unicode 字母都映射到
// 其大写形式。
func ToUpper(s []byte) []byte {
	isASCII, hasLower := true, false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= utf8.RuneSelf {
			isASCII = false
			break
		}
		hasLower = hasLower || ('a' <= c && c <= 'z')
	}

	if isASCII { // 针对仅 ASCII 的字节切片进行优化。
		if !hasLower {
			// 只返回一个副本。
			return append([]byte(""), s...)
		}
		b := bytealg.MakeNoZero(len(s))[:len(s):len(s)]
		for i := 0; i < len(s); i++ {
			c := s[i]
			if 'a' <= c && c <= 'z' {
				c -= 'a' - 'A'
			}
			b[i] = c
		}
		return b
	}
	return Map(unicode.ToUpper, s)
}

// ToLower 返回字节切片 s 的副本，所有 Unicode 字母都映射到
// 其小写形式。
func ToLower(s []byte) []byte {
	isASCII, hasUpper := true, false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= utf8.RuneSelf {
			isASCII = false
			break
		}
		hasUpper = hasUpper || ('A' <= c && c <= 'Z')
	}

	if isASCII { // 针对仅 ASCII 的字节切片进行优化。
		if !hasUpper {
			return append([]byte(""), s...)
		}
		b := bytealg.MakeNoZero(len(s))[:len(s):len(s)]
		for i := 0; i < len(s); i++ {
			c := s[i]
			if 'A' <= c && c <= 'Z' {
				c += 'a' - 'A'
			}
			b[i] = c
		}
		return b
	}
	return Map(unicode.ToLower, s)
}

// ToTitle 将 s 视为 UTF-8 编码的字节，并返回一个副本，其中所有 Unicode 字母都映射到其标题大小写。
func ToTitle(s []byte) []byte { return Map(unicode.ToTitle, s) }

// ToUpperSpecial 将 s 视为 UTF-8 编码的字节，并返回一个副本，其中所有 Unicode 字母都映射到其
// 大写形式，优先考虑特殊的大小写规则。
func ToUpperSpecial(c unicode.SpecialCase, s []byte) []byte {
	return Map(c.ToUpper, s)
}

// ToLowerSpecial 将 s 视为 UTF-8 编码的字节，并返回一个副本，其中所有 Unicode 字母都映射到其
// 小写形式，优先考虑特殊的大小写规则。
func ToLowerSpecial(c unicode.SpecialCase, s []byte) []byte {
	return Map(c.ToLower, s)
}

// ToTitleSpecial 将 s 视为 UTF-8 编码的字节，并返回一个副本，其中所有 Unicode 字母都映射到其
// 标题大小写形式，优先考虑特殊的大小写规则。
func ToTitleSpecial(c unicode.SpecialCase, s []byte) []byte {
	return Map(c.ToTitle, s)
}

// ToValidUTF8 将 s 视为 UTF-8 编码的字节，并返回一个副本，其中表示无效 UTF-8 的每个字节运行
// 都被替换为 replacement 中的字节，这可能是空的。
func ToValidUTF8(s, replacement []byte) []byte {
	b := make([]byte, 0, len(s)+len(replacement))
	invalid := false // 前一个字节来自无效的 UTF-8 序列
	for i := 0; i < len(s); {
		c := s[i]
		if c < utf8.RuneSelf {
			i++
			invalid = false
			b = append(b, c)
			continue
		}
		_, wid := utf8.DecodeRune(s[i:])
		if wid == 1 {
			i++
			if !invalid {
				invalid = true
				b = append(b, replacement...)
			}
			continue
		}
		invalid = false
		b = append(b, s[i:i+wid]...)
		i += wid
	}
	return b
}

// isSeparator 报告 rune 是否可以标记字边界。
// TODO: 当 unicode 包捕获更多属性时更新。
func isSeparator(r rune) bool {
	// ASCII 字母数字和下划线不是分隔符
	if r <= 0x7F {
		switch {
		case '0' <= r && r <= '9':
			return false
		case 'a' <= r && r <= 'z':
			return false
		case 'A' <= r && r <= 'Z':
			return false
		case r == '_':
			return false
		}
		return true
	}
	// 字母和数字不是分隔符
	if unicode.IsLetter(r) || unicode.IsDigit(r) {
		return false
	}
	// 否则，我们现在能做的就是将空格视为分隔符。
	return unicode.IsSpace(r)
}

// Title 将 s 视为 UTF-8 编码的字节，并返回一个副本，其中开始
// 单词的所有 Unicode 字母都映射到其标题大小写。
//
// 已弃用：Title 使用的单词边界规则不能正确处理 Unicode
// 标点符号。请改用 golang.org/x/text/cases。
func Title(s []byte) []byte {
	// 在这里使用闭包来记住状态。
	// 有点不正规但有效。取决于 Map 按顺序扫描并调用
	// 每个 rune 一次的闭包。
	prev := ' '
	return Map(
		func(r rune) rune {
			if isSeparator(prev) {
				prev = r
				return unicode.ToTitle(r)
			}
			prev = r
			return r
		},
		s)
}

// TrimLeftFunc 将 s 视为 UTF-8 编码的字节，并通过切掉
// 所有满足 f(c) 的前导 UTF-8 编码的代码点 c 来返回 s 的子切片。
func TrimLeftFunc(s []byte, f func(r rune) bool) []byte {
	i := indexFunc(s, f, false)
	if i == -1 {
		return nil
	}
	return s[i:]
}

// TrimRightFunc 通过切掉所有满足 f(c) 的尾部
// UTF-8 编码的代码点 c 来返回 s 的子切片。
func TrimRightFunc(s []byte, f func(r rune) bool) []byte {
	i := lastIndexFunc(s, f, false)
	if i >= 0 && s[i] >= utf8.RuneSelf {
		_, wid := utf8.DecodeRune(s[i:])
		i += wid
	} else {
		i++
	}
	return s[0:i]
}

// TrimFunc 通过切掉所有前导和尾部满足 f(c) 的
// UTF-8 编码的代码点 c 来返回 s 的子切片。
func TrimFunc(s []byte, f func(r rune) bool) []byte {
	return TrimRightFunc(TrimLeftFunc(s, f), f)
}

// TrimPrefix 返回 s，不包括提供的前导前缀字符串。
// 如果 s 不以 prefix 开始，s 将原样返回。
func TrimPrefix(s, prefix []byte) []byte {
	if HasPrefix(s, prefix) {
		return s[len(prefix):]
	}
	return s
}

// TrimSuffix 返回 s，不包括提供的尾部后缀字符串。
// 如果 s 不以 suffix 结束，s 将原样返回。
func TrimSuffix(s, suffix []byte) []byte {
	if HasSuffix(s, suffix) {
		return s[:len(s)-len(suffix)]
	}
	return s
}

// IndexFunc 将 s 解释为 UTF-8 编码的代码点序列。
// 它返回 s 中满足 f(c) 的第一个 Unicode
// 代码点的字节索引，如果没有则返回 -1。
func IndexFunc(s []byte, f func(r rune) bool) int {
	return indexFunc(s, f, true)
}

// LastIndexFunc 将 s 解释为 UTF-8 编码的代码点序列。
// 它返回 s 中满足 f(c) 的最后一个 Unicode
// 代码点的字节索引，如果没有则返回 -1。
func LastIndexFunc(s []byte, f func(r rune) bool) int {
	return lastIndexFunc(s, f, true)
}

// indexFunc 与 IndexFunc 相同，除了如果
// truth==false，则谓词函数的含义被
// 反转。
func indexFunc(s []byte, f func(r rune) bool, truth bool) int {
	start := 0
	for start < len(s) {
		r, wid := utf8.DecodeRune(s[start:])
		if f(r) == truth {
			return start
		}
		start += wid
	}
	return -1
}

// lastIndexFunc 与 LastIndexFunc 相同，除了如果
// truth==false，则谓词函数的含义被
// 反转。
func lastIndexFunc(s []byte, f func(r rune) bool, truth bool) int {
	for i := len(s); i > 0; {
		r, size := rune(s[i-1]), 1
		if r >= utf8.RuneSelf {
			r, size = utf8.DecodeLastRune(s[0:i])
		}
		i -= size
		if f(r) == truth {
			return i
		}
	}
	return -1
}

// asciiSet 是一个 32 字节的值，其中每一位表示一个
// 给定 ASCII 字符在集合中的存在。下面 16 个字节的 128 位，
// 从最低字的最不重要的位开始到最高字的最重要的位，
// 映射到全部 128 个 ASCII 字符的范围。上面 16 个字节的 128 位将
// 被清零，确保任何非 ASCII 字符都被报告为不在集合中。
// 这分配了总共 32 个字节，尽管上半部分
// 未使用，以避免 asciiSet.contains 中的边界检查。
type asciiSet [8]uint32

// makeASCIISet 创建一个 ASCII 字符集，并报告 chars 中的所有
// 字符是否为 ASCII。
func makeASCIISet(chars string) (as asciiSet, ok bool) {
	for i := 0; i < len(chars); i++ {
		c := chars[i]
		if c >= utf8.RuneSelf {
			return as, false
		}
		as[c/32] |= 1 << (c % 32)
	}
	return as, true
}

// contains 报告 c 是否在集合内。
func (as *asciiSet) contains(c byte) bool {
	return (as[c/32] & (1 << (c % 32))) != 0
}

// containsRune 是 strings.ContainsRune 的简化版本，
// 以避免导入 strings 包。
// 我们避免 bytes.ContainsRune 以避免分配 s 的临时副本。
func containsRune(s string, r rune) bool {
	for _, c := range s {
		if c == r {
			return true
		}
	}
	return false
}

// Trim 通过切掉所有前导和尾部
// cutset 中包含的 UTF-8 编码的代码点来返回 s 的子切片。
func Trim(s []byte, cutset string) []byte {
	if len(s) == 0 {
		// 这是我们历来所做的。
		return nil
	}
	if cutset == "" {
		return s
	}
	if len(cutset) == 1 && cutset[0] < utf8.RuneSelf {
		return trimLeftByte(trimRightByte(s, cutset[0]), cutset[0])
	}
	if as, ok := makeASCIISet(cutset); ok {
		return trimLeftASCII(trimRightASCII(s, &as), &as)
	}
	return trimLeftUnicode(trimRightUnicode(s, cutset), cutset)
}

// TrimLeft 通过切掉所有前导
// cutset 中包含的 UTF-8 编码的代码点来返回 s 的子切片。
func TrimLeft(s []byte, cutset string) []byte {
	if len(s) == 0 {
		// 这是我们历来所做的。
		return nil
	}
	if cutset == "" {
		return s
	}
	if len(cutset) == 1 && cutset[0] < utf8.RuneSelf {
		return trimLeftByte(s, cutset[0])
	}
	if as, ok := makeASCIISet(cutset); ok {
		return trimLeftASCII(s, &as)
	}
	return trimLeftUnicode(s, cutset)
}

func trimLeftByte(s []byte, c byte) []byte {
	for len(s) > 0 && s[0] == c {
		s = s[1:]
	}
	if len(s) == 0 {
		// 这是我们历来所做的。
		return nil
	}
	return s
}

func trimLeftASCII(s []byte, as *asciiSet) []byte {
	for len(s) > 0 {
		if !as.contains(s[0]) {
			break
		}
		s = s[1:]
	}
	if len(s) == 0 {
		// 这是我们历来所做的。
		return nil
	}
	return s
}

func trimLeftUnicode(s []byte, cutset string) []byte {
	for len(s) > 0 {
		r, n := utf8.DecodeRune(s)
		if !containsRune(cutset, r) {
			break
		}
		s = s[n:]
	}
	if len(s) == 0 {
		// 这是我们历来所做的。
		return nil
	}
	return s
}

// TrimRight 通过切掉所有尾部
// cutset 中包含的 UTF-8 编码的代码点来返回 s 的子切片。
func TrimRight(s []byte, cutset string) []byte {
	if len(s) == 0 || cutset == "" {
		return s
	}
	if len(cutset) == 1 && cutset[0] < utf8.RuneSelf {
		return trimRightByte(s, cutset[0])
	}
	if as, ok := makeASCIISet(cutset); ok {
		return trimRightASCII(s, &as)
	}
	return trimRightUnicode(s, cutset)
}

func trimRightByte(s []byte, c byte) []byte {
	for len(s) > 0 && s[len(s)-1] == c {
		s = s[:len(s)-1]
	}
	return s
}

func trimRightASCII(s []byte, as *asciiSet) []byte {
	for len(s) > 0 {
		if !as.contains(s[len(s)-1]) {
			break
		}
		s = s[:len(s)-1]
	}
	return s
}

func trimRightUnicode(s []byte, cutset string) []byte {
	for len(s) > 0 {
		r, n := rune(s[len(s)-1]), 1
		if r >= utf8.RuneSelf {
			r, n = utf8.DecodeLastRune(s)
		}
		if !containsRune(cutset, r) {
			break
		}
		s = s[:len(s)-n]
	}
	return s
}

// TrimSpace 通过切掉所有前导和尾部
// 空白（如 Unicode 定义的）来返回 s 的子切片。
func TrimSpace(s []byte) []byte {
	// ASCII 快速路径：查找第一个 ASCII 非空格字节。
	for lo, c := range s {
		if c >= utf8.RuneSelf {
			// 如果我们遇到非 ASCII 字节，则回退到
			// 剩余字节上更慢的 Unicode 感知方法。
			return TrimFunc(s[lo:], unicode.IsSpace)
		}
		if asciiSpace[c] != 0 {
			continue
		}
		s = s[lo:]
		// 现在从末尾查找第一个 ASCII 非空格字节。
		for hi := len(s) - 1; hi >= 0; hi-- {
			c := s[hi]
			if c >= utf8.RuneSelf {
				return TrimFunc(s[:hi+1], unicode.IsSpace)
			}
			if asciiSpace[c] == 0 {
				// 此时，s[:hi+1] 以 ASCII
				// 非空格字节开始和结束，所以我们完成了。非 ASCII 情况已经
				// 在上面处理了。
				return s[:hi+1]
			}
		}
	}
	// 特殊情况以保持之前的 TrimLeftFunc 行为，
	// 如果都是空格则返回 nil 而不是空切片。
	return nil
}

// Runes 将 s 解释为 UTF-8 编码的代码点序列。
// 它返回与 s 等价的 rune（Unicode 代码点）切片。
func Runes(s []byte) []rune {
	t := make([]rune, utf8.RuneCount(s))
	i := 0
	for len(s) > 0 {
		r, l := utf8.DecodeRune(s)
		t[i] = r
		i++
		s = s[l:]
	}
	return t
}

// Replace 返回切片 s 的副本，其中前 n 个
// old 的非重叠实例被 new 替换。
// 如果 old 为空，它匹配切片的开始
// 和每个 UTF-8 序列之后，为 k-rune 切片产生最多 k+1 个替换。
// 如果 n < 0，则对替换数量没有限制。
func Replace(s, old, new []byte, n int) []byte {
	m := 0
	if n != 0 {
		// 计算替换数量。
		m = Count(s, old)
	}
	if m == 0 {
		// 只返回一个副本。
		return append([]byte(nil), s...)
	}
	if n < 0 || m < n {
		n = m
	}

	// 将替换应用于缓冲区。
	t := make([]byte, len(s)+n*(len(new)-len(old)))
	w := 0
	start := 0
	if len(old) > 0 {
		for range n {
			j := start + Index(s[start:], old)
			w += copy(t[w:], s[start:j])
			w += copy(t[w:], new)
			start = j + len(old)
		}
	} else { // len(old) == 0
		w += copy(t[w:], new)
		for range n - 1 {
			_, wid := utf8.DecodeRune(s[start:])
			j := start + wid
			w += copy(t[w:], s[start:j])
			w += copy(t[w:], new)
			start = j
		}
	}
	w += copy(t[w:], s[start:])
	return t[0:w]
}

// ReplaceAll 返回切片 s 的副本，其中
// old 的所有非重叠实例都被 new 替换。
// 如果 old 为空，它匹配切片的开始
// 和每个 UTF-8 序列之后，为 k-rune 切片产生最多 k+1 个替换。
func ReplaceAll(s, old, new []byte) []byte {
	return Replace(s, old, new, -1)
}

// EqualFold 报告 s 和 t（解释为 UTF-8 字符串）
// 是否在简单 Unicode 大小写折叠下相等，这是一种更通用的
// 不区分大小写的形式。
func EqualFold(s, t []byte) bool {
	// ASCII 快速路径
	i := 0
	for n := min(len(s), len(t)); i < n; i++ {
		sr := s[i]
		tr := t[i]
		if sr|tr >= utf8.RuneSelf {
			goto hasUnicode
		}

		// 简单情况。
		if tr == sr {
			continue
		}

		// 让 sr < tr 以简化后续内容。
		if tr < sr {
			tr, sr = sr, tr
		}
		// 仅 ASCII，sr/tr 必须是大写/小写
		if 'A' <= sr && sr <= 'Z' && tr == sr+'a'-'A' {
			continue
		}
		return false
	}
	// 检查我们是否已用尽两个字符串。
	return len(s) == len(t)

hasUnicode:
	s = s[i:]
	t = t[i:]
	for len(s) != 0 && len(t) != 0 {
		// 从每个中提取第一个 rune。
		sr, size := utf8.DecodeRune(s)
		s = s[size:]
		tr, size := utf8.DecodeRune(t)
		t = t[size:]

		// 如果它们匹配，继续；否则返回 false。

		// 简单情况。
		if tr == sr {
			continue
		}

		// 让 sr < tr 以简化后续内容。
		if tr < sr {
			tr, sr = sr, tr
		}
		// 快速 ASCII 检查。
		if tr < utf8.RuneSelf {
			// 仅 ASCII，sr/tr 必须是大写/小写
			if 'A' <= sr && sr <= 'Z' && tr == sr+'a'-'A' {
				continue
			}
			return false
		}

		// 通用情况。SimpleFold(x) 返回下一个等价的 rune > x
		// 或环绕到较小的值。
		r := unicode.SimpleFold(sr)
		for r != sr && r < tr {
			r = unicode.SimpleFold(r)
		}
		if r == tr {
			continue
		}
		return false
	}

	// 一个字符串为空。两者都是吗？
	return len(s) == len(t)
}

// Index 返回 s 中 sep 第一个实例的索引，如果 sep 不在 s 中返回 -1。
func Index(s, sep []byte) int {
	n := len(sep)
	switch {
	case n == 0:
		return 0
	case n == 1:
		return IndexByte(s, sep[0])
	case n == len(s):
		if Equal(sep, s) {
			return 0
		}
		return -1
	case n > len(s):
		return -1
	case n <= bytealg.MaxLen:
		// 当 s 和 sep 都很小时使用蛮力
		if len(s) <= bytealg.MaxBruteForce {
			return bytealg.Index(s, sep)
		}
		c0 := sep[0]
		c1 := sep[1]
		i := 0
		t := len(s) - n + 1
		fails := 0
		for i < t {
			if s[i] != c0 {
				// IndexByte 比 bytealg.Index 快，所以只要
				// 我们没有得到很多假正检查就使用它。
				o := IndexByte(s[i+1:t], c0)
				if o < 0 {
					return -1
				}
				i += o + 1
			}
			if s[i+1] == c1 && Equal(s[i:i+n], sep) {
				return i
			}
			fails++
			i++
			// 当 IndexByte 产生太多假正检查时切换到 bytealg.Index。
			if fails > bytealg.Cutover(i) {
				r := bytealg.Index(s[i:], sep)
				if r >= 0 {
					return r + i
				}
				return -1
			}
		}
		return -1
	}
	c0 := sep[0]
	c1 := sep[1]
	i := 0
	fails := 0
	t := len(s) - n + 1
	for i < t {
		if s[i] != c0 {
			o := IndexByte(s[i+1:t], c0)
			if o < 0 {
				break
			}
			i += o + 1
		}
		if s[i+1] == c1 && Equal(s[i:i+n], sep) {
			return i
		}
		i++
		fails++
		if fails >= 4+i>>4 && i < t {
			// 放弃 IndexByte，它跳过的距离不足以优于 Rabin-Karp。
			// 实验（使用 IndexPeriodic）建议
			// 转折点大约是 16 字节的跳过。
			// TODO: 如果 sep 的大前缀匹配，
			// 我们应该以更大的平均跳过转折，
			// 因为 Equal 变得那样更昂贵。
			// 此代码未考虑该效果。
			j := bytealg.IndexRabinKarp(s[i:], sep)
			if j < 0 {
				return -1
			}
			return i + j
		}
	}
	return -1
}

// Cut 围绕 sep 的第一个实例拆分 s，
// 返回 sep 前后的文本。
// found 结果报告 sep 是否出现在 s 中。
// 如果 sep 不在 s 中，cut 返回 s, nil, false。
//
// Cut 返回原始切片 s 的切片，不是副本。
func Cut(s, sep []byte) (before, after []byte, found bool) {
	if i := Index(s, sep); i >= 0 {
		return s[:i], s[i+len(sep):], true
	}
	return s, nil, false
}

// Clone 返回 b[:len(b)] 的副本。
// 结果可能有额外的未使用容量。
// Clone(nil) 返回 nil。
func Clone(b []byte) []byte {
	if b == nil {
		return nil
	}
	return append([]byte{}, b...)
}

// CutPrefix 返回 s 不包括提供的前导前缀字节切片
// 并报告是否找到了前缀。
// 如果 s 不以 prefix 开始，CutPrefix 返回 s, false。
// 如果 prefix 是空字节切片，CutPrefix 返回 s, true。
//
// CutPrefix 返回原始切片 s 的切片，不是副本。
func CutPrefix(s, prefix []byte) (after []byte, found bool) {
	if !HasPrefix(s, prefix) {
		return s, false
	}
	return s[len(prefix):], true
}

// CutSuffix 返回 s 不包括提供的末尾后缀字节切片
// 并报告是否找到了后缀。
// 如果 s 不以 suffix 结束，CutSuffix 返回 s, false。
// 如果 suffix 是空字节切片，CutSuffix 返回 s, true。
//
// CutSuffix 返回原始切片 s 的切片，不是副本。
func CutSuffix(s, suffix []byte) (before []byte, found bool) {
	if !HasSuffix(s, suffix) {
		return s, false
	}
	return s[:len(s)-len(suffix)], true
}
