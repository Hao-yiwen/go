// 版权所有 2012 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package strings

// stringFinder 在源文本中高效查找字符串。它使用
// Boyer-Moore 字符串搜索算法实现：
// https://en.wikipedia.org/wiki/Boyer-Moore_string_search_algorithm
// https://www.cs.utexas.edu/~moore/publications/fstrpos.pdf（注意：这份较旧的
// 文档使用基于 1 的索引）
type stringFinder struct {
	// pattern 是我们在文本中搜索的字符串。
	pattern string

	// badCharSkip[b] 包含 pattern 最后一个字节与 pattern 中 b 最右边出现位置
	// 之间的距离。如果 b 不在 pattern 中，badCharSkip[b] 为 len(pattern)。
	//
	// 每当在文本中发现与字节 b 不匹配时，我们可以安全地
	// 将匹配框架至少移动 badCharSkip[b]，直到下次
	// 匹配字符可能对齐为止。
	badCharSkip [256]int

	// goodSuffixSkip[i] 定义了在后缀 pattern[i+1:] 匹配但
	// 字节 pattern[i] 不匹配的情况下，我们可以将匹配框架移动多远。
	// 有两种情况需要考虑：
	//
	// 1. 匹配的后缀在 pattern 中的其他位置出现（前面有一个不同的
	// 字节，我们可能会匹配到）。在这种情况下，我们可以
	// 移动匹配框架以与下一个后缀块对齐。例如，
	// 模式 "mississi" 的后缀 "issi" 下一次出现
	// （从右到左顺序）在索引 1，所以 goodSuffixSkip[3] ==
	// shift+len(suffix) == 3+4 == 7。
	//
	// 2. 如果匹配的后缀没有在 pattern 的其他位置出现，那么
	// 匹配框架可能与匹配后缀的末尾共享其前缀的一部分。
	// 在这种情况下，goodSuffixSkip[i] 将包含移动框架以使
	// 前缀的这部分与后缀对齐的距离。例如，在模式 "abcxxxabc" 中，
	// 当从后面发现的第一个不匹配在位置 3 时，匹配
	// 后缀 "xxabc" 在模式中的其他位置找不到。然而，它
	// 最右边的 "abc"（在位置 6）是整个模式的前缀，所以
	// goodSuffixSkip[3] == shift+len(suffix) == 6+5 == 11。
	goodSuffixSkip []int
}

func makeStringFinder(pattern string) *stringFinder {
	f := &stringFinder{
		pattern:        pattern,
		goodSuffixSkip: make([]int, len(pattern)),
	}
	// last 是模式中最后一个字符的索引。
	last := len(pattern) - 1

	// 构建坏字符表。
	// 不在模式中的字节可以跳过一个模式长度。
	for i := range f.badCharSkip {
		f.badCharSkip[i] = len(pattern)
	}
	// 循环条件是 < 而不是 <=，这样最后一个字节不会
	// 与自身的距离为零。发现这个字节不在其位置意味着
	// 它不在最后位置。
	for i := 0; i < last; i++ {
		f.badCharSkip[pattern[i]] = last - i
	}

	// 构建好后缀表。
	// 第一遍：将每个值设置为下一个开始 pattern 前缀的索引。
	lastPrefix := last
	for i := last; i >= 0; i-- {
		if HasPrefix(pattern, pattern[i+1:]) {
			lastPrefix = i + 1
		}
		// lastPrefix 是移动量，(last-i) 是 len(suffix)。
		f.goodSuffixSkip[i] = lastPrefix + last - i
	}
	// 第二遍：从前面开始查找 pattern 后缀的重复。
	for i := 0; i < last; i++ {
		lenSuffix := longestCommonSuffix(pattern, pattern[1:i+1])
		if pattern[i-lenSuffix] != pattern[last-lenSuffix] {
			// (last-i) 是移动量，lenSuffix 是 len(suffix)。
			f.goodSuffixSkip[last-lenSuffix] = lenSuffix + last - i
		}
	}

	return f
}

func longestCommonSuffix(a, b string) (i int) {
	for ; i < len(a) && i < len(b); i++ {
		if a[len(a)-1-i] != b[len(b)-1-i] {
			break
		}
	}
	return
}

// next 返回模式在 text 中第一次出现的索引。如果
// 未找到模式，则返回 -1。
func (f *stringFinder) next(text string) int {
	i := len(f.pattern) - 1
	for i < len(text) {
		// 从末尾向后比较，直到第一个不匹配的字符。
		j := len(f.pattern) - 1
		for j >= 0 && text[i] == f.pattern[j] {
			i--
			j--
		}
		if j < 0 {
			return i + 1 // 匹配
		}
		i += max(f.badCharSkip[text[i]], f.goodSuffixSkip[j])
	}
	return -1
}
