// 版权所有 2011 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package ast

import (
	"cmp"
	"go/token"
	"slices"
	"strconv"
)

// SortImports 对 f 中 import 块内连续 import 行的序列进行排序。
// 在可以不丢失数据的情况下，它还会移除重复的导入。
func SortImports(fset *token.FileSet, f *File) {
	for _, d := range f.Decls {
		d, ok := d.(*GenDecl)
		if !ok || d.Tok != token.IMPORT {
			// 不是 import 声明，所以我们完成了。
			// import 总是在最前面。
			break
		}

		if !d.Lparen.IsValid() {
			// 不是块：默认已排序。
			continue
		}

		// 识别并排序连续行上的 spec 序列。
		i := 0
		specs := d.Specs[:0]
		for j, s := range d.Specs {
			if j > i && lineAt(fset, s.Pos()) > 1+lineAt(fset, d.Specs[j-1].End()) {
				// j 开始一个新序列。结束当前序列。
				specs = append(specs, sortSpecs(fset, f, d, d.Specs[i:j])...)
				i = j
			}
		}
		specs = append(specs, sortSpecs(fset, f, d, d.Specs[i:])...)
		d.Specs = specs

		// 去重可能在 rparen 前留下空行；清理它。
		if len(d.Specs) > 0 {
			lastSpec := d.Specs[len(d.Specs)-1]
			lastLine := lineAt(fset, lastSpec.Pos())
			rParenLine := lineAt(fset, d.Rparen)
			for rParenLine > lastLine+1 {
				rParenLine--
				fset.File(d.Rparen).MergeLine(rParenLine)
			}
		}
	}

	// 使 File.Imports 顺序一致。
	f.Imports = f.Imports[:0]
	for _, decl := range f.Decls {
		if decl, ok := decl.(*GenDecl); ok && decl.Tok == token.IMPORT {
			for _, spec := range decl.Specs {
				f.Imports = append(f.Imports, spec.(*ImportSpec))
			}
		}
	}
}

func lineAt(fset *token.FileSet, pos token.Pos) int {
	return fset.PositionFor(pos, false).Line
}

func importPath(s Spec) string {
	t, err := strconv.Unquote(s.(*ImportSpec).Path.Value)
	if err == nil {
		return t
	}
	return ""
}

func importName(s Spec) string {
	n := s.(*ImportSpec).Name
	if n == nil {
		return ""
	}
	return n.Name
}

func importComment(s Spec) string {
	c := s.(*ImportSpec).Comment
	if c == nil {
		return ""
	}
	return c.Text()
}

// collapse 指示是否可以移除 prev，只保留 next。
func collapse(prev, next Spec) bool {
	if importPath(next) != importPath(prev) || importName(next) != importName(prev) {
		return false
	}
	return prev.(*ImportSpec).Comment == nil
}

type posSpan struct {
	Start token.Pos
	End   token.Pos
}

type cgPos struct {
	left bool // 如果注释在 spec 的左边则为 true，否则为 false。
	cg   *CommentGroup
}

