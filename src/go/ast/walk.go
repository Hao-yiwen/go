// 版权所有 2009 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package ast

import (
	"fmt"
	"iter"
)

// Visitor 的 Visit 方法会在 [Walk] 遇到的每个节点上被调用。
// 如果返回的访问者 w 不为 nil，[Walk] 将使用访问者 w 访问
// node 的每个子节点，随后调用 w.Visit(nil)。
type Visitor interface {
	Visit(node Node) (w Visitor)
}

func walkList[N Node](v Visitor, list []N) {
	for _, node := range list {
		Walk(v, node)
	}
}

// TODO(gri): 研究向 Walk 提供闭包是否会导致
// 更简单的使用（并可能有助于消除 Inspect）。

// Walk 以深度优先顺序遍历 AST：它首先调用 v.Visit(node)；
// node 不能为 nil。如果 v.Visit(node) 返回的访问者 w 不为 nil，
// 则对 node 的每个非 nil 子节点递归调用 Walk（使用访问者 w），
// 随后调用 w.Visit(nil)。
func Walk(v Visitor, node Node) {
	if v = v.Visit(node); v == nil {
		return
	}

	// 遍历子节点
	// （case 的顺序与 ast.go 中相应节点类型的顺序匹配）
	switch n := node.(type) {
	// 注释和字段
	case *Comment:
		// 无需处理

	case *CommentGroup:
		walkList(v, n.List)

	case *Field:
		if n.Doc != nil {
			Walk(v, n.Doc)
		}
		walkList(v, n.Names)
		if n.Type != nil {
			Walk(v, n.Type)
		}
		if n.Tag != nil {
			Walk(v, n.Tag)
		}
		if n.Comment != nil {
			Walk(v, n.Comment)
		}

	case *FieldList:
		walkList(v, n.List)

	// 表达式
	case *BadExpr, *Ident, *BasicLit:
		// 无需处理

	case *Ellipsis:
		if n.Elt != nil {
			Walk(v, n.Elt)
		}

	case *FuncLit:
		Walk(v, n.Type)
		Walk(v, n.Body)

	case *CompositeLit:
		if n.Type != nil {
			Walk(v, n.Type)
		}
		walkList(v, n.Elts)

	case *ParenExpr:
		Walk(v, n.X)

	case *SelectorExpr:
		Walk(v, n.X)
		Walk(v, n.Sel)

	case *IndexExpr:
		Walk(v, n.X)
		Walk(v, n.Index)

	case *IndexListExpr:
		Walk(v, n.X)
		walkList(v, n.Indices)

	case *SliceExpr:
		Walk(v, n.X)
		if n.Low != nil {
			Walk(v, n.Low)
		}
		if n.High != nil {
			Walk(v, n.High)
		}
		if n.Max != nil {
			Walk(v, n.Max)
		}

	case *TypeAssertExpr:
		Walk(v, n.X)
		if n.Type != nil {
			Walk(v, n.Type)
		}

	case *CallExpr:
		Walk(v, n.Fun)
		walkList(v, n.Args)

	case *StarExpr:
		Walk(v, n.X)

	case *UnaryExpr:
		Walk(v, n.X)

	case *BinaryExpr:
		Walk(v, n.X)
		Walk(v, n.Y)

	case *KeyValueExpr:
		Walk(v, n.Key)
		Walk(v, n.Value)

	// 类型
	case *ArrayType:
		if n.Len != nil {
			Walk(v, n.Len)
		}
		Walk(v, n.Elt)

	case *StructType:
		Walk(v, n.Fields)

	case *FuncType:
		if n.TypeParams != nil {
			Walk(v, n.TypeParams)
		}
		if n.Params != nil {
			Walk(v, n.Params)
		}
		if n.Results != nil {
			Walk(v, n.Results)
		}

	case *InterfaceType:
		Walk(v, n.Methods)

	case *MapType:
		Walk(v, n.Key)
		Walk(v, n.Value)

	case *ChanType:
		Walk(v, n.Value)

	// 语句
	case *BadStmt:
		// 无需处理

	case *DeclStmt:
		Walk(v, n.Decl)

	case *EmptyStmt:
		// 无需处理

	case *LabeledStmt:
		Walk(v, n.Label)
		Walk(v, n.Stmt)

	case *ExprStmt:
		Walk(v, n.X)

	case *SendStmt:
		Walk(v, n.Chan)
		Walk(v, n.Value)

	case *IncDecStmt:
		Walk(v, n.X)

	case *AssignStmt:
		walkList(v, n.Lhs)
		walkList(v, n.Rhs)

	case *GoStmt:
		Walk(v, n.Call)

	case *DeferStmt:
		Walk(v, n.Call)

	case *ReturnStmt:
		walkList(v, n.Results)

	case *BranchStmt:
		if n.Label != nil {
			Walk(v, n.Label)
		}

	case *BlockStmt:
		walkList(v, n.List)

	case *IfStmt:
		if n.Init != nil {
			Walk(v, n.Init)
		}
		Walk(v, n.Cond)
		Walk(v, n.Body)
		if n.Else != nil {
			Walk(v, n.Else)
		}

	case *CaseClause:
		walkList(v, n.List)
		walkList(v, n.Body)

	case *SwitchStmt:
		if n.Init != nil {
			Walk(v, n.Init)
		}
		if n.Tag != nil {
			Walk(v, n.Tag)
		}
		Walk(v, n.Body)

	case *TypeSwitchStmt:
		if n.Init != nil {
			Walk(v, n.Init)
		}
		Walk(v, n.Assign)
		Walk(v, n.Body)

	case *CommClause:
		if n.Comm != nil {
			Walk(v, n.Comm)
		}
		walkList(v, n.Body)

	case *SelectStmt:
		Walk(v, n.Body)

	case *ForStmt:
		if n.Init != nil {
			Walk(v, n.Init)
		}
		if n.Cond != nil {
			Walk(v, n.Cond)
		}
		if n.Post != nil {
			Walk(v, n.Post)
		}
		Walk(v, n.Body)

	case *RangeStmt:
		if n.Key != nil {
			Walk(v, n.Key)
		}
		if n.Value != nil {
			Walk(v, n.Value)
		}
		Walk(v, n.X)
		Walk(v, n.Body)

	// 声明
	case *ImportSpec:
		if n.Doc != nil {
			Walk(v, n.Doc)
		}
		if n.Name != nil {
			Walk(v, n.Name)
		}
		Walk(v, n.Path)
		if n.Comment != nil {
			Walk(v, n.Comment)
		}

	case *ValueSpec:
		if n.Doc != nil {
			Walk(v, n.Doc)
		}
		walkList(v, n.Names)
		if n.Type != nil {
			Walk(v, n.Type)
		}
		walkList(v, n.Values)
		if n.Comment != nil {
			Walk(v, n.Comment)
		}

	case *TypeSpec:
		if n.Doc != nil {
			Walk(v, n.Doc)
		}
		Walk(v, n.Name)
		if n.TypeParams != nil {
			Walk(v, n.TypeParams)
		}
		Walk(v, n.Type)
		if n.Comment != nil {
			Walk(v, n.Comment)
		}

	case *BadDecl:
		// 无需处理

	case *GenDecl:
		if n.Doc != nil {
			Walk(v, n.Doc)
		}
		walkList(v, n.Specs)

	case *FuncDecl:
		if n.Doc != nil {
			Walk(v, n.Doc)
		}
		if n.Recv != nil {
			Walk(v, n.Recv)
		}
		Walk(v, n.Name)
		Walk(v, n.Type)
		if n.Body != nil {
			Walk(v, n.Body)
		}

	// 文件和包
	case *File:
		if n.Doc != nil {
			Walk(v, n.Doc)
		}
		Walk(v, n.Name)
		walkList(v, n.Decls)
		// 不遍历 n.Comments - 它们已经通过
		// 各个节点被访问过了

	case *Package:
		for _, f := range n.Files {
			Walk(v, f)
		}

	default:
		panic(fmt.Sprintf("ast.Walk: unexpected node type %T", n))
	}

	v.Visit(nil)
}

