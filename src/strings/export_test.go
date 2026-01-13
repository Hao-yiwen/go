// 版权所有 2011 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package strings

func (r *Replacer) Replacer() any {
	r.once.Do(r.buildOnce)
	return r.r
}

func (r *Replacer) PrintTrie() string {
	r.once.Do(r.buildOnce)
	gen := r.r.(*genericReplacer)
	return gen.printNode(&gen.root, 0)
}

func (r *genericReplacer) printNode(t *trieNode, depth int) (s string) {
	if t.priority > 0 {
		s += "+"
	} else {
		s += "-"
	}
	s += "\n"

	if t.prefix != "" {
		s += Repeat(".", depth) + t.prefix
		s += r.printNode(t.next, depth+len(t.prefix))
	} else if t.table != nil {
		for b, m := range r.mapping {
			if int(m) != r.tableSize && t.table[m] != nil {
				s += Repeat(".", depth) + string([]byte{byte(b)})
				s += r.printNode(t.table[m], depth+1)
			}
		}
	}
	return
}

func StringFind(pattern, text string) int {
	return makeStringFinder(pattern).next(text)
}

func DumpTables(pattern string) ([]int, []int) {
	finder := makeStringFinder(pattern)
	return finder.badCharSkip[:], finder.goodSuffixSkip
}
