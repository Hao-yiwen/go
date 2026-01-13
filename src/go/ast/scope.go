// 版权所有 2009 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// 本文件实现作用域及其包含的对象。

package ast

import (
	"fmt"
	"go/token"
	"strings"
)

// Scope 维护在作用域中声明的命名语言实体集合，
// 以及指向直接包围（外层）作用域的链接。
//
// 已弃用：改用类型检查器 [go/types]；参见 [Object]。
type Scope struct {
	Outer   *Scope
	Objects map[string]*Object
}

// NewScope 创建一个嵌套在外层作用域中的新作用域。
func NewScope(outer *Scope) *Scope {
	const n = 4 // 初始作用域容量
	return &Scope{outer, make(map[string]*Object, n)}
}

// Lookup 如果在作用域 s 中找到给定名称的对象则返回它，
// 否则返回 nil。外层作用域被忽略。
func (s *Scope) Lookup(name string) *Object {
	return s.Objects[name]
}

// Insert 尝试将命名对象 obj 插入作用域 s。
// 如果作用域已包含同名的对象 alt，Insert 保持作用域不变并返回 alt。
// 否则插入 obj 并返回 nil。
func (s *Scope) Insert(obj *Object) (alt *Object) {
	if alt = s.Objects[obj.Name]; alt == nil {
		s.Objects[obj.Name] = obj
	}
	return
}

// 调试支持
func (s *Scope) String() string {
	var buf strings.Builder
	fmt.Fprintf(&buf, "scope %p {", s)
	if s != nil && len(s.Objects) > 0 {
		fmt.Fprintln(&buf)
		for _, obj := range s.Objects {
			fmt.Fprintf(&buf, "\t%s %s\n", obj.Kind, obj.Name)
		}
	}
	fmt.Fprintf(&buf, "}\n")
	return buf.String()
}

// ----------------------------------------------------------------------------
// 对象

// Object 描述命名语言实体，如包、常量、类型、变量、
// 函数（包括方法）或标签。
//
// Data 字段包含特定于对象的数据：
//
//	Kind    Data 类型         Data 值
//	Pkg     *Scope            包作用域
//	Con     int               相应声明的 iota
//
// 已弃用：没有类型信息就无法正确计算 Ident 和 Object 之间的关系。
// 例如，表达式 T{K: 0} 可能表示结构体、映射、切片或数组字面量，
// 具体取决于 T 的类型。如果 T 是结构体，那么 K 引用 T 的字段，
// 而对于其他类型，它引用环境中的值。
//
// 新程序应设置 [parser.SkipObjectResolution] 解析器标志以禁用
// 语法对象解析（这也节省 CPU 和内存），如果需要对象解析，
// 则改用类型检查器 [go/types]。详情参见 [types.Info] 结构体的
// Defs、Uses 和 Implicits 字段。
type Object struct {
	Kind ObjKind
	Name string // 声明的名称
	Decl any    // 对应的 Field、XxxSpec、FuncDecl、LabeledStmt、AssignStmt、Scope；或 nil
	Data any    // 特定于对象的数据；或 nil
	Type any    // 类型信息的占位符；可以为 nil
}

// NewObj 创建给定类型和名称的新对象。
func NewObj(kind ObjKind, name string) *Object {
	return &Object{Kind: kind, Name: name}
}

// Pos 计算对象名称声明的源代码位置。
// 如果无法计算（obj.Decl 可能为 nil 或不正确），
// 结果可能是无效位置。
func (obj *Object) Pos() token.Pos {
	name := obj.Name
	switch d := obj.Decl.(type) {
	case *Field:
		for _, n := range d.Names {
			if n.Name == name {
				return n.Pos()
			}
		}
	case *ImportSpec:
		if d.Name != nil && d.Name.Name == name {
			return d.Name.Pos()
		}
		return d.Path.Pos()
	case *ValueSpec:
		for _, n := range d.Names {
			if n.Name == name {
				return n.Pos()
			}
		}
	case *TypeSpec:
		if d.Name.Name == name {
			return d.Name.Pos()
		}
	case *FuncDecl:
		if d.Name.Name == name {
			return d.Name.Pos()
		}
	case *LabeledStmt:
		if d.Label.Name == name {
			return d.Label.Pos()
		}
	case *AssignStmt:
		for _, x := range d.Lhs {
			if ident, isIdent := x.(*Ident); isIdent && ident.Name == name {
				return ident.Pos()
			}
		}
	case *Scope:
		// 预声明对象 - 目前无需处理
	}
	return token.NoPos
}

// ObjKind 描述 [Object] 表示什么。
type ObjKind int

// 可能的 [Object] 类型列表。
const (
	Bad ObjKind = iota // 用于错误处理
	Pkg                // 包
	Con                // 常量
	Typ                // 类型
	Var                // 变量
	Fun                // 函数或方法
	Lbl                // 标签
)

var objKindStrings = [...]string{
	Bad: "bad",
	Pkg: "package",
	Con: "const",
	Typ: "type",
	Var: "var",
	Fun: "func",
	Lbl: "label",
}

func (kind ObjKind) String() string { return objKindStrings[kind] }
