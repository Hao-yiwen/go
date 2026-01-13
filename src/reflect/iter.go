// 版权所有 2024 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package reflect

import (
	"iter"
)

func rangeNum[T int8 | int16 | int32 | int64 | int |
	uint8 | uint16 | uint32 | uint64 | uint |
	uintptr, N int64 | uint64](num N, t Type) iter.Seq[Value] {
	return func(yield func(v Value) bool) {
		convert := t.PkgPath() != ""
		// 不能使用 range T(v)，因为没有核心类型。
		for i := T(0); i < T(num); i++ {
			tmp := ValueOf(i)
			// 如果迭代值类型是由 type T 内置类型定义的。
			if convert {
				tmp = tmp.Convert(t)
			}
			if !yield(tmp) {
				return
			}
		}
	}
}

// Seq 返回一个 iter.Seq[Value]，遍历 v 的元素。
// 如果 v 的 kind 是 Func，它必须是一个没有返回值的函数，
// 并且接受一个类型为 func(T) bool 的单一参数（T 是某种类型）。
// 如果 v 的 kind 是 Pointer，指针元素类型必须是 Array 类型。
// 否则 v 的 kind 必须是 Int、Int8、Int16、Int32、Int64、
// Uint、Uint8、Uint16、Uint32、Uint64、Uintptr、
// Array、Chan、Map、Slice 或 String。
func (v Value) Seq() iter.Seq[Value] {
	if canRangeFunc(v.abiType()) {
		return func(yield func(Value) bool) {
			rf := MakeFunc(v.Type().In(0), func(in []Value) []Value {
				return []Value{ValueOf(yield(in[0]))}
			})
			v.Call([]Value{rf})
		}
	}
	switch v.kind() {
	case Int:
		return rangeNum[int](v.Int(), v.Type())
	case Int8:
		return rangeNum[int8](v.Int(), v.Type())
	case Int16:
		return rangeNum[int16](v.Int(), v.Type())
	case Int32:
		return rangeNum[int32](v.Int(), v.Type())
	case Int64:
		return rangeNum[int64](v.Int(), v.Type())
	case Uint:
		return rangeNum[uint](v.Uint(), v.Type())
	case Uint8:
		return rangeNum[uint8](v.Uint(), v.Type())
	case Uint16:
		return rangeNum[uint16](v.Uint(), v.Type())
	case Uint32:
		return rangeNum[uint32](v.Uint(), v.Type())
	case Uint64:
		return rangeNum[uint64](v.Uint(), v.Type())
	case Uintptr:
		return rangeNum[uintptr](v.Uint(), v.Type())
	case Pointer:
		if v.Elem().kind() != Array {
			break
		}
		return func(yield func(Value) bool) {
			v = v.Elem()
			for i := range v.Len() {
				if !yield(ValueOf(i)) {
					return
				}
			}
		}
	case Array, Slice:
		return func(yield func(Value) bool) {
			for i := range v.Len() {
				if !yield(ValueOf(i)) {
					return
				}
			}
		}
	case String:
		return func(yield func(Value) bool) {
			for i := range v.String() {
				if !yield(ValueOf(i)) {
					return
				}
			}
		}
	case Map:
		return func(yield func(Value) bool) {
			i := v.MapRange()
			for i.Next() {
				if !yield(i.Key()) {
					return
				}
			}
		}
	case Chan:
		return func(yield func(Value) bool) {
			for value, ok := v.Recv(); ok; value, ok = v.Recv() {
				if !yield(value) {
					return
				}
			}
		}
	}
	panic("reflect: " + v.Type().String() + " cannot produce iter.Seq[Value]")
}

// Seq2 返回一个 iter.Seq2[Value, Value]，遍历 v 的元素。
// 如果 v 的 kind 是 Func，它必须是一个没有返回值的函数，
// 并且接受一个类型为 func(K, V) bool 的单一参数（K、V 是某种类型）。
// 如果 v 的 kind 是 Pointer，指针元素类型必须是 Array 类型。
// 否则 v 的 kind 必须是 Array、Map、Slice 或 String。
func (v Value) Seq2() iter.Seq2[Value, Value] {
	if canRangeFunc2(v.abiType()) {
		return func(yield func(Value, Value) bool) {
			rf := MakeFunc(v.Type().In(0), func(in []Value) []Value {
				return []Value{ValueOf(yield(in[0], in[1]))}
			})
			v.Call([]Value{rf})
		}
	}
	switch v.Kind() {
	case Pointer:
		if v.Elem().kind() != Array {
			break
		}
		return func(yield func(Value, Value) bool) {
			v = v.Elem()
			for i := range v.Len() {
				if !yield(ValueOf(i), v.Index(i)) {
					return
				}
			}
		}
	case Array, Slice:
		return func(yield func(Value, Value) bool) {
			for i := range v.Len() {
				if !yield(ValueOf(i), v.Index(i)) {
					return
				}
			}
		}
	case String:
		return func(yield func(Value, Value) bool) {
			for i, v := range v.String() {
				if !yield(ValueOf(i), ValueOf(v)) {
					return
				}
			}
		}
	case Map:
		return func(yield func(Value, Value) bool) {
			i := v.MapRange()
			for i.Next() {
				if !yield(i.Key(), i.Value()) {
					return
				}
			}
		}
	}
	panic("reflect: " + v.Type().String() + " cannot produce iter.Seq2[Value, Value]")
}
