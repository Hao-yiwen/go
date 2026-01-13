// 版权所有 2014 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package regexp

import (
	"regexp/syntax"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// "单遍"正则表达式执行。
// 某些正则表达式可以通过分析确定它们永远不需要回溯：
// 它们保证在字符串上一遍运行完成，而无需保存所有通常的 NFA 状态。
// 检测这些情况并更快地执行它们。

// onePassProg 是一个编译后的单遍正则表达式程序。
// 除了使用 onePassInst 之外，它与 syntax.Prog 相同。
type onePassProg struct {
	Inst   []onePassInst
	Start  int // 起始指令的索引
	NumCap int // re 中 InstCapture 指令的数量
}

// onePassInst 是单遍正则表达式程序中的单个指令。
// 除了新的 'Next' 字段之外，它与 syntax.Inst 相同。
type onePassInst struct {
	syntax.Inst
	Next []uint32
}

// onePassPrefix 返回正则表达式的所有匹配必须以之开头的字面字符串。
// 如果前缀是整个匹配，则 complete 为 true。
// pc 是字符串中最后一个 rune 指令的索引。
// onePassPrefix 跳过强制的 EmptyBeginText。
func onePassPrefix(p *syntax.Prog) (prefix string, complete bool, pc uint32) {
	i := &p.Inst[p.Start]
	if i.Op != syntax.InstEmptyWidth || (syntax.EmptyOp(i.Arg))&syntax.EmptyBeginText == 0 {
		return "", i.Op == syntax.InstMatch, uint32(p.Start)
	}
	pc = i.Out
	i = &p.Inst[pc]
	for i.Op == syntax.InstNop {
		pc = i.Out
		i = &p.Inst[pc]
	}
	// 如果前缀为空，避免分配缓冲区。
	if iop(i) != syntax.InstRune || len(i.Rune) != 1 {
		return "", i.Op == syntax.InstMatch, uint32(p.Start)
	}

	// 有前缀；收集字符。
	var buf strings.Builder
	for iop(i) == syntax.InstRune && len(i.Rune) == 1 && syntax.Flags(i.Arg)&syntax.FoldCase == 0 && i.Rune[0] != utf8.RuneError {
		buf.WriteRune(i.Rune[0])
		pc, i = i.Out, &p.Inst[i.Out]
	}
	if i.Op == syntax.InstEmptyWidth &&
		syntax.EmptyOp(i.Arg)&syntax.EmptyEndText != 0 &&
		p.Inst[i.Out].Op == syntax.InstMatch {
		complete = true
	}
	return buf.String(), complete, pc
}

// onePassNext 根据输入字符选择程序的下一个可操作状态。
// 它只应在 i.Op == InstAlt 或 InstAltMatch 时从单遍机器调用。
// 其中一个替代可能最终在没有输入的情况下到达行尾。如果指令是 InstAltMatch，
// 到 InstMatch 的路径在 i.Out 中，普通节点在 i.Next 中。
func onePassNext(i *onePassInst, r rune) uint32 {
	next := i.MatchRunePos(r)
	if next >= 0 {
		return i.Next[next]
	}
	if i.Op == syntax.InstAltMatch {
		return i.Out
	}
	return 0
}

func iop(i *syntax.Inst) syntax.InstOp {
	op := i.Op
	switch op {
	case syntax.InstRune1, syntax.InstRuneAny, syntax.InstRuneAnyNotNL:
		op = syntax.InstRune
	}
	return op
}

// 稀疏数组实现用作 queueOnePass。
type queueOnePass struct {
	sparse          []uint32
	dense           []uint32
	size, nextIndex uint32
}

func (q *queueOnePass) empty() bool {
	return q.nextIndex >= q.size
}

func (q *queueOnePass) next() (n uint32) {
	n = q.dense[q.nextIndex]
	q.nextIndex++
	return
}

func (q *queueOnePass) clear() {
	q.size = 0
	q.nextIndex = 0
}

func (q *queueOnePass) contains(u uint32) bool {
	if u >= uint32(len(q.sparse)) {
		return false
	}
	return q.sparse[u] < q.size && q.dense[q.sparse[u]] == u
}

func (q *queueOnePass) insert(u uint32) {
	if !q.contains(u) {
		q.insertNew(u)
	}
}

func (q *queueOnePass) insertNew(u uint32) {
	if u >= uint32(len(q.sparse)) {
		return
	}
	q.sparse[u] = q.size
	q.dense[q.size] = u
	q.size++
}

func newQueue(size int) (q *queueOnePass) {
	return &queueOnePass{
		sparse: make([]uint32, size),
		dense:  make([]uint32, size),
	}
}

// mergeRuneSets 合并两个不相交的 rune 集，并返回合并结果
// 和一个 NextIp 数组。其思想是，如果一个 rune 匹配索引 i 处的
// OnePassRunes，则 NextIp[i/2] 是目标。如果输入集相交，
// 则返回一个空的 rune 集和一个包含单元素 mergeFailed 的 NextIp 数组。
// 代码假设两个输入都包含有序且不相交的 rune 对。
const mergeFailed = uint32(0xffffffff)

var (
	noRune = []rune{}
	noNext = []uint32{mergeFailed}
)

func mergeRuneSets(leftRunes, rightRunes *[]rune, leftPC, rightPC uint32) ([]rune, []uint32) {
	leftLen := len(*leftRunes)
	rightLen := len(*rightRunes)
	if leftLen&0x1 != 0 || rightLen&0x1 != 0 {
		panic("mergeRuneSets odd length []rune")
	}
	var (
		lx, rx int
	)
	merged := make([]rune, 0)
	next := make([]uint32, 0)
	ok := true
	defer func() {
		if !ok {
			merged = nil
			next = nil
		}
	}()

	ix := -1
	extend := func(newLow *int, newArray *[]rune, pc uint32) bool {
		if ix > 0 && (*newArray)[*newLow] <= merged[ix] {
			return false
		}
		merged = append(merged, (*newArray)[*newLow], (*newArray)[*newLow+1])
		*newLow += 2
		ix += 2
		next = append(next, pc)
		return true
	}

	for lx < leftLen || rx < rightLen {
		switch {
		case rx >= rightLen:
			ok = extend(&lx, leftRunes, leftPC)
		case lx >= leftLen:
			ok = extend(&rx, rightRunes, rightPC)
		case (*rightRunes)[rx] < (*leftRunes)[lx]:
			ok = extend(&rx, rightRunes, rightPC)
		default:
			ok = extend(&lx, leftRunes, leftPC)
		}
		if !ok {
			return noRune, noNext
		}
	}
	return merged, next
}

// cleanupOnePass 释放工作内存，并恢复某些快捷指令。
func cleanupOnePass(prog *onePassProg, original *syntax.Prog) {
	for ix, instOriginal := range original.Inst {
		switch instOriginal.Op {
		case syntax.InstAlt, syntax.InstAltMatch, syntax.InstRune:
		case syntax.InstCapture, syntax.InstEmptyWidth, syntax.InstNop, syntax.InstMatch, syntax.InstFail:
			prog.Inst[ix].Next = nil
		case syntax.InstRune1, syntax.InstRuneAny, syntax.InstRuneAnyNotNL:
			prog.Inst[ix].Next = nil
			prog.Inst[ix] = onePassInst{Inst: instOriginal}
		}
	}
}

// onePassCopy 创建原始 Prog 的副本，因为我们将修改它。
func onePassCopy(prog *syntax.Prog) *onePassProg {
	p := &onePassProg{
		Start:  prog.Start,
		NumCap: prog.NumCap,
		Inst:   make([]onePassInst, len(prog.Inst)),
	}
	for i, inst := range prog.Inst {
		p.Inst[i] = onePassInst{Inst: inst}
	}

	// 重写一个或多个常见的 Prog 结构，使一些原本非单遍的 Prog 可以变成单遍。
	// A:BD（例如）表示在 ip A 处的 InstAlt，指向 ips B 和 C。
	// A:BC + B:DA => A:BC + B:CD
	// A:BC + B:DC => A:DC + B:DC
	for pc := range p.Inst {
		switch p.Inst[pc].Op {
		default:
			continue
		case syntax.InstAlt, syntax.InstAltMatch:
			// A:Bx + B:Ay
			p_A_Other := &p.Inst[pc].Out
			p_A_Alt := &p.Inst[pc].Arg
			// 确保目标是另一个 Alt
			instAlt := p.Inst[*p_A_Alt]
			if !(instAlt.Op == syntax.InstAlt || instAlt.Op == syntax.InstAltMatch) {
				p_A_Alt, p_A_Other = p_A_Other, p_A_Alt
				instAlt = p.Inst[*p_A_Alt]
				if !(instAlt.Op == syntax.InstAlt || instAlt.Op == syntax.InstAltMatch) {
					continue
				}
			}
			instOther := p.Inst[*p_A_Other]
			// 分析两条腿都指向 Alt 的情况留待以后
			if instOther.Op == syntax.InstAlt || instOther.Op == syntax.InstAltMatch {
				// 太复杂了
				continue
			}
			// 简单的空转换循环
			// A:BC + B:DA => A:BC + B:DC
			p_B_Alt := &p.Inst[*p_A_Alt].Out
			p_B_Other := &p.Inst[*p_A_Alt].Arg
			patch := false
			if instAlt.Out == uint32(pc) {
				patch = true
			} else if instAlt.Arg == uint32(pc) {
				patch = true
				p_B_Alt, p_B_Other = p_B_Other, p_B_Alt
			}
			if patch {
				*p_B_Alt = *p_A_Other
			}

			// 空转换到公共目标
			// A:BC + B:DC => A:DC + B:DC
			if *p_A_Other == *p_B_Alt {
				*p_A_Alt = *p_B_Other
			}
		}
	}
	return p
}

var anyRuneNotNL = []rune{0, '\n' - 1, '\n' + 1, unicode.MaxRune}
var anyRune = []rune{0, unicode.MaxRune}

// makeOnePass 如果可能的话，创建一个单遍 Prog。如果在任何 alt 处，
// 匹配引擎总是能确定走哪个分支，则是可能的。如果它变成单遍 Prog，
// 该例程可能会修改 p。如果不可能成为单遍 Prog，则返回 nil。
// makeOnePass 是与 Prog 大小成递归的。
func makeOnePass(p *onePassProg) *onePassProg {
	// 如果机器非常长，检查是否可以使用单遍就不值得花时间了。
	if len(p.Inst) >= 1000 {
		return nil
	}

	var (
		instQueue    = newQueue(len(p.Inst))
		visitQueue   = newQueue(len(p.Inst))
		check        func(uint32, []bool) bool
		onePassRunes = make([][]rune, len(p.Inst))
	)

	// 检查从 Alt 指令出发的路径是否无歧义，并将新程序重建为单遍程序
	check = func(pc uint32, m []bool) (ok bool) {
		ok = true
		inst := &p.Inst[pc]
		if visitQueue.contains(pc) {
			return
		}
		visitQueue.insert(pc)
		switch inst.Op {
		case syntax.InstAlt, syntax.InstAltMatch:
			ok = check(inst.Out, m) && check(inst.Arg, m)
			// 检查到 InstMatch 的无输入路径
			matchOut := m[inst.Out]
			matchArg := m[inst.Arg]
			if matchOut && matchArg {
				ok = false
				break
			}
			// 空匹配进入 inst.Out
			if matchArg {
				inst.Out, inst.Arg = inst.Arg, inst.Out
				matchOut, matchArg = matchArg, matchOut
			}
			if matchOut {
				m[pc] = true
				inst.Op = syntax.InstAltMatch
			}

			// 从 alt 的两条腿构建调度操作符。
			onePassRunes[pc], inst.Next = mergeRuneSets(
				&onePassRunes[inst.Out], &onePassRunes[inst.Arg], inst.Out, inst.Arg)
			if len(inst.Next) > 0 && inst.Next[0] == mergeFailed {
				ok = false
				break
			}
		case syntax.InstCapture, syntax.InstNop:
			ok = check(inst.Out, m)
			m[pc] = m[inst.Out]
			// 通过这些空操作传回匹配的 rune。
			onePassRunes[pc] = append([]rune{}, onePassRunes[inst.Out]...)
			inst.Next = make([]uint32, len(onePassRunes[pc])/2+1)
			for i := range inst.Next {
				inst.Next[i] = inst.Out
			}
		case syntax.InstEmptyWidth:
			ok = check(inst.Out, m)
			m[pc] = m[inst.Out]
			onePassRunes[pc] = append([]rune{}, onePassRunes[inst.Out]...)
			inst.Next = make([]uint32, len(onePassRunes[pc])/2+1)
			for i := range inst.Next {
				inst.Next[i] = inst.Out
			}
		case syntax.InstMatch, syntax.InstFail:
			m[pc] = inst.Op == syntax.InstMatch
		case syntax.InstRune:
			m[pc] = false
			if len(inst.Next) > 0 {
				break
			}
			instQueue.insert(inst.Out)
			if len(inst.Rune) == 0 {
				onePassRunes[pc] = []rune{}
				inst.Next = []uint32{inst.Out}
				break
			}
			runes := make([]rune, 0)
			if len(inst.Rune) == 1 && syntax.Flags(inst.Arg)&syntax.FoldCase != 0 {
				r0 := inst.Rune[0]
				runes = append(runes, r0, r0)
				for r1 := unicode.SimpleFold(r0); r1 != r0; r1 = unicode.SimpleFold(r1) {
					runes = append(runes, r1, r1)
				}
				slices.Sort(runes)
			} else {
				runes = append(runes, inst.Rune...)
			}
			onePassRunes[pc] = runes
			inst.Next = make([]uint32, len(onePassRunes[pc])/2+1)
			for i := range inst.Next {
				inst.Next[i] = inst.Out
			}
			inst.Op = syntax.InstRune
		case syntax.InstRune1:
			m[pc] = false
			if len(inst.Next) > 0 {
				break
			}
			instQueue.insert(inst.Out)
			runes := []rune{}
			// 展开大小写折叠的 rune
			if syntax.Flags(inst.Arg)&syntax.FoldCase != 0 {
				r0 := inst.Rune[0]
				runes = append(runes, r0, r0)
				for r1 := unicode.SimpleFold(r0); r1 != r0; r1 = unicode.SimpleFold(r1) {
					runes = append(runes, r1, r1)
				}
				slices.Sort(runes)
			} else {
				runes = append(runes, inst.Rune[0], inst.Rune[0])
			}
			onePassRunes[pc] = runes
			inst.Next = make([]uint32, len(onePassRunes[pc])/2+1)
			for i := range inst.Next {
				inst.Next[i] = inst.Out
			}
			inst.Op = syntax.InstRune
		case syntax.InstRuneAny:
			m[pc] = false
			if len(inst.Next) > 0 {
				break
			}
			instQueue.insert(inst.Out)
			onePassRunes[pc] = append([]rune{}, anyRune...)
			inst.Next = []uint32{inst.Out}
		case syntax.InstRuneAnyNotNL:
			m[pc] = false
			if len(inst.Next) > 0 {
				break
			}
			instQueue.insert(inst.Out)
			onePassRunes[pc] = append([]rune{}, anyRuneNotNL...)
			inst.Next = make([]uint32, len(onePassRunes[pc])/2+1)
			for i := range inst.Next {
				inst.Next[i] = inst.Out
			}
		}
		return
	}

	instQueue.clear()
	instQueue.insert(uint32(p.Start))
	m := make([]bool, len(p.Inst))
	for !instQueue.empty() {
		visitQueue.clear()
		pc := instQueue.next()
		if !check(pc, m) {
			p = nil
			break
		}
	}
	if p != nil {
		for i := range p.Inst {
			p.Inst[i].Rune = onePassRunes[i]
		}
	}
	return p
}

// compileOnePass 如果原始 Prog 可以被重新描述为单遍正则表达式程序，
// 则返回适合单遍执行的新 *syntax.Prog，否则返回 nil。
// 对于单遍程序，必须满足的基本条件是：在任何 InstAlt 处，
// 必须没有关于走哪个分支的歧义。
func compileOnePass(prog *syntax.Prog) (p *onePassProg) {
	if prog.Start == 0 {
		return nil
	}
	// 单遍正则表达式是锚定的
	if prog.Inst[prog.Start].Op != syntax.InstEmptyWidth ||
		syntax.EmptyOp(prog.Inst[prog.Start].Arg)&syntax.EmptyBeginText != syntax.EmptyBeginText {
		return nil
	}
	hasAlt := false
	for _, inst := range prog.Inst {
		if inst.Op == syntax.InstAlt || inst.Op == syntax.InstAltMatch {
			hasAlt = true
			break
		}
	}
	// 如果我们有替代，每个导向 InstMatch 的指令必须是 EmptyEndText。
	// 此外，任何空文本上的匹配必须是 $。
	for _, inst := range prog.Inst {
		opOut := prog.Inst[inst.Out].Op
		switch inst.Op {
		default:
			if opOut == syntax.InstMatch && hasAlt {
				return nil
			}
		case syntax.InstAlt, syntax.InstAltMatch:
			if opOut == syntax.InstMatch || prog.Inst[inst.Arg].Op == syntax.InstMatch {
				return nil
			}
		case syntax.InstEmptyWidth:
			if opOut == syntax.InstMatch {
				if syntax.EmptyOp(inst.Arg)&syntax.EmptyEndText == syntax.EmptyEndText {
					continue
				}
				return nil
			}
		}
	}
	// 创建原始 Prog 的略微优化的副本，
	// 清理一些阻止有效单遍程序的 Prog 惯用法
	p = onePassCopy(prog)

	// 在 InstAlt 上检查歧义，如果可能则构建单遍 Prog
	p = makeOnePass(p)

	if p != nil {
		cleanupOnePass(p, prog)
	}
	return p
}
