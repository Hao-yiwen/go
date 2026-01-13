// 版权所有 2009 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// unicode 包提供数据和函数来测试 Unicode 码点的某些属性。
package unicode

const (
	MaxRune         = '\U0010FFFF' // 最大有效 Unicode 码点。
	ReplacementChar = '\uFFFD'     // 表示无效码点。
	MaxASCII        = '\u007F'     // 最大 ASCII 值。
	MaxLatin1       = '\u00FF'     // 最大 Latin-1 值。
)

// RangeTable 通过列出集合内码点的范围来定义一组 Unicode 码点。
// 范围分别列在两个切片中以节省空间：一个 16 位范围的切片和一个 32 位范围的切片。
// 两个切片必须是有序的且不重叠。
// 此外，R32 应只包含 >= 0x10000 (1<<16) 的值。
type RangeTable struct {
	R16         []Range16
	R32         []Range32
	LatinOffset int // R16 中 Hi <= MaxLatin1 的条目数量
}

// Range16 表示 16 位 Unicode 码点的范围。范围从 Lo 到 Hi（包含两端），
// 并具有指定的步长。
type Range16 struct {
	Lo     uint16
	Hi     uint16
	Stride uint16
}

// Range32 表示 Unicode 码点的范围，当一个或多个值无法用 16 位表示时使用。
// 范围从 Lo 到 Hi（包含两端），并具有指定的步长。
// Lo 和 Hi 必须始终 >= 1<<16。
type Range32 struct {
	Lo     uint32
	Hi     uint32
	Stride uint32
}

// CaseRange 表示用于简单（一对一码点）大小写转换的 Unicode 码点范围。
// 范围从 Lo 到 Hi（包含两端），步长固定为 1。Delta
// 是要添加到码点以达到该字符不同大小写的码点的数值。它们可能为负数。
// 如果为零，表示该字符已经是对应的大小写。有一种特殊情况表示
// 交替对应的大写和小写对的序列。它以固定的 Delta 出现：
//
//	{UpperLower, UpperLower, UpperLower}
//
// 常量 UpperLower 具有一个不可能的 delta 值。
type CaseRange struct {
	Lo    uint32
	Hi    uint32
	Delta d
}

// SpecialCase 表示特定语言的大小写映射，如土耳其语。
// SpecialCase 的方法通过覆盖来定制标准映射。
type SpecialCase []CaseRange

// BUG(r): 没有完全大小写折叠的机制，即涉及输入或输出中多个 rune 的字符。

// 用于大小写映射的 CaseRanges 中 Delta 数组的索引。
const (
	UpperCase = iota
	LowerCase
	TitleCase
	MaxCase
)

type d [MaxCase]rune // 使 CaseRanges 文本更短

// 如果 [CaseRange] 的 Delta 字段是 UpperLower，表示
// 此 CaseRange 代表形如 [Upper] [Lower] [Upper] [Lower] 的序列。
const (
	UpperLower = MaxRune + 1 // （不能是有效的 delta 值。）
)

// linearMax 是对非 Latin1 rune 进行线性搜索的最大表大小。
// 通过运行 'go test -calibrate' 得出。
const linearMax = 18

// is16 报告 r 是否在已排序的 16 位范围切片中。
func is16(ranges []Range16, r uint16) bool {
	if len(ranges) <= linearMax || r <= MaxLatin1 {
		for i := range ranges {
			range_ := &ranges[i]
			if r < range_.Lo {
				return false
			}
			if r <= range_.Hi {
				return range_.Stride == 1 || (r-range_.Lo)%range_.Stride == 0
			}
		}
		return false
	}

	// 对范围进行二分搜索
	lo := 0
	hi := len(ranges)
	for lo < hi {
		m := int(uint(lo+hi) >> 1)
		range_ := &ranges[m]
		if range_.Lo <= r && r <= range_.Hi {
			return range_.Stride == 1 || (r-range_.Lo)%range_.Stride == 0
		}
		if r < range_.Lo {
			hi = m
		} else {
			lo = m + 1
		}
	}
	return false
}

