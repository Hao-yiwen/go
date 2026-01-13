// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package template

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

// FuncMap 是定义从名称到函数映射的映射的类型。
// 每个函数必须有单个返回值或两个返回值，
// 其中第二个的类型为 error。在这种情况下，如果第二个 (error)
// 返回值在执行期间计算为非 nil，执行终止并且
// Execute 返回该错误。
//
// Execute 返回的错误包装底层错误；调用 [errors.AsType] 来
// 解包它们。
//
// 当模板执行使用参数列表调用函数时，该列表
// 必须可分配给函数的参数类型。用于应用于
// 任意类型参数的函数可以使用 interface{} 类型或
// [reflect.Value] 类型的参数。类似地，用于返回任意
// 类型结果的函数可以返回 interface{} 或 [reflect.Value]。
type FuncMap map[string]any

// builtins 返回 FuncMap。
// 它不是一个全局变量，所以链接器可以更多地进行死代码消除
// 当这个不被调用时。参见 golang.org/issue/36021。
// TODO: 一旦 golang.org/issue/2559 被修复，将其恢复为全局映射。
func builtins() FuncMap {
	return FuncMap{
		"and":      and,
		"call":     emptyCall,
		"html":     HTMLEscaper,
		"index":    index,
		"slice":    slice,
		"js":       JSEscaper,
		"len":      length,
		"not":      not,
		"or":       or,
		"print":    fmt.Sprint,
		"printf":   fmt.Sprintf,
		"println":  fmt.Sprintln,
		"urlquery": URLQueryEscaper,

		// Comparisons
		"eq": eq, // ==
		"ge": ge, // >=
		"gt": gt, // >
		"le": le, // <=
		"lt": lt, // <
		"ne": ne, // !=
	}
}

// builtinFuncs 延迟计算并缓存 builtinFuncs 映射。
var builtinFuncs = sync.OnceValue(func() map[string]reflect.Value {
	funcMap := builtins()
	m := make(map[string]reflect.Value, len(funcMap))
	addValueFuncs(m, funcMap)
	return m
})

// addValueFuncs 将 funcs 中的函数添加到值中，将它们转换为 reflect.Values。
func addValueFuncs(out map[string]reflect.Value, in FuncMap) {
	for name, fn := range in {
		if !goodName(name) {
			panic(fmt.Errorf("function name %q is not a valid identifier", name))
		}
		v := reflect.ValueOf(fn)
		if v.Kind() != reflect.Func {
			panic("value for " + name + " not a function")
		}
		if err := goodFunc(name, v.Type()); err != nil {
			panic(err)
		}
		out[name] = v
	}
}

// addFuncs 将 funcs 中的函数添加到值中。它不对输入进行检查 -
// 首先调用 addValueFuncs。
func addFuncs(out, in FuncMap) {
	for name, fn := range in {
		out[name] = fn
	}
}

// goodFunc 报告函数或方法是否有正确的结果签名。
func goodFunc(name string, typ reflect.Type) error {
	// 我们允许有 1 个结果或 2 个结果的函数，其中第二个是 error。
	switch numOut := typ.NumOut(); {
	case numOut == 1:
		return nil
	case numOut == 2 && typ.Out(1) == errorType:
		return nil
	case numOut == 2:
		return fmt.Errorf("invalid function signature for %s: second return value should be error; is %s", name, typ.Out(1))
	default:
		return fmt.Errorf("function %s has %d return values; should be 1 or 2", name, typ.NumOut())
	}
}

// goodName 报告函数名称是否是有效的标识符。
func goodName(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		switch {
		case r == '_':
		case i == 0 && !unicode.IsLetter(r):
			return false
		case !unicode.IsLetter(r) && !unicode.IsDigit(r):
			return false
		}
	}
	return true
}

