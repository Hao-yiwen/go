// 版权所有 2011 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package syntax

// 实现者注意：
// 在这个包中，re 始终是 *Regexp，r 始终是 rune。

import (
	"slices"
	"strconv"
	"strings"
	"unicode"
)

// Regexp 是正则表达式语法树中的一个节点。
type Regexp struct {
	Op       Op // 操作符
	Flags    Flags
	Sub      []*Regexp  // 子表达式（如果有）
	Sub0     [1]*Regexp // 短 Sub 的存储
	Rune     []rune     // 匹配的 rune，用于 OpLiteral、OpCharClass
	Rune0    [2]rune    // 短 Rune 的存储
	Min, Max int        // 用于 OpRepeat 的最小值和最大值
	Cap      int        // 捕获索引，用于 OpCapture
	Name     string     // 捕获名称，用于 OpCapture
}

//go:generate stringer -type Op -trimprefix Op

// Op 是单个正则表达式操作符。
type Op uint8

// 操作符按优先级顺序列出，从最紧密绑定到最松散绑定。
// 字符类操作符按从简单到复杂的顺序列出
// （OpLiteral、OpCharClass、OpAnyCharNotNL、OpAnyChar）。

const (
	OpNoMatch        Op = 1 + iota // 不匹配任何字符串
	OpEmptyMatch                   // 匹配空字符串
	OpLiteral                      // 匹配 Runes 序列
	OpCharClass                    // 匹配解释为范围对列表的 Runes
	OpAnyCharNotNL                 // 匹配除换行符外的任意字符
	OpAnyChar                      // 匹配任意字符
	OpBeginLine                    // 匹配行首的空字符串
	OpEndLine                      // 匹配行尾的空字符串
	OpBeginText                    // 匹配文本开头的空字符串
	OpEndText                      // 匹配文本结尾的空字符串
	OpWordBoundary                 // 匹配单词边界 `\b`
	OpNoWordBoundary               // 匹配非单词边界 `\B`
	OpCapture                      // 带索引 Cap 的捕获子表达式，可选名称 Name
	OpStar                         // 匹配 Sub[0] 零次或多次
	OpPlus                         // 匹配 Sub[0] 一次或多次
	OpQuest                        // 匹配 Sub[0] 零次或一次
	OpRepeat                       // 匹配 Sub[0] 至少 Min 次，最多 Max 次（Max == -1 表示无限制）
	OpConcat                       // 匹配 Subs 的连接
	OpAlternate                    // 匹配 Subs 的交替
)

const opPseudo Op = 128 // 伪操作符的起始位置

// Equal 报告 x 和 y 是否具有相同的结构。
func (x *Regexp) Equal(y *Regexp) bool {
	if x == nil || y == nil {
		return x == y
	}
	if x.Op != y.Op {
		return false
	}
	switch x.Op {
	case OpEndText:
		// 解析标志记住这是 \z 还是 \Z。
		if x.Flags&WasDollar != y.Flags&WasDollar {
			return false
		}

	case OpLiteral, OpCharClass:
		return x.Flags&FoldCase == y.Flags&FoldCase && slices.Equal(x.Rune, y.Rune)

	case OpAlternate, OpConcat:
		return slices.EqualFunc(x.Sub, y.Sub, (*Regexp).Equal)

	case OpStar, OpPlus, OpQuest:
		if x.Flags&NonGreedy != y.Flags&NonGreedy || !x.Sub[0].Equal(y.Sub[0]) {
			return false
		}

	case OpRepeat:
		if x.Flags&NonGreedy != y.Flags&NonGreedy || x.Min != y.Min || x.Max != y.Max || !x.Sub[0].Equal(y.Sub[0]) {
			return false
		}

	case OpCapture:
		if x.Cap != y.Cap || x.Name != y.Name || !x.Sub[0].Equal(y.Sub[0]) {
			return false
		}
	}
	return true
}

// printFlags 是一个位集，指示在正则表达式周围打印哪些标志（包括非捕获括号）。
type printFlags uint8

const (
	flagI    printFlags = 1 << iota // (?i:
	flagM                           // (?m:
	flagS                           // (?s:
	flagOff                         // )
	flagPrec                        // (?: )
	negShift = 5                    // flagI<<negShift 是 (?-i:
)

// addSpan 通过设置 flags[start] = f 和 flags[last] = flagOff，
// 在 start..last 范围内启用标志 f。
func addSpan(start, last *Regexp, f printFlags, flags *map[*Regexp]printFlags) {
	if *flags == nil {
		*flags = make(map[*Regexp]printFlags)
	}
	(*flags)[start] = f
	(*flags)[last] |= flagOff // 可能 start==last
}