// is32 报告 r 是否在已排序的 32 位范围切片中。
func is32(ranges []Range32, r uint32) bool {
	if len(ranges) <= linearMax {
		for i := range ranges {
			range_ := &ranges[i]
			if r < range_.Lo {
				return false
			}
			if r <= range_.Hi {
				return range_.Stride == 1 || (r-range_.Lo)%range_.Stride == 0
			}
		}
		return false
	}

	// 对范围进行二分搜索
	lo := 0
	hi := len(ranges)
	for lo < hi {
		m := int(uint(lo+hi) >> 1)
		range_ := ranges[m]
		if range_.Lo <= r && r <= range_.Hi {
			return range_.Stride == 1 || (r-range_.Lo)%range_.Stride == 0
		}
		if r < range_.Lo {
			hi = m
		} else {
			lo = m + 1
		}
	}
	return false
}

// Is 报告 rune 是否在指定的范围表中。
func Is(rangeTab *RangeTable, r rune) bool {
	r16 := rangeTab.R16
	// 作为 uint32 比较以正确处理负数 rune。
	if len(r16) > 0 && uint32(r) <= uint32(r16[len(r16)-1].Hi) {
		return is16(r16, uint16(r))
	}
	r32 := rangeTab.R32
	if len(r32) > 0 && r >= rune(r32[0].Lo) {
		return is32(r32, uint32(r))
	}
	return false
}

func isExcludingLatin(rangeTab *RangeTable, r rune) bool {
	r16 := rangeTab.R16
	// 作为 uint32 比较以正确处理负数 rune。
	if off := rangeTab.LatinOffset; len(r16) > off && uint32(r) <= uint32(r16[len(r16)-1].Hi) {
		return is16(r16[off:], uint16(r))
	}
	r32 := rangeTab.R32
	if len(r32) > 0 && r >= rune(r32[0].Lo) {
		return is32(r32, uint32(r))
	}
	return false
}

// IsUpper 报告 rune 是否是大写字母。
func IsUpper(r rune) bool {
	// 参见 IsGraphic 中的注释。
	if uint32(r) <= MaxLatin1 {
		return properties[uint8(r)]&pLmask == pLu
	}
	return isExcludingLatin(Upper, r)
}

// IsLower 报告 rune 是否是小写字母。
func IsLower(r rune) bool {
	// 参见 IsGraphic 中的注释。
	if uint32(r) <= MaxLatin1 {
		return properties[uint8(r)]&pLmask == pLl
	}
	return isExcludingLatin(Lower, r)
}

// IsTitle 报告 rune 是否是标题大小写字母。
func IsTitle(r rune) bool {
	if r <= MaxLatin1 {
		return false
	}
	return isExcludingLatin(Title, r)
}

// lookupCaseRange 返回 rune r 的 CaseRange 映射，如果 r 没有映射则返回 nil。
func lookupCaseRange(r rune, caseRange []CaseRange) *CaseRange {
	// 对范围进行二分搜索
	lo := 0
	hi := len(caseRange)
	for lo < hi {
		m := int(uint(lo+hi) >> 1)
		cr := &caseRange[m]
		if rune(cr.Lo) <= r && r <= rune(cr.Hi) {
			return cr
		}
		if r < rune(cr.Lo) {
			hi = m
		} else {
			lo = m + 1
		}
	}
	return nil
}

// convertCase 使用 CaseRange cr 将 r 转换为 _case。
func convertCase(_case int, r rune, cr *CaseRange) rune {
	delta := cr.Delta[_case]
	if delta > MaxRune {
		// 在一个总是以大写字母开始的 Upper-Lower 序列中，
		// 实际的 delta 总是看起来像：
		//	{0, 1, 0}    UpperCase（Lower 是下一个）
		//	{-1, 0, -1}  LowerCase（Upper、Title 是前一个）
		// 从序列开头偏移量为偶数的字符是大写的；
		// 偏移量为奇数的是小写的。
		// 正确的映射可以通过清除或设置序列偏移量中的低位来完成。
		// 常量 UpperCase 和 TitleCase 是偶数，而 LowerCase
		// 是奇数，所以我们从 _case 取低位。
		return rune(cr.Lo) + ((r-rune(cr.Lo))&^1 | rune(_case&1))
	}
	return r + delta
}

// to 使用指定的大小写映射转换 rune。
// 它还报告 caseRange 是否包含 r 的映射。
func to(_case int, r rune, caseRange []CaseRange) (mappedRune rune, foundMapping bool) {
	if _case < 0 || MaxCase <= _case {
		return ReplacementChar, false // 作为任何合理的错误
	}
	if cr := lookupCaseRange(r, caseRange); cr != nil {
		return convertCase(_case, r, cr), true
	}
	return r, false
}