// findFunction 在模板和全局映射中查找函数。
func findFunction(name string, tmpl *Template) (v reflect.Value, isBuiltin, ok bool) {
	if tmpl != nil && tmpl.common != nil {
		tmpl.muFuncs.RLock()
		defer tmpl.muFuncs.RUnlock()
		if fn := tmpl.execFuncs[name]; fn.IsValid() {
			return fn, false, true
		}
	}
	if fn := builtinFuncs()[name]; fn.IsValid() {
		return fn, true, true
	}
	return reflect.Value{}, false, false
}

// prepareArg 检查值是否可用作 argType 类型的参数，并且
// 如果可能的话，将无效值转换为适当的零值。
func prepareArg(value reflect.Value, argType reflect.Type) (reflect.Value, error) {
	if !value.IsValid() {
		if !canBeNil(argType) {
			return reflect.Value{}, fmt.Errorf("value is nil; should be of type %s", argType)
		}
		value = reflect.Zero(argType)
	}
	if value.Type().AssignableTo(argType) {
		return value, nil
	}
	if intLike(value.Kind()) && intLike(argType.Kind()) && value.Type().ConvertibleTo(argType) {
		value = value.Convert(argType)
		return value, nil
	}
	return reflect.Value{}, fmt.Errorf("value has type %s; should be %s", value.Type(), argType)
}

func intLike(typ reflect.Kind) bool {
	switch typ {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return true
	}
	return false
}

// indexArg 检查 reflect.Value 是否可用作索引，如果可能的话将其转换为 int。
func indexArg(index reflect.Value, cap int) (int, error) {
	var x int64
	switch index.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		x = index.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		x = int64(index.Uint())
	case reflect.Invalid:
		return 0, fmt.Errorf("cannot index slice/array with nil")
	default:
		return 0, fmt.Errorf("cannot index slice/array with type %s", index.Type())
	}
	if x < 0 || int(x) < 0 || int(x) > cap {
		return 0, fmt.Errorf("index out of range: %d", x)
	}
	return int(x), nil
}

// 索引。

// index 返回通过后续参数对其第一个参数进行索引的结果。
// 因此"index x 1 2 3"在 Go 语法中是 x[1][2][3]。每个
// 被索引的项必须是映射、切片或数组。
func index(item reflect.Value, indexes ...reflect.Value) (reflect.Value, error) {
	item = indirectInterface(item)
	if !item.IsValid() {
		return reflect.Value{}, fmt.Errorf("index of untyped nil")
	}
	for _, index := range indexes {
		index = indirectInterface(index)
		var isNil bool
		if item, isNil = indirect(item); isNil {
			return reflect.Value{}, fmt.Errorf("index of nil pointer")
		}
		switch item.Kind() {
		case reflect.Array, reflect.Slice, reflect.String:
			x, err := indexArg(index, item.Len())
			if err != nil {
				return reflect.Value{}, err
			}
			item = item.Index(x)
		case reflect.Map:
			index, err := prepareArg(index, item.Type().Key())
			if err != nil {
				return reflect.Value{}, err
			}
			if x := item.MapIndex(index); x.IsValid() {
				item = x
			} else {
				item = reflect.Zero(item.Type().Elem())
			}
		case reflect.Invalid:
			// 循环保持不变：item.IsValid()
			panic("unreachable")
		default:
			return reflect.Value{}, fmt.Errorf("can't index item of type %s", item.Type())
		}
	}
	return item, nil
}

// 切片。

