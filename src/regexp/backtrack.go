// 版权所有 2015 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// backtrack 是一个用于小型正则表达式和文本的带子匹配跟踪的正则表达式搜索。
// 它分配一个 (输入长度) * (程序长度) 位的位向量，
// 以确保永远不会多次探索同一个 (字符位置, 指令) 状态。
// 这将搜索限制为在与测试长度成线性的时间内运行。
//
// 当不能使用 onepass 时，backtrack 是小型正则表达式上
// NFA 代码的快速替代品。

package regexp

import (
	"regexp/syntax"
	"sync"
)

// job 是回溯器作业栈上的一个条目。它保存指令程序计数器和输入中的位置。
type job struct {
	pc  uint32
	arg bool
	pos int
}

const (
	visitedBits        = 32
	maxBacktrackProg   = 500        // len(prog.Inst) <= max
	maxBacktrackVector = 256 * 1024 // 位向量大小 <= max（位）
)

// bitState 保存回溯器的状态。
type bitState struct {
	end      int
	cap      []int
	matchcap []int
	jobs     []job
	visited  []uint32

	inputs inputs
}

var bitStatePool sync.Pool

func newBitState() *bitState {
	b, ok := bitStatePool.Get().(*bitState)
	if !ok {
		b = new(bitState)
	}
	return b
}

func freeBitState(b *bitState) {
	b.inputs.clear()
	bitStatePool.Put(b)
}

// maxBitStateLen 返回使用 prog 通过回溯器搜索的字符串的最大长度。
func maxBitStateLen(prog *syntax.Prog) int {
	if !shouldBacktrack(prog) {
		return 0
	}
	return maxBacktrackVector / len(prog.Inst)
}

// shouldBacktrack 报告程序是否太长而无法运行回溯器。
func shouldBacktrack(prog *syntax.Prog) bool {
	return len(prog.Inst) <= maxBacktrackProg
}

// reset 重置回溯器的状态。
// end 是输入中的结束位置。
// ncap 是捕获的数量。
func (b *bitState) reset(prog *syntax.Prog, end int, ncap int) {
	b.end = end

	if cap(b.jobs) == 0 {
		b.jobs = make([]job, 0, 256)
	} else {
		b.jobs = b.jobs[:0]
	}

	visitedSize := (len(prog.Inst)*(end+1) + visitedBits - 1) / visitedBits
	if cap(b.visited) < visitedSize {
		b.visited = make([]uint32, visitedSize, maxBacktrackVector/visitedBits)
	} else {
		b.visited = b.visited[:visitedSize]
		clear(b.visited) // 设置为 0
	}

	if cap(b.cap) < ncap {
		b.cap = make([]int, ncap)
	} else {
		b.cap = b.cap[:ncap]
	}
	for i := range b.cap {
		b.cap[i] = -1
	}

	if cap(b.matchcap) < ncap {
		b.matchcap = make([]int, ncap)
	} else {
		b.matchcap = b.matchcap[:ncap]
	}
	for i := range b.matchcap {
		b.matchcap[i] = -1
	}
}

// shouldVisit 报告 (pc, pos) 的组合是否尚未被访问过。
func (b *bitState) shouldVisit(pc uint32, pos int) bool {
	n := uint(int(pc)*(b.end+1) + pos)
	if b.visited[n/visitedBits]&(1<<(n&(visitedBits-1))) != 0 {
		return false
	}
	b.visited[n/visitedBits] |= 1 << (n & (visitedBits - 1))
	return true
}

// push 如果应该被访问，则将 (pc, pos, arg) 压入作业栈。
func (b *bitState) push(re *Regexp, pc uint32, pos int, arg bool) {
	// 仅当 arg 为 false 时检查 shouldVisit。
	// 当 arg 为 true 时，我们正在继续之前的访问。
	if re.prog.Inst[pc].Op != syntax.InstFail && (arg || b.shouldVisit(pc, pos)) {
		b.jobs = append(b.jobs, job{pc: pc, arg: arg, pos: pos})
	}
}

