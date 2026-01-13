// 版权所有 2011 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package syntax

import (
	"sort"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

// Error 描述了正则表达式解析失败
// 并提供了导致失败的表达式。
type Error struct {
	Code ErrorCode
	Expr string
}

func (e *Error) Error() string {
	return "error parsing regexp: " + e.Code.String() + ": `" + e.Expr + "`"
}

// ErrorCode 描述了正则表达式解析失败的原因。
type ErrorCode string

const (
	// 意外错误
	ErrInternalError ErrorCode = "regexp/syntax: internal error"

	// 解析错误
	ErrInvalidCharClass      ErrorCode = "invalid character class"
	ErrInvalidCharRange      ErrorCode = "invalid character class range"
	ErrInvalidEscape         ErrorCode = "invalid escape sequence"
	ErrInvalidNamedCapture   ErrorCode = "invalid named capture"
	ErrInvalidPerlOp         ErrorCode = "invalid or unsupported Perl syntax"
	ErrInvalidRepeatOp       ErrorCode = "invalid nested repetition operator"
	ErrInvalidRepeatSize     ErrorCode = "invalid repeat count"
	ErrInvalidUTF8           ErrorCode = "invalid UTF-8"
	ErrMissingBracket        ErrorCode = "missing closing ]"
	ErrMissingParen          ErrorCode = "missing closing )"
	ErrMissingRepeatArgument ErrorCode = "missing argument to repetition operator"
	ErrTrailingBackslash     ErrorCode = "trailing backslash at end of expression"
	ErrUnexpectedParen       ErrorCode = "unexpected )"
	ErrNestingDepth          ErrorCode = "expression nests too deeply"
	ErrLarge                 ErrorCode = "expression too large"
)

func (e ErrorCode) String() string {
	return string(e)
}

// Flags 控制解析器的行为并记录关于正则表达式上下文的信息。
type Flags uint16

const (
	FoldCase      Flags = 1 << iota // 不区分大小写的匹配
	Literal                         // 将模式视为字面字符串
	ClassNL                         // 允许字符类（如 [^a-z] 和 [[:space:]]）匹配换行符
	DotNL                           // 允许 . 匹配换行符
	OneLine                         // 将 ^ 和 $ 视为仅在文本开头和结尾匹配
	NonGreedy                       // 使重复操作符默认为非贪婪
	PerlX                           // 允许 Perl 扩展
	UnicodeGroups                   // 允许 \p{Han}、\P{Han} 用于 Unicode 组和取反
	WasDollar                       // 正则表达式 OpEndText 是 $，不是 \z
	Simple                          // 正则表达式不包含计数重复

	MatchNL = ClassNL | DotNL

	Perl        = ClassNL | OneLine | PerlX | UnicodeGroups // 尽可能接近 Perl
	POSIX Flags = 0                                         // POSIX 语法
)

// 解析栈的伪操作。
const (
	opLeftParen = opPseudo + iota
	opVerticalBar
)

// maxHeight 是正则表达式解析树的最大高度。
// 它的选择有点武断，但想法是足够大，
// 以至于在实际使用中没有人会遇到，但同时足够小，
// 以至于 Regexp 树的递归不会超过 1GB 的 Go 栈限制。
// 单个递归框架的最大栈量可能
// 接近 1kB，所以这可能会被提高，但似乎不太可能
// 人们有正则表达式嵌套得这么深。
// 我们在谷歌的 C++ 代码库上进行了测试，发现
// 只有一个深度 > 100 的用例；它的深度为 128。
// 使用深度 1000 应该有足够的余地。
// 作为优化，我们甚至不费力计算高度
// 直到我们分配了至少 maxHeight 个 Regexp 结构。
const maxHeight = 1000

// maxSize 是编译的正则表达式中 Insts 的最大大小。
// 它也被武断地选择，但想法是足够大
// 以允许重要的正则表达式，同时足够小，
// 编译形式不会占用太多内存。
// 128 MB 足以容纳 330 万个 Inst 结构，大致
// 对应于 3.3 MB 的正则表达式。
const (
	maxSize  = 128 << 20 / instSize
	instSize = 5 * 8 // byte、2 个 uint32、slice 是 5 个 64 位字
)

// maxRunes 是正则表达式树中允许的最大 rune 数
// 计算所有节点中的 runes。
// 忽略字符类，p.numRunes 总是小于正则表达式的长度。
// 字符类可以使其大得多：每个 \pL 添加 1292 个 runes。
// 128 MB 足以容纳 32M 个 runes，这超过 26k 个 \pL 实例。
// 请注意，重复不会复制 rune 切片，
// 所以 \pL{1000} 只是一个 rune 切片，而不是 1000 个。
// 我们可以保留我们看过的字符类的缓存，
// 使我们看到的所有 \pL 使用同一个 rune 列表，
// 但这并不能完全消除问题：
// 考虑类似 [\pL01234][\pL01235][\pL01236]...[\pL^&*()]。
// 而且因为 Rune 切片在 Regexp 中直接暴露，
// 没有机会改变表示形式以允许
// 不同字符类之间的部分共享。
// 所以限制是我们能做的最好的。
const (
	maxRunes = 128 << 20 / runeSize
	runeSize = 4 // rune 是 int32
)

type parser struct {
	flags       Flags     // parse mode flags
	stack       []*Regexp // stack of parsed expressions
	free        *Regexp
	numCap      int // number of capturing groups seen
	wholeRegexp string
	tmpClass    []rune            // temporary char class work space
	numRegexp   int               // number of regexps allocated
	numRunes    int               // number of runes in char classes
	repeats     int64             // product of all repetitions seen
	height      map[*Regexp]int   // regexp height, for height limit check
	size        map[*Regexp]int64 // regexp compiled size, for size limit check
}

func (p *parser) newRegexp(op Op) *Regexp {
	re := p.free
	if re != nil {
		p.free = re.Sub0[0]
		*re = Regexp{}
	} else {
		re = new(Regexp)
		p.numRegexp++
	}
	re.Op = op
	return re
}

func (p *parser) reuse(re *Regexp) {
	if p.height != nil {
		delete(p.height, re)
	}
	re.Sub0[0] = p.free
	p.free = re
}

func (p *parser) checkLimits(re *Regexp) {
	if p.numRunes > maxRunes {
		panic(ErrLarge)
	}
	p.checkSize(re)
	p.checkHeight(re)
}

func (p *parser) checkSize(re *Regexp) {
	if p.size == nil {
		// We haven't started tracking size yet.
		// Do a relatively cheap check to see if we need to start.
		// Maintain the product of all the repeats we've seen
		// and don't track if the total number of regexp nodes
		// we've seen times the repeat product is in budget.
		if p.repeats == 0 {
			p.repeats = 1
		}
		if re.Op == OpRepeat {
			n := re.Max
			if n == -1 {
				n = re.Min
			}
			if n <= 0 {
				n = 1
			}
			if int64(n) > maxSize/p.repeats {
				p.repeats = maxSize
			} else {
				p.repeats *= int64(n)
			}
		}
		if int64(p.numRegexp) < maxSize/p.repeats {
			return
		}

		// We need to start tracking size.
		// Make the map and belatedly populate it
		// with info about everything we've constructed so far.
		p.size = make(map[*Regexp]int64)
		for _, re := range p.stack {
			p.checkSize(re)
		}
	}

	if p.calcSize(re, true) > maxSize {
		panic(ErrLarge)
	}
}

func (p *parser) calcSize(re *Regexp, force bool) int64 {
	if !force {
		if size, ok := p.size[re]; ok {
			return size
		}
	}

	var size int64
	switch re.Op {
	case OpLiteral:
		size = int64(len(re.Rune))
	case OpCapture, OpStar:
		// star can be 1+ or 2+; assume 2 pessimistically
		size = 2 + p.calcSize(re.Sub[0], false)
	case OpPlus, OpQuest:
		size = 1 + p.calcSize(re.Sub[0], false)
	case OpConcat:
		for _, sub := range re.Sub {
			size += p.calcSize(sub, false)
		}
	case OpAlternate:
		for _, sub := range re.Sub {
			size += p.calcSize(sub, false)
		}
		if len(re.Sub) > 1 {
			size += int64(len(re.Sub)) - 1
		}
	case OpRepeat:
		sub := p.calcSize(re.Sub[0], false)
		if re.Max == -1 {
			if re.Min == 0 {
				size = 2 + sub // x*
			} else {
				size = 1 + int64(re.Min)*sub // xxx+
			}
			break
		}
		// x{2,5} = xx(x(x(x)?)?)?
		size = int64(re.Max)*sub + int64(re.Max-re.Min)
	}

	size = max(1, size)
	p.size[re] = size
	return size
}

func (p *parser) checkHeight(re *Regexp) {
	if p.numRegexp < maxHeight {
		return
	}
	if p.height == nil {
		p.height = make(map[*Regexp]int)
		for _, re := range p.stack {
			p.checkHeight(re)
		}
	}
	if p.calcHeight(re, true) > maxHeight {
		panic(ErrNestingDepth)
	}
}

func (p *parser) calcHeight(re *Regexp, force bool) int {
	if !force {
		if h, ok := p.height[re]; ok {
			return h
		}
	}
	h := 1
	for _, sub := range re.Sub {
		hsub := p.calcHeight(sub, false)
		if h < 1+hsub {
			h = 1 + hsub
		}
	}
	p.height[re] = h
	return h
}

// 解析栈操作。

// push 将正则表达式 re 推送到解析栈上并返回该正则表达式。
func (p *parser) push(re *Regexp) *Regexp {
	p.numRunes += len(re.Rune)
	if re.Op == OpCharClass && len(re.Rune) == 2 && re.Rune[0] == re.Rune[1] {
		// Single rune.
		if p.maybeConcat(re.Rune[0], p.flags&^FoldCase) {
			return nil
		}
		re.Op = OpLiteral
		re.Rune = re.Rune[:1]
		re.Flags = p.flags &^ FoldCase
	} else if re.Op == OpCharClass && len(re.Rune) == 4 &&
		re.Rune[0] == re.Rune[1] && re.Rune[2] == re.Rune[3] &&
		unicode.SimpleFold(re.Rune[0]) == re.Rune[2] &&
		unicode.SimpleFold(re.Rune[2]) == re.Rune[0] ||
		re.Op == OpCharClass && len(re.Rune) == 2 &&
			re.Rune[0]+1 == re.Rune[1] &&
			unicode.SimpleFold(re.Rune[0]) == re.Rune[1] &&
			unicode.SimpleFold(re.Rune[1]) == re.Rune[0] {
		// Case-insensitive rune like [Aa] or [Δδ].
		if p.maybeConcat(re.Rune[0], p.flags|FoldCase) {
			return nil
		}

		// Rewrite as (case-insensitive) literal.
		re.Op = OpLiteral
		re.Rune = re.Rune[:1]
		re.Flags = p.flags | FoldCase
	} else {
		// Incremental concatenation.
		p.maybeConcat(-1, 0)
	}

	p.stack = append(p.stack, re)
	p.checkLimits(re)
	return re
}

// maybeConcat 实现了字面 runes 到字符串节点的增量连接。
// 解析器在每次 push 之前调用此方法，
// 因此只有栈的顶部片段
// 可能需要处理。由于这是在 push 之前调用的，
// 最顶层的字面将不再受 * 等操作符的约束
// （否则 ab* 会变成 (ab)*。）
// 如果 r >= 0 且有剩余节点，maybeConcat 使用它
// 用给定的标志推送 r。
// maybeConcat 报告 r 是否被推送。
func (p *parser) maybeConcat(r rune, flags Flags) bool {
	n := len(p.stack)
	if n < 2 {
		return false
	}

	re1 := p.stack[n-1]
	re2 := p.stack[n-2]
	if re1.Op != OpLiteral || re2.Op != OpLiteral || re1.Flags&FoldCase != re2.Flags&FoldCase {
		return false
	}

	// Push re1 into re2.
	re2.Rune = append(re2.Rune, re1.Rune...)

	// Reuse re1 if possible.
	if r >= 0 {
		re1.Rune = re1.Rune0[:1]
		re1.Rune[0] = r
		re1.Flags = flags
		return true
	}

	p.stack = p.stack[:n-1]
	p.reuse(re1)
	return false // did not push r
}

// literal 为栈上的 rune r 推送一个字面正则表达式。
func (p *parser) literal(r rune) {
	re := p.newRegexp(OpLiteral)
	re.Flags = p.flags
	if p.flags&FoldCase != 0 {
		r = minFoldRune(r)
	}
	re.Rune0[0] = r
	re.Rune = re.Rune0[:1]
	p.push(re)
}

// minFoldRune 返回与 r 等价的最小 rune 折叠。
func minFoldRune(r rune) rune {
	if r < minFold || r > maxFold {
		return r
	}
	m := r
	r0 := r
	for r = unicode.SimpleFold(r); r != r0; r = unicode.SimpleFold(r) {
		m = min(m, r)
	}
	return m
}

// op 将具有给定 op 的正则表达式推送到栈上
// 并返回该正则表达式。
func (p *parser) op(op Op) *Regexp {
	re := p.newRegexp(op)
	re.Flags = p.flags
	return p.push(re)
}

// repeat 根据 op、min、max 用自身重复替换栈的顶部元素。
// before 是从重复操作符开始的正则表达式后缀。
// after 是重复操作符之后的正则表达式后缀。
// repeat 返回更新的"after"和错误（如果有）。
func (p *parser) repeat(op Op, min, max int, before, after, lastRepeat string) (string, error) {
	flags := p.flags
	if p.flags&PerlX != 0 {
		if len(after) > 0 && after[0] == '?' {
			after = after[1:]
			flags ^= NonGreedy
		}
		if lastRepeat != "" {
			// In Perl it is not allowed to stack repetition operators:
			// a** is a syntax error, not a doubled star, and a++ means
			// something else entirely, which we don't support!
			return "", &Error{ErrInvalidRepeatOp, lastRepeat[:len(lastRepeat)-len(after)]}
		}
	}
	n := len(p.stack)
	if n == 0 {
		return "", &Error{ErrMissingRepeatArgument, before[:len(before)-len(after)]}
	}
	sub := p.stack[n-1]
	if sub.Op >= opPseudo {
		return "", &Error{ErrMissingRepeatArgument, before[:len(before)-len(after)]}
	}

	re := p.newRegexp(op)
	re.Min = min
	re.Max = max
	re.Flags = flags
	re.Sub = re.Sub0[:1]
	re.Sub[0] = sub
	p.stack[n-1] = re
	p.checkLimits(re)

	if op == OpRepeat && (min >= 2 || max >= 2) && !repeatIsValid(re, 1000) {
		return "", &Error{ErrInvalidRepeatSize, before[:len(before)-len(after)]}
	}

	return after, nil
}

// repeatIsValid 报告重复 re 是否有效。
// 有效意味着顶层重复和任何内部重复的组合
// 不超过最内层事物的 n 个副本。
// 此函数重新遍历正则表达式树，并为每个重复调用，
// 所以我们必须担心在解析器中引入二次行为。
// 我们通过仅在 min 或 max >= 2 时调用 repeatIsValid 来避免这种情况。
// 在这种情况下，任何 >= 2 嵌套的深度只能达到 9 而不会
// 触发解析错误，所以每个子树只能被重新遍历 9 次。
func repeatIsValid(re *Regexp, n int) bool {
	if re.Op == OpRepeat {
		m := re.Max
		if m == 0 {
			return true
		}
		if m < 0 {
			m = re.Min
		}
		if m > n {
			return false
		}
		if m > 0 {
			n /= m
		}
	}
	for _, sub := range re.Sub {
		if !repeatIsValid(sub, n) {
			return false
		}
	}
	return true
}

// concat 用其连接替换栈的顶部（最顶层 '|' 或 '(' 上方）。
func (p *parser) concat() *Regexp {
	p.maybeConcat(-1, 0)

	// Scan down to find pseudo-operator | or (.
	i := len(p.stack)
	for i > 0 && p.stack[i-1].Op < opPseudo {
		i--
	}
	subs := p.stack[i:]
	p.stack = p.stack[:i]

	// Empty concatenation is special case.
	if len(subs) == 0 {
		return p.push(p.newRegexp(OpEmptyMatch))
	}

	return p.push(p.collapse(subs, OpConcat))
}

// alternate 用其交替替换栈的顶部（最顶层 '(' 上方）。
func (p *parser) alternate() *Regexp {
	// Scan down to find pseudo-operator (.
	// There are no | above (.
	i := len(p.stack)
	for i > 0 && p.stack[i-1].Op < opPseudo {
		i--
	}
	subs := p.stack[i:]
	p.stack = p.stack[:i]

	// Make sure top class is clean.
	// All the others already are (see swapVerticalBar).
	if len(subs) > 0 {
		cleanAlt(subs[len(subs)-1])
	}

	// Empty alternate is special case
	// (shouldn't happen but easy to handle).
	if len(subs) == 0 {
		return p.push(p.newRegexp(OpNoMatch))
	}

	return p.push(p.collapse(subs, OpAlternate))
}

// cleanAlt 清理 re 以便最终包含在交替中。
func cleanAlt(re *Regexp) {
	switch re.Op {
	case OpCharClass:
		re.Rune = cleanClass(&re.Rune)
		if len(re.Rune) == 2 && re.Rune[0] == 0 && re.Rune[1] == unicode.MaxRune {
			re.Rune = nil
			re.Op = OpAnyChar
			return
		}
		if len(re.Rune) == 4 && re.Rune[0] == 0 && re.Rune[1] == '\n'-1 && re.Rune[2] == '\n'+1 && re.Rune[3] == unicode.MaxRune {
			re.Rune = nil
			re.Op = OpAnyCharNotNL
			return
		}
		if cap(re.Rune)-len(re.Rune) > 100 {
			// re.Rune will not grow any more.
			// Make a copy or inline to reclaim storage.
			re.Rune = append(re.Rune0[:0], re.Rune...)
		}
	}
}

// collapse 返回对 sub 应用 op 的结果。
// 如果 sub 包含 op 节点，它们都会被提升
// 使得永远不会有 concat 的 concat 或
// alternate 的 alternate。
func (p *parser) collapse(subs []*Regexp, op Op) *Regexp {
	if len(subs) == 1 {
		return subs[0]
	}
	re := p.newRegexp(op)
	re.Sub = re.Sub0[:0]
	for _, sub := range subs {
		if sub.Op == op {
			re.Sub = append(re.Sub, sub.Sub...)
			p.reuse(sub)
		} else {
			re.Sub = append(re.Sub, sub)
		}
	}
	if op == OpAlternate {
		re.Sub = p.factor(re.Sub)
		if len(re.Sub) == 1 {
			old := re
			re = re.Sub[0]
			p.reuse(old)
		}
	}
	return re
}

// factor 从交替列表 sub 中分解公共前缀。
// 它返回重用相同存储的替换列表，
// 并释放（传递给 p.reuse）任何移除的 *Regexps。
//
// 例如，
//
//	ABC|ABD|AEF|BCX|BCY
//
// 通过字面前缀提取简化为
//
//	A(B(C|D)|EF)|BC(X|Y)
//
// 这通过字符类引入简化为
//
//	A(B[CD]|EF)|BC[XY]
func (p *parser) factor(sub []*Regexp) []*Regexp {
	if len(sub) < 2 {
		return sub
	}

	// Round 1: Factor out common literal prefixes.
	var str []rune
	var strflags Flags
	start := 0
	out := sub[:0]
	for i := 0; i <= len(sub); i++ {
		// Invariant: the Regexps that were in sub[0:start] have been
		// used or marked for reuse, and the slice space has been reused
		// for out (len(out) <= start).
		//
		// Invariant: sub[start:i] consists of regexps that all begin
		// with str as modified by strflags.
		var istr []rune
		var iflags Flags
		if i < len(sub) {
			istr, iflags = p.leadingString(sub[i])
			if iflags == strflags {
				same := 0
				for same < len(str) && same < len(istr) && str[same] == istr[same] {
					same++
				}
				if same > 0 {
					// Matches at least one rune in current range.
					// Keep going around.
					str = str[:same]
					continue
				}
			}
		}

		// Found end of a run with common leading literal string:
		// sub[start:i] all begin with str[:len(str)], but sub[i]
		// does not even begin with str[0].
		//
		// Factor out common string and append factored expression to out.
		if i == start {
			// Nothing to do - run of length 0.
		} else if i == start+1 {
			// Just one: don't bother factoring.
			out = append(out, sub[start])
		} else {
			// Construct factored form: prefix(suffix1|suffix2|...)
			prefix := p.newRegexp(OpLiteral)
			prefix.Flags = strflags
			prefix.Rune = append(prefix.Rune[:0], str...)

			for j := start; j < i; j++ {
				sub[j] = p.removeLeadingString(sub[j], len(str))
				p.checkLimits(sub[j])
			}
			suffix := p.collapse(sub[start:i], OpAlternate) // recurse

			re := p.newRegexp(OpConcat)
			re.Sub = append(re.Sub[:0], prefix, suffix)
			out = append(out, re)
		}

		// Prepare for next iteration.
		start = i
		str = istr
		strflags = iflags
	}
	sub = out

	// Round 2: Factor out common simple prefixes,
	// just the first piece of each concatenation.
	// This will be good enough a lot of the time.
	//
	// Complex subexpressions (e.g. involving quantifiers)
	// are not safe to factor because that collapses their
	// distinct paths through the automaton, which affects
	// correctness in some cases.
	start = 0
	out = sub[:0]
	var first *Regexp
	for i := 0; i <= len(sub); i++ {
		// Invariant: the Regexps that were in sub[0:start] have been
		// used or marked for reuse, and the slice space has been reused
		// for out (len(out) <= start).
		//
		// Invariant: sub[start:i] consists of regexps that all begin with ifirst.
		var ifirst *Regexp
		if i < len(sub) {
			ifirst = p.leadingRegexp(sub[i])
			if first != nil && first.Equal(ifirst) &&
				// first must be a character class OR a fixed repeat of a character class.
				(isCharClass(first) || (first.Op == OpRepeat && first.Min == first.Max && isCharClass(first.Sub[0]))) {
				continue
			}
		}

		// Found end of a run with common leading regexp:
		// sub[start:i] all begin with first but sub[i] does not.
		//
		// Factor out common regexp and append factored expression to out.
		if i == start {
			// Nothing to do - run of length 0.
		} else if i == start+1 {
			// Just one: don't bother factoring.
			out = append(out, sub[start])
		} else {
			// Construct factored form: prefix(suffix1|suffix2|...)
			prefix := first
			for j := start; j < i; j++ {
				reuse := j != start // prefix came from sub[start]
				sub[j] = p.removeLeadingRegexp(sub[j], reuse)
				p.checkLimits(sub[j])
			}
			suffix := p.collapse(sub[start:i], OpAlternate) // recurse

			re := p.newRegexp(OpConcat)
			re.Sub = append(re.Sub[:0], prefix, suffix)
			out = append(out, re)
		}

		// Prepare for next iteration.
		start = i
		first = ifirst
	}
	sub = out

	// Round 3: Collapse runs of single literals into character classes.
	start = 0
	out = sub[:0]
	for i := 0; i <= len(sub); i++ {
		// Invariant: the Regexps that were in sub[0:start] have been
		// used or marked for reuse, and the slice space has been reused
		// for out (len(out) <= start).
		//
		// Invariant: sub[start:i] consists of regexps that are either
		// literal runes or character classes.
		if i < len(sub) && isCharClass(sub[i]) {
			continue
		}

		// sub[i] is not a char or char class;
		// emit char class for sub[start:i]...
		if i == start {
			// Nothing to do - run of length 0.
		} else if i == start+1 {
			out = append(out, sub[start])
		} else {
			// Make new char class.
			// Start with most complex regexp in sub[start].
			max := start
			for j := start + 1; j < i; j++ {
				if sub[max].Op < sub[j].Op || sub[max].Op == sub[j].Op && len(sub[max].Rune) < len(sub[j].Rune) {
					max = j
				}
			}
			sub[start], sub[max] = sub[max], sub[start]

			for j := start + 1; j < i; j++ {
				mergeCharClass(sub[start], sub[j])
				p.reuse(sub[j])
			}
			cleanAlt(sub[start])
			out = append(out, sub[start])
		}

		// ... and then emit sub[i].
		if i < len(sub) {
			out = append(out, sub[i])
		}
		start = i + 1
	}
	sub = out

	// Round 4: Collapse runs of empty matches into a single empty match.
	start = 0
	out = sub[:0]
	for i := range sub {
		if i+1 < len(sub) && sub[i].Op == OpEmptyMatch && sub[i+1].Op == OpEmptyMatch {
			continue
		}
		out = append(out, sub[i])
	}
	sub = out

	return sub
}

// leadingString 返回 re 开头的前导字面字符串。
// 该字符串引用 re 或其子元素中的存储。
func (p *parser) leadingString(re *Regexp) ([]rune, Flags) {
	if re.Op == OpConcat && len(re.Sub) > 0 {
		re = re.Sub[0]
	}
	if re.Op != OpLiteral {
		return nil, 0
	}
	return re.Rune, re.Flags & FoldCase
}

// removeLeadingString 从 re 的开头移除前 n 个前导 runes。
// 它返回 re 的替换。
func (p *parser) removeLeadingString(re *Regexp, n int) *Regexp {
	if re.Op == OpConcat && len(re.Sub) > 0 {
		// Removing a leading string in a concatenation
		// might simplify the concatenation.
		sub := re.Sub[0]
		sub = p.removeLeadingString(sub, n)
		re.Sub[0] = sub
		if sub.Op == OpEmptyMatch {
			p.reuse(sub)
			switch len(re.Sub) {
			case 0, 1:
				// Impossible but handle.
				re.Op = OpEmptyMatch
				re.Sub = nil
			case 2:
				old := re
				re = re.Sub[1]
				p.reuse(old)
			default:
				copy(re.Sub, re.Sub[1:])
				re.Sub = re.Sub[:len(re.Sub)-1]
			}
		}
		return re
	}

	if re.Op == OpLiteral {
		re.Rune = re.Rune[:copy(re.Rune, re.Rune[n:])]
		if len(re.Rune) == 0 {
			re.Op = OpEmptyMatch
		}
	}
	return re
}

// leadingRegexp 返回 re 开头的前导正则表达式。
// 该正则表达式引用 re 或其子元素中的存储。
func (p *parser) leadingRegexp(re *Regexp) *Regexp {
	if re.Op == OpEmptyMatch {
		return nil
	}
	if re.Op == OpConcat && len(re.Sub) > 0 {
		sub := re.Sub[0]
		if sub.Op == OpEmptyMatch {
			return nil
		}
		return sub
	}
	return re
}

// removeLeadingRegexp 移除 re 中的前导正则表达式。
// 它返回 re 的替换。
// 如果 reuse 为 true，它将移除的正则表达式（如果不再需要）传递给 p.reuse。
func (p *parser) removeLeadingRegexp(re *Regexp, reuse bool) *Regexp {
	if re.Op == OpConcat && len(re.Sub) > 0 {
		if reuse {
			p.reuse(re.Sub[0])
		}
		re.Sub = re.Sub[:copy(re.Sub, re.Sub[1:])]
		switch len(re.Sub) {
		case 0:
			re.Op = OpEmptyMatch
			re.Sub = nil
		case 1:
			old := re
			re = re.Sub[0]
			p.reuse(old)
		}
		return re
	}
	if reuse {
		p.reuse(re)
	}
	return p.newRegexp(OpEmptyMatch)
}

func literalRegexp(s string, flags Flags) *Regexp {
	re := &Regexp{Op: OpLiteral}
	re.Flags = flags
	re.Rune = re.Rune0[:0] // use local storage for small strings
	for _, c := range s {
		if len(re.Rune) >= cap(re.Rune) {
			// string is too long to fit in Rune0.  let Go handle it
			re.Rune = []rune(s)
			break
		}
		re.Rune = append(re.Rune, c)
	}
	return re
}

// 解析。

// Parse 解析由指定 Flags 控制的正则表达式字符串 s，
// 并返回正则表达式解析树。语法在
// 顶层注释中描述。
func Parse(s string, flags Flags) (*Regexp, error) {
	return parse(s, flags)
}

func parse(s string, flags Flags) (_ *Regexp, err error) {
	defer func() {
		switch r := recover(); r {
		default:
			panic(r)
		case nil:
			// ok
		case ErrLarge: // too big
			err = &Error{Code: ErrLarge, Expr: s}
		case ErrNestingDepth:
			err = &Error{Code: ErrNestingDepth, Expr: s}
		}
	}()

	if flags&Literal != 0 {
		// Trivial parser for literal string.
		if err := checkUTF8(s); err != nil {
			return nil, err
		}
		return literalRegexp(s, flags), nil
	}

	// Otherwise, must do real work.
	var (
		p          parser
		c          rune
		op         Op
		lastRepeat string
	)
	p.flags = flags
	p.wholeRegexp = s
	t := s
	for t != "" {
		repeat := ""
	BigSwitch:
		switch t[0] {
		default:
			if c, t, err = nextRune(t); err != nil {
				return nil, err
			}
			p.literal(c)

		case '(':
			if p.flags&PerlX != 0 && len(t) >= 2 && t[1] == '?' {
				// Flag changes and non-capturing groups.
				if t, err = p.parsePerlFlags(t); err != nil {
					return nil, err
				}
				break
			}
			p.numCap++
			p.op(opLeftParen).Cap = p.numCap
			t = t[1:]
		case '|':
			p.parseVerticalBar()
			t = t[1:]
		case ')':
			if err = p.parseRightParen(); err != nil {
				return nil, err
			}
			t = t[1:]
		case '^':
			if p.flags&OneLine != 0 {
				p.op(OpBeginText)
			} else {
				p.op(OpBeginLine)
			}
			t = t[1:]
		case '$':
			if p.flags&OneLine != 0 {
				p.op(OpEndText).Flags |= WasDollar
			} else {
				p.op(OpEndLine)
			}
			t = t[1:]
		case '.':
			if p.flags&DotNL != 0 {
				p.op(OpAnyChar)
			} else {
				p.op(OpAnyCharNotNL)
			}
			t = t[1:]
		case '[':
			if t, err = p.parseClass(t); err != nil {
				return nil, err
			}
		case '*', '+', '?':
			before := t
			switch t[0] {
			case '*':
				op = OpStar
			case '+':
				op = OpPlus
			case '?':
				op = OpQuest
			}
			after := t[1:]
			if after, err = p.repeat(op, 0, 0, before, after, lastRepeat); err != nil {
				return nil, err
			}
			repeat = before
			t = after
		case '{':
			op = OpRepeat
			before := t
			min, max, after, ok := p.parseRepeat(t)
			if !ok {
				// If the repeat cannot be parsed, { is a literal.
				p.literal('{')
				t = t[1:]
				break
			}
			if min < 0 || min > 1000 || max > 1000 || max >= 0 && min > max {
				// Numbers were too big, or max is present and min > max.
				return nil, &Error{ErrInvalidRepeatSize, before[:len(before)-len(after)]}
			}
			if after, err = p.repeat(op, min, max, before, after, lastRepeat); err != nil {
				return nil, err
			}
			repeat = before
			t = after
		case '\\':
			if p.flags&PerlX != 0 && len(t) >= 2 {
				switch t[1] {
				case 'A':
					p.op(OpBeginText)
					t = t[2:]
					break BigSwitch
				case 'b':
					p.op(OpWordBoundary)
					t = t[2:]
					break BigSwitch
				case 'B':
					p.op(OpNoWordBoundary)
					t = t[2:]
					break BigSwitch
				case 'C':
					// any byte; not supported
					return nil, &Error{ErrInvalidEscape, t[:2]}
				case 'Q':
					// \Q ... \E: the ... is always literals
					var lit string
					lit, t, _ = strings.Cut(t[2:], `\E`)
					for lit != "" {
						c, rest, err := nextRune(lit)
						if err != nil {
							return nil, err
						}
						p.literal(c)
						lit = rest
					}
					break BigSwitch
				case 'z':
					p.op(OpEndText)
					t = t[2:]
					break BigSwitch
				}
			}

			re := p.newRegexp(OpCharClass)
			re.Flags = p.flags

			// Look for Unicode character group like \p{Han}
			if len(t) >= 2 && (t[1] == 'p' || t[1] == 'P') {
				r, rest, err := p.parseUnicodeClass(t, re.Rune0[:0])
				if err != nil {
					return nil, err
				}
				if r != nil {
					re.Rune = r
					t = rest
					p.push(re)
					break BigSwitch
				}
			}

			// Perl character class escape.
			if r, rest := p.parsePerlClassEscape(t, re.Rune0[:0]); r != nil {
				re.Rune = r
				t = rest
				p.push(re)
				break BigSwitch
			}
			p.reuse(re)

			// Ordinary single-character escape.
			if c, t, err = p.parseEscape(t); err != nil {
				return nil, err
			}
			p.literal(c)
		}
		lastRepeat = repeat
	}

	p.concat()
	if p.swapVerticalBar() {
		// pop vertical bar
		p.stack = p.stack[:len(p.stack)-1]
	}
	p.alternate()

	n := len(p.stack)
	if n != 1 {
		return nil, &Error{ErrMissingParen, s}
	}
	return p.stack[0], nil
}

// parseRepeat 解析 {min}（max=min）或 {min,}（max=-1）或 {min,max}。
// 如果 s 不是那种形式，它返回 ok == false。
// 如果 s 形式正确但值太大，它返回 min == -1、ok == true。
func (p *parser) parseRepeat(s string) (min, max int, rest string, ok bool) {
	if s == "" || s[0] != '{' {
		return
	}
	s = s[1:]
	var ok1 bool
	if min, s, ok1 = p.parseInt(s); !ok1 {
		return
	}
	if s == "" {
		return
	}
	if s[0] != ',' {
		max = min
	} else {
		s = s[1:]
		if s == "" {
			return
		}
		if s[0] == '}' {
			max = -1
		} else if max, s, ok1 = p.parseInt(s); !ok1 {
			return
		} else if max < 0 {
			// parseInt found too big a number
			min = -1
		}
	}
	if s == "" || s[0] != '}' {
		return
	}
	rest = s[1:]
	ok = true
	return
}

// parsePerlFlags 解析 Perl 标志设置或非捕获组或两者，
// 如 (?i) 或 (?: 或 (?i:。它从 s 中移除前缀并更新解析状态。
// 调用者必须确保 s 以 "(?" 开头。
func (p *parser) parsePerlFlags(s string) (rest string, err error) {
	t := s

	// 检查命名捕获，最初在 Python 的正则表达式库中引入。
	// 如往常一样，有三种略微不同的语法：
	//
	//   (?P<name>expr)   原始形式，由 Python 引入
	//   (?<name>expr)    .NET 变体，由 Perl 5.10 采用
	//   (?'name'expr)    另一个 .NET 变体，由 Perl 5.10 采用
	//
	// Perl 5.10 让步并也实现了 Python 版本，
	// 但他们声称后两者是首选形式。
	// PCRE 及其基础的语言（特别是 PHP 和 Ruby）
	// 也支持全部三种。EcmaScript 4 仅使用 Python 形式。
	//
	// 在开源世界（通过代码搜索）和
	// Google 源树中，(?P<expr>name) 和 (?<expr>name) 是
	// 命名捕获的主导形式，两者都支持。
	startsWithP := len(t) > 4 && t[2] == 'P' && t[3] == '<'
	startsWithName := len(t) > 3 && t[2] == '<'

	if startsWithP || startsWithName {
		// position of expr start
		exprStartPos := 4
		if startsWithName {
			exprStartPos = 3
		}

		// Pull out name.
		end := strings.IndexRune(t, '>')
		if end < 0 {
			if err = checkUTF8(t); err != nil {
				return "", err
			}
			return "", &Error{ErrInvalidNamedCapture, s}
		}

		capture := t[:end+1]        // "(?P<name>" or "(?<name>"
		name := t[exprStartPos:end] // "name"
		if err = checkUTF8(name); err != nil {
			return "", err
		}
		if !isValidCaptureName(name) {
			return "", &Error{ErrInvalidNamedCapture, capture}
		}

		// Like ordinary capture, but named.
		p.numCap++
		re := p.op(opLeftParen)
		re.Cap = p.numCap
		re.Name = name
		return t[end+1:], nil
	}

	// Non-capturing group. Might also twiddle Perl flags.
	var c rune
	t = t[2:] // skip (?
	flags := p.flags
	sign := +1
	sawFlag := false
Loop:
	for t != "" {
		if c, t, err = nextRune(t); err != nil {
			return "", err
		}
		switch c {
		default:
			break Loop

		// Flags.
		case 'i':
			flags |= FoldCase
			sawFlag = true
		case 'm':
			flags &^= OneLine
			sawFlag = true
		case 's':
			flags |= DotNL
			sawFlag = true
		case 'U':
			flags |= NonGreedy
			sawFlag = true

		// Switch to negation.
		case '-':
			if sign < 0 {
				break Loop
			}
			sign = -1
			// Invert flags so that | above turn into &^ and vice versa.
			// We'll invert flags again before using it below.
			flags = ^flags
			sawFlag = false

		// End of flags, starting group or not.
		case ':', ')':
			if sign < 0 {
				if !sawFlag {
					break Loop
				}
				flags = ^flags
			}
			if c == ':' {
				// Open new group
				p.op(opLeftParen)
			}
			p.flags = flags
			return t, nil
		}
	}

	return "", &Error{ErrInvalidPerlOp, s[:len(s)-len(t)]}
}

// isValidCaptureName 报告 name
// 是否是有效的捕获名称：[A-Za-z0-9_]+。
// PCRE 将名称限制为 32 字节。
// Python 拒绝以数字开头的名称。
// 我们不强制任何这些。
func isValidCaptureName(name string) bool {
	if name == "" {
		return false
	}
	for _, c := range name {
		if c != '_' && !isalnum(c) {
			return false
		}
	}
	return true
}

// parseInt 解析一个十进制整数。
func (p *parser) parseInt(s string) (n int, rest string, ok bool) {
	if s == "" || s[0] < '0' || '9' < s[0] {
		return
	}
	// Disallow leading zeros.
	if len(s) >= 2 && s[0] == '0' && '0' <= s[1] && s[1] <= '9' {
		return
	}
	t := s
	for s != "" && '0' <= s[0] && s[0] <= '9' {
		s = s[1:]
	}
	rest = s
	ok = true
	// Have digits, compute value.
	t = t[:len(t)-len(s)]
	for i := 0; i < len(t); i++ {
		// Avoid overflow.
		if n >= 1e8 {
			n = -1
			break
		}
		n = n*10 + int(t[i]) - '0'
	}
	return
}

// 这可以表示为字符类吗?
// 单 rune 字面字符串、char 类、.、和 .|\n。
func isCharClass(re *Regexp) bool {
	return re.Op == OpLiteral && len(re.Rune) == 1 ||
		re.Op == OpCharClass ||
		re.Op == OpAnyCharNotNL ||
		re.Op == OpAnyChar
}

// re 匹配 r 吗？
func matchRune(re *Regexp, r rune) bool {
	switch re.Op {
	case OpLiteral:
		return len(re.Rune) == 1 && re.Rune[0] == r
	case OpCharClass:
		for i := 0; i < len(re.Rune); i += 2 {
			if re.Rune[i] <= r && r <= re.Rune[i+1] {
				return true
			}
		}
		return false
	case OpAnyCharNotNL:
		return r != '\n'
	case OpAnyChar:
		return true
	}
	return false
}

// parseVerticalBar 处理输入中的 |。
func (p *parser) parseVerticalBar() {
	p.concat()

	// The concatenation we just parsed is on top of the stack.
	// If it sits above an opVerticalBar, swap it below
	// (things below an opVerticalBar become an alternation).
	// Otherwise, push a new vertical bar.
	if !p.swapVerticalBar() {
		p.op(opVerticalBar)
	}
}

// mergeCharClass 创建 dst = dst|src。
// 调用者必须确保 dst.Op >= src.Op，
// 以减少复制量。
func mergeCharClass(dst, src *Regexp) {
	switch dst.Op {
	case OpAnyChar:
		// src doesn't add anything.
	case OpAnyCharNotNL:
		// src might add \n
		if matchRune(src, '\n') {
			dst.Op = OpAnyChar
		}
	case OpCharClass:
		// src is simpler, so either literal or char class
		if src.Op == OpLiteral {
			dst.Rune = appendLiteral(dst.Rune, src.Rune[0], src.Flags)
		} else {
			dst.Rune = appendClass(dst.Rune, src.Rune)
		}
	case OpLiteral:
		// both literal
		if src.Rune[0] == dst.Rune[0] && src.Flags == dst.Flags {
			break
		}
		dst.Op = OpCharClass
		dst.Rune = appendLiteral(dst.Rune[:0], dst.Rune[0], dst.Flags)
		dst.Rune = appendLiteral(dst.Rune, src.Rune[0], src.Flags)
	}
}

// 如果栈的顶部是一个元素后跟 opVerticalBar
// swapVerticalBar 交换这两个并返回 true。
// 否则返回 false。
func (p *parser) swapVerticalBar() bool {
	// If above and below vertical bar are literal or char class,
	// can merge into a single char class.
	n := len(p.stack)
	if n >= 3 && p.stack[n-2].Op == opVerticalBar && isCharClass(p.stack[n-1]) && isCharClass(p.stack[n-3]) {
		re1 := p.stack[n-1]
		re3 := p.stack[n-3]
		// Make re3 the more complex of the two.
		if re1.Op > re3.Op {
			re1, re3 = re3, re1
			p.stack[n-3] = re3
		}
		mergeCharClass(re3, re1)
		p.reuse(re1)
		p.stack = p.stack[:n-1]
		return true
	}

	if n >= 2 {
		re1 := p.stack[n-1]
		re2 := p.stack[n-2]
		if re2.Op == opVerticalBar {
			if n >= 3 {
				// Now out of reach.
				// Clean opportunistically.
				cleanAlt(p.stack[n-3])
			}
			p.stack[n-2] = re1
			p.stack[n-1] = re2
			return true
		}
	}
	return false
}

// parseRightParen 处理输入中的 )。
func (p *parser) parseRightParen() error {
	p.concat()
	if p.swapVerticalBar() {
		// pop vertical bar
		p.stack = p.stack[:len(p.stack)-1]
	}
	p.alternate()

	n := len(p.stack)
	if n < 2 {
		return &Error{ErrUnexpectedParen, p.wholeRegexp}
	}
	re1 := p.stack[n-1]
	re2 := p.stack[n-2]
	p.stack = p.stack[:n-2]
	if re2.Op != opLeftParen {
		return &Error{ErrUnexpectedParen, p.wholeRegexp}
	}
	// Restore flags at time of paren.
	p.flags = re2.Flags
	if re2.Cap == 0 {
		// Just for grouping.
		p.push(re1)
	} else {
		re2.Op = OpCapture
		re2.Sub = re2.Sub0[:1]
		re2.Sub[0] = re1
		p.push(re2)
	}
	return nil
}

// parseEscape 解析 s 开头的转义序列
// 并返回该 rune。
func (p *parser) parseEscape(s string) (r rune, rest string, err error) {
	t := s[1:]
	if t == "" {
		return 0, "", &Error{ErrTrailingBackslash, ""}
	}
	c, t, err := nextRune(t)
	if err != nil {
		return 0, "", err
	}

Switch:
	switch c {
	default:
		if c < utf8.RuneSelf && !isalnum(c) {
			// Escaped non-word characters are always themselves.
			// PCRE is not quite so rigorous: it accepts things like
			// \q, but we don't. We once rejected \_, but too many
			// programs and people insist on using it, so allow \_.
			return c, t, nil
		}

	// Octal escapes.
	case '1', '2', '3', '4', '5', '6', '7':
		// Single non-zero digit is a backreference; not supported
		if t == "" || t[0] < '0' || t[0] > '7' {
			break
		}
		fallthrough
	case '0':
		// Consume up to three octal digits; already have one.
		r = c - '0'
		for i := 1; i < 3; i++ {
			if t == "" || t[0] < '0' || t[0] > '7' {
				break
			}
			r = r*8 + rune(t[0]) - '0'
			t = t[1:]
		}
		return r, t, nil

	// Hexadecimal escapes.
	case 'x':
		if t == "" {
			break
		}
		if c, t, err = nextRune(t); err != nil {
			return 0, "", err
		}
		if c == '{' {
			// Any number of digits in braces.
			// Perl accepts any text at all; it ignores all text
			// after the first non-hex digit. We require only hex digits,
			// and at least one.
			nhex := 0
			r = 0
			for {
				if t == "" {
					break Switch
				}
				if c, t, err = nextRune(t); err != nil {
					return 0, "", err
				}
				if c == '}' {
					break
				}
				v := unhex(c)
				if v < 0 {
					break Switch
				}
				r = r*16 + v
				if r > unicode.MaxRune {
					break Switch
				}
				nhex++
			}
			if nhex == 0 {
				break Switch
			}
			return r, t, nil
		}

		// Easy case: two hex digits.
		x := unhex(c)
		if c, t, err = nextRune(t); err != nil {
			return 0, "", err
		}
		y := unhex(c)
		if x < 0 || y < 0 {
			break
		}
		return x*16 + y, t, nil

	// C 转义。没有 'b' 情况，以避免误解
	// Perl 单词边界 \b 作为 C 退格 \b
	// 在 POSIX 模式下。在 Perl 中，/\b/ 表示单词边界
	// 但 /[\b]/ 表示退格。我们不支持那个。
	// 如果你想要退格，嵌入字面退格
	// 字符或使用 \x08。
	case 'a':
		return '\a', t, err
	case 'f':
		return '\f', t, err
	case 'n':
		return '\n', t, err
	case 'r':
		return '\r', t, err
	case 't':
		return '\t', t, err
	case 'v':
		return '\v', t, err
	}
	return 0, "", &Error{ErrInvalidEscape, s[:len(s)-len(t)]}
}

// parseClassChar 解析 s 开头的字符类字符
// 并返回它。
func (p *parser) parseClassChar(s, wholeClass string) (r rune, rest string, err error) {
	if s == "" {
		return 0, "", &Error{Code: ErrMissingBracket, Expr: wholeClass}
	}

	// Allow regular escape sequences even though
	// many need not be escaped in this context.
	if s[0] == '\\' {
		return p.parseEscape(s)
	}

	return nextRune(s)
}

type charGroup struct {
	sign  int
	class []rune
}

//go:generate perl make_perl_groups.pl perl_groups.go

// parsePerlClassEscape 解析从 s 开头开始的 \d 之类的前导 Perl 字符类转义。
// 如果存在，它将字符追加到 r
// 并返回新的切片 r 和字符串的其余部分。
func (p *parser) parsePerlClassEscape(s string, r []rune) (out []rune, rest string) {
	if p.flags&PerlX == 0 || len(s) < 2 || s[0] != '\\' {
		return
	}
	g := perlGroup[s[0:2]]
	if g.sign == 0 {
		return
	}
	return p.appendGroup(r, g), s[2:]
}

// parseNamedClass 解析从 s 开头开始的 [:alnum:] 之类的前导 POSIX 命名字符类。
// 如果存在，它将字符追加到 r
// 并返回新的切片 r 和字符串的其余部分。
func (p *parser) parseNamedClass(s string, r []rune) (out []rune, rest string, err error) {
	if len(s) < 2 || s[0] != '[' || s[1] != ':' {
		return
	}

	i := strings.Index(s[2:], ":]")
	if i < 0 {
		return
	}
	i += 2
	name, s := s[0:i+2], s[i+2:]
	g := posixGroup[name]
	if g.sign == 0 {
		return nil, "", &Error{ErrInvalidCharRange, name}
	}
	return p.appendGroup(r, g), s, nil
}

func (p *parser) appendGroup(r []rune, g charGroup) []rune {
	if p.flags&FoldCase == 0 {
		if g.sign < 0 {
			r = appendNegatedClass(r, g.class)
		} else {
			r = appendClass(r, g.class)
		}
	} else {
		tmp := p.tmpClass[:0]
		tmp = appendFoldedClass(tmp, g.class)
		p.tmpClass = tmp
		tmp = cleanClass(&p.tmpClass)
		if g.sign < 0 {
			r = appendNegatedClass(r, tmp)
		} else {
			r = appendClass(r, tmp)
		}
	}
	return r
}

var anyTable = &unicode.RangeTable{
	R16: []unicode.Range16{{Lo: 0, Hi: 1<<16 - 1, Stride: 1}},
	R32: []unicode.Range32{{Lo: 1 << 16, Hi: unicode.MaxRune, Stride: 1}},
}

var asciiTable = &unicode.RangeTable{
	R16: []unicode.Range16{{Lo: 0, Hi: 0x7F, Stride: 1}},
}

var asciiFoldTable = &unicode.RangeTable{
	R16: []unicode.Range16{
		{Lo: 0, Hi: 0x7F, Stride: 1},
		{Lo: 0x017F, Hi: 0x017F, Stride: 1}, // Old English long s (ſ), folds to S/s.
		{Lo: 0x212A, Hi: 0x212A, Stride: 1}, // Kelvin K, folds to K/k.
	},
}

// categoryAliases 是 unicode.CategoryAliases 的惰性构造副本
// 但关键字通过 canonicalName 传递，以支持不精确匹配。
var categoryAliases struct {
	once sync.Once
	m    map[string]string
}

// initCategoryAliases 通过规范化 unicode.CategoryAliases 来初始化 categoryAliases。
func initCategoryAliases() {
	categoryAliases.m = make(map[string]string)
	for name, actual := range unicode.CategoryAliases {
		categoryAliases.m[canonicalName(name)] = actual
	}
}

// canonicalName 返回 name 的规范查找字符串。
// 规范名称有一个前导大写字母，然后是小写字母，
// 并且它省略了所有下划线、空格和连字符。
// （我们可以使用全小写，但这样大多数包 unicode
// 映射关键字已经是规范的。）
func canonicalName(name string) string {
	var b []byte
	first := true
	for i := range len(name) {
		c := name[i]
		switch {
		case c == '_' || c == '-' || c == ' ':
			c = ' '
		case first:
			if 'a' <= c && c <= 'z' {
				c -= 'a' - 'A'
			}
			first = false
		default:
			if 'A' <= c && c <= 'Z' {
				c += 'a' - 'A'
			}
		}
		if b == nil {
			if c == name[i] && c != ' ' {
				// No changes so far, avoid allocating b.
				continue
			}
			b = make([]byte, i, len(name))
			copy(b, name[:i])
		}
		if c == ' ' {
			continue
		}
		b = append(b, c)
	}
	if b == nil {
		return name
	}
	return string(b)
}

// unicodeTable 返回由 name 标识的 unicode.RangeTable
// 和额外的折叠等价码点表。
// 如果 sign < 0，结果应该被反演。
func unicodeTable(name string) (tab, fold *unicode.RangeTable, sign int) {
	name = canonicalName(name)

	// Special cases: Any, Assigned, and ASCII.
	// Also LC is the only non-canonical Categories key, so handle it here.
	switch name {
	case "Any":
		return anyTable, anyTable, +1
	case "Assigned":
		return unicode.Cn, unicode.Cn, -1 // invert Cn (unassigned)
	case "Ascii":
		return asciiTable, asciiFoldTable, +1
	case "Lc":
		return unicode.Categories["LC"], unicode.FoldCategory["LC"], +1
	}
	if t := unicode.Categories[name]; t != nil {
		return t, unicode.FoldCategory[name], +1
	}
	if t := unicode.Scripts[name]; t != nil {
		return t, unicode.FoldScript[name], +1
	}

	// unicode.CategoryAliases 在其名称中大量使用下划线
	// （它们由 Unicode 以这种方式定义），但我们想忽略
	// 下划线进行匹配，所以使用规范名称制作我们自己的映射。
	categoryAliases.once.Do(initCategoryAliases)
	if actual := categoryAliases.m[name]; actual != "" {
		t := unicode.Categories[actual]
		return t, unicode.FoldCategory[actual], +1
	}
	return nil, nil, 0
}

// parseUnicodeClass 解析从 s 开头开始的 \p{Han} 之类的前导 Unicode 字符类。
// 如果存在，它将字符追加到 r
// 并返回新的切片 r 和字符串的其余部分。
func (p *parser) parseUnicodeClass(s string, r []rune) (out []rune, rest string, err error) {
	if p.flags&UnicodeGroups == 0 || len(s) < 2 || s[0] != '\\' || s[1] != 'p' && s[1] != 'P' {
		return
	}

	// Committed to parse or return error.
	sign := +1
	if s[1] == 'P' {
		sign = -1
	}
	t := s[2:]
	c, t, err := nextRune(t)
	if err != nil {
		return
	}
	var seq, name string
	if c != '{' {
		// Single-letter name.
		seq = s[:len(s)-len(t)]
		name = seq[2:]
	} else {
		// Name is in braces.
		end := strings.IndexRune(s, '}')
		if end < 0 {
			if err = checkUTF8(s); err != nil {
				return
			}
			return nil, "", &Error{ErrInvalidCharRange, s}
		}
		seq, t = s[:end+1], s[end+1:]
		name = s[3:end]
		if err = checkUTF8(name); err != nil {
			return
		}
	}

	// 组也可以有前导取反。\p{^Han} == \P{Han}、\P{^Han} == \p{Han}。
	if name != "" && name[0] == '^' {
		sign = -sign
		name = name[1:]
	}

	tab, fold, tsign := unicodeTable(name)
	if tab == nil {
		return nil, "", &Error{ErrInvalidCharRange, seq}
	}
	if tsign < 0 {
		sign = -sign
	}

	if p.flags&FoldCase == 0 || fold == nil {
		if sign > 0 {
			r = appendTable(r, tab)
		} else {
			r = appendNegatedTable(r, tab)
		}
	} else {
		// 在临时缓冲区中合并和清理 tab 和 fold。
		// 这对于负数情况是必要的，对于
		// 正数情况只是整理。
		tmp := p.tmpClass[:0]
		tmp = appendTable(tmp, tab)
		tmp = appendTable(tmp, fold)
		p.tmpClass = tmp
		tmp = cleanClass(&p.tmpClass)
		if sign > 0 {
			r = appendClass(r, tmp)
		} else {
			r = appendNegatedClass(r, tmp)
		}
	}
	return r, t, nil
}

// parseClass 解析 s 开头的字符类
// 并将其推送到解析栈上。
func (p *parser) parseClass(s string) (rest string, err error) {
	t := s[1:] // chop [
	re := p.newRegexp(OpCharClass)
	re.Flags = p.flags
	re.Rune = re.Rune0[:0]

	sign := +1
	if t != "" && t[0] == '^' {
		sign = -1
		t = t[1:]

		// If character class does not match \n, add it here,
		// so that negation later will do the right thing.
		if p.flags&ClassNL == 0 {
			re.Rune = append(re.Rune, '\n', '\n')
		}
	}

	class := re.Rune
	first := true // ] and - are okay as first char in class
	for t == "" || t[0] != ']' || first {
		// POSIX: - is only okay unescaped as first or last in class.
		// Perl: - is okay anywhere.
		if t != "" && t[0] == '-' && p.flags&PerlX == 0 && !first && (len(t) == 1 || t[1] != ']') {
			_, size := utf8.DecodeRuneInString(t[1:])
			return "", &Error{Code: ErrInvalidCharRange, Expr: t[:1+size]}
		}
		first = false

		// Look for POSIX [:alnum:] etc.
		if len(t) > 2 && t[0] == '[' && t[1] == ':' {
			nclass, nt, err := p.parseNamedClass(t, class)
			if err != nil {
				return "", err
			}
			if nclass != nil {
				class, t = nclass, nt
				continue
			}
		}

		// Look for Unicode character group like \p{Han}.
		nclass, nt, err := p.parseUnicodeClass(t, class)
		if err != nil {
			return "", err
		}
		if nclass != nil {
			class, t = nclass, nt
			continue
		}

		// Look for Perl character class symbols (extension).
		if nclass, nt := p.parsePerlClassEscape(t, class); nclass != nil {
			class, t = nclass, nt
			continue
		}

		// Single character or simple range.
		rng := t
		var lo, hi rune
		if lo, t, err = p.parseClassChar(t, s); err != nil {
			return "", err
		}
		hi = lo
		// [a-] means (a|-) so check for final ].
		if len(t) >= 2 && t[0] == '-' && t[1] != ']' {
			t = t[1:]
			if hi, t, err = p.parseClassChar(t, s); err != nil {
				return "", err
			}
			if hi < lo {
				rng = rng[:len(rng)-len(t)]
				return "", &Error{Code: ErrInvalidCharRange, Expr: rng}
			}
		}
		if p.flags&FoldCase == 0 {
			class = appendRange(class, lo, hi)
		} else {
			class = appendFoldedRange(class, lo, hi)
		}
	}
	t = t[1:] // chop ]

	// Use &re.Rune instead of &class to avoid allocation.
	re.Rune = class
	class = cleanClass(&re.Rune)
	if sign < 0 {
		class = negateClass(class)
	}
	re.Rune = class
	p.push(re)
	return t, nil
}

// cleanClass 对范围（r 的元素对）进行排序，
// 合并它们，并消除重复项。
func cleanClass(rp *[]rune) []rune {

	// 按 lo 递增、hi 递减排序以打破平局。
	sort.Sort(ranges{rp})

	r := *rp
	if len(r) < 2 {
		return r
	}

	// 合并相邻、重叠。
	w := 2 // 写索引
	for i := 2; i < len(r); i += 2 {
		lo, hi := r[i], r[i+1]
		if lo <= r[w-1]+1 {
			// 与前一个范围合并
			if hi > r[w-1] {
				r[w-1] = hi
			}
			continue
		}
		// 新的不相交范围
		r[w] = lo
		r[w+1] = hi
		w += 2
	}

	return r[:w]
}

// inCharClass 报告 r 是否在该类中。
// 它假设该类已被 cleanClass 清理。
func inCharClass(r rune, class []rune) bool {
	_, ok := sort.Find(len(class)/2, func(i int) int {
		lo, hi := class[2*i], class[2*i+1]
		if r > hi {
			return +1
		}
		if r < lo {
			return -1
		}
		return 0
	})
	return ok
}

// appendLiteral 返回将字面 x 追加到类 r 的结果。
func appendLiteral(r []rune, x rune, flags Flags) []rune {
	if flags&FoldCase != 0 {
		return appendFoldedRange(r, x, x)
	}
	return appendRange(r, x, x)
}

// appendRange 返回将范围 lo-hi 追加到类 r 的结果。
func appendRange(r []rune, lo, hi rune) []rune {
	// 展开最后一个范围或倒数第二个范围（如果它重叠或相邻）。
	// 检查两个范围有助于附加大小写折叠的
	// 字母表，以便一个范围可以扩展 A-Z，另一个
	// 扩展 a-z。
	n := len(r)
	for i := 2; i <= 4; i += 2 { // 两次，使用 i=2、i=4
		if n >= i {
			rlo, rhi := r[n-i], r[n-i+1]
			if lo <= rhi+1 && rlo <= hi+1 {
				if lo < rlo {
					r[n-i] = lo
				}
				if hi > rhi {
					r[n-i+1] = hi
				}
				return r
			}
		}
	}

	return append(r, lo, hi)
}

const (
	// 涉及折叠的最小和最大 runes。
	// 在测试期间检查。
	minFold = 0x0041
	maxFold = 0x1e943
)

// appendFoldedRange 返回追加范围 lo-hi 的结果
// 和其大小写折叠等价的 runes 到类 r。
func appendFoldedRange(r []rune, lo, hi rune) []rune {
	// 优化。
	if lo <= minFold && hi >= maxFold {
		// 范围是完整的：折叠无法添加更多。
		return appendRange(r, lo, hi)
	}
	if hi < minFold || lo > maxFold {
		// 范围在折叠可能性之外。
		return appendRange(r, lo, hi)
	}
	if lo < minFold {
		// [lo, minFold-1] 不需要折叠。
		r = appendRange(r, lo, minFold-1)
		lo = minFold
	}
	if hi > maxFold {
		// [maxFold+1, hi] 不需要折叠。
		r = appendRange(r, maxFold+1, hi)
		hi = maxFold
	}

	// 蛮力。依靠 appendRange 即时合并范围。
	for c := lo; c <= hi; c++ {
		r = appendRange(r, c, c)
		f := unicode.SimpleFold(c)
		for f != c {
			r = appendRange(r, f, f)
			f = unicode.SimpleFold(f)
		}
	}
	return r
}

// appendClass 返回将类 x 追加到类 r 的结果。
// 它假设 x 是干净的。
func appendClass(r []rune, x []rune) []rune {
	for i := 0; i < len(x); i += 2 {
		r = appendRange(r, x[i], x[i+1])
	}
	return r
}

// appendFoldedClass 返回将类 x 的大小写折叠追加到类 r 的结果。
func appendFoldedClass(r []rune, x []rune) []rune {
	for i := 0; i < len(x); i += 2 {
		r = appendFoldedRange(r, x[i], x[i+1])
	}
	return r
}

// appendNegatedClass 返回将类 x 的取反追加到类 r 的结果。
// 它假设 x 是干净的。
func appendNegatedClass(r []rune, x []rune) []rune {
	nextLo := '\u0000'
	for i := 0; i < len(x); i += 2 {
		lo, hi := x[i], x[i+1]
		if nextLo <= lo-1 {
			r = appendRange(r, nextLo, lo-1)
		}
		nextLo = hi + 1
	}
	if nextLo <= unicode.MaxRune {
		r = appendRange(r, nextLo, unicode.MaxRune)
	}
	return r
}

// appendTable 返回将 x 追加到类 r 的结果。
func appendTable(r []rune, x *unicode.RangeTable) []rune {
	for _, xr := range x.R16 {
		lo, hi, stride := rune(xr.Lo), rune(xr.Hi), rune(xr.Stride)
		if stride == 1 {
			r = appendRange(r, lo, hi)
			continue
		}
		for c := lo; c <= hi; c += stride {
			r = appendRange(r, c, c)
		}
	}
	for _, xr := range x.R32 {
		lo, hi, stride := rune(xr.Lo), rune(xr.Hi), rune(xr.Stride)
		if stride == 1 {
			r = appendRange(r, lo, hi)
			continue
		}
		for c := lo; c <= hi; c += stride {
			r = appendRange(r, c, c)
		}
	}
	return r
}

// appendNegatedTable 返回将 x 的取反追加到类 r 的结果。
func appendNegatedTable(r []rune, x *unicode.RangeTable) []rune {
	nextLo := '\u0000' // 要添加的下一个类的 lo 端
	for _, xr := range x.R16 {
		lo, hi, stride := rune(xr.Lo), rune(xr.Hi), rune(xr.Stride)
		if stride == 1 {
			if nextLo <= lo-1 {
				r = appendRange(r, nextLo, lo-1)
			}
			nextLo = hi + 1
			continue
		}
		for c := lo; c <= hi; c += stride {
			if nextLo <= c-1 {
				r = appendRange(r, nextLo, c-1)
			}
			nextLo = c + 1
		}
	}
	for _, xr := range x.R32 {
		lo, hi, stride := rune(xr.Lo), rune(xr.Hi), rune(xr.Stride)
		if stride == 1 {
			if nextLo <= lo-1 {
				r = appendRange(r, nextLo, lo-1)
			}
			nextLo = hi + 1
			continue
		}
		for c := lo; c <= hi; c += stride {
			if nextLo <= c-1 {
				r = appendRange(r, nextLo, c-1)
			}
			nextLo = c + 1
		}
	}
	if nextLo <= unicode.MaxRune {
		r = appendRange(r, nextLo, unicode.MaxRune)
	}
	return r
}

// negateClass 覆盖 r 并返回 r 的取反。
// 它假设类 r 已经是干净的。
func negateClass(r []rune) []rune {
	nextLo := '\u0000' // 要添加的下一个类的 lo 端
	w := 0             // write index
	for i := 0; i < len(r); i += 2 {
		lo, hi := r[i], r[i+1]
		if nextLo <= lo-1 {
			r[w] = nextLo
			r[w+1] = lo - 1
			w += 2
		}
		nextLo = hi + 1
	}
	r = r[:w]
	if nextLo <= unicode.MaxRune {
		// 取反可能有一个范围——这个——比原始类更多，
		// 所以使用 append。
		r = append(r, nextLo, unicode.MaxRune)
	}
	return r
}

// ranges 在 []rune 上实现 sort.Interface。
// 接收器类型定义的选择很奇怪
// 但避免了分配，因为我们已经有
// 一个 *[]rune。
type ranges struct {
	p *[]rune
}

func (ra ranges) Less(i, j int) bool {
	p := *ra.p
	i *= 2
	j *= 2
	return p[i] < p[j] || p[i] == p[j] && p[i+1] > p[j+1]
}

func (ra ranges) Len() int {
	return len(*ra.p) / 2
}

func (ra ranges) Swap(i, j int) {
	p := *ra.p
	i *= 2
	j *= 2
	p[i], p[i+1], p[j], p[j+1] = p[j], p[j+1], p[i], p[i+1]
}

func checkUTF8(s string) error {
	for s != "" {
		rune, size := utf8.DecodeRuneInString(s)
		if rune == utf8.RuneError && size == 1 {
			return &Error{Code: ErrInvalidUTF8, Expr: s}
		}
		s = s[size:]
	}
	return nil
}

func nextRune(s string) (c rune, t string, err error) {
	c, size := utf8.DecodeRuneInString(s)
	if c == utf8.RuneError && size == 1 {
		return 0, "", &Error{Code: ErrInvalidUTF8, Expr: s}
	}
	return c, s[size:], nil
}

func isalnum(c rune) bool {
	return '0' <= c && c <= '9' || 'A' <= c && c <= 'Z' || 'a' <= c && c <= 'z'
}

func unhex(c rune) rune {
	if '0' <= c && c <= '9' {
		return c - '0'
	}
	if 'a' <= c && c <= 'f' {
		return c - 'a' + 10
	}
	if 'A' <= c && c <= 'F' {
		return c - 'A' + 10
	}
	return -1
}
