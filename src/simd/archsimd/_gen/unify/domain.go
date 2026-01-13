// 版权所有 2025 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package unify

import (
	"fmt"
	"iter"
	"maps"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// A Domain 是一个非空的值集合，都是相同类型的。
//
// Domain 可以是标量：
//
//   - [String] - 表示字符串类型的值。
//
// 或复合：
//
//   - [Def] - 从固定键到 [Domain] 的映射。
//
//   - [Tuple] - 固定长度的 [Domain] 序列或
//     重复 [Domain] 的所有可能长度。
//
// 或顶或底：
//
//   - [Top] - 表示所有可能类型的所有可能值。
//
//   - nil - 表示没有值。
//
// 或变量：
//
//   - [Var] - 在环境中捕获的值。
type Domain interface {
	Exact() bool
	WhyNotExact() string

	// decode 将此值存储在 Go 值中。如果此值不精确，
	// 返回一个可能包装的 *inexactError。
	decode(reflect.Value) error
}

type inexactError struct {
	valueType string
	goType    string
}

func (e *inexactError) Error() string {
	return fmt.Sprintf("cannot store inexact %s value in %s", e.valueType, e.goType)
}

type decodeError struct {
	path string
	err  error
}

func newDecodeError(path string, err error) *decodeError {
	if err, ok := err.(*decodeError); ok {
		return &decodeError{path: path + "." + err.path, err: err.err}
	}
	return &decodeError{path: path, err: err}
}

func (e *decodeError) Unwrap() error {
	return e.err
}

func (e *decodeError) Error() string {
	return fmt.Sprintf("%s: %s", e.path, e.err)
}

// Top 表示所有可能类型的所有可能值。
type Top struct{}

func (t Top) Exact() bool         { return false }
func (t Top) WhyNotExact() string { return "is top" }

func (t Top) decode(rv reflect.Value) error {
	// 我们可以将 Top 解码为指针类型值为 nil。
	if rv.Kind() != reflect.Pointer {
		return &inexactError{"top", rv.Type().String()}
	}
	rv.SetZero()
	return nil
}

// A Def 是从字段名到 [Value] 的映射。任何未明确
// 列出的字段都有 [Value] [Top]。
type Def struct {
	fields map[string]*Value
}

// A DefBuilder 一次一个字段地构建 [Def]。零值是空的
// [Def]。
type DefBuilder struct {
	fields map[string]*Value
}

func (b *DefBuilder) Add(name string, v *Value) {
	if b.fields == nil {
		b.fields = make(map[string]*Value)
	}
	if old, ok := b.fields[name]; ok {
		panic(fmt.Sprintf("duplicate field %q, added value is %v, old value is %v", name, v, old))
	}
	b.fields[name] = v
}

// Build 从添加到此生成器的字段构建 [Def]。
func (b *DefBuilder) Build() Def {
	return Def{maps.Clone(b.fields)}
}

// Exact 如果所有字段值都精确，则返回 true。
func (d Def) Exact() bool {
	for _, v := range d.fields {
		if !v.Exact() {
			return false
		}
	}
	return true
}

// WhyNotExact 返回值不精确的原因。
func (d Def) WhyNotExact() string {
	for s, v := range d.fields {
		if !v.Exact() {
			w := v.WhyNotExact()
			return "field " + s + ": " + w
		}
	}
	return ""
}

func (d Def) decode(rv reflect.Value) error {
	if rv.Kind() != reflect.Struct {
		return fmt.Errorf("cannot decode Def into %s", rv.Type())
	}

	var lowered map[string]string // Lower case -> canonical for d.fields.
	rt := rv.Type()
	for fi := range rv.NumField() {
		fType := rt.Field(fi)
		if fType.PkgPath != "" {
			continue
		}
		v := d.fields[fType.Name]
		if v == nil {
			v = topValue

			// Try a case-insensitive match
			canon, ok := d.fields[strings.ToLower(fType.Name)]
			if ok {
				v = canon
			} else {
				if lowered == nil {
					lowered = make(map[string]string, len(d.fields))
					for k := range d.fields {
						l := strings.ToLower(k)
						if k != l {
							lowered[l] = k
						}
					}
				}
				canon, ok := lowered[strings.ToLower(fType.Name)]
				if ok {
					v = d.fields[canon]
				}
			}
		}
		if err := decodeReflect(v, rv.Field(fi)); err != nil {
			return newDecodeError(fType.Name, err)
		}
	}
	return nil
}

func (d Def) keys() []string {
	return slices.Sorted(maps.Keys(d.fields))
}

func (d Def) All() iter.Seq2[string, *Value] {
	// TODO: We call All fairly often. It's probably bad to sort this every
	// time.
	keys := slices.Sorted(maps.Keys(d.fields))
	return func(yield func(string, *Value) bool) {
		for _, k := range keys {
			if !yield(k, d.fields[k]) {
				return
			}
		}
	}
}

// A Tuple is a sequence of Values in one of two forms: 1. a fixed-length tuple,
// where each Value can be different or 2. a "repeated tuple", which is a Value
// repeated 0 or more times.
type Tuple struct {
	vs []*Value

	// repeat, if non-nil, means this Tuple consists of an element repeated 0 or
	// more times. If repeat is non-nil, vs must be nil. This is a generator
	// function because we don't necessarily want *exactly* the same Value
	// repeated. For example, in YAML encoding, a !sum in a repeated tuple needs
	// a fresh variable in each instance.
	repeat []func(envSet) (*Value, envSet)
}

func NewTuple(vs ...*Value) Tuple {
	return Tuple{vs: vs}
}

func NewRepeat(gens ...func(envSet) (*Value, envSet)) Tuple {
	return Tuple{repeat: gens}
}

func (d Tuple) Exact() bool {
	if d.repeat != nil {
		return false
	}
	for _, v := range d.vs {
		if !v.Exact() {
			return false
		}
	}
	return true
}

func (d Tuple) WhyNotExact() string {
	if d.repeat != nil {
		return "d.repeat is not nil"
	}
	for i, v := range d.vs {
		if !v.Exact() {
			w := v.WhyNotExact()
			return "index " + strconv.FormatInt(int64(i), 10) + ": " + w
		}
	}
	return ""
}

func (d Tuple) decode(rv reflect.Value) error {
	if d.repeat != nil {
		return &inexactError{"repeated tuple", rv.Type().String()}
	}
	// TODO: We could also do arrays.
	if rv.Kind() != reflect.Slice {
		return fmt.Errorf("cannot decode Tuple into %s", rv.Type())
	}
	if rv.IsNil() || rv.Cap() < len(d.vs) {
		rv.Set(reflect.MakeSlice(rv.Type(), len(d.vs), len(d.vs)))
	} else {
		rv.SetLen(len(d.vs))
	}
	for i, v := range d.vs {
		if err := decodeReflect(v, rv.Index(i)); err != nil {
			return newDecodeError(fmt.Sprintf("%d", i), err)
		}
	}
	return nil
}

// A String represents a set of strings. It can represent the intersection of a
// set of regexps, or a single exact string. In general, the domain of a String
// is non-empty, but we do not attempt to prove emptiness of a regexp value.
type String struct {
	kind  stringKind
	re    []*regexp.Regexp // Intersection of regexps
	exact string
}

type stringKind int

const (
	stringRegex stringKind = iota
	stringExact
)

func NewStringRegex(exprs ...string) (String, error) {
	if len(exprs) == 0 {
		exprs = []string{""}
	}
	v := String{kind: -1}
	for _, expr := range exprs {
		if expr == "" {
			// Skip constructing the regexp. It won't have a "literal prefix"
			// and so we wind up thinking this is a regexp instead of an exact
			// (empty) string.
			v = String{kind: stringExact, exact: ""}
			continue
		}

		re, err := regexp.Compile(`\A(?:` + expr + `)\z`)
		if err != nil {
			return String{}, fmt.Errorf("parsing value: %s", err)
		}

		// An exact value narrows the whole domain to exact, so we're done, but
		// should keep parsing.
		if v.kind == stringExact {
			continue
		}

		if exact, complete := re.LiteralPrefix(); complete {
			v = String{kind: stringExact, exact: exact}
		} else {
			v.kind = stringRegex
			v.re = append(v.re, re)
		}
	}
	return v, nil
}

func NewStringExact(s string) String {
	return String{kind: stringExact, exact: s}
}

// Exact returns whether this Value is known to consist of a single string.
func (d String) Exact() bool {
	return d.kind == stringExact
}

func (d String) WhyNotExact() string {
	if d.kind == stringExact {
		return ""
	}
	return "string is not exact"
}

func (d String) decode(rv reflect.Value) error {
	if d.kind != stringExact {
		return &inexactError{"regex", rv.Type().String()}
	}
	switch rv.Kind() {
	default:
		return fmt.Errorf("cannot decode String into %s", rv.Type())
	case reflect.String:
		rv.SetString(d.exact)
	case reflect.Int:
		i, err := strconv.Atoi(d.exact)
		if err != nil {
			return fmt.Errorf("cannot decode String into %s: %s", rv.Type(), err)
		}
		rv.SetInt(int64(i))
	case reflect.Bool:
		b, err := strconv.ParseBool(d.exact)
		if err != nil {
			return fmt.Errorf("cannot decode String into %s: %s", rv.Type(), err)
		}
		rv.SetBool(b)
	}
	return nil
}