// calcFlags 计算要在 re 中每个子表达式周围打印的标志，
// 将该信息存储在 (*flags)[sub] 中，用于每个受影响的子表达式。
// 第一次需要向 *flags 写入条目时，calcFlags 分配映射。
// calcFlags 还计算必须在 re 周围激活或不能激活的标志，
// 并返回这些标志。
func calcFlags(re *Regexp, flags *map[*Regexp]printFlags) (must, cant printFlags) {
	switch re.Op {
	default:
		return 0, 0

	case OpLiteral:
		// 如果字面量对大小写折叠敏感，根据 (?i) 是否激活，
		// 返回 (flagI, 0) 或 (0, flagI)。
		// 如果字面量对大小写折叠不敏感，返回 0, 0。
		for _, r := range re.Rune {
			if minFold <= r && r <= maxFold && unicode.SimpleFold(r) != r {
				if re.Flags&FoldCase != 0 {
					return flagI, 0
				} else {
					return 0, flagI
				}
			}
		}
		return 0, 0

	case OpCharClass:
		// 如果字面量对大小写折叠敏感，返回 0, flagI - (?i) 已被编译掉。
		// 如果字面量对大小写折叠不敏感，返回 0, 0。
		for i := 0; i < len(re.Rune); i += 2 {
			lo := max(minFold, re.Rune[i])
			hi := min(maxFold, re.Rune[i+1])
			for r := lo; r <= hi; r++ {
				for f := unicode.SimpleFold(r); f != r; f = unicode.SimpleFold(f) {
					if !(lo <= f && f <= hi) && !inCharClass(f, re.Rune) {
						return 0, flagI
					}
				}
			}
		}
		return 0, 0

	case OpAnyCharNotNL: // (?-s).
		return 0, flagS

	case OpAnyChar: // (?s).
		return flagS, 0

	case OpBeginLine, OpEndLine: // (?m)^ (?m)$
		return flagM, 0

	case OpEndText:
		if re.Flags&WasDollar != 0 { // (?-m)$
			return 0, flagM
		}
		return 0, 0

	case OpCapture, OpStar, OpPlus, OpQuest, OpRepeat:
		return calcFlags(re.Sub[0], flags)

	case OpConcat, OpAlternate:
		// 收集每个子表达式的 must 和 cant。
		// 当发现冲突的子表达式时，在先前识别的范围周围插入必要的标志并重新开始。
		var must, cant, allCant printFlags
		start := 0
		last := 0
		did := false
		for i, sub := range re.Sub {
			subMust, subCant := calcFlags(sub, flags)
			if must&subCant != 0 || subMust&cant != 0 {
				if must != 0 {
					addSpan(re.Sub[start], re.Sub[last], must, flags)
				}
				must = 0
				cant = 0
				start = i
				did = true
			}
			must |= subMust
			cant |= subCant
			allCant |= subCant
			if subMust != 0 {
				last = i
			}
			if must == 0 && start == i {
				start++
			}
		}
		if !did {
			// 无冲突：向上传递累积的 must 和 cant。
			return must, cant
		}
		if must != 0 {
			// 发现冲突；需要完成最后的范围。
			addSpan(re.Sub[start], re.Sub[last], must, flags)
		}
		return 0, allCant
	}
}

