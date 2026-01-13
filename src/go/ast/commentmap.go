// 版权所有 2012 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package ast

import (
	"bytes"
	"cmp"
	"fmt"
	"go/token"
	"slices"
	"strings"
)

// sortComments 按源代码顺序对注释组列表进行排序。
func sortComments(list []*CommentGroup) {
	slices.SortFunc(list, func(a, b *CommentGroup) int {
		return cmp.Compare(a.Pos(), b.Pos())
	})
}

// CommentMap 将 AST 节点映射到与其关联的注释组列表。
// 有关关联的描述，请参见 [NewCommentMap]。
type CommentMap map[Node][]*CommentGroup

func (cmap CommentMap) addComment(n Node, c *CommentGroup) {
	list := cmap[n]
	if len(list) == 0 {
		list = []*CommentGroup{c}
	} else {
		list = append(list, c)
	}
	cmap[n] = list
}

// nodeList 返回 AST n 中按源代码顺序排列的节点列表。
func nodeList(n Node) []Node {
	var list []Node
	Inspect(n, func(n Node) bool {
		// 不收集注释
		switch n.(type) {
		case nil, *CommentGroup, *Comment:
			return false
		}
		list = append(list, n)
		return true
	})

	// 注意：当前实现假设 Inspect 以深度优先顺序遍历 AST，
	//       因此也是按_源代码_顺序。如果 AST 遍历不遵循源代码顺序，
	//       则需要下面的排序调用。
	// slices.Sort(list, func(a, b Node) int {
	//       r := cmp.Compare(a.Pos(), b.Pos())
	//       if r != 0 {
	//               return r
	//       }
	//       return cmp.Compare(a.End(), b.End())
	// })

	return list
}

// commentListReader 帮助遍历注释组列表。
type commentListReader struct {
	fset     *token.FileSet
	list     []*CommentGroup
	index    int
	comment  *CommentGroup  // 当前索引处的注释组
	pos, end token.Position // 当前索引处注释组的源代码区间
}

func (r *commentListReader) eol() bool {
	return r.index >= len(r.list)
}

func (r *commentListReader) next() {
	if !r.eol() {
		r.comment = r.list[r.index]
		r.pos = r.fset.Position(r.comment.Pos())
		r.end = r.fset.Position(r.comment.End())
		r.index++
	}
}

// nodeStack 跟踪嵌套的节点。
// 栈中较低的节点在词法上包含栈中较高的节点。
type nodeStack []Node

// push 弹出所有在词法上位于 n 之前的节点，
// 然后将 n 压入栈中。
func (s *nodeStack) push(n Node) {
	s.pop(n.Pos())
	*s = append((*s), n)
}

// pop 弹出所有在词法上位于 pos 之前的节点
// （即其词法范围在 pos 之前或恰好在 pos 处结束的节点）。
// 它返回最后弹出的节点。
func (s *nodeStack) pop(pos token.Pos) (top Node) {
	i := len(*s)
	for i > 0 && (*s)[i-1].End() <= pos {
		top = (*s)[i-1]
		i--
	}
	*s = (*s)[0:i]
	return top
}

