// 版权所有 2011 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package syntax

// Simplify 返回一个与 re 等价的正则表达式，但没有计数的重复
// 和各种其他简化，例如将 /(?:a+)+/ 重写为 /a+/。
// 生成的正则表达式将正确执行，但其字符串表示形式
// 将不会产生相同的解析树，因为捕获括号
// 可能已被重复或删除。例如，简化的形式
// 对于 /(x){1,2}/ 是 /(x)(x)?/，但两个括号都作为 $1 捕获。
// 返回的正则表达式可能与原始正则表达式共享结构或就是原始正则表达式。
func (re *Regexp) Simplify() *Regexp {
	if re == nil {
		return nil
	}
	switch re.Op {
	case OpCapture, OpConcat, OpAlternate:
		// 简化子元素，如果子元素改变则构建新的 Regexp。
		nre := re
		for i, sub := range re.Sub {
			nsub := sub.Simplify()
			if nre == re && nsub != sub {
				// 开始复制。
				nre = new(Regexp)
				*nre = *re
				nre.Rune = nil
				nre.Sub = append(nre.Sub0[:0], re.Sub[:i]...)
			}
			if nre != re {
				nre.Sub = append(nre.Sub, nsub)
			}
		}
		return nre

	case OpStar, OpPlus, OpQuest:
		sub := re.Sub[0].Simplify()
		return simplify1(re.Op, re.Flags, sub, re)

	case OpRepeat:
		// 特殊特殊情况：x{0} 匹配空字符串
		// 甚至不需要考虑 x。
		if re.Min == 0 && re.Max == 0 {
			return &Regexp{Op: OpEmptyMatch}
		}

		// 乐趣开始。
		sub := re.Sub[0].Simplify()

		// x{n,} 表示至少 n 个 x 的匹配。
		if re.Max == -1 {
			// 特殊情况：x{0,} 是 x*。
			if re.Min == 0 {
				return simplify1(OpStar, re.Flags, sub, nil)
			}

			// 特殊情况：x{1,} 是 x+。
			if re.Min == 1 {
				return simplify1(OpPlus, re.Flags, sub, nil)
			}

			// 一般情况：x{4,} 是 xxxx+。
			nre := &Regexp{Op: OpConcat}
			nre.Sub = nre.Sub0[:0]
			for i := 0; i < re.Min-1; i++ {
				nre.Sub = append(nre.Sub, sub)
			}
			nre.Sub = append(nre.Sub, simplify1(OpPlus, re.Flags, sub, nil))
			return nre
		}

		// 特殊情况 x{0} 在上面处理。

		// 特殊情况：x{1} 只是 x。
		if re.Min == 1 && re.Max == 1 {
			return sub
		}

		// 一般情况：x{n,m} 表示 n 个 x 和 m 个 x?
		// 如果我们嵌套最后 m 个副本，机器会做更少的工作，
		// 使得 x{2,5} = xx(x(x(x)?)?)?

		// 构建前导前缀：xx。
		var prefix *Regexp
		if re.Min > 0 {
			prefix = &Regexp{Op: OpConcat}
			prefix.Sub = prefix.Sub0[:0]
			for i := 0; i < re.Min; i++ {
				prefix.Sub = append(prefix.Sub, sub)
			}
		}

		// 构建和附加后缀：(x(x(x)?)?)?
		if re.Max > re.Min {
			suffix := simplify1(OpQuest, re.Flags, sub, nil)
			for i := re.Min + 1; i < re.Max; i++ {
				nre2 := &Regexp{Op: OpConcat}
				nre2.Sub = append(nre2.Sub0[:0], sub, suffix)
				suffix = simplify1(OpQuest, re.Flags, nre2, nil)
			}
			if prefix == nil {
				return suffix
			}
			prefix.Sub = append(prefix.Sub, suffix)
		}
		if prefix != nil {
			return prefix
		}

		// 某个退化情况，如 min > max 或 min < max < 0。
		// 作为不可能的匹配处理。
		return &Regexp{Op: OpNoMatch}
	}

	return re
}

// simplify1 为一元 OpStar、
// OpPlus 和 OpQuest 操作符实现 Simplify。它返回简单的正则表达式
// 等价于
//
//	Regexp{Op: op, Flags: flags, Sub: {sub}}
//
// 假设 sub 已经是简单的，且
// 没有首先分配该结构。如果要
// 返回的正则表达式等价于 re，simplify1
// 返回 re。
//
// simplify1 从 Simplify 中分解出来，因为
// 其他操作符的实现生成这些一元表达式。
// 让它们调用 simplify1 确保它们
// 生成的表达式是简单的。
func simplify1(op Op, flags Flags, sub, re *Regexp) *Regexp {
	// 特殊情况：尽可能多地重复空字符串
	// 但它仍然是空字符串。
	if sub.Op == OpEmptyMatch {
		return sub
	}
	// 如果标志匹配，操作符是等幂的。
	if op == sub.Op && flags&NonGreedy == sub.Flags&NonGreedy {
		return sub
	}
	if re != nil && re.Op == op && re.Flags&NonGreedy == flags&NonGreedy && sub == re.Sub[0] {
		return re
	}

	re = &Regexp{Op: op, Flags: flags}
	re.Sub = append(re.Sub0[:0], sub)
	return re
}