type inspector func(Node) bool

func (f inspector) Visit(node Node) Visitor {
	if f(node) {
		return f
	}
	return nil
}

// Inspect 以深度优先顺序遍历 AST：它首先调用 f(node)；
// node 不能为 nil。如果 f 返回 true，Inspect 对 node 的
// 每个非 nil 子节点递归调用 f，随后调用 f(nil)。
//
// 在许多情况下，使用 [Preorder] 可能更方便，它返回节点序列的
// 迭代器；或者使用 [PreorderStack]，它（像 [Inspect] 一样）
// 提供对子树下降的控制，但还额外报告包围节点的栈。
func Inspect(node Node, f func(Node) bool) {
	Walk(inspector(f), node)
}

// Preorder 返回指定 root 下（包括 root）语法树中所有节点的
// 迭代器，按深度优先前序遍历。
//
// 要对每个子树的遍历有更大的控制，请使用 [Inspect] 或 [PreorderStack]。
func Preorder(root Node) iter.Seq[Node] {
	return func(yield func(Node) bool) {
		ok := true
		Inspect(root, func(n Node) bool {
			if n != nil {
				// 一旦 ok 为 false，就不能再调用 yield。
				ok = ok && yield(n)
			}
			return ok
		})
	}
}

// PreorderStack 遍历以 root 为根的树，
// 在访问每个节点之前调用 f。
//
// 每次调用 f 都提供当前节点和遍历栈，遍历栈由 stack 的原始值
// 加上从 root 到 n 的所有节点组成（不包括 n 本身）。
// （这种设计允许 PreorderStack 的调用嵌套而不会重复计数。）
//
// 如果 f 返回 false，遍历将跳过该子树。与 [Inspect] 不同，
// 访问节点 n 后不会第二次调用 f。
// （实际上，第二次调用几乎总是仅用于弹出栈，
// 而正确执行此操作出乎意料地棘手。）
func PreorderStack(root Node, stack []Node, f func(n Node, stack []Node) bool) {
	before := len(stack)
	Inspect(root, func(n Node) bool {
		if n != nil {
			if !f(n, stack) {
				// 不压栈，因为不会有相应的弹出操作。
				return false
			}
			stack = append(stack, n) // 压栈
		} else {
			stack = stack[:len(stack)-1] // 弹栈
		}
		return true
	})
	if len(stack) != before {
		panic("push/pop mismatch")
	}
}
