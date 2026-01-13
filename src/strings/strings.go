// 版权所有 2009 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// Package strings 实现了操作 UTF-8 编码字符串的简单函数。
//
// 有关 Go 中 UTF-8 字符串的信息，请参见 https://blog.golang.org/strings。
package strings

import (
	"internal/bytealg"
	"internal/stringslite"
	"math/bits"
	"unicode"
	"unicode/utf8"
)

const maxInt = int(^uint(0) >> 1)

// explode 将 s 分割成 UTF-8 字符串切片，
// 每个 Unicode 字符一个字符串，最多 n 个（n < 0 表示无限制）。
// 无效的 UTF-8 字节会被单独分割。
func explode(s string, n int) []string {
	l := utf8.RuneCountInString(s)
	if n < 0 || n > l {
		n = l
	}
	a := make([]string, n)
	for i := 0; i < n-1; i++ {
		_, size := utf8.DecodeRuneInString(s)
		a[i] = s[:size]
		s = s[size:]
	}
	if n > 0 {
		a[n-1] = s
	}
	return a
}

// Count 计算 s 中不重叠的 substr 实例数量。
// 如果 substr 是空字符串，Count 返回 1 + s 中 Unicode 码点的数量。
func Count(s, substr string) int {
	// 特殊情况
	if len(substr) == 0 {
		return utf8.RuneCountInString(s) + 1
	}
	if len(substr) == 1 {
		return bytealg.CountString(s, substr[0])
	}
	n := 0
	for {
		i := Index(s, substr)
		if i == -1 {
			return n
		}
		n++
		s = s[i+len(substr):]
	}
}

// Contains 报告 substr 是否在 s 中。
func Contains(s, substr string) bool {
	return Index(s, substr) >= 0
}

// ContainsAny 报告 chars 中的任何 Unicode 码点是否在 s 中。
func ContainsAny(s, chars string) bool {
	return IndexAny(s, chars) >= 0
}

// ContainsRune 报告 Unicode 码点 r 是否在 s 中。
func ContainsRune(s string, r rune) bool {
	return IndexRune(s, r) >= 0
}

// ContainsFunc 报告 s 中是否有任何 Unicode 码点 r 满足 f(r)。
func ContainsFunc(s string, f func(rune) bool) bool {
	return IndexFunc(s, f) >= 0
}

// LastIndex 返回 substr 在 s 中最后一次出现的索引，如果 substr 不在 s 中则返回 -1。
func LastIndex(s, substr string) int {
	n := len(substr)
	switch {
	case n == 0:
		return len(s)
	case n == 1:
		return bytealg.LastIndexByteString(s, substr[0])
	case n == len(s):
		if substr == s {
			return 0
		}
		return -1
	case n > len(s):
		return -1
	}
	// 从字符串末尾开始的 Rabin-Karp 搜索
	hashss, pow := bytealg.HashStrRev(substr)
	last := len(s) - n
	var h uint32
	for i := len(s) - 1; i >= last; i-- {
		h = h*bytealg.PrimeRK + uint32(s[i])
	}
	if h == hashss && s[last:] == substr {
		return last
	}
	for i := last - 1; i >= 0; i-- {
		h *= bytealg.PrimeRK
		h += uint32(s[i])
		h -= pow * uint32(s[i+n])
		if h == hashss && s[i:i+n] == substr {
			return i
		}
	}
	return -1
}

// IndexByte 返回 c 在 s 中第一次出现的索引，如果 c 不在 s 中则返回 -1。
func IndexByte(s string, c byte) int {
	return stringslite.IndexByte(s, c)
}

