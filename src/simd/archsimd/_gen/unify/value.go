// 版权所有 2025 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package unify

import (
	"fmt"
	"iter"
	"reflect"
)

// A Value 代表由字符串、Values 的元组和字符串键映射的 Values 组成的
// 结构化的非确定性值。非确定性 Value 也会包含变量，这些变量通过
// 环境作为 [Closure] 的一部分来解析。
//
// 对于调试，Value 还可以跟踪它从输入文件中读取的源位置，
// 以及它来自其他 Values 的出处。
type Value struct {
	Domain Domain

	// A Value 有 pos 或 parents（或两者都没有）。
	pos     *Pos
	parents *[2]*Value
}

var (
	topValue    = &Value{Domain: Top{}}
	bottomValue = &Value{Domain: nil}
)

// NewValue 返回一个具有给定域且没有位置的新 [Value]。
// 信息。
func NewValue(d Domain) *Value {
	return &Value{Domain: d}
}

// NewValuePos 返回一个位置为 p 的给定域的新 [Value]。
func NewValuePos(d Domain, p Pos) *Value {
	return &Value{Domain: d, pos: &p}
}

// newValueFrom 返回一个具有给定域的新 [Value]，该域复制
// p 的位置信息。
func newValueFrom(d Domain, p *Value) *Value {
	return &Value{Domain: d, pos: p.pos, parents: p.parents}
}

func unified(d Domain, p1, p2 *Value) *Value {
	return &Value{Domain: d, parents: &[2]*Value{p1, p2}}
}

func (v *Value) Pos() Pos {
	if v.pos == nil {
		return Pos{}
	}
	return *v.pos
}

func (v *Value) PosString() string {
	var b []byte
	for root := range v.Provenance() {
		if len(b) > 0 {
			b = append(b, ' ')
		}
		b, _ = root.pos.AppendText(b)
	}
	return string(b)
}

func (v *Value) WhyNotExact() string {
	if v.Domain == nil {
		return "v.Domain is nil"
	}
	return v.Domain.WhyNotExact()
}

func (v *Value) Exact() bool {
	if v.Domain == nil {
		return false
	}
	return v.Domain.Exact()
}

// Decode 将 v 解码为 Go 值。
//
// v 必须是精确的，除了它可以包括 Top。into 必须是指针。
// [Def] 被解码为结构体。[Tuple] 被解码为切片。[String]
// 被解码为字符串或整数。任何字段本身可以是指向其中之一的指针
// 这些类型。Top 可以被解码为指针类型的字段并会将
// 字段设置为 nil。其他任何内容都会在必要时分配一个值。
//
// 任何类型都可以实现 [Decoder]，在这种情况下，它的 DecodeUnified 方法将
// 被调用，而不是使用默认解码方案。
func (v *Value) Decode(into any) error {
	rv := reflect.ValueOf(into)
	if rv.Kind() != reflect.Pointer {
		return fmt.Errorf("cannot decode into non-pointer %T", into)
	}
	return decodeReflect(v, rv.Elem())
}

func decodeReflect(v *Value, rv reflect.Value) error {
	var ptr reflect.Value
	if rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			// 透明地通过指针分配，*除了* Top，它
			// 想要将指针设置为 nil。
			//
			// TODO: 如果我切换到显式 Optional[T]，请删除此条件
			// 或将 Top 逻辑移到 Def。
			if _, ok := v.Domain.(Top); !ok {
				// 分配要填写的值，但不要实际存储它
				// 指针，直到我们成功解码。
				ptr = rv
				rv = reflect.New(rv.Type().Elem()).Elem()
			}
		} else {
			rv = rv.Elem()
		}
	}

	var err error
	if reflect.PointerTo(rv.Type()).Implements(decoderType) {
		// 使用自定义解码器。
		err = rv.Addr().Interface().(Decoder).DecodeUnified(v)
	} else {
		err = v.Domain.decode(rv)
	}
	if err == nil && ptr.IsValid() {
		ptr.Set(rv.Addr())
	}
	return err
}

// Decoder 可以由类型实现作为 [Decode] 的自定义实现
// 对于该类型。
type Decoder interface {
	DecodeUnified(v *Value) error
}

var decoderType = reflect.TypeOf((*Decoder)(nil)).Elem()

// Provenance 遍历所有贡献于这个 Value 的源 Values。
func (v *Value) Provenance() iter.Seq[*Value] {
	return func(yield func(*Value) bool) {
		var rec func(d *Value) bool
		rec = func(d *Value) bool {
			if d.pos != nil {
				if !yield(d) {
					return false
				}
			}
			if d.parents != nil {
				for _, p := range d.parents {
					if !rec(p) {
						return false
					}
				}
			}
			return true
		}
		rec(v)
	}
}