func sortSpecs(fset *token.FileSet, f *File, d *GenDecl, specs []Spec) []Spec {
	// 即使 specs 已经排序也不能短路，
	// 因为它们可能还需要去重。
	// 但是，单个 import 可以安全地忽略。
	if len(specs) <= 1 {
		return specs
	}

	// 记录 specs 的位置。
	pos := make([]posSpan, len(specs))
	for i, s := range specs {
		pos[i] = posSpan{s.Pos(), s.End()}
	}

	// 识别此范围内的注释。
	begSpecs := pos[0].Start
	endSpecs := pos[len(pos)-1].End
	beg := fset.File(begSpecs).LineStart(lineAt(fset, begSpecs))
	endLine := lineAt(fset, endSpecs)
	endFile := fset.File(endSpecs)
	var end token.Pos
	if endLine == endFile.LineCount() {
		end = endSpecs
	} else {
		end = endFile.LineStart(endLine + 1) // 下一行的开始
	}
	first := len(f.Comments)
	last := -1
	for i, g := range f.Comments {
		if g.End() >= end {
			break
		}
		// g.End() < end
		if beg <= g.Pos() {
			// 注释在 import 声明的范围 [beg, end[ 内
			if i < first {
				first = i
			}
			if i > last {
				last = i
			}
		}
	}

	var comments []*CommentGroup
	if last >= 0 {
		comments = f.Comments[first : last+1]
	}

	// 将每个注释分配给同一行的 import spec。
	importComments := map[*ImportSpec][]cgPos{}
	specIndex := 0
	for _, g := range comments {
		for specIndex+1 < len(specs) && pos[specIndex+1].Start <= g.Pos() {
			specIndex++
		}
		var left bool
		// 块注释可以出现在第一个 import spec 之前。
		if specIndex == 0 && pos[specIndex].Start > g.Pos() {
			left = true
		} else if specIndex+1 < len(specs) && // 或者它可以出现在 import spec 的左边。
			lineAt(fset, pos[specIndex].Start)+1 == lineAt(fset, g.Pos()) {
			specIndex++
			left = true
		}
		s := specs[specIndex].(*ImportSpec)
		importComments[s] = append(importComments[s], cgPos{left: left, cg: g})
	}

	// 按导入路径对 import specs 排序。
	// 在可能不丢失数据的情况下移除重复项。
	// 重新分配导入路径以具有相同的位置序列。
	// 将每个注释重新分配给同一行的 spec。
	// 按新位置对注释排序。
	slices.SortFunc(specs, func(a, b Spec) int {
		ipath := importPath(a)
		jpath := importPath(b)
		r := cmp.Compare(ipath, jpath)
		if r != 0 {
			return r
		}
		iname := importName(a)
		jname := importName(b)
		r = cmp.Compare(iname, jname)
		if r != 0 {
			return r
		}
		return cmp.Compare(importComment(a), importComment(b))
	})

	// 去重。由于我们的排序，我们只需要考虑相邻的导入对。
	deduped := specs[:0]
	for i, s := range specs {
		if i == len(specs)-1 || !collapse(s, specs[i+1]) {
			deduped = append(deduped, s)
		} else {
			p := s.Pos()
			// 当 len(specs) <= 1 时此函数提前退出，
			// 所以 d.Rparen 必须已填充（d.Rparen.IsValid() == true）。
			if l := lineAt(fset, p); l != lineAt(fset, d.Rparen) {
				fset.File(p).MergeLine(l)
			}
		}
	}
	specs = deduped

	// 修复注释位置
	for i, s := range specs {
		s := s.(*ImportSpec)
		if s.Name != nil {
			s.Name.NamePos = pos[i].Start
		}
		updateBasicLitPos(s.Path, pos[i].Start)
		s.EndPos = pos[i].End
		for _, g := range importComments[s] {
			for _, c := range g.cg.List {
				if g.left {
					c.Slash = pos[i].Start - 1
				} else {
					// 一个 import spec 的右边可以同时有块注释和行注释。
					// 在这种情况下，它们的 pos 相同。
					// 但在格式化 AST 时，行注释会被移到块注释之后。
					c.Slash = pos[i].End
				}
			}
		}
	}

	slices.SortFunc(comments, func(a, b *CommentGroup) int {
		return cmp.Compare(a.Pos(), b.Pos())
	})

	return specs
}

// updateBasicLitPos 更新 lit.Pos，
// 确保 lit.End 移动相同的量。
// （参见 https://go.dev/issue/76395。）
func updateBasicLitPos(lit *BasicLit, pos token.Pos) {
	len := lit.End() - lit.Pos()
	lit.ValuePos = pos
	if lit.ValueEnd.IsValid() {
		lit.ValueEnd = pos + len
	}
}