// slice 返回通过剩余参数对其第一个参数进行切片的结果。
// 因此"slice x 1 2"在 Go 语法中是 x[1:2]，而"slice x"
// 是 x[:]，"slice x 1"是 x[1:]，"slice x 1 2 3"是 x[1:2:3]。第一个
// 参数必须是字符串、切片或数组。
func slice(item reflect.Value, indexes ...reflect.Value) (reflect.Value, error) {
	item = indirectInterface(item)
	if !item.IsValid() {
		return reflect.Value{}, fmt.Errorf("slice of untyped nil")
	}
	var isNil bool
	if item, isNil = indirect(item); isNil {
		return reflect.Value{}, fmt.Errorf("slice of nil pointer")
	}
	if len(indexes) > 3 {
		return reflect.Value{}, fmt.Errorf("too many slice indexes: %d", len(indexes))
	}
	var cap int
	switch item.Kind() {
	case reflect.String:
		if len(indexes) == 3 {
			return reflect.Value{}, fmt.Errorf("cannot 3-index slice a string")
		}
		cap = item.Len()
	case reflect.Array, reflect.Slice:
		cap = item.Cap()
	default:
		return reflect.Value{}, fmt.Errorf("can't slice item of type %s", item.Type())
	}
	// 为情况 item[:]、item[i:] 设置默认值。
	idx := [3]int{0, item.Len()}
	for i, index := range indexes {
		x, err := indexArg(index, cap)
		if err != nil {
			return reflect.Value{}, err
		}
		idx[i] = x
	}
	// 给定 item[i:j]，确保 i <= j。
	if idx[0] > idx[1] {
		return reflect.Value{}, fmt.Errorf("invalid slice index: %d > %d", idx[0], idx[1])
	}
	if len(indexes) < 3 {
		return item.Slice(idx[0], idx[1]), nil
	}
	// 给定 item[i:j:k]，确保 i <= j <= k。
	if idx[1] > idx[2] {
		return reflect.Value{}, fmt.Errorf("invalid slice index: %d > %d", idx[1], idx[2])
	}
	return item.Slice3(idx[0], idx[1], idx[2]), nil
}

// 长度

// length 返回项的长度，如果其没有定义的长度，则返回错误。
func length(item reflect.Value) (int, error) {
	item, isNil := indirect(item)
	if isNil {
		return 0, fmt.Errorf("len of nil pointer")
	}
	switch item.Kind() {
	case reflect.Array, reflect.Chan, reflect.Map, reflect.Slice, reflect.String:
		return item.Len(), nil
	}
	return 0, fmt.Errorf("len of type %s", item.Type())
}

// 函数调用

func emptyCall(fn reflect.Value, args ...reflect.Value) reflect.Value {
	panic("unreachable") // 在 evalCall 中作为特殊情况实现
}

// call 返回将第一个参数评估为函数的结果。
// 该函数必须返回 1 个结果或 2 个结果，其中第二个是 error。
func call(name string, fn reflect.Value, args ...reflect.Value) (reflect.Value, error) {
	fn = indirectInterface(fn)
	if !fn.IsValid() {
		return reflect.Value{}, fmt.Errorf("call of nil")
	}
	typ := fn.Type()
	if typ.Kind() != reflect.Func {
		return reflect.Value{}, fmt.Errorf("non-function %s of type %s", name, typ)
	}

	if err := goodFunc(name, typ); err != nil {
		return reflect.Value{}, err
	}
	numIn := typ.NumIn()
	var dddType reflect.Type
	if typ.IsVariadic() {
		if len(args) < numIn-1 {
			return reflect.Value{}, fmt.Errorf("wrong number of args for %s: got %d want at least %d", name, len(args), numIn-1)
		}
		dddType = typ.In(numIn - 1).Elem()
	} else {
		if len(args) != numIn {
			return reflect.Value{}, fmt.Errorf("wrong number of args for %s: got %d want %d", name, len(args), numIn)
		}
	}
	argv := make([]reflect.Value, len(args))
	for i, arg := range args {
		arg = indirectInterface(arg)
		// 计算预期类型。因为变长参数而复杂。
		argType := dddType
		if !typ.IsVariadic() || i < numIn-1 {
			argType = typ.In(i)
		}

		var err error
		if argv[i], err = prepareArg(arg, argType); err != nil {
			return reflect.Value{}, fmt.Errorf("arg %d: %w", i, err)
		}
	}
	return safeCall(fn, argv)
}