// IndexRune 返回 Unicode 码点 r 第一次出现的索引，
// 如果 rune 不在 s 中则返回 -1。
// 如果 r 是 [utf8.RuneError]，它返回任何无效 UTF-8 字节序列
// 的第一个实例。
func IndexRune(s string, r rune) int {
	const haveFastIndex = bytealg.MaxBruteForce > 0
	switch {
	case 0 <= r && r < utf8.RuneSelf:
		return IndexByte(s, byte(r))
	case r == utf8.RuneError:
		for i, r := range s {
			if r == utf8.RuneError {
				return i
			}
		}
		return -1
	case !utf8.ValidRune(r):
		return -1
	default:
		// 使用 rune r 的 UTF-8 编码形式的最后一个字节来搜索。
		// 与第一个字节相比，最后一个字节的分布更均匀，
		// 第一个字节有 78% 的概率是 [240, 243, 244]。
		rs := string(r)
		last := len(rs) - 1
		i := last
		fails := 0
		for i < len(s) {
			if s[i] != rs[last] {
				o := IndexByte(s[i+1:], rs[last])
				if o < 0 {
					return -1
				}
				i += o + 1
			}
			// 向后逐步比较字节。
			for j := 1; j < len(rs); j++ {
				if s[i-j] != rs[last-j] {
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
		// 参见 ../bytes/bytes.go 中的注释
		if haveFastIndex {
			if j := bytealg.IndexString(s[i-last:], string(r)); j >= 0 {
				return i + j - last
			}
		} else {
			c0 := rs[last]
			c1 := rs[last-1]
		loop:
			for ; i < len(s); i++ {
				if s[i] == c0 && s[i-1] == c1 {
					for k := 2; k < len(rs); k++ {
						if s[i-k] != rs[last-k] {
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

// IndexAny 返回 chars 中任何 Unicode 码点在 s 中第一次出现的索引，
// 如果 chars 中没有 Unicode 码点出现在 s 中则返回 -1。
func IndexAny(s, chars string) int {
	if chars == "" {
		// 避免扫描整个 s。
		return -1
	}
	if len(chars) == 1 {
		// 避免扫描整个 s。
		r := rune(chars[0])
		if r >= utf8.RuneSelf {
			r = utf8.RuneError
		}
		return IndexRune(s, r)
	}
	if len(s) > 8 {
		if as, isASCII := makeASCIISet(chars); isASCII {
			for i := 0; i < len(s); i++ {
				if as.contains(s[i]) {
					return i
				}
			}
			return -1
		}
	}
	for i, c := range s {
		if IndexRune(chars, c) >= 0 {
			return i
		}
	}
	return -1
}

// LastIndexAny 返回 chars 中任何 Unicode 码点在 s 中最后一次出现的索引，
// 如果 chars 中没有 Unicode 码点出现在 s 中则返回 -1。
func LastIndexAny(s, chars string) int {
	if chars == "" {
		// 避免扫描整个 s。
		return -1
	}
	if len(s) == 1 {
		rc := rune(s[0])
		if rc >= utf8.RuneSelf {
			rc = utf8.RuneError
		}
		if IndexRune(chars, rc) >= 0 {
			return 0
		}
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
	if len(chars) == 1 {
		rc := rune(chars[0])
		if rc >= utf8.RuneSelf {
			rc = utf8.RuneError
		}
		for i := len(s); i > 0; {
			r, size := utf8.DecodeLastRuneInString(s[:i])
			i -= size
			if rc == r {
				return i
			}
		}
		return -1
	}
	for i := len(s); i > 0; {
		r, size := utf8.DecodeLastRuneInString(s[:i])
		i -= size
		if IndexRune(chars, r) >= 0 {
			return i
		}
	}
	return -1
}

// LastIndexByte 返回 c 在 s 中最后一次出现的索引，如果 c 不在 s 中则返回 -1。
func LastIndexByte(s string, c byte) int {
	return bytealg.LastIndexByteString(s, c)
}

// 通用分割：在 sep 的每个实例之后分割，
// 在子数组中包含 sep 的 sepSave 个字节。
func genSplit(s, sep string, sepSave, n int) []string {
	if n == 0 {
		return nil
	}
	if sep == "" {
		return explode(s, n)
	}
	if n < 0 {
		n = Count(s, sep) + 1
	}

	if n > len(s)+1 {
		n = len(s) + 1
	}
	a := make([]string, n)
	n--
	i := 0
	for i < n {
		m := Index(s, sep)
		if m < 0 {
			break
		}
		a[i] = s[:m+sepSave]
		s = s[m+len(sep):]
		i++
	}
	a[i] = s
	return a[:i+1]
}

// SplitN 将 s 切分成由 sep 分隔的子字符串，并返回这些分隔符之间的子字符串切片。
//
// count 决定返回的子字符串数量：
//   - n > 0: 最多 n 个子字符串；最后一个子字符串将是未分割的剩余部分；
//   - n == 0: 结果为 nil（零个子字符串）；
//   - n < 0: 所有子字符串。
//
// s 和 sep 的边界情况（例如空字符串）按照 [Split] 文档中的描述处理。
//
// 要围绕分隔符的第一个实例进行分割，请参见 [Cut]。
func SplitN(s, sep string, n int) []string { return genSplit(s, sep, 0, n) }

// SplitAfterN 在 sep 的每个实例之后将 s 切分成子字符串，
// 并返回这些子字符串的切片。
//
// count 决定返回的子字符串数量：
//   - n > 0: 最多 n 个子字符串；最后一个子字符串将是未分割的剩余部分；
//   - n == 0: 结果为 nil（零个子字符串）；
//   - n < 0: 所有子字符串。
//
// s 和 sep 的边界情况（例如空字符串）按照 [SplitAfter] 文档中的描述处理。
func SplitAfterN(s, sep string, n int) []string {
	return genSplit(s, sep, len(sep), n)
}

// Split 将 s 切分成由 sep 分隔的所有子字符串，
// 并返回这些分隔符之间的子字符串切片。
//
// 如果 s 不包含 sep 且 sep 不为空，Split 返回一个长度为 1 的切片，
// 其唯一元素是 s。
//
// 如果 sep 为空，Split 在每个 UTF-8 序列之后分割。如果 s 和 sep
// 都为空，Split 返回一个空切片。
//
// 它等价于 count 为 -1 的 [SplitN]。
//
// 要围绕分隔符的第一个实例进行分割，请参见 [Cut]。
func Split(s, sep string) []string { return genSplit(s, sep, 0, -1) }

// SplitAfter 在 sep 的每个实例之后将 s 切分成所有子字符串，
// 并返回这些子字符串的切片。
//
// 如果 s 不包含 sep 且 sep 不为空，SplitAfter 返回一个长度为 1 的切片，
// 其唯一元素是 s。
//
// 如果 sep 为空，SplitAfter 在每个 UTF-8 序列之后分割。如果 s 和 sep
// 都为空，SplitAfter 返回一个空切片。
//
// 它等价于 count 为 -1 的 [SplitAfterN]。
func SplitAfter(s, sep string) []string {
	return genSplit(s, sep, len(sep), -1)
}

var asciiSpace = [256]uint8{'\t': 1, '\n': 1, '\v': 1, '\f': 1, '\r': 1, ' ': 1}

// Fields 围绕一个或多个连续空白字符的每个实例分割字符串 s，
// 空白字符由 [unicode.IsSpace] 定义，返回 s 的子字符串切片，
// 如果 s 只包含空白字符则返回空切片。返回切片的每个元素都是非空的。
// 与 [Split] 不同，前导和尾随的空白字符序列会被丢弃。
func Fields(s string) []string {
	// 首先计算字段数。
	// 如果 s 是 ASCII，这是精确计数，否则是近似值。
	n := 0
	wasSpace := 1
	// setBits 用于跟踪 s 的字节中设置了哪些位。
	setBits := uint8(0)
	for i := 0; i < len(s); i++ {
		r := s[i]
		setBits |= r
		isSpace := int(asciiSpace[r])
		n += wasSpace & ^isSpace
		wasSpace = isSpace
	}

	if setBits >= utf8.RuneSelf {
		// 输入字符串中的一些 rune 不是 ASCII。
		return FieldsFunc(s, unicode.IsSpace)
	}
	// ASCII 快速路径
	a := make([]string, n)
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
		a[na] = s[fieldStart:i]
		na++
		i++
		// 跳过字段之间的空格。
		for i < len(s) && asciiSpace[s[i]] != 0 {
			i++
		}
		fieldStart = i
	}
	if fieldStart < len(s) { // 最后一个字段可能在 EOF 处结束。
		a[na] = s[fieldStart:]
	}
	return a
}

// FieldsFunc 在满足 f(c) 的每个 Unicode 码点 c 的序列处分割字符串 s，
// 并返回 s 的切片数组。如果 s 中的所有码点都满足 f(c) 或字符串为空，
// 则返回空切片。返回切片的每个元素都是非空的。与 [Split] 不同，
// 满足 f(c) 的前导和尾随码点序列会被丢弃。
//
// FieldsFunc 不保证调用 f(c) 的顺序，
// 并假设 f 对于给定的 c 总是返回相同的值。
func FieldsFunc(s string, f func(rune) bool) []string {
	// span 用于记录形式为 s[start:end] 的 s 切片。
	// start 索引是包含的，end 索引是不包含的。
	type span struct {
		start int
		end   int
	}
	spans := make([]span, 0, 32)

	// 查找字段的开始和结束索引。
	// 在单独的遍历中执行此操作（而不是立即切分字符串 s 并收集结果子字符串）
	// 效率显著提高，可能是由于缓存效应。
	start := -1 // 如果 >= 0 则为有效的 span 开始位置
	for end, rune := range s {
		if f(rune) {
			if start >= 0 {
				spans = append(spans, span{start, end})
				// 将 start 设置为负值。
				// 注意：在这里始终使用 -1 会在 amd64 上使此代码
				// 可重复地减慢几个百分点。
				start = ^start
			}
		} else {
			if start < 0 {
				start = end
			}
		}
	}

	// 最后一个字段可能在 EOF 处结束。
	if start >= 0 {
		spans = append(spans, span{start, len(s)})
	}

	// 从记录的字段索引创建字符串。
	a := make([]string, len(spans))
	for i, span := range spans {
		a[i] = s[span.start:span.end]
	}

	return a
}

// Join 连接其第一个参数的元素以创建单个字符串。分隔符字符串 sep
// 被放置在结果字符串的元素之间。
func Join(elems []string, sep string) string {
	switch len(elems) {
	case 0:
		return ""
	case 1:
		return elems[0]
	}

	var n int
	if len(sep) > 0 {
		if len(sep) >= maxInt/(len(elems)-1) {
			panic("strings: Join output length overflow")
		}
		n += len(sep) * (len(elems) - 1)
	}
	for _, elem := range elems {
		if len(elem) > maxInt-n {
			panic("strings: Join output length overflow")
		}
		n += len(elem)
	}

	var b Builder
	b.Grow(n)
	b.WriteString(elems[0])
	for _, s := range elems[1:] {
		b.WriteString(sep)
		b.WriteString(s)
	}
	return b.String()
}

// HasPrefix 报告字符串 s 是否以 prefix 开头。
func HasPrefix(s, prefix string) bool {
	return stringslite.HasPrefix(s, prefix)
}

// HasSuffix 报告字符串 s 是否以 suffix 结尾。
func HasSuffix(s, suffix string) bool {
	return stringslite.HasSuffix(s, suffix)
}

// Map 返回字符串 s 的副本，其中所有字符根据映射函数进行修改。
// 如果 mapping 返回负值，该字符将从字符串中删除，不进行替换。
func Map(mapping func(rune) rune, s string) string {
	// 在最坏的情况下，映射后字符串可能会增长，这会使事情变得不愉快。
	// 但这种情况非常罕见，我们假设它没问题就直接处理了。
	// 它也可能会缩小，但这自然会处理好。

	// 输出缓冲区 b 按需初始化，在第一次字符不同时初始化。
	var b Builder

	for i, c := range s {
		r := mapping(c)
		if r == c && c != utf8.RuneError {
			continue
		}

		var width int
		if c == utf8.RuneError {
			c, width = utf8.DecodeRuneInString(s[i:])
			if width != 1 && r == c {
				continue
			}
		} else {
			width = utf8.RuneLen(c)
		}

		b.Grow(len(s) + utf8.UTFMax)
		b.WriteString(s[:i])
		if r >= 0 {
			b.WriteRune(r)
		}

		s = s[i+width:]
		break
	}

	// 未更改输入的快速路径
	if b.Cap() == 0 { // 上面没有调用 b.Grow
		return s
	}

	for _, c := range s {
		r := mapping(c)

		if r >= 0 {
			// 常见情况
			// 由于内联，确定是否应调用 WriteByte 比总是调用 WriteRune 性能更好
			if r < utf8.RuneSelf {
				b.WriteByte(byte(r))
			} else {
				// r 不是 ASCII rune。
				b.WriteRune(r)
			}
		}
	}

	return b.String()
}

// 根据静态分析，空格、破折号、零、等号和制表符是最常重复的字符串字面量，
// 通常用于在固定宽度的终端窗口上显示。
// 为这些预声明常量，以便在常见情况下实现 O(1) 的重复。
const (
	repeatedSpaces = "" +
		"                                                                " +
		"                                                                "
	repeatedDashes = "" +
		"----------------------------------------------------------------" +
		"----------------------------------------------------------------"
	repeatedZeroes = "" +
		"0000000000000000000000000000000000000000000000000000000000000000"
	repeatedEquals = "" +
		"================================================================" +
		"================================================================"
	repeatedTabs = "" +
		"\t\t\t\t\t\t\t\t\t\t\t\t\t\t\t\t\t\t\t\t\t\t\t\t\t\t\t\t\t\t\t\t" +
		"\t\t\t\t\t\t\t\t\t\t\t\t\t\t\t\t\t\t\t\t\t\t\t\t\t\t\t\t\t\t\t\t"
)

// Repeat 返回一个由 count 个字符串 s 副本组成的新字符串。
//
// 如果 count 为负数或 (len(s) * count) 的结果溢出，它会 panic。
func Repeat(s string, count int) string {
	switch count {
	case 0:
		return ""
	case 1:
		return s
	}

	// 由于我们无法在溢出时返回错误，
	// 如果重复将产生溢出，我们应该 panic。
	// 参见 golang.org/issue/16237。
	if count < 0 {
		panic("strings: negative Repeat count")
	}
	hi, lo := bits.Mul(uint(len(s)), uint(count))
	if hi > 0 || lo > uint(maxInt) {
		panic("strings: Repeat output length overflow")
	}
	n := int(lo) // lo = len(s) * count（lo 等于 len(s) 乘以 count）

	if len(s) == 0 {
		return ""
	}

	// 针对相对较短长度的常见重复字符串进行优化。
	switch s[0] {
	case ' ', '-', '0', '=', '\t':
		switch {
		case n <= len(repeatedSpaces) && HasPrefix(repeatedSpaces, s):
			return repeatedSpaces[:n]
		case n <= len(repeatedDashes) && HasPrefix(repeatedDashes, s):
			return repeatedDashes[:n]
		case n <= len(repeatedZeroes) && HasPrefix(repeatedZeroes, s):
			return repeatedZeroes[:n]
		case n <= len(repeatedEquals) && HasPrefix(repeatedEquals, s):
			return repeatedEquals[:n]
		case n <= len(repeatedTabs) && HasPrefix(repeatedTabs, s):
			return repeatedTabs[:n]
		}
	}

	// 超过某个块大小后，使用更大的块作为写入源是适得其反的，
	// 因为当源太大时，我们基本上只是在抖动 CPU D 缓存。
	// 所以如果结果长度大于经验发现的限制（8KB），我们在达到限制后
	// 停止增长源字符串，并继续重用相同的源字符串——因此它应该
	// 始终驻留在 L1 缓存中——直到我们完成结果的构建。
	// 在结果长度较大的情况下（大致超过 L2 缓存大小），
	// 这会产生显著的加速（高达 +100%）。
	const chunkLimit = 8 * 1024
	chunkMax := n
	if n > chunkLimit {
		chunkMax = chunkLimit / len(s) * len(s)
		if chunkMax == 0 {
			chunkMax = len(s)
		}
	}

	var b Builder
	b.Grow(n)
	b.WriteString(s)
	for b.Len() < n {
		chunk := min(n-b.Len(), b.Len(), chunkMax)
		b.WriteString(b.String()[:chunk])
	}
	return b.String()
}

// ToUpper 返回将 s 中所有 Unicode 字母映射为大写后的字符串。
func ToUpper(s string) string {
	isASCII, hasLower := true, false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= utf8.RuneSelf {
			isASCII = false
			break
		}
		hasLower = hasLower || ('a' <= c && c <= 'z')
	}

	if isASCII { // 针对纯 ASCII 字符串进行优化。
		if !hasLower {
			return s
		}
		var (
			b   Builder
			pos int
		)
		b.Grow(len(s))
		for i := 0; i < len(s); i++ {
			c := s[i]
			if 'a' <= c && c <= 'z' {
				c -= 'a' - 'A'
				if pos < i {
					b.WriteString(s[pos:i])
				}
				b.WriteByte(c)
				pos = i + 1
			}
		}
		if pos < len(s) {
			b.WriteString(s[pos:])
		}
		return b.String()
	}
	return Map(unicode.ToUpper, s)
}

// ToLower 返回将 s 中所有 Unicode 字母映射为小写后的字符串。
func ToLower(s string) string {
	isASCII, hasUpper := true, false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= utf8.RuneSelf {
			isASCII = false
			break
		}
		hasUpper = hasUpper || ('A' <= c && c <= 'Z')
	}

	if isASCII { // 针对纯 ASCII 字符串进行优化。
		if !hasUpper {
			return s
		}
		var (
			b   Builder
			pos int
		)
		b.Grow(len(s))
		for i := 0; i < len(s); i++ {
			c := s[i]
			if 'A' <= c && c <= 'Z' {
				c += 'a' - 'A'
				if pos < i {
					b.WriteString(s[pos:i])
				}
				b.WriteByte(c)
				pos = i + 1
			}
		}
		if pos < len(s) {
			b.WriteString(s[pos:])
		}
		return b.String()
	}
	return Map(unicode.ToLower, s)
}

// ToTitle 返回字符串 s 的副本，其中所有 Unicode 字母映射为其 Unicode 标题大小写。
func ToTitle(s string) string { return Map(unicode.ToTitle, s) }

// ToUpperSpecial 返回字符串 s 的副本，其中所有 Unicode 字母使用 c 指定的
// 大小写映射规则映射为大写。
func ToUpperSpecial(c unicode.SpecialCase, s string) string {
	return Map(c.ToUpper, s)
}

// ToLowerSpecial 返回字符串 s 的副本，其中所有 Unicode 字母使用 c 指定的
// 大小写映射规则映射为小写。
func ToLowerSpecial(c unicode.SpecialCase, s string) string {
	return Map(c.ToLower, s)
}

// ToTitleSpecial 返回字符串 s 的副本，其中所有 Unicode 字母映射为其
// Unicode 标题大小写，优先使用特殊大小写规则。
func ToTitleSpecial(c unicode.SpecialCase, s string) string {
	return Map(c.ToTitle, s)
}

// ToValidUTF8 返回字符串 s 的副本，其中每个无效 UTF-8 字节序列的连续段
// 都被替换字符串替换，替换字符串可以为空。
func ToValidUTF8(s, replacement string) string {
	var b Builder

	for i, c := range s {
		if c != utf8.RuneError {
			continue
		}

		_, wid := utf8.DecodeRuneInString(s[i:])
		if wid == 1 {
			b.Grow(len(s) + len(replacement))
			b.WriteString(s[:i])
			s = s[i:]
			break
		}
	}

	// 未更改输入的快速路径
	if b.Cap() == 0 { // 上面没有调用 b.Grow
		return s
	}

	invalid := false // 前一个字节来自无效的 UTF-8 序列
	for i := 0; i < len(s); {
		c := s[i]
		if c < utf8.RuneSelf {
			i++
			invalid = false
			b.WriteByte(c)
			continue
		}
		_, wid := utf8.DecodeRuneInString(s[i:])
		if wid == 1 {
			i++
			if !invalid {
				invalid = true
				b.WriteString(replacement)
			}
			continue
		}
		invalid = false
		b.WriteString(s[i : i+wid])
		i += wid
	}

	return b.String()
}

// isSeparator 报告该 rune 是否可以标记单词边界。
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
	// 否则，目前我们能做的就是将空格视为分隔符。
	return unicode.IsSpace(r)
}

// Title 返回字符串 s 的副本，其中所有开始单词的 Unicode 字母
// 都映射为其 Unicode 标题大小写。
//
// 已弃用：Title 用于单词边界的规则不能正确处理 Unicode 标点符号。
// 请改用 golang.org/x/text/cases。
func Title(s string) string {
	// 在这里使用闭包来记住状态。
	// 有点取巧但有效。依赖于 Map 按顺序扫描并为每个 rune 调用一次闭包。
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

// TrimLeftFunc 返回字符串 s 的切片，其中所有满足 f(c) 的
// 前导 Unicode 码点 c 都被移除。
func TrimLeftFunc(s string, f func(rune) bool) string {
	i := indexFunc(s, f, false)
	if i == -1 {
		return ""
	}
	return s[i:]
}

// TrimRightFunc 返回字符串 s 的切片，其中所有满足 f(c) 的
// 尾随 Unicode 码点 c 都被移除。
func TrimRightFunc(s string, f func(rune) bool) string {
	i := lastIndexFunc(s, f, false)
	if i >= 0 {
		_, wid := utf8.DecodeRuneInString(s[i:])
		i += wid
	} else {
		i++
	}
	return s[0:i]
}

// TrimFunc 返回字符串 s 的切片，其中所有满足 f(c) 的
// 前导和尾随 Unicode 码点 c 都被移除。
func TrimFunc(s string, f func(rune) bool) string {
	return TrimRightFunc(TrimLeftFunc(s, f), f)
}

// IndexFunc 返回第一个满足 f(c) 的 Unicode 码点在 s 中的索引，
// 如果没有则返回 -1。
func IndexFunc(s string, f func(rune) bool) int {
	return indexFunc(s, f, true)
}

// LastIndexFunc 返回最后一个满足 f(c) 的 Unicode 码点在 s 中的索引，
// 如果没有则返回 -1。
func LastIndexFunc(s string, f func(rune) bool) int {
	return lastIndexFunc(s, f, true)
}

// indexFunc is the same as IndexFunc except that if
// truth==false, the sense of the predicate function is
// inverted.
func indexFunc(s string, f func(rune) bool, truth bool) int {
	for i, r := range s {
		if f(r) == truth {
			return i
		}
	}
	return -1
}

// lastIndexFunc is the same as LastIndexFunc except that if
// truth==false, the sense of the predicate function is
// inverted.
func lastIndexFunc(s string, f func(rune) bool, truth bool) int {
	for i := len(s); i > 0; {
		r, size := utf8.DecodeLastRuneInString(s[0:i])
		i -= size
		if f(r) == truth {
			return i
		}
	}
	return -1
}

// asciiSet is a 32-byte value, where each bit represents the presence of a
// given ASCII character in the set. The 128-bits of the lower 16 bytes,
// starting with the least-significant bit of the lowest word to the
// most-significant bit of the highest word, map to the full range of all
// 128 ASCII characters. The 128-bits of the upper 16 bytes will be zeroed,
// ensuring that any non-ASCII character will be reported as not in the set.
// This allocates a total of 32 bytes even though the upper half
// is unused to avoid bounds checks in asciiSet.contains.
type asciiSet [8]uint32

// makeASCIISet creates a set of ASCII characters and reports whether all
// characters in chars are ASCII.
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

// contains reports whether c is inside the set.
func (as *asciiSet) contains(c byte) bool {
	return (as[c/32] & (1 << (c % 32))) != 0
}

// Trim returns a slice of the string s with all leading and
// trailing Unicode code points contained in cutset removed.
func Trim(s, cutset string) string {
	if s == "" || cutset == "" {
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

// TrimLeft returns a slice of the string s with all leading
// Unicode code points contained in cutset removed.
//
// To remove a prefix, use [TrimPrefix] instead.
func TrimLeft(s, cutset string) string {
	if s == "" || cutset == "" {
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

func trimLeftByte(s string, c byte) string {
	for len(s) > 0 && s[0] == c {
		s = s[1:]
	}
	return s
}

func trimLeftASCII(s string, as *asciiSet) string {
	for len(s) > 0 {
		if !as.contains(s[0]) {
			break
		}
		s = s[1:]
	}
	return s
}

func trimLeftUnicode(s, cutset string) string {
	for len(s) > 0 {
		r, n := utf8.DecodeRuneInString(s)
		if !ContainsRune(cutset, r) {
			break
		}
		s = s[n:]
	}
	return s
}

// TrimRight returns a slice of the string s, with all trailing
// Unicode code points contained in cutset removed.
//
// To remove a suffix, use [TrimSuffix] instead.
func TrimRight(s, cutset string) string {
	if s == "" || cutset == "" {
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

func trimRightByte(s string, c byte) string {
	for len(s) > 0 && s[len(s)-1] == c {
		s = s[:len(s)-1]
	}
	return s
}

func trimRightASCII(s string, as *asciiSet) string {
	for len(s) > 0 {
		if !as.contains(s[len(s)-1]) {
			break
		}
		s = s[:len(s)-1]
	}
	return s
}

func trimRightUnicode(s, cutset string) string {
	for len(s) > 0 {
		r, n := rune(s[len(s)-1]), 1
		if r >= utf8.RuneSelf {
			r, n = utf8.DecodeLastRuneInString(s)
		}
		if !ContainsRune(cutset, r) {
			break
		}
		s = s[:len(s)-n]
	}
	return s
}

// TrimSpace returns a slice (substring) of the string s,
// with all leading and trailing white space removed,
// as defined by Unicode.
func TrimSpace(s string) string {
	// Fast path for ASCII: look for the first ASCII non-space byte.
	for lo, c := range []byte(s) {
		if c >= utf8.RuneSelf {
			// If we run into a non-ASCII byte, fall back to the
			// slower unicode-aware method on the remaining bytes.
			return TrimFunc(s[lo:], unicode.IsSpace)
		}
		if asciiSpace[c] != 0 {
			continue
		}
		s = s[lo:]
		// Now look for the first ASCII non-space byte from the end.
		for hi := len(s) - 1; hi >= 0; hi-- {
			c := s[hi]
			if c >= utf8.RuneSelf {
				return TrimRightFunc(s[:hi+1], unicode.IsSpace)
			}
			if asciiSpace[c] == 0 {
				// At this point, s[:hi+1] starts and ends with ASCII
				// non-space bytes, so we're done. Non-ASCII cases have
				// already been handled above.
				return s[:hi+1]
			}
		}
	}
	return ""
}

// TrimPrefix returns s without the provided leading prefix string.
// If s doesn't start with prefix, s is returned unchanged.
func TrimPrefix(s, prefix string) string {
	return stringslite.TrimPrefix(s, prefix)
}

// TrimSuffix returns s without the provided trailing suffix string.
// If s doesn't end with suffix, s is returned unchanged.
func TrimSuffix(s, suffix string) string {
	return stringslite.TrimSuffix(s, suffix)
}

// Replace returns a copy of the string s with the first n
// non-overlapping instances of old replaced by new.
// If old is empty, it matches at the beginning of the string
// and after each UTF-8 sequence, yielding up to k+1 replacements
// for a k-rune string.
// If n < 0, there is no limit on the number of replacements.
func Replace(s, old, new string, n int) string {
	if old == new || n == 0 {
		return s // avoid allocation
	}

	// Compute number of replacements.
	if m := Count(s, old); m == 0 {
		return s // avoid allocation
	} else if n < 0 || m < n {
		n = m
	}

	// Apply replacements to buffer.
	var b Builder
	b.Grow(len(s) + n*(len(new)-len(old)))
	start := 0
	if len(old) > 0 {
		for range n {
			j := start + Index(s[start:], old)
			b.WriteString(s[start:j])
			b.WriteString(new)
			start = j + len(old)
		}
	} else { // len(old) == 0
		b.WriteString(new)
		for range n - 1 {
			_, wid := utf8.DecodeRuneInString(s[start:])
			j := start + wid
			b.WriteString(s[start:j])
			b.WriteString(new)
			start = j
		}
	}
	b.WriteString(s[start:])
	return b.String()
}

// ReplaceAll returns a copy of the string s with all
// non-overlapping instances of old replaced by new.
// If old is empty, it matches at the beginning of the string
// and after each UTF-8 sequence, yielding up to k+1 replacements
// for a k-rune string.
func ReplaceAll(s, old, new string) string {
	return Replace(s, old, new, -1)
}

// EqualFold reports whether s and t, interpreted as UTF-8 strings,
// are equal under simple Unicode case-folding, which is a more general
// form of case-insensitivity.
func EqualFold(s, t string) bool {
	// ASCII fast path
	i := 0
	for n := min(len(s), len(t)); i < n; i++ {
		sr := s[i]
		tr := t[i]
		if sr|tr >= utf8.RuneSelf {
			goto hasUnicode
		}

		// Easy case.
		if tr == sr {
			continue
		}

		// Make sr < tr to simplify what follows.
		if tr < sr {
			tr, sr = sr, tr
		}
		// ASCII only, sr/tr must be upper/lower case
		if 'A' <= sr && sr <= 'Z' && tr == sr+'a'-'A' {
			continue
		}
		return false
	}
	// Check if we've exhausted both strings.
	return len(s) == len(t)

hasUnicode:
	s = s[i:]
	t = t[i:]
	for _, sr := range s {
		// If t is exhausted the strings are not equal.
		if len(t) == 0 {
			return false
		}

		// Extract first rune from second string.
		tr, size := utf8.DecodeRuneInString(t)
		t = t[size:]

		// If they match, keep going; if not, return false.

		// Easy case.
		if tr == sr {
			continue
		}

		// Make sr < tr to simplify what follows.
		if tr < sr {
			tr, sr = sr, tr
		}
		// Fast check for ASCII.
		if tr < utf8.RuneSelf {
			// ASCII only, sr/tr must be upper/lower case
			if 'A' <= sr && sr <= 'Z' && tr == sr+'a'-'A' {
				continue
			}
			return false
		}

		// General case. SimpleFold(x) returns the next equivalent rune > x
		// or wraps around to smaller values.
		r := unicode.SimpleFold(sr)
		for r != sr && r < tr {
			r = unicode.SimpleFold(r)
		}
		if r == tr {
			continue
		}
		return false
	}

	// First string is empty, so check if the second one is also empty.
	return len(t) == 0
}

// Index returns the index of the first instance of substr in s, or -1 if substr is not present in s.
func Index(s, substr string) int {
	return stringslite.Index(s, substr)
}

// Cut slices s around the first instance of sep,
// returning the text before and after sep.
// The found result reports whether sep appears in s.
// If sep does not appear in s, cut returns s, "", false.
func Cut(s, sep string) (before, after string, found bool) {
	return stringslite.Cut(s, sep)
}

// CutPrefix returns s without the provided leading prefix string
// and reports whether it found the prefix.
// If s doesn't start with prefix, CutPrefix returns s, false.
// If prefix is the empty string, CutPrefix returns s, true.
func CutPrefix(s, prefix string) (after string, found bool) {
	return stringslite.CutPrefix(s, prefix)
}

// CutSuffix returns s without the provided ending suffix string
// and reports whether it found the suffix.
// If s doesn't end with suffix, CutSuffix returns s, false.
// If suffix is the empty string, CutSuffix returns s, true.
func CutSuffix(s, suffix string) (before string, found bool) {
	return stringslite.CutSuffix(s, suffix)
}
