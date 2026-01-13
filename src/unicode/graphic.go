// 版权所有 2011 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package unicode

// U+0100 以下每个码点的位掩码，用于快速查找。
const (
	pC     = 1 << iota // 控制字符。
	pP                 // 标点字符。
	pN                 // 数字。
	pS                 // 符号字符。
	pZ                 // 空白字符。
	pLu                // 大写字母。
	pLl                // 小写字母。
	pp                 // 根据 Go 定义的可打印字符。
	pg     = pp | pZ   // 根据 Unicode 定义的图形字符。
	pLo    = pLl | pLu // 既非大写也非小写的字母。
	pLmask = pLo
)

// GraphicRanges 根据 Unicode 定义图形字符的集合。
var GraphicRanges = []*RangeTable{
	L, M, N, P, S, Zs,
}

// PrintRanges 根据 Go 定义可打印字符的集合。
// ASCII 空格 U+0020 单独处理。
var PrintRanges = []*RangeTable{
	L, M, N, P, S,
}

// IsGraphic 报告 rune 是否被 Unicode 定义为图形字符。
// 这些字符包括字母、标记、数字、标点、符号和空格，
// 来自类别 [L]、[M]、[N]、[P]、[S]、[Zs]。
func IsGraphic(r rune) bool {
	// 我们转换为 uint32 以避免对负数的额外测试，
	// 并在索引中转换为 uint8 以避免范围检查。
	if uint32(r) <= MaxLatin1 {
		return properties[uint8(r)]&pg != 0
	}
	return In(r, GraphicRanges...)
}

// IsPrint 报告 rune 是否被 Go 定义为可打印字符。这些字符包括
// 字母、标记、数字、标点、符号和 ASCII 空格字符，
// 来自类别 [L]、[M]、[N]、[P]、[S] 和 ASCII 空格字符。
// 此分类与 [IsGraphic] 相同，除了唯一的空白字符是 ASCII 空格 U+0020。
func IsPrint(r rune) bool {
	if uint32(r) <= MaxLatin1 {
		return properties[uint8(r)]&pp != 0
	}
	return In(r, PrintRanges...)
}

// IsOneOf 报告 rune 是否是范围之一的成员。
// 函数 "In" 提供了更好的签名，应优先于 IsOneOf 使用。
func IsOneOf(ranges []*RangeTable, r rune) bool {
	for _, inside := range ranges {
		if Is(inside, r) {
			return true
		}
	}
	return false
}

// In 报告 rune 是否是范围之一的成员。
func In(r rune, ranges ...*RangeTable) bool {
	for _, inside := range ranges {
		if Is(inside, r) {
			return true
		}
	}
	return false
}

// IsControl 报告 rune 是否是控制字符。
// [C]（[Other]）Unicode 类别包含更多码点，
// 如代理项；使用 [Is](C, r) 来测试它们。
func IsControl(r rune) bool {
	if uint32(r) <= MaxLatin1 {
		return properties[uint8(r)]&pC != 0
	}
	// 所有控制字符都 < MaxLatin1。
	return false
}

// IsLetter 报告 rune 是否是字母（类别 [L]）。
func IsLetter(r rune) bool {
	if uint32(r) <= MaxLatin1 {
		return properties[uint8(r)]&(pLmask) != 0
	}
	return isExcludingLatin(Letter, r)
}

// IsMark 报告 rune 是否是标记字符（类别 [M]）。
func IsMark(r rune) bool {
	// Latin-1 中没有标记字符。
	return isExcludingLatin(Mark, r)
}

// IsNumber 报告 rune 是否是数字（类别 [N]）。
func IsNumber(r rune) bool {
	if uint32(r) <= MaxLatin1 {
		return properties[uint8(r)]&pN != 0
	}
	return isExcludingLatin(Number, r)
}

// IsPunct 报告 rune 是否是 Unicode 标点字符（类别 [P]）。
func IsPunct(r rune) bool {
	if uint32(r) <= MaxLatin1 {
		return properties[uint8(r)]&pP != 0
	}
	return Is(Punct, r)
}

// IsSpace 报告 rune 是否是由 Unicode 的 White Space 属性定义的空白字符；
// 在 Latin-1 空间中包括：
//
//	'\t', '\n', '\v', '\f', '\r', ' ', U+0085 (NEL), U+00A0 (NBSP)。
//
// 空白字符的其他定义由类别 Z 和属性 [Pattern_White_Space] 设定。
func IsSpace(r rune) bool {
	// 此属性与 Z 不同；特殊处理它。
	if uint32(r) <= MaxLatin1 {
		switch r {
		case '\t', '\n', '\v', '\f', '\r', ' ', 0x85, 0xA0:
			return true
		}
		return false
	}
	return isExcludingLatin(White_Space, r)
}

// IsSymbol 报告 rune 是否是符号字符。
func IsSymbol(r rune) bool {
	if uint32(r) <= MaxLatin1 {
		return properties[uint8(r)]&pS != 0
	}
	return isExcludingLatin(Symbol, r)
}
