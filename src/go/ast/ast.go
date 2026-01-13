// 版权所有 2009 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// ast 包声明了用于表示 Go 包语法树的类型。
//
// 语法树可以直接构建，但通常由解析器从 Go 源代码生成；
// 参见 [go/parser] 包中的 ParseFile 函数。
package ast

import (
	"go/token"
	"strings"
)

// ----------------------------------------------------------------------------
// 接口
//
// 节点主要分为三类：表达式和类型节点、语句节点以及声明节点。
// 节点名称通常与对应的 Go 语言规范产生式名称相匹配。
// 节点的字段对应于各自产生式的各个部分。
//
// 所有节点都包含位置信息，标记相应源代码文本片段的起始位置；
// 可以通过 Pos 访问器方法获取该信息。节点可能包含额外的位置信息，
// 用于那些在构造的各部分之间可能存在注释的语言结构
// （通常是任何较大的、带括号的子部分）。
// 在打印构造时需要这些位置信息来正确定位注释。

// 所有节点类型都实现 Node 接口。
type Node interface {
	Pos() token.Pos // 属于该节点的第一个字符的位置
	End() token.Pos // 紧跟该节点之后的第一个字符的位置
}

// 所有表达式节点都实现 Expr 接口。
type Expr interface {
	Node
	exprNode()
}

// 所有语句节点都实现 Stmt 接口。
type Stmt interface {
	Node
	stmtNode()
}

// 所有声明节点都实现 Decl 接口。
type Decl interface {
	Node
	declNode()
}

// ----------------------------------------------------------------------------
// 注释

// Comment 节点表示单个 // 风格或 /* 风格的注释。
//
// Text 字段包含注释文本，其中不包含源代码中可能存在的回车符（\r）。
// 由于注释的结束位置是使用 len(Text) 计算的，因此对于包含回车符的注释，
// [Comment.End] 报告的位置与源代码的实际结束位置不匹配。
type Comment struct {
	Slash token.Pos // 注释开始处 "/" 的位置
	Text  string    // 注释文本（对于 // 风格的注释不包括 '\n'）
}

func (c *Comment) Pos() token.Pos { return c.Slash }
func (c *Comment) End() token.Pos { return token.Pos(int(c.Slash) + len(c.Text)) }

// CommentGroup 表示一组连续的注释，
// 它们之间没有其他标记也没有空行。
type CommentGroup struct {
	List []*Comment // len(List) > 0
}

func (g *CommentGroup) Pos() token.Pos { return g.List[0].Pos() }
func (g *CommentGroup) End() token.Pos { return g.List[len(g.List)-1].End() }