// writeRegexp 将正则表达式 re 的 Perl 语法写入 b。
func writeRegexp(b *strings.Builder, re *Regexp, f printFlags, flags map[*Regexp]printFlags) {
	f |= flags[re]
	if f&flagPrec != 0 && f&^(flagOff|flagPrec) != 0 && f&flagOff != 0 {
		// flagPrec 与其他被添加和终止的标志冗余
		f &^= flagPrec
	}
	if f&^(flagOff|flagPrec) != 0 {
		b.WriteString(`(?`)
		if f&flagI != 0 {
			b.WriteString(`i`)
		}
		if f&flagM != 0 {
			b.WriteString(`m`)
		}
		if f&flagS != 0 {
			b.WriteString(`s`)
		}
		if f&((flagM|flagS)<<negShift) != 0 {
			b.WriteString(`-`)
			if f&(flagM<<negShift) != 0 {
				b.WriteString(`m`)
			}
			if f&(flagS<<negShift) != 0 {
				b.WriteString(`s`)
			}
		}
		b.WriteString(`:`)
	}
	if f&flagOff != 0 {
		defer b.WriteString(`)`)
	}
	if f&flagPrec != 0 {
		b.WriteString(`(?:`)
		defer b.WriteString(`)`)
	}

	switch re.Op {
	default:
		b.WriteString("<invalid op" + strconv.Itoa(int(re.Op)) + ">")
	case OpNoMatch:
		b.WriteString(`[^\x00-\x{10FFFF}]`)
	case OpEmptyMatch:
		b.WriteString(`(?:)`)
	case OpLiteral:
		for _, r := range re.Rune {
			escape(b, r, false)
		}
	case OpCharClass:
		if len(re.Rune)%2 != 0 {
			b.WriteString(`[invalid char class]`)
			break
		}
		b.WriteRune('[')
		if len(re.Rune) == 0 {
			b.WriteString(`^\x00-\x{10FFFF}`)
		} else if re.Rune[0] == 0 && re.Rune[len(re.Rune)-1] == unicode.MaxRune && len(re.Rune) > 2 {
			// 包含 0 和 MaxRune。可能是一个否定类。
			// 打印间隙。
			b.WriteRune('^')
			for i := 1; i < len(re.Rune)-1; i += 2 {
				lo, hi := re.Rune[i]+1, re.Rune[i+1]-1
				escape(b, lo, lo == '-')
				if lo != hi {
					if hi != lo+1 {
						b.WriteRune('-')
					}
					escape(b, hi, hi == '-')
				}
			}
		} else {
			for i := 0; i < len(re.Rune); i += 2 {
				lo, hi := re.Rune[i], re.Rune[i+1]
				escape(b, lo, lo == '-')
				if lo != hi {
					if hi != lo+1 {
						b.WriteRune('-')
					}
					escape(b, hi, hi == '-')
				}
			}
		}
		b.WriteRune(']')
	case OpAnyCharNotNL, OpAnyChar:
		b.WriteString(`.`)
	case OpBeginLine:
		b.WriteString(`^`)
	case OpEndLine:
		b.WriteString(`$`)
	case OpBeginText:
		b.WriteString(`\A`)
	case OpEndText:
		if re.Flags&WasDollar != 0 {
			b.WriteString(`$`)
		} else {
			b.WriteString(`\z`)
		}
	case OpWordBoundary:
		b.WriteString(`\b`)
	case OpNoWordBoundary:
		b.WriteString(`\B`)
	case OpCapture:
		if re.Name != "" {
			b.WriteString(`(?P<`)
			b.WriteString(re.Name)
			b.WriteRune('>')
		} else {
			b.WriteRune('(')
		}
		if re.Sub[0].Op != OpEmptyMatch {
			writeRegexp(b, re.Sub[0], flags[re.Sub[0]], flags)
		}
		b.WriteRune(')')
	case OpStar, OpPlus, OpQuest, OpRepeat:
		p := printFlags(0)
		sub := re.Sub[0]
		if sub.Op > OpCapture || sub.Op == OpLiteral && len(sub.Rune) > 1 {
			p = flagPrec
		}
		writeRegexp(b, sub, p, flags)

		switch re.Op {
		case OpStar:
			b.WriteRune('*')
		case OpPlus:
			b.WriteRune('+')
		case OpQuest:
			b.WriteRune('?')
		case OpRepeat:
			b.WriteRune('{')
			b.WriteString(strconv.Itoa(re.Min))
			if re.Max != re.Min {
				b.WriteRune(',')
				if re.Max >= 0 {
					b.WriteString(strconv.Itoa(re.Max))
				}
			}
			b.WriteRune('}')
		}
		if re.Flags&NonGreedy != 0 {
			b.WriteRune('?')
		}
	case OpConcat:
		for _, sub := range re.Sub {
			p := printFlags(0)
			if sub.Op == OpAlternate {
				p = flagPrec
			}
			writeRegexp(b, sub, p, flags)
		}
	case OpAlternate:
		for i, sub := range re.Sub {
			if i > 0 {
				b.WriteRune('|')
			}
			writeRegexp(b, sub, 0, flags)
		}
	}
}

func (re *Regexp) String() string {
	var b strings.Builder
	var flags map[*Regexp]printFlags
	must, cant := calcFlags(re, &flags)
	must |= (cant &^ flagI) << negShift
	if must != 0 {
		must |= flagOff
	}
	writeRegexp(&b, re, must, flags)
	return b.String()
}

const meta = `\.+*?()|[]{}^$`

func escape(b *strings.Builder, r rune, force bool) {
	if unicode.IsPrint(r) {
		if strings.ContainsRune(meta, r) || force {
			b.WriteRune('\\')
		}
		b.WriteRune(r)
		return
	}

	switch r {
	case '\a':
		b.WriteString(`\a`)
	case '\f':
		b.WriteString(`\f`)
	case '\n':
		b.WriteString(`\n`)
	case '\r':
		b.WriteString(`\r`)
	case '\t':
		b.WriteString(`\t`)
	case '\v':
		b.WriteString(`\v`)
	default:
		if r < 0x100 {
			b.WriteString(`\x`)
			s := strconv.FormatInt(int64(r), 16)
			if len(s) == 1 {
				b.WriteRune('0')
			}
			b.WriteString(s)
			break
		}
		b.WriteString(`\x{`)
		b.WriteString(strconv.FormatInt(int64(r), 16))
		b.WriteString(`}`)
	}
}

// MaxCap 遍历正则表达式以找到最大捕获索引。
func (re *Regexp) MaxCap() int {
	m := 0
	if re.Op == OpCapture {
		m = re.Cap
	}
	for _, sub := range re.Sub {
		if n := sub.MaxCap(); m < n {
			m = n
		}
	}
	return m
}

// CapNames 遍历正则表达式以找到捕获组的名称。
func (re *Regexp) CapNames() []string {
	names := make([]string, re.MaxCap()+1)
	re.capNames(names)
	return names
}

func (re *Regexp) capNames(names []string) {
	if re.Op == OpCapture {
		names[re.Cap] = re.Name
	}
	for _, sub := range re.Sub {
		sub.capNames(names)
	}
}