// safeCall 运行 fun.Call(args)，并返回结果值和错误（如果有）。
// 如果调用 panic，panic 值作为错误返回。
func safeCall(fun reflect.Value, args []reflect.Value) (val reflect.Value, err error) {
	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(error); ok {
				err = e
			} else {
				err = fmt.Errorf("%v", r)
			}
		}
	}()
	ret := fun.Call(args)
	if len(ret) == 2 && !ret[1].IsNil() {
		return ret[0], ret[1].Interface().(error)
	}
	return ret[0], nil
}

// 布尔逻辑。

func truth(arg reflect.Value) bool {
	t, _ := isTrue(indirectInterface(arg))
	return t
}

// and 计算其参数的布尔 AND，返回
// 它遇到的第一个 false 参数或最后一个参数。
func and(arg0 reflect.Value, args ...reflect.Value) reflect.Value {
	panic("unreachable") // 在 evalCall 中作为特殊情况实现
}

// or 计算其参数的布尔 OR，返回
// 它遇到的第一个 true 参数或最后一个参数。
func or(arg0 reflect.Value, args ...reflect.Value) reflect.Value {
	panic("unreachable") // 在 evalCall 中作为特殊情况实现
}

// not 返回其参数的布尔否定。
func not(arg reflect.Value) bool {
	return !truth(arg)
}

// 比较。

// TODO: 也许允许有符号和无符号整数之间的比较。

var (
	errBadComparisonType = errors.New("invalid type for comparison")
	errNoComparison      = errors.New("missing argument for comparison")
)

type kind int

const (
	invalidKind kind = iota
	boolKind
	complexKind
	intKind
	floatKind
	stringKind
	uintKind
)

func basicKind(v reflect.Value) (kind, error) {
	switch v.Kind() {
	case reflect.Bool:
		return boolKind, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return intKind, nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return uintKind, nil
	case reflect.Float32, reflect.Float64:
		return floatKind, nil
	case reflect.Complex64, reflect.Complex128:
		return complexKind, nil
	case reflect.String:
		return stringKind, nil
	}
	return invalidKind, errBadComparisonType
}

// isNil returns true if v is the zero reflect.Value, or nil of its type.
func isNil(v reflect.Value) bool {
	if !v.IsValid() {
		return true
	}
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	}
	return false
}

// canCompare reports whether v1 and v2 are both the same kind, or one is nil.
// Called only when dealing with nillable types, or there's about to be an error.
func canCompare(v1, v2 reflect.Value) bool {
	k1 := v1.Kind()
	k2 := v2.Kind()
	if k1 == k2 {
		return true
	}
	// We know the type can be compared to nil.
	return k1 == reflect.Invalid || k2 == reflect.Invalid
}

// eq evaluates the comparison a == b || a == c || ...
func eq(arg1 reflect.Value, arg2 ...reflect.Value) (bool, error) {
	arg1 = indirectInterface(arg1)
	if len(arg2) == 0 {
		return false, errNoComparison
	}
	k1, _ := basicKind(arg1)
	for _, arg := range arg2 {
		arg = indirectInterface(arg)
		k2, _ := basicKind(arg)
		truth := false
		if k1 != k2 {
			// Special case: Can compare integer values regardless of type's sign.
			switch {
			case k1 == intKind && k2 == uintKind:
				truth = arg1.Int() >= 0 && uint64(arg1.Int()) == arg.Uint()
			case k1 == uintKind && k2 == intKind:
				truth = arg.Int() >= 0 && arg1.Uint() == uint64(arg.Int())
			default:
				if arg1.IsValid() && arg.IsValid() {
					return false, fmt.Errorf("incompatible types for comparison: %v and %v", arg1.Type(), arg.Type())
				}
			}
		} else {
			switch k1 {
			case boolKind:
				truth = arg1.Bool() == arg.Bool()
			case complexKind:
				truth = arg1.Complex() == arg.Complex()
			case floatKind:
				truth = arg1.Float() == arg.Float()
			case intKind:
				truth = arg1.Int() == arg.Int()
			case stringKind:
				truth = arg1.String() == arg.String()
			case uintKind:
				truth = arg1.Uint() == arg.Uint()
			default:
				if !canCompare(arg1, arg) {
					return false, fmt.Errorf("non-comparable types %s: %v, %s: %v", arg1, arg1.Type(), arg.Type(), arg)
				}
				if isNil(arg1) || isNil(arg) {
					truth = isNil(arg) == isNil(arg1)
				} else {
					if !arg.Type().Comparable() {
						return false, fmt.Errorf("non-comparable type %s: %v", arg, arg.Type())
					}
					truth = arg1.Interface() == arg.Interface()
				}
			}
		}
		if truth {
			return true, nil
		}
	}
	return false, nil
}