// NewCommentMap 通过将 comments 列表中的注释组与 node 指定的 AST 节点
// 关联来创建新的注释映射。
//
// 注释组 g 与节点 n 关联的条件：
//
//   - g 开始于 n 结束的同一行
//   - g 开始于 n 之后的下一行，并且在 g 之后、下一个节点之前
//     至少有一个空行
//   - g 开始于 n 之前，且未通过上述规则与 n 之前的节点关联
//
// NewCommentMap 尝试将注释组与尽可能"大"的节点关联：
// 例如，如果注释是赋值语句后的行注释，则注释与整个赋值语句关联，
// 而不仅仅是赋值中的最后一个操作数。
func NewCommentMap(fset *token.FileSet, node Node, comments []*CommentGroup) CommentMap {
	if len(comments) == 0 {
		return nil // 没有需要映射的注释
	}

	cmap := make(CommentMap)

	// 设置注释读取器 r
	tmp := make([]*CommentGroup, len(comments))
	copy(tmp, comments) // 不修改传入的注释
	sortComments(tmp)
	r := commentListReader{fset: fset, list: tmp} // !r.eol() 因为 len(comments) > 0
	r.next()

	// 按词法顺序创建节点列表
	nodes := nodeList(node)
	nodes = append(nodes, nil) // 追加哨兵

	// 设置迭代变量
	var (
		p     Node           // 前一个节点
		pend  token.Position // p 的结束位置
		pg    Node           // 前一个节点组（"重要"的包围节点）
		pgend token.Position // pg 的结束位置
		stack nodeStack      // 节点组栈
	)

	for _, q := range nodes {
		var qpos token.Position
		if q != nil {
			qpos = fset.Position(q.Pos()) // 当前节点位置
		} else {
			// 将假的哨兵位置设为无穷大，以便
			// 所有注释在哨兵之前被处理
			const infinity = 1 << 30
			qpos.Offset = infinity
			qpos.Line = infinity
		}

		// 处理当前节点之前的注释
		for r.end.Offset <= qpos.Offset {
			// 确定最近的节点组
			if top := stack.pop(r.comment.Pos()); top != nil {
				pg = top
				pgend = fset.Position(pg.End())
			}
			// 首先尝试将注释与节点组关联
			// （即"重要"节点，如声明）；
			// 如果失败，尝试将其与最近的节点关联。
			// TODO(gri) 尝试简化下面的逻辑
			var assoc Node
			switch {
			case pg != nil &&
				(pgend.Line == r.pos.Line ||
					pgend.Line+1 == r.pos.Line && r.end.Line+1 < qpos.Line):
				// 1) 注释开始于前一个节点组结束的同一行，或
				// 2) 注释开始于前一个节点组之后的下一行，
				//    且在当前节点之前有一个空行
				// => 将注释与前一个节点组关联
				assoc = pg
			case p != nil &&
				(pend.Line == r.pos.Line ||
					pend.Line+1 == r.pos.Line && r.end.Line+1 < qpos.Line ||
					q == nil):
				// 与上述规则相同，但适用于 p 而不是 pg，
				// 如果到达末尾（q == nil）也与 p 关联
				assoc = p
			default:
				// 否则，将注释与当前节点关联
				if q == nil {
					// 只有在没有 p 的情况下才能到达这里
					// 这意味着没有节点
					panic("internal error: no comments 应该是 associated with sentinel")
				}
				assoc = q
			}
			cmap.addComment(assoc, r.comment)
			if r.eol() {
				return cmap
			}
			r.next()
		}

		// 更新前一个节点
		p = q
		pend = fset.Position(p.End())

		// 如果遇到"重要"节点，更新前一个节点组
		switch q.(type) {
		case *File, *Field, Decl, Spec, Stmt:
			stack.push(q)
		}
	}

	return cmap
}

// Update 用新节点替换注释映射中的旧节点并返回新节点。
// 与旧节点关联的注释将与新节点关联。
func (cmap CommentMap) Update(old, new Node) Node {
	if list := cmap[old]; len(list) > 0 {
		delete(cmap, old)
		cmap[new] = append(cmap[new], list...)
	}
	return new
}

// Filter 返回一个新的注释映射，只包含 cmap 中那些
// 在 node 指定的 AST 中存在对应节点的条目。
func (cmap CommentMap) Filter(node Node) CommentMap {
	umap := make(CommentMap)
	Inspect(node, func(n Node) bool {
		if g := cmap[n]; len(g) > 0 {
			umap[n] = g
		}
		return true
	})
	return umap
}

// Comments 返回注释映射中的注释组列表。
// 结果按源代码顺序排序。
func (cmap CommentMap) Comments() []*CommentGroup {
	list := make([]*CommentGroup, 0, len(cmap))
	for _, e := range cmap {
		list = append(list, e...)
	}
	sortComments(list)
	return list
}

func summary(list []*CommentGroup) string {
	const maxLen = 40
	var buf bytes.Buffer

	// 收集注释文本
loop:
	for _, group := range list {
		// 注意：CommentGroup.Text() 对于我们的需求做了太多工作，
		//       它只会替换这个最内层循环。
		//       直接显式处理。
		for _, comment := range group.List {
			if buf.Len() >= maxLen {
				break loop
			}
			buf.WriteString(comment.Text)
		}
	}

	// 如果太长则截断
	if buf.Len() > maxLen {
		buf.Truncate(maxLen - 3)
		buf.WriteString("...")
	}

	// 用空格替换所有不可见字符
	bytes := buf.Bytes()
	for i, b := range bytes {
		switch b {
		case '\t', '\n', '\r':
			bytes[i] = ' '
		}
	}

	return string(bytes)
}

func (cmap CommentMap) String() string {
	// 按排序顺序打印映射条目
	var nodes []Node
	for node := range cmap {
		nodes = append(nodes, node)
	}
	slices.SortFunc(nodes, func(a, b Node) int {
		r := cmp.Compare(a.Pos(), b.Pos())
		if r != 0 {
			return r
		}
		return cmp.Compare(a.End(), b.End())
	})

	var buf strings.Builder
	fmt.Fprintln(&buf, "CommentMap {")
	for _, node := range nodes {
		comment := cmap[node]
		// 打印标识符的名称；对于其他节点打印节点类型
		var s string
		if ident, ok := node.(*Ident); ok {
			s = ident.Name
		} else {
			s = fmt.Sprintf("%T", node)
		}
		fmt.Fprintf(&buf, "\t%p  %20s:  %s\n", node, s, summary(comment))
	}
	fmt.Fprintln(&buf, "}")
	return buf.String()
}
