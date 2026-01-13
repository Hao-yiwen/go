// 版权所有 2010 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// 本文件包含 AST 的打印支持。

package ast

import (
	"fmt"
	"go/token"
	"io"
	"os"
	"reflect"
)

// FieldFilter 可以提供给 [Fprint] 来控制输出。
type FieldFilter func(name string, value reflect.Value) bool

// NotNilFilter 是一个 [FieldFilter]，对于非 nil 的字段值返回 true；
// 否则返回 false。
func NotNilFilter(_ string, v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return !v.IsNil()
	}
	return true
}

// Fprint 将从 AST 节点 x 开始的（子）树打印到 w。
// 如果 fset != nil，位置信息相对于该文件集解释。
// 否则位置将作为整数值（文件集特定的偏移量）打印。
//
// 可以提供非 nil 的 [FieldFilter] f 来控制输出：
// f(fieldname, fieldvalue) 为 true 的结构体字段会被打印；
// 所有其他字段都从输出中过滤掉。未导出的结构体字段永远不会被打印。
func Fprint(w io.Writer, fset *token.FileSet, x any, f FieldFilter) error {
	return fprint(w, fset, x, f)
}

func fprint(w io.Writer, fset *token.FileSet, x any, f FieldFilter) (err error) {
	// 设置打印器
	p := printer{
		output: w,
		fset:   fset,
		filter: f,
		ptrmap: make(map[any]int),
		last:   '\n', // 强制在第一行打印行号
	}

	// 安装错误处理器
	defer func() {
		if e := recover(); e != nil {
			err = e.(localError).err // 如果不是 localError 则重新 panic
		}
	}()

	// 打印 x
	if x == nil {
		p.printf("nil\n")
		return
	}
	p.print(reflect.ValueOf(x))
	p.printf("\n")

	return
}

// Print 将 x 打印到标准输出，跳过 nil 字段。
// Print(fset, x) 等同于 Fprint(os.Stdout, fset, x, NotNilFilter)。
func Print(fset *token.FileSet, x any) error {
	return Fprint(os.Stdout, fset, x, NotNilFilter)
}

type printer struct {
	output io.Writer
	fset   *token.FileSet
	filter FieldFilter
	ptrmap map[any]int // *T -> 行号
	indent int         // 当前缩进级别
	last   byte        // Write 处理的最后一个字节
	line   int         // 当前行号
}

var indent = []byte(".  ")

func (p *printer) Write(data []byte) (n int, err error) {
	var m int
	for i, b := range data {
		// 不变式：data[0:n] 已被写入
		if b == '\n' {
			m, err = p.output.Write(data[n : i+1])
			n += m
			if err != nil {
				return
			}
			p.line++
		} else if p.last == '\n' {
			_, err = fmt.Fprintf(p.output, "%6d  ", p.line)
			if err != nil {
				return
			}
			for j := p.indent; j > 0; j-- {
				_, err = p.output.Write(indent)
				if err != nil {
					return
				}
			}
		}
		p.last = b
	}
	if len(data) > n {
		m, err = p.output.Write(data[n:])
		n += m
	}
	return
}

// localError 包装本地捕获的错误，以便我们可以将它们
// 与我们不想作为错误返回的真正 panic 区分开来。
type localError struct {
	err error
}

// printf 是一个便利包装器，处理打印错误。
func (p *printer) printf(format string, args ...any) {
	if _, err := fmt.Fprintf(p, format, args...); err != nil {
		panic(localError{err})
	}
}

// 实现说明：Print 是为 AST 节点编写的，但可以用于打印任意数据结构；
// 这样的版本可能应该放在不同的包中。
//
// 注意：此代码检测通过指针创建的（某些）循环，但不检测通过包含
// 相同切片或映射的切片或映射创建的循环。用于通用数据结构的代码
// 可能也应该捕获这些情况。

func (p *printer) print(x reflect.Value) {
	if !NotNilFilter("", x) {
		p.printf("nil")
		return
	}

	switch x.Kind() {
	case reflect.Interface:
		p.print(x.Elem())

	case reflect.Map:
		p.printf("%s (len = %d) {", x.Type(), x.Len())
		if x.Len() > 0 {
			p.indent++
			p.printf("\n")
			for _, key := range x.MapKeys() {
				p.print(key)
				p.printf(": ")
				p.print(x.MapIndex(key))
				p.printf("\n")
			}
			p.indent--
		}
		p.printf("}")

	case reflect.Pointer:
		p.printf("*")
		// 类型检查的 AST 可能包含循环 - 使用 ptrmap
		// 跟踪已经打印的对象，并打印相应的行号
		ptr := x.Interface()
		if line, exists := p.ptrmap[ptr]; exists {
			p.printf("(obj @ %d)", line)
		} else {
			p.ptrmap[ptr] = p.line
			p.print(x.Elem())
		}

	case reflect.Array:
		p.printf("%s {", x.Type())
		if x.Len() > 0 {
			p.indent++
			p.printf("\n")
			for i, n := 0, x.Len(); i < n; i++ {
				p.printf("%d: ", i)
				p.print(x.Index(i))
				p.printf("\n")
			}
			p.indent--
		}
		p.printf("}")

	case reflect.Slice:
		if s, ok := x.Interface().([]byte); ok {
			p.printf("%#q", s)
			return
		}
		p.printf("%s (len = %d) {", x.Type(), x.Len())
		if x.Len() > 0 {
			p.indent++
			p.printf("\n")
			for i, n := 0, x.Len(); i < n; i++ {
				p.printf("%d: ", i)
				p.print(x.Index(i))
				p.printf("\n")
			}
			p.indent--
		}
		p.printf("}")

	case reflect.Struct:
		t := x.Type()
		p.printf("%s {", t)
		p.indent++
		first := true
		for i, n := 0, t.NumField(); i < n; i++ {
			// 排除未导出的字段，因为无法通过反射访问它们的值
			if name := t.Field(i).Name; IsExported(name) {
				value := x.Field(i)
				if p.filter == nil || p.filter(name, value) {
					if first {
						p.printf("\n")
						first = false
					}
					p.printf("%s: ", name)
					p.print(value)
					p.printf("\n")
				}
			}
		}
		p.indent--
		p.printf("}")

	default:
		v := x.Interface()
		switch v := v.(type) {
		case string:
			// 在引号中打印字符串
			p.printf("%q", v)
			return
		case token.Pos:
			// 如果有文件集，位置值可以很好地打印
			if p.fset != nil {
				p.printf("%s", p.fset.Position(v))
				return
			}
		}
		// 默认
		p.printf("%v", v)
	}
}