func isWhitespace(ch byte) bool { return ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' }

func stripTrailingWhitespace(s string) string {
	i := len(s)
	for i > 0 && isWhitespace(s[i-1]) {
		i--
	}
	return s[0:i]
}

// Text 返回注释的文本内容。
// 注释标记（//、/* 和 */）、行注释的第一个空格以及
// 前导和尾随的空行会被移除。
// 注释指令（如 "//line" 和 "//go:noinline"）也会被移除。
// 多个空行会被合并为一个，行尾空格会被裁剪。
// 除非结果为空，否则以换行符结尾。
func (g *CommentGroup) Text() string {
	if g == nil {
		return ""
	}
	comments := make([]string, len(g.List))
	for i, c := range g.List {
		comments[i] = c.Text
	}

	lines := make([]string, 0, 10) // 大多数注释少于 10 行
	for _, c := range comments {
		// 移除注释标记。
		// 解析器已经给我们提供了确切的注释文本。
		switch c[1] {
		case '/':
			// // 风格的注释（末尾没有换行符）
			c = c[2:]
			if len(c) == 0 {
				// 空行
				break
			}
			if c[0] == ' ' {
				// 去除第一个空格 - Example 测试需要
				c = c[1:]
				break
			}
			if isDirective(c) {
				// 忽略 //go:noinline、//line 等指令。
				continue
			}
		case '*':
			/* /* 风格的注释 */
			c = c[2 : len(c)-2]
		}

		// 按换行符分割。
		cl := strings.SplitSeq(c, "\n")

		// 遍历各行，去除尾部空白并添加到列表中。
		for l := range cl {
			lines = append(lines, stripTrailingWhitespace(l))
		}
	}

	// 移除前导空行；将内部连续的空行合并为单个空行。
	n := 0
	for _, line := range lines {
		if line != "" || n > 0 && lines[n-1] != "" {
			lines[n] = line
			n++
		}
	}
	lines = lines[0:n]

	// 添加最后的 "" 条目以便 Join 生成尾随换行符。
	if n > 0 && lines[n-1] != "" {
		lines = append(lines, "")
	}

	return strings.Join(lines, "\n")
}

// isDirective 报告 c 是否是注释指令。
// 此代码也存在于 go/printer 中。
func isDirective(c string) bool {
	// "//line " 是行指令。
	// "//extern " 用于 gccgo。
	// "//export " 用于 cgo。
	// （// 已被移除。）
	if strings.HasPrefix(c, "line ") || strings.HasPrefix(c, "extern ") || strings.HasPrefix(c, "export ") {
		return true
	}

	// "//[a-z0-9]+:[a-z0-9]"
	// （// 已被移除。）
	colon := strings.Index(c, ":")
	if colon <= 0 || colon+1 >= len(c) {
		return false
	}
	for i := 0; i <= colon+1; i++ {
		if i == colon {
			continue
		}
		b := c[i]
		if !('a' <= b && b <= 'z' || '0' <= b && b <= '9') {
			return false
		}
	}
	return true
}

// ----------------------------------------------------------------------------
// 表达式和类型

// Field 表示结构体类型中的字段声明列表、接口类型中的方法列表，
// 或者签名中的参数/结果声明。
// 对于未命名参数（仅包含类型的参数列表）和嵌入式结构体字段，
// [Field.Names] 为 nil。对于后者，字段名就是类型名。
type Field struct {
	Doc     *CommentGroup // 关联的文档；或 nil
	Names   []*Ident      // 字段/方法/（类型）参数名；或 nil
	Type    Expr          // 字段/方法/参数类型；或 nil
	Tag     *BasicLit     // 字段标签；或 nil
	Comment *CommentGroup // 行注释；或 nil
}

func (f *Field) Pos() token.Pos {
	if len(f.Names) > 0 {
		return f.Names[0].Pos()
	}
	if f.Type != nil {
		return f.Type.Pos()
	}
	return token.NoPos
}

func (f *Field) End() token.Pos {
	if f.Tag != nil {
		return f.Tag.End()
	}
	if f.Type != nil {
		return f.Type.End()
	}
	if len(f.Names) > 0 {
		return f.Names[len(f.Names)-1].End()
	}
	return token.NoPos
}

// FieldList 表示由圆括号、花括号或方括号包围的字段列表。
type FieldList struct {
	Opening token.Pos // 开括号（圆括号/花括号/方括号）的位置（如有）
	List    []*Field  // 字段列表；或 nil
	Closing token.Pos // 闭括号（圆括号/花括号/方括号）的位置（如有）
}

func (f *FieldList) Pos() token.Pos {
	if f.Opening.IsValid() {
		return f.Opening
	}
	// 在这种情况下列表不应为空；
	// 保守起见，防范错误的 AST
	if len(f.List) > 0 {
		return f.List[0].Pos()
	}
	return token.NoPos
}

func (f *FieldList) End() token.Pos {
	if f.Closing.IsValid() {
		return f.Closing + 1
	}
	// 在这种情况下列表不应为空；
	// 保守起见，防范错误的 AST
	if n := len(f.List); n > 0 {
		return f.List[n-1].End()
	}
	return token.NoPos
}

// NumFields 返回 [FieldList] 表示的参数或结构体字段的数量。
func (f *FieldList) NumFields() int {
	n := 0
	if f != nil {
		for _, g := range f.List {
			m := len(g.Names)
			if m == 0 {
				m = 1
			}
			n += m
		}
	}
	return n
}

// 表达式由一个或多个以下具体表达式节点组成的树表示。
type (
	// BadExpr 节点是包含语法错误的表达式的占位符，
	// 无法为其创建正确的表达式节点。
	//
	BadExpr struct {
		From, To token.Pos // 错误表达式的位置范围
	}

	// Ident 节点表示一个标识符。
	Ident struct {
		NamePos token.Pos // 标识符位置
		Name    string    // 标识符名称
		Obj     *Object   // 表示的对象，或 nil。已弃用：参见 Object。
	}

	// Ellipsis 节点表示参数列表中的 "..." 类型
	// 或数组类型中的 "..." 长度。
	//
	Ellipsis struct {
		Ellipsis token.Pos // "..." 的位置
		Elt      Expr      // 省略号元素类型（仅用于参数列表）；或 nil
	}

	// BasicLit 节点表示基本类型的字面量。
	//
	// 注意，对于 CHAR 和 STRING 类型，字面量是带引号存储的。
	// 例如，对于双引号的 STRING，Value 字段的第一个和最后一个
	// 字符将是 "。可以使用 [strconv.Unquote] 和 [strconv.UnquoteChar]
	// 函数分别对 STRING 和 CHAR 值进行去引号处理。
	//
	// 对于原始字符串字面量（Kind == token.STRING && Value[0] == '`'），
	// Value 字段包含的字符串文本不包含源代码中可能存在的回车符（\r）。
	BasicLit struct {
		ValuePos token.Pos   // 字面量位置
		ValueEnd token.Pos   // 紧跟字面量之后的位置
		Kind     token.Token // token.INT、token.FLOAT、token.IMAG、token.CHAR 或 token.STRING
		Value    string      // 字面量字符串；如 42、0x7f、3.14、1e-9、2.4i、'a'、'\x7f'、"foo" 或 `\m\n\o`
	}

	// FuncLit 节点表示函数字面量。
	FuncLit struct {
		Type *FuncType  // 函数类型
		Body *BlockStmt // 函数体
	}

	// CompositeLit 节点表示复合字面量。
	CompositeLit struct {
		Type       Expr      // 字面量类型；或 nil
		Lbrace     token.Pos // "{" 的位置
		Elts       []Expr    // 复合元素列表；或 nil
		Rbrace     token.Pos // "}" 的位置
		Incomplete bool      // 如果 Elts 列表中缺少（源）表达式则为 true
	}

	// ParenExpr 节点表示带括号的表达式。
	ParenExpr struct {
		Lparen token.Pos // "(" 的位置
		X      Expr      // 括号内的表达式
		Rparen token.Pos // ")" 的位置
	}

	// SelectorExpr 节点表示后跟选择器的表达式。
	SelectorExpr struct {
		X   Expr   // 表达式
		Sel *Ident // 字段选择器
	}

	// IndexExpr 节点表示后跟索引的表达式。
	IndexExpr struct {
		X      Expr      // 表达式
		Lbrack token.Pos // "[" 的位置
		Index  Expr      // 索引表达式
		Rbrack token.Pos // "]" 的位置
	}

	// IndexListExpr 节点表示后跟多个索引的表达式。
	IndexListExpr struct {
		X       Expr      // 表达式
		Lbrack  token.Pos // "[" 的位置
		Indices []Expr    // 索引表达式
		Rbrack  token.Pos // "]" 的位置
	}

	// SliceExpr 节点表示后跟切片索引的表达式。
	SliceExpr struct {
		X      Expr      // 表达式
		Lbrack token.Pos // "[" 的位置
		Low    Expr      // 切片范围的起始；或 nil
		High   Expr      // 切片范围的结束；或 nil
		Max    Expr      // 切片的最大容量；或 nil
		Slice3 bool      // 如果是三索引切片（有两个冒号）则为 true
		Rbrack token.Pos // "]" 的位置
	}

	// TypeAssertExpr 节点表示后跟类型断言的表达式。
	//
	TypeAssertExpr struct {
		X      Expr      // 表达式
		Lparen token.Pos // "(" 的位置
		Type   Expr      // 断言的类型；nil 表示类型选择 X.(type)
		Rparen token.Pos // ")" 的位置
	}

	// CallExpr 节点表示后跟参数列表的表达式。
	CallExpr struct {
		Fun      Expr      // 函数表达式
		Lparen   token.Pos // "(" 的位置
		Args     []Expr    // 函数参数；或 nil
		Ellipsis token.Pos // "..." 的位置（如果没有 "..." 则为 token.NoPos）
		Rparen   token.Pos // ")" 的位置
	}

	// StarExpr 节点表示形如 "*" Expression 的表达式。
	// 语义上它可以是一元 "*" 表达式，或者是指针类型。
	//
	StarExpr struct {
		Star token.Pos // "*" 的位置
		X    Expr      // 操作数
	}

	// UnaryExpr 节点表示一元表达式。
	// 一元 "*" 表达式通过 StarExpr 节点表示。
	//
	UnaryExpr struct {
		OpPos token.Pos   // Op 的位置
		Op    token.Token // 运算符
		X     Expr        // 操作数
	}

	// BinaryExpr 节点表示二元表达式。
	BinaryExpr struct {
		X     Expr        // 左操作数
		OpPos token.Pos   // Op 的位置
		Op    token.Token // 运算符
		Y     Expr        // 右操作数
	}

	// KeyValueExpr 节点表示复合字面量中的（键 : 值）对。
	//
	KeyValueExpr struct {
		Key   Expr
		Colon token.Pos // ":" 的位置
		Value Expr
	}
)

// 通道类型的方向由包含以下一个或两个常量的位掩码表示。
type ChanDir int

const (
	SEND ChanDir = 1 << iota
	RECV
)

// 类型由一个或多个以下特定于类型的表达式节点组成的树表示。
type (
	// ArrayType 节点表示数组或切片类型。
	ArrayType struct {
		Lbrack token.Pos // "[" 的位置
		Len    Expr      // [...]T 数组类型的 Ellipsis 节点，切片类型为 nil
		Elt    Expr      // 元素类型
	}

	// StructType 节点表示结构体类型。
	StructType struct {
		Struct     token.Pos  // "struct" 关键字的位置
		Fields     *FieldList // 字段声明列表
		Incomplete bool       // 如果 Fields 列表中缺少（源）字段则为 true
	}

	// 指针类型通过 StarExpr 节点表示。

	// FuncType 节点表示函数类型。
	FuncType struct {
		Func       token.Pos  // "func" 关键字的位置（如果没有 "func" 则为 token.NoPos）
		TypeParams *FieldList // 类型参数；或 nil
		Params     *FieldList // （传入）参数；非 nil
		Results    *FieldList // （传出）结果；或 nil
	}

	// InterfaceType 节点表示接口类型。
	InterfaceType struct {
		Interface  token.Pos  // "interface" 关键字的位置
		Methods    *FieldList // 嵌入接口、方法或类型的列表
		Incomplete bool       // 如果 Methods 列表中缺少（源）方法或类型则为 true
	}

	// MapType 节点表示映射类型。
	MapType struct {
		Map   token.Pos // "map" 关键字的位置
		Key   Expr
		Value Expr
	}

	// ChanType 节点表示通道类型。
	ChanType struct {
		Begin token.Pos // "chan" 关键字或 "<-" 的位置（以先出现的为准）
		Arrow token.Pos // "<-" 的位置（如果没有 "<-" 则为 token.NoPos）
		Dir   ChanDir   // 通道方向
		Value Expr      // 值类型
	}
)

// 表达式/类型节点的 Pos 和 End 实现。

func (x *BadExpr) Pos() token.Pos  { return x.From }
func (x *Ident) Pos() token.Pos    { return x.NamePos }
func (x *Ellipsis) Pos() token.Pos { return x.Ellipsis }
func (x *BasicLit) Pos() token.Pos { return x.ValuePos }
func (x *FuncLit) Pos() token.Pos  { return x.Type.Pos() }
func (x *CompositeLit) Pos() token.Pos {
	if x.Type != nil {
		return x.Type.Pos()
	}
	return x.Lbrace
}
func (x *ParenExpr) Pos() token.Pos      { return x.Lparen }
func (x *SelectorExpr) Pos() token.Pos   { return x.X.Pos() }
func (x *IndexExpr) Pos() token.Pos      { return x.X.Pos() }
func (x *IndexListExpr) Pos() token.Pos  { return x.X.Pos() }
func (x *SliceExpr) Pos() token.Pos      { return x.X.Pos() }
func (x *TypeAssertExpr) Pos() token.Pos { return x.X.Pos() }
func (x *CallExpr) Pos() token.Pos       { return x.Fun.Pos() }
func (x *StarExpr) Pos() token.Pos       { return x.Star }
func (x *UnaryExpr) Pos() token.Pos      { return x.OpPos }
func (x *BinaryExpr) Pos() token.Pos     { return x.X.Pos() }
func (x *KeyValueExpr) Pos() token.Pos   { return x.Key.Pos() }
func (x *ArrayType) Pos() token.Pos      { return x.Lbrack }
func (x *StructType) Pos() token.Pos     { return x.Struct }
func (x *FuncType) Pos() token.Pos {
	if x.Func.IsValid() || x.Params == nil { // 参见 issue 3870
		return x.Func
	}
	return x.Params.Pos() // 接口方法声明没有 "func" 关键字
}
func (x *InterfaceType) Pos() token.Pos { return x.Interface }
func (x *MapType) Pos() token.Pos       { return x.Map }
func (x *ChanType) Pos() token.Pos      { return x.Begin }

func (x *BadExpr) End() token.Pos { return x.To }
func (x *Ident) End() token.Pos   { return token.Pos(int(x.NamePos) + len(x.Name)) }
func (x *Ellipsis) End() token.Pos {
	if x.Elt != nil {
		return x.Elt.End()
	}
	return x.Ellipsis + 3 // len("...")
}
func (x *BasicLit) End() token.Pos {
	if !x.ValueEnd.IsValid() {
		// 不是来自解析器；使用启发式方法。
		// （对于包含 \r\n 的 `...` 是不正确的；
		// 参见 https://go.dev/issue/76031。）
		return token.Pos(int(x.ValuePos) + len(x.Value))
	}
	return x.ValueEnd
}
func (x *FuncLit) End() token.Pos        { return x.Body.End() }
func (x *CompositeLit) End() token.Pos   { return x.Rbrace + 1 }
func (x *ParenExpr) End() token.Pos      { return x.Rparen + 1 }
func (x *SelectorExpr) End() token.Pos   { return x.Sel.End() }
func (x *IndexExpr) End() token.Pos      { return x.Rbrack + 1 }
func (x *IndexListExpr) End() token.Pos  { return x.Rbrack + 1 }
func (x *SliceExpr) End() token.Pos      { return x.Rbrack + 1 }
func (x *TypeAssertExpr) End() token.Pos { return x.Rparen + 1 }
func (x *CallExpr) End() token.Pos       { return x.Rparen + 1 }
func (x *StarExpr) End() token.Pos       { return x.X.End() }
func (x *UnaryExpr) End() token.Pos      { return x.X.End() }
func (x *BinaryExpr) End() token.Pos     { return x.Y.End() }
func (x *KeyValueExpr) End() token.Pos   { return x.Value.End() }
func (x *ArrayType) End() token.Pos      { return x.Elt.End() }
func (x *StructType) End() token.Pos     { return x.Fields.End() }
func (x *FuncType) End() token.Pos {
	if x.Results != nil {
		return x.Results.End()
	}
	return x.Params.End()
}
func (x *InterfaceType) End() token.Pos { return x.Methods.End() }
func (x *MapType) End() token.Pos       { return x.Value.End() }
func (x *ChanType) End() token.Pos      { return x.Value.End() }

// exprNode() 确保只有表达式/类型节点可以赋值给 Expr。
func (*BadExpr) exprNode()        {}
func (*Ident) exprNode()          {}
func (*Ellipsis) exprNode()       {}
func (*BasicLit) exprNode()       {}
func (*FuncLit) exprNode()        {}
func (*CompositeLit) exprNode()   {}
func (*ParenExpr) exprNode()      {}
func (*SelectorExpr) exprNode()   {}
func (*IndexExpr) exprNode()      {}
func (*IndexListExpr) exprNode()  {}
func (*SliceExpr) exprNode()      {}
func (*TypeAssertExpr) exprNode() {}
func (*CallExpr) exprNode()       {}
func (*StarExpr) exprNode()       {}
func (*UnaryExpr) exprNode()      {}
func (*BinaryExpr) exprNode()     {}
func (*KeyValueExpr) exprNode()   {}

func (*ArrayType) exprNode()     {}
func (*StructType) exprNode()    {}
func (*FuncType) exprNode()      {}
func (*InterfaceType) exprNode() {}
func (*MapType) exprNode()       {}
func (*ChanType) exprNode()      {}

// ----------------------------------------------------------------------------
// Ident 的便利函数

// NewIdent 创建一个没有位置信息的新 [Ident]。
// 对于由 Go 解析器以外的代码生成的 AST 很有用。
func NewIdent(name string) *Ident { return &Ident{token.NoPos, name, nil} }

// IsExported 报告 name 是否以大写字母开头。
func IsExported(name string) bool { return token.IsExported(name) }

// IsExported 报告 id 是否以大写字母开头。
func (id *Ident) IsExported() bool { return token.IsExported(id.Name) }

func (id *Ident) String() string {
	if id != nil {
		return id.Name
	}
	return "<nil>"
}

// ----------------------------------------------------------------------------
// 语句

// 语句由一个或多个以下具体语句节点组成的树表示。
type (
	// BadStmt 节点是包含语法错误的语句的占位符，
	// 无法为其创建正确的语句节点。
	//
	BadStmt struct {
		From, To token.Pos // 错误语句的位置范围
	}

	// DeclStmt 节点表示语句列表中的声明。
	DeclStmt struct {
		Decl Decl // 带有 CONST、TYPE 或 VAR 标记的 *GenDecl
	}

	// EmptyStmt 节点表示空语句。
	// 空语句的"位置"是紧随其后的（显式或隐式）分号的位置。
	//
	EmptyStmt struct {
		Semicolon token.Pos // 后续 ";" 的位置
		Implicit  bool      // 如果设置，表示源代码中省略了 ";"
	}

	// LabeledStmt 节点表示带标签的语句。
	LabeledStmt struct {
		Label *Ident
		Colon token.Pos // ":" 的位置
		Stmt  Stmt
	}

	// ExprStmt 节点表示语句列表中的（独立）表达式。
	//
	ExprStmt struct {
		X Expr // 表达式
	}

	// SendStmt 节点表示发送语句。
	SendStmt struct {
		Chan  Expr
		Arrow token.Pos // "<-" 的位置
		Value Expr
	}

	// IncDecStmt 节点表示递增或递减语句。
	IncDecStmt struct {
		X      Expr
		TokPos token.Pos   // Tok 的位置
		Tok    token.Token // INC 或 DEC
	}

	// AssignStmt 节点表示赋值语句或短变量声明。
	//
	AssignStmt struct {
		Lhs    []Expr
		TokPos token.Pos   // Tok 的位置
		Tok    token.Token // 赋值标记，DEFINE
		Rhs    []Expr
	}

	// GoStmt 节点表示 go 语句。
	GoStmt struct {
		Go   token.Pos // "go" 关键字的位置
		Call *CallExpr
	}

	// DeferStmt 节点表示 defer 语句。
	DeferStmt struct {
		Defer token.Pos // "defer" 关键字的位置
		Call  *CallExpr
	}

	// ReturnStmt 节点表示 return 语句。
	ReturnStmt struct {
		Return  token.Pos // "return" 关键字的位置
		Results []Expr    // 结果表达式；或 nil
	}

	// BranchStmt 节点表示 break、continue、goto 或 fallthrough 语句。
	//
	BranchStmt struct {
		TokPos token.Pos   // Tok 的位置
		Tok    token.Token // 关键字标记（BREAK、CONTINUE、GOTO、FALLTHROUGH）
		Label  *Ident      // 标签名；或 nil
	}

	// BlockStmt 节点表示花括号包围的语句列表。
	BlockStmt struct {
		Lbrace token.Pos // "{" 的位置
		List   []Stmt
		Rbrace token.Pos // "}" 的位置（如有）（可能因语法错误而不存在）
	}

	// IfStmt 节点表示 if 语句。
	IfStmt struct {
		If   token.Pos // "if" 关键字的位置
		Init Stmt      // 初始化语句；或 nil
		Cond Expr      // 条件
		Body *BlockStmt
		Else Stmt // else 分支；或 nil
	}

	// CaseClause 表示表达式 switch 或类型 switch 语句的一个 case。
	CaseClause struct {
		Case  token.Pos // "case" 或 "default" 关键字的位置
		List  []Expr    // 表达式或类型列表；nil 表示 default case
		Colon token.Pos // ":" 的位置
		Body  []Stmt    // 语句列表；或 nil
	}

	// SwitchStmt 节点表示表达式 switch 语句。
	SwitchStmt struct {
		Switch token.Pos  // "switch" 关键字的位置
		Init   Stmt       // 初始化语句；或 nil
		Tag    Expr       // 标签表达式；或 nil
		Body   *BlockStmt // 仅 CaseClauses
	}

	// TypeSwitchStmt 节点表示类型 switch 语句。
	TypeSwitchStmt struct {
		Switch token.Pos  // "switch" 关键字的位置
		Init   Stmt       // 初始化语句；或 nil
		Assign Stmt       // x := y.(type) 或 y.(type)
		Body   *BlockStmt // 仅 CaseClauses
	}

	// CommClause 节点表示 select 语句的一个 case。
	CommClause struct {
		Case  token.Pos // "case" 或 "default" 关键字的位置
		Comm  Stmt      // 发送或接收语句；nil 表示 default case
		Colon token.Pos // ":" 的位置
		Body  []Stmt    // 语句列表；或 nil
	}

	// SelectStmt 节点表示 select 语句。
	SelectStmt struct {
		Select token.Pos  // "select" 关键字的位置
		Body   *BlockStmt // 仅 CommClauses
	}

	// ForStmt 表示 for 语句。
	ForStmt struct {
		For  token.Pos // "for" 关键字的位置
		Init Stmt      // 初始化语句；或 nil
		Cond Expr      // 条件；或 nil
		Post Stmt      // 后迭代语句；或 nil
		Body *BlockStmt
	}

	// RangeStmt 表示带 range 子句的 for 语句。
	RangeStmt struct {
		For        token.Pos   // "for" 关键字的位置
		Key, Value Expr        // Key、Value 可以为 nil
		TokPos     token.Pos   // Tok 的位置；如果 Key == nil 则无效
		Tok        token.Token // 如果 Key == nil 则为 ILLEGAL，否则为 ASSIGN 或 DEFINE
		Range      token.Pos   // "range" 关键字的位置
		X          Expr        // 要遍历的值
		Body       *BlockStmt
	}
)

// 语句节点的 Pos 和 End 实现。

func (s *BadStmt) Pos() token.Pos        { return s.From }
func (s *DeclStmt) Pos() token.Pos       { return s.Decl.Pos() }
func (s *EmptyStmt) Pos() token.Pos      { return s.Semicolon }
func (s *LabeledStmt) Pos() token.Pos    { return s.Label.Pos() }
func (s *ExprStmt) Pos() token.Pos       { return s.X.Pos() }
func (s *SendStmt) Pos() token.Pos       { return s.Chan.Pos() }
func (s *IncDecStmt) Pos() token.Pos     { return s.X.Pos() }
func (s *AssignStmt) Pos() token.Pos     { return s.Lhs[0].Pos() }
func (s *GoStmt) Pos() token.Pos         { return s.Go }
func (s *DeferStmt) Pos() token.Pos      { return s.Defer }
func (s *ReturnStmt) Pos() token.Pos     { return s.Return }
func (s *BranchStmt) Pos() token.Pos     { return s.TokPos }
func (s *BlockStmt) Pos() token.Pos      { return s.Lbrace }
func (s *IfStmt) Pos() token.Pos         { return s.If }
func (s *CaseClause) Pos() token.Pos     { return s.Case }
func (s *SwitchStmt) Pos() token.Pos     { return s.Switch }
func (s *TypeSwitchStmt) Pos() token.Pos { return s.Switch }
func (s *CommClause) Pos() token.Pos     { return s.Case }
func (s *SelectStmt) Pos() token.Pos     { return s.Select }
func (s *ForStmt) Pos() token.Pos        { return s.For }
func (s *RangeStmt) Pos() token.Pos      { return s.For }

func (s *BadStmt) End() token.Pos  { return s.To }
func (s *DeclStmt) End() token.Pos { return s.Decl.End() }
func (s *EmptyStmt) End() token.Pos {
	if s.Implicit {
		return s.Semicolon
	}
	return s.Semicolon + 1 /* len(";") */
}
func (s *LabeledStmt) End() token.Pos { return s.Stmt.End() }
func (s *ExprStmt) End() token.Pos    { return s.X.End() }
func (s *SendStmt) End() token.Pos    { return s.Value.End() }
func (s *IncDecStmt) End() token.Pos {
	return s.TokPos + 2 /* len("++") */
}
func (s *AssignStmt) End() token.Pos { return s.Rhs[len(s.Rhs)-1].End() }
func (s *GoStmt) End() token.Pos     { return s.Call.End() }
func (s *DeferStmt) End() token.Pos  { return s.Call.End() }
func (s *ReturnStmt) End() token.Pos {
	if n := len(s.Results); n > 0 {
		return s.Results[n-1].End()
	}
	return s.Return + 6 // len("return")
}
func (s *BranchStmt) End() token.Pos {
	if s.Label != nil {
		return s.Label.End()
	}
	return token.Pos(int(s.TokPos) + len(s.Tok.String()))
}
func (s *BlockStmt) End() token.Pos {
	if s.Rbrace.IsValid() {
		return s.Rbrace + 1
	}
	if n := len(s.List); n > 0 {
		return s.List[n-1].End()
	}
	return s.Lbrace + 1
}
func (s *IfStmt) End() token.Pos {
	if s.Else != nil {
		return s.Else.End()
	}
	return s.Body.End()
}
func (s *CaseClause) End() token.Pos {
	if n := len(s.Body); n > 0 {
		return s.Body[n-1].End()
	}
	return s.Colon + 1
}
func (s *SwitchStmt) End() token.Pos     { return s.Body.End() }
func (s *TypeSwitchStmt) End() token.Pos { return s.Body.End() }
func (s *CommClause) End() token.Pos {
	if n := len(s.Body); n > 0 {
		return s.Body[n-1].End()
	}
	return s.Colon + 1
}
func (s *SelectStmt) End() token.Pos { return s.Body.End() }
func (s *ForStmt) End() token.Pos    { return s.Body.End() }
func (s *RangeStmt) End() token.Pos  { return s.Body.End() }

// stmtNode() 确保只有语句节点可以赋值给 Stmt。
func (*BadStmt) stmtNode()        {}
func (*DeclStmt) stmtNode()       {}
func (*EmptyStmt) stmtNode()      {}
func (*LabeledStmt) stmtNode()    {}
func (*ExprStmt) stmtNode()       {}
func (*SendStmt) stmtNode()       {}
func (*IncDecStmt) stmtNode()     {}
func (*AssignStmt) stmtNode()     {}
func (*GoStmt) stmtNode()         {}
func (*DeferStmt) stmtNode()      {}
func (*ReturnStmt) stmtNode()     {}
func (*BranchStmt) stmtNode()     {}
func (*BlockStmt) stmtNode()      {}
func (*IfStmt) stmtNode()         {}
func (*CaseClause) stmtNode()     {}
func (*SwitchStmt) stmtNode()     {}
func (*TypeSwitchStmt) stmtNode() {}
func (*CommClause) stmtNode()     {}
func (*SelectStmt) stmtNode()     {}
func (*ForStmt) stmtNode()        {}
func (*RangeStmt) stmtNode()      {}

// ----------------------------------------------------------------------------
// 声明

// Spec 节点表示单个（非括号化的）import、const、type 或 var 声明。
type (
	// Spec 类型代表 *ImportSpec、*ValueSpec 和 *TypeSpec 中的任何一种。
	Spec interface {
		Node
		specNode()
	}

	// ImportSpec 节点表示单个包导入。
	ImportSpec struct {
		Doc     *CommentGroup // 关联的文档；或 nil
		Name    *Ident        // 本地包名（包括 "."）；或 nil
		Path    *BasicLit     // 导入路径
		Comment *CommentGroup // 行注释；或 nil
		EndPos  token.Pos     // 规范的结束位置（如果非零则覆盖 Path.Pos）
	}

	// ValueSpec 节点表示常量或变量声明
	// （ConstSpec 或 VarSpec 产生式）。
	//
	ValueSpec struct {
		Doc     *CommentGroup // 关联的文档；或 nil
		Names   []*Ident      // 值名称（len(Names) > 0）
		Type    Expr          // 值类型；或 nil
		Values  []Expr        // 初始值；或 nil
		Comment *CommentGroup // 行注释；或 nil
	}

	// TypeSpec 节点表示类型声明（TypeSpec 产生式）。
	TypeSpec struct {
		Doc        *CommentGroup // 关联的文档；或 nil
		Name       *Ident        // 类型名
		TypeParams *FieldList    // 类型参数；或 nil
		Assign     token.Pos     // '=' 的位置（如有）
		Type       Expr          // *Ident、*ParenExpr、*SelectorExpr、*StarExpr 或任何 *XxxType
		Comment    *CommentGroup // 行注释；或 nil
	}
)

// spec 节点的 Pos 和 End 实现。

func (s *ImportSpec) Pos() token.Pos {
	if s.Name != nil {
		return s.Name.Pos()
	}
	return s.Path.Pos()
}
func (s *ValueSpec) Pos() token.Pos { return s.Names[0].Pos() }
func (s *TypeSpec) Pos() token.Pos  { return s.Name.Pos() }

func (s *ImportSpec) End() token.Pos {
	if s.EndPos != 0 {
		return s.EndPos
	}
	return s.Path.End()
}

func (s *ValueSpec) End() token.Pos {
	if n := len(s.Values); n > 0 {
		return s.Values[n-1].End()
	}
	if s.Type != nil {
		return s.Type.End()
	}
	return s.Names[len(s.Names)-1].End()
}
func (s *TypeSpec) End() token.Pos { return s.Type.End() }

// specNode() 确保只有 spec 节点可以赋值给 Spec。
func (*ImportSpec) specNode() {}
func (*ValueSpec) specNode()  {}
func (*TypeSpec) specNode()   {}

// 声明由以下声明节点之一表示。
type (
	// BadDecl 节点是包含语法错误的声明的占位符，
	// 无法为其创建正确的声明节点。
	//
	BadDecl struct {
		From, To token.Pos // 错误声明的位置范围
	}

	// GenDecl 节点（通用声明节点）表示 import、const、type 或 var 声明。
	// 有效的 Lparen 位置（Lparen.IsValid()）表示括号化的声明。
	//
	// Tok 值与 Specs 元素类型之间的关系：
	//
	//	token.IMPORT  *ImportSpec
	//	token.CONST   *ValueSpec
	//	token.TYPE    *TypeSpec
	//	token.VAR     *ValueSpec
	//
	GenDecl struct {
		Doc    *CommentGroup // 关联的文档；或 nil
		TokPos token.Pos     // Tok 的位置
		Tok    token.Token   // IMPORT、CONST、TYPE 或 VAR
		Lparen token.Pos     // '(' 的位置（如有）
		Specs  []Spec
		Rparen token.Pos // ')' 的位置（如有）
	}

	// FuncDecl 节点表示函数声明。
	FuncDecl struct {
		Doc  *CommentGroup // 关联的文档；或 nil
		Recv *FieldList    // 接收者（方法）；或 nil（函数）
		Name *Ident        // 函数/方法名
		Type *FuncType     // 函数签名：类型和值参数、结果以及 "func" 关键字的位置
		Body *BlockStmt    // 函数体；对于外部（非 Go）函数为 nil
	}
)

// 声明节点的 Pos 和 End 实现。

func (d *BadDecl) Pos() token.Pos  { return d.From }
func (d *GenDecl) Pos() token.Pos  { return d.TokPos }
func (d *FuncDecl) Pos() token.Pos { return d.Type.Pos() }

func (d *BadDecl) End() token.Pos { return d.To }
func (d *GenDecl) End() token.Pos {
	if d.Rparen.IsValid() {
		return d.Rparen + 1
	}
	return d.Specs[0].End()
}
func (d *FuncDecl) End() token.Pos {
	if d.Body != nil {
		return d.Body.End()
	}
	return d.Type.End()
}

// declNode() 确保只有声明节点可以赋值给 Decl。
func (*BadDecl) declNode()  {}
func (*GenDecl) declNode()  {}
func (*FuncDecl) declNode() {}

// ----------------------------------------------------------------------------
// 文件和包

// File 节点表示一个 Go 源文件。
//
// Comments 列表按出现顺序包含源文件中的所有注释，
// 包括通过 Doc 和 Comment 字段从其他节点指向的注释。
//
// 为了正确打印包含注释的源代码（使用 go/format 和 go/printer 包），
// 在修改 File 的语法树时必须特别注意更新注释：打印时，注释根据其位置
// 穿插在标记之间。如果语法树节点被移除或移动，其附近的相关注释也必须
// 被移除（从 [File.Comments] 列表中）或相应移动（通过更新其位置）。
// 可以使用 [CommentMap] 来简化其中一些操作。
//
// 注释是否以及如何与节点关联取决于操作程序对语法树的解释：
// 除了直接与节点关联的 Doc 和 [Comment] 注释外，
// 其余注释是"自由浮动的"（另见 issues [#18593]、[#20744]）。
//
// [#18593]: https://go.dev/issue/18593
// [#20744]: https://go.dev/issue/20744
type File struct {
	Doc     *CommentGroup // 关联的文档；或 nil
	Package token.Pos     // "package" 关键字的位置
	Name    *Ident        // 包名
	Decls   []Decl        // 顶级声明；或 nil

	FileStart, FileEnd token.Pos       // 整个文件的起始和结束位置
	Scope              *Scope          // 包作用域（仅此文件）。已弃用：参见 Object
	Imports            []*ImportSpec   // 此文件中的导入
	Unresolved         []*Ident        // 此文件中未解析的标识符。已弃用：参见 Object
	Comments           []*CommentGroup // 文件中的注释，按词法顺序
	GoVersion          string          // //go:build 或 // +build 指令要求的最低 Go 版本
}

// Pos 返回包声明的位置。
// 它可能是无效的，例如在空文件中。
//
// （使用 FileStart 获取整个文件的开始位置。它始终有效。）
func (f *File) Pos() token.Pos { return f.Package }

// End 返回文件中最后一个声明的结束位置。
// 它可能是无效的，例如在空文件中。
//
// （使用 FileEnd 获取整个文件的结束位置。它始终有效。）
func (f *File) End() token.Pos {
	if n := len(f.Decls); n > 0 {
		return f.Decls[n-1].End()
	}
	return f.Name.End()
}

// Package 节点表示共同构建一个 Go 包的一组源文件。
//
// 已弃用：改用类型检查器 [go/types]；参见 [Object]。
type Package struct {
	Name    string             // 包名
	Scope   *Scope             // 跨所有文件的包作用域
	Imports map[string]*Object // 包 id -> 包对象的映射
	Files   map[string]*File   // 按文件名索引的 Go 源文件
}

func (p *Package) Pos() token.Pos { return token.NoPos }
func (p *Package) End() token.Pos { return token.NoPos }

// IsGenerated 通过检测 https://go.dev/s/generatedcode 中描述的特殊注释，
// 报告文件是否由程序生成而非手写。
//
// 语法树必须使用 [parser.ParseComments] 标志解析。
// 示例：
//
//	f, err := parser.ParseFile(fset, filename, src, parser.ParseComments|parser.PackageClauseOnly)
//	if err != nil { ... }
//	gen := ast.IsGenerated(f)
func IsGenerated(file *File) bool {
	_, ok := generator(file)
	return ok
}

func generator(file *File) (string, bool) {
	for _, group := range file.Comments {
		for _, comment := range group.List {
			if comment.Pos() > file.Package {
				break // 在包声明之后
			}
			// 优化：先检查 包含 以避免 Split 中不必要的数组分配。
			const prefix = "// Code generated "
			if strings.包含(comment.Text, prefix) {
				for line := range strings.SplitSeq(comment.Text, "\n") {
					if rest, ok := strings.CutPrefix(line, prefix); ok {
						if gen, ok := strings.CutSuffix(rest, " DO NOT EDIT."); ok {
							return gen, true
						}
					}
				}
			}
		}
	}
	return "", false
}

// Unparen 返回移除所有外层括号后的表达式。
func Unparen(e Expr) Expr {
	for {
		paren, ok := e.(*ParenExpr)
		if !ok {
			return e
		}
		e = paren.X
	}
}