// To 将 rune 映射到指定的大小写：[UpperCase]、[LowerCase] 或 [TitleCase]。
func To(_case int, r rune) rune {
	r, _ = to(_case, r, CaseRanges)
	return r
}

// ToUpper 将 rune 映射到大写。
func ToUpper(r rune) rune {
	if r <= MaxASCII {
		if 'a' <= r && r <= 'z' {
			r -= 'a' - 'A'
		}
		return r
	}
	return To(UpperCase, r)
}

// ToLower 将 rune 映射到小写。
func ToLower(r rune) rune {
	if r <= MaxASCII {
		if 'A' <= r && r <= 'Z' {
			r += 'a' - 'A'
		}
		return r
	}
	return To(LowerCase, r)
}

// ToTitle 将 rune 映射到标题大小写。
func ToTitle(r rune) rune {
	if r <= MaxASCII {
		if 'a' <= r && r <= 'z' { // 对于 ASCII，标题大小写就是大写
			r -= 'a' - 'A'
		}
		return r
	}
	return To(TitleCase, r)
}

// ToUpper 将 rune 映射到大写，优先使用特殊映射。
func (special SpecialCase) ToUpper(r rune) rune {
	r1, hadMapping := to(UpperCase, r, []CaseRange(special))
	if r1 == r && !hadMapping {
		r1 = ToUpper(r)
	}
	return r1
}

// ToTitle 将 rune 映射到标题大小写，优先使用特殊映射。
func (special SpecialCase) ToTitle(r rune) rune {
	r1, hadMapping := to(TitleCase, r, []CaseRange(special))
	if r1 == r && !hadMapping {
		r1 = ToTitle(r)
	}
	return r1
}

// ToLower 将 rune 映射到小写，优先使用特殊映射。
func (special SpecialCase) ToLower(r rune) rune {
	r1, hadMapping := to(LowerCase, r, []CaseRange(special))
	if r1 == r && !hadMapping {
		r1 = ToLower(r)
	}
	return r1
}

// caseOrbit 在 tables.go 中定义为 []foldPair。目前所有条目都适合 uint16，
// 所以使用 uint16。如果这种情况改变，编译将失败
// （复合字面量中的常量将无法放入 uint16），
// 此处的类型可以改为 uint32。
type foldPair struct {
	From uint16
	To   uint16
}

// SimpleFold 遍历在 Unicode 定义的简单大小写折叠下等价的 Unicode 码点。
// 在与 rune 等价的码点中（包括 rune 本身），SimpleFold 返回
// 大于 r 的最小 rune（如果存在），否则返回 >= 0 的最小 rune。
// 如果 r 不是有效的 Unicode 码点，SimpleFold(r) 返回 r。
//
// For example:
//
//	SimpleFold('A') = 'a'
//	SimpleFold('a') = 'A'
//
//	SimpleFold('K') = 'k'
//	SimpleFold('k') = '\u212A' (Kelvin symbol, K)
//	SimpleFold('\u212A') = 'K'
//
//	SimpleFold('1') = '1'
//
//	SimpleFold(-2) = -2
func SimpleFold(r rune) rune {
	if r < 0 || r > MaxRune {
		return r
	}

	if int(r) < len(asciiFold) {
		return rune(asciiFold[r])
	}

	// 查阅 caseOrbit 表处理特殊情况。
	lo := 0
	hi := len(caseOrbit)
	for lo < hi {
		m := int(uint(lo+hi) >> 1)
		if rune(caseOrbit[m].From) < r {
			lo = m + 1
		} else {
			hi = m
		}
	}
	if lo < len(caseOrbit) && rune(caseOrbit[lo].From) == r {
		return rune(caseOrbit[lo].To)
	}

	// 没有指定折叠。这是一个包含 rune 和 ToLower(rune)
	// 以及 ToUpper(rune)（如果它们与 rune 不同）的
	// 一元或二元等价类。
	if cr := lookupCaseRange(r, CaseRanges); cr != nil {
		if l := convertCase(LowerCase, r, cr); l != r {
			return l
		}
		return convertCase(UpperCase, r, cr)
	}
	return r
}