// ne evaluates the comparison a != b.
func ne(arg1, arg2 reflect.Value) (bool, error) {
	// != is the inverse of ==.
	equal, err := eq(arg1, arg2)
	return !equal, err
}

// lt evaluates the comparison a < b.
func lt(arg1, arg2 reflect.Value) (bool, error) {
	arg1 = indirectInterface(arg1)
	k1, err := basicKind(arg1)
	if err != nil {
		return false, err
	}
	arg2 = indirectInterface(arg2)
	k2, err := basicKind(arg2)
	if err != nil {
		return false, err
	}
	truth := false
	if k1 != k2 {
		// Special case: Can compare integer values regardless of type's sign.
		switch {
		case k1 == intKind && k2 == uintKind:
			truth = arg1.Int() < 0 || uint64(arg1.Int()) < arg2.Uint()
		case k1 == uintKind && k2 == intKind:
			truth = arg2.Int() >= 0 && arg1.Uint() < uint64(arg2.Int())
		default:
			return false, fmt.Errorf("incompatible types for comparison: %v and %v", arg1.Type(), arg2.Type())
		}
	} else {
		switch k1 {
		case boolKind, complexKind:
			return false, errBadComparisonType
		case floatKind:
			truth = arg1.Float() < arg2.Float()
		case intKind:
			truth = arg1.Int() < arg2.Int()
		case stringKind:
			truth = arg1.String() < arg2.String()
		case uintKind:
			truth = arg1.Uint() < arg2.Uint()
		default:
			panic("invalid kind")
		}
	}
	return truth, nil
}

// le evaluates the comparison <= b.
func le(arg1, arg2 reflect.Value) (bool, error) {
	// <= is < or ==.
	lessThan, err := lt(arg1, arg2)
	if lessThan || err != nil {
		return lessThan, err
	}
	return eq(arg1, arg2)
}

// gt evaluates the comparison a > b.
func gt(arg1, arg2 reflect.Value) (bool, error) {
	// > is the inverse of <=.
	lessOrEqual, err := le(arg1, arg2)
	if err != nil {
		return false, err
	}
	return !lessOrEqual, nil
}

// ge evaluates the comparison a >= b.
func ge(arg1, arg2 reflect.Value) (bool, error) {
	// >= is the inverse of <.
	lessThan, err := lt(arg1, arg2)
	if err != nil {
		return false, err
	}
	return !lessThan, nil
}

// HTML escaping.

var (
	htmlQuot = []byte("&#34;") // shorter than "&quot;"
	htmlApos = []byte("&#39;") // shorter than "&apos;" and apos was not in HTML until HTML5
	htmlAmp  = []byte("&amp;")
	htmlLt   = []byte("&lt;")
	htmlGt   = []byte("&gt;")
	htmlNull = []byte("\uFFFD")
)

// HTMLEscape writes to w the escaped HTML equivalent of the plain text data b.
func HTMLEscape(w io.Writer, b []byte) {
	last := 0
	for i, c := range b {
		var html []byte
		switch c {
		case '\000':
			html = htmlNull
		case '"':
			html = htmlQuot
		case '\'':
			html = htmlApos
		case '&':
			html = htmlAmp
		case '<':
			html = htmlLt
		case '>':
			html = htmlGt
		default:
			continue
		}
		w.Write(b[last:i])
		w.Write(html)
		last = i + 1
	}
	w.Write(b[last:])
}