// tryBacktrack 从 pos 开始运行回溯搜索。
func (re *Regexp) tryBacktrack(b *bitState, i input, pc uint32, pos int) bool {
	longest := re.longest

	b.push(re, pc, pos, false)
	for len(b.jobs) > 0 {
		l := len(b.jobs) - 1
		// 从栈中弹出作业。
		pc := b.jobs[l].pc
		pos := b.jobs[l].pos
		arg := b.jobs[l].arg
		b.jobs = b.jobs[:l]

		// 优化：与其压入和弹出，
		// 将要执行 Push 并继续循环的代码
		// 只需更新 ip、p 和 arg 并跳转到 CheckAndLoop。
		// 我们必须执行 Push 本应执行的 ShouldVisit 检查，
		// 但我们避免了栈操作。
		goto Skip
	CheckAndLoop:
		if !b.shouldVisit(pc, pos) {
			continue
		}
	Skip:

		inst := &re.prog.Inst[pc]

		switch inst.Op {
		default:
			panic("bad inst")
		case syntax.InstFail:
			panic("unexpected InstFail")
		case syntax.InstAlt:
			// 不能只是
			//   b.push(inst.Out, pos, false)
			//   b.push(inst.Arg, pos, false)
			// 如果在处理 inst.Out 期间，我们通过另一条路径遇到
			// inst.Arg，我们希望在那时处理它。
			// 在这里推入它会阻止那样做。相反，重新推入
			// arg==true 的 inst 作为稍后推出 inst.Arg 的提醒。
			if arg {
				// 完成 inst.Out；尝试 inst.Arg。
				arg = false
				pc = inst.Arg
				goto CheckAndLoop
			} else {
				b.push(re, pc, pos, true)
				pc = inst.Out
				goto CheckAndLoop
			}

		case syntax.InstAltMatch:
			// 一个操作码消耗 rune；另一个导向匹配。
			switch re.prog.Inst[inst.Out].Op {
			case syntax.InstRune, syntax.InstRune1, syntax.InstRuneAny, syntax.InstRuneAnyNotNL:
				// inst.Arg 是匹配。
				b.push(re, inst.Arg, pos, false)
				pc = inst.Arg
				pos = b.end
				goto CheckAndLoop
			}
			// inst.Out 是匹配 - 非贪婪
			b.push(re, inst.Out, b.end, false)
			pc = inst.Out
			goto CheckAndLoop

		case syntax.InstRune:
			r, width := i.step(pos)
			if !inst.MatchRune(r) {
				continue
			}
			pos += width
			pc = inst.Out
			goto CheckAndLoop

		case syntax.InstRune1:
			r, width := i.step(pos)
			if r != inst.Rune[0] {
				continue
			}
			pos += width
			pc = inst.Out
			goto CheckAndLoop

		case syntax.InstRuneAnyNotNL:
			r, width := i.step(pos)
			if r == '\n' || r == endOfText {
				continue
			}
			pos += width
			pc = inst.Out
			goto CheckAndLoop

		case syntax.InstRuneAny:
			r, width := i.step(pos)
			if r == endOfText {
				continue
			}
			pos += width
			pc = inst.Out
			goto CheckAndLoop

		case syntax.InstCapture:
			if arg {
				// 完成 inst.Out；恢复旧值。
				b.cap[inst.Arg] = pos
				continue
			} else {
				if inst.Arg < uint32(len(b.cap)) {
					// 将 pos 捕获到寄存器，但保存旧值。
					b.push(re, pc, b.cap[inst.Arg], true) // 完成后回来。
					b.cap[inst.Arg] = pos
				}
				pc = inst.Out
				goto CheckAndLoop
			}

		case syntax.InstEmptyWidth:
			flag := i.context(pos)
			if !flag.match(syntax.EmptyOp(inst.Arg)) {
				continue
			}
			pc = inst.Out
			goto CheckAndLoop

		case syntax.InstNop:
			pc = inst.Out
			goto CheckAndLoop

		case syntax.InstMatch:
			// 我们找到了一个匹配。如果调用者不关心
			// 匹配在哪里，就没必要继续了。
			if len(b.cap) == 0 {
				return true
			}

			// 记录到目前为止的最佳匹配。
			// 只需要检查结束点，因为整个调用只考虑一个起始位置。
			if len(b.cap) > 1 {
				b.cap[1] = pos
			}
			if old := b.matchcap[1]; old == -1 || (longest && pos > 0 && pos > old) {
				copy(b.matchcap, b.cap)
			}

			// 如果寻找第一个匹配，我们完成了。
			if !longest {
				return true
			}

			// 如果我们使用了整个文本，不可能有更长的匹配。
			if pos == b.end {
				return true
			}

			// 否则，继续希望找到更长的匹配。
			continue
		}
	}

	return longest && len(b.matchcap) > 1 && b.matchcap[1] >= 0
}

// backtrack 在从 pos 开始的输入上运行 prog 的回溯搜索。
func (re *Regexp) backtrack(ib []byte, is string, pos int, ncap int, dstCap []int) []int {
	startCond := re.cond
	if startCond == ^syntax.EmptyOp(0) { // 不可能
		return nil
	}
	if startCond&syntax.EmptyBeginText != 0 && pos != 0 {
		// 锚定匹配，已经超过文本开头。
		return nil
	}

	b := newBitState()
	i, end := b.inputs.init(nil, ib, is)
	b.reset(re.prog, end, ncap)

	// 锚定搜索必须从输入开头开始
	if startCond&syntax.EmptyBeginText != 0 {
		if len(b.cap) > 0 {
			b.cap[0] = pos
		}
		if !re.tryBacktrack(b, i, uint32(re.prog.Start), pos) {
			freeBitState(b)
			return nil
		}
	} else {

		// 非锚定搜索，从每个可能的文本位置开始。
		// 注意我们必须尝试文本末尾的空字符串，
		// 所以循环条件是 pos <= end，而不是 pos < end。
		// 这看起来像是文本大小的平方，
		// 但我们没有在 TrySearch 调用之间清除 visited，
		// 所以没有重复的工作，最终仍然是线性的。
		width := -1
		for ; pos <= end && width != 0; pos += width {
			if len(re.prefix) > 0 {
				// 匹配需要字面前缀；快速搜索它。
				advance := i.index(re, pos)
				if advance < 0 {
					freeBitState(b)
					return nil
				}
				pos += advance
			}

			if len(b.cap) > 0 {
				b.cap[0] = pos
			}
			if re.tryBacktrack(b, i, uint32(re.prog.Start), pos) {
				// 匹配必须是最左的；完成。
				goto Match
			}
			_, width = i.step(pos)
		}
		freeBitState(b)
		return nil
	}

Match:
	dstCap = append(dstCap, b.matchcap...)
	freeBitState(b)
	return dstCap
}