// HTMLEscapeString returns the escaped HTML equivalent of the plain text data s.
func HTMLEscapeString(s string) string {
	// Avoid allocation if we can.
	if !strings.ContainsAny(s, "'\"&<>\000") {
		return s
	}
	var b strings.Builder
	HTMLEscape(&b, []byte(s))
	return b.String()
}

// HTMLEscaper returns the escaped HTML equivalent of the textual
// representation of its arguments.
func HTMLEscaper(args ...any) string {
	return HTMLEscapeString(evalArgs(args))
}

// JavaScript escaping.

var (
	jsLowUni = []byte(`\u00`)
	hex      = []byte("0123456789ABCDEF")

	jsBackslash = []byte(`\\`)
	jsApos      = []byte(`\'`)
	jsQuot      = []byte(`\"`)
	jsLt        = []byte(`\u003C`)
	jsGt        = []byte(`\u003E`)
	jsAmp       = []byte(`\u0026`)
	jsEq        = []byte(`\u003D`)
)

// JSEscape writes to w the escaped JavaScript equivalent of the plain text data b.
func JSEscape(w io.Writer, b []byte) {
	last := 0
	for i := 0; i < len(b); i++ {
		c := b[i]

		if !jsIsSpecial(rune(c)) {
			// fast path: nothing to do
			continue
		}
		w.Write(b[last:i])

		if c < utf8.RuneSelf {
			// Quotes, slashes and angle brackets get quoted.
			// Control characters get written as \u00XX.
			switch c {
			case '\\':
				w.Write(jsBackslash)
			case '\'':
				w.Write(jsApos)
			case '"':
				w.Write(jsQuot)
			case '<':
				w.Write(jsLt)
			case '>':
				w.Write(jsGt)
			case '&':
				w.Write(jsAmp)
			case '=':
				w.Write(jsEq)
			default:
				w.Write(jsLowUni)
				t, b := c>>4, c&0x0f
				w.Write(hex[t : t+1])
				w.Write(hex[b : b+1])
			}
		} else {
			// Unicode rune.
			r, size := utf8.DecodeRune(b[i:])
			if unicode.IsPrint(r) {
				w.Write(b[i : i+size])
			} else {
				fmt.Fprintf(w, "\\u%04X", r)
			}
			i += size - 1
		}
		last = i + 1
	}
	w.Write(b[last:])
}

// JSEscapeString returns the escaped JavaScript equivalent of the plain text data s.
func JSEscapeString(s string) string {
	// Avoid allocation if we can.
	if strings.IndexFunc(s, jsIsSpecial) < 0 {
		return s
	}
	var b strings.Builder
	JSEscape(&b, []byte(s))
	return b.String()
}

func jsIsSpecial(r rune) bool {
	switch r {
	case '\\', '\'', '"', '<', '>', '&', '=':
		return true
	}
	return r < ' ' || utf8.RuneSelf <= r
}

// JSEscaper returns the escaped JavaScript equivalent of the textual
// representation of its arguments.
func JSEscaper(args ...any) string {
	return JSEscapeString(evalArgs(args))
}

// URLQueryEscaper returns the escaped value of the textual representation of
// its arguments in a form suitable for embedding in a URL query.
func URLQueryEscaper(args ...any) string {
	return url.QueryEscape(evalArgs(args))
}

// evalArgs formats the list of arguments into a string. It is therefore equivalent to
//
//	fmt.Sprint(args...)
//
// except that each argument is indirected (if a pointer), as required,
// using the same rules as the default string evaluation during template
// execution.
func evalArgs(args []any) string {
	ok := false
	var s string
	// Fast path for simple common case.
	if len(args) == 1 {
		s, ok = args[0].(string)
	}
	if !ok {
		for i, arg := range args {
			a, ok := printableValue(reflect.ValueOf(arg))
			if ok {
				args[i] = a
			} // else let fmt do its thing
		}
		s = fmt.Sprint(args...)
	}
	return s
}
