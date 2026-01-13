// 版权所有 2022 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package slog

import (
	"time"
)

// Attr 是一个键值对。
type Attr struct {
	Key   string
	Value Value
}

// String 返回一个字符串值的 Attr。
func String(key, value string) Attr {
	return Attr{key, StringValue(value)}
}

// Int64 返回一个 int64 的 Attr。
func Int64(key string, value int64) Attr {
	return Attr{key, Int64Value(value)}
}

// Int 将 int 转换为 int64 并返回具有该值的 Attr。
func Int(key string, value int) Attr {
	return Int64(key, int64(value))
}

// Uint64 返回一个 uint64 的 Attr。
func Uint64(key string, v uint64) Attr {
	return Attr{key, Uint64Value(v)}
}

// Float64 返回一个浮点数的 Attr。
func Float64(key string, v float64) Attr {
	return Attr{key, Float64Value(v)}
}

// Bool 返回一个 bool 的 Attr。
func Bool(key string, v bool) Attr {
	return Attr{key, BoolValue(v)}
}

// Time 返回一个 [time.Time] 的 Attr。
// 它会丢弃单调时钟部分。
func Time(key string, v time.Time) Attr {
	return Attr{key, TimeValue(v)}
}

// Duration 返回一个 [time.Duration] 的 Attr。
func Duration(key string, v time.Duration) Attr {
	return Attr{key, DurationValue(v)}
}

// Group 返回一个 Group [Value] 的 Attr。
// 第一个参数是键；其余参数按照 [Logger.Log] 的方式转换为 Attr。
//
// 使用 Group 在日志行中将多个键值对收集在单个键下，
// 或者作为 LogValue 的结果，以便将单个值记录为多个 Attr。
func Group(key string, args ...any) Attr {
	return Attr{key, GroupValue(argsToAttrSlice(args)...)}
}

// GroupAttrs 返回一个由给定 Attr 组成的 Group [Value] 的 Attr。
//
// GroupAttrs 是 [Group] 的更高效版本，
// 它只接受 [Attr] 值。
func GroupAttrs(key string, attrs ...Attr) Attr {
	return Attr{key, GroupValue(attrs...)}
}

func argsToAttrSlice(args []any) []Attr {
	var (
		attr  Attr
		attrs []Attr
	)
	for len(args) > 0 {
		attr, args = argsToAttr(args)
		attrs = append(attrs, attr)
	}
	return attrs
}

// Any 返回提供的值的 Attr。
// 有关如何处理值，请参阅 [AnyValue]。
func Any(key string, value any) Attr {
	return Attr{key, AnyValue(value)}
}

// Equal 报告 a 和 b 是否具有相等的键和值。
func (a Attr) Equal(b Attr) bool {
	return a.Key == b.Key && a.Value.Equal(b.Value)
}

func (a Attr) String() string {
	return a.Key + "=" + a.Value.String()
}

// isEmpty 报告 a 是否具有空键和 nil 值。
// 这可以写成 Attr{} 或 Any("", nil)。
func (a Attr) isEmpty() bool {
	return a.Key == "" && a.Value.num == 0 && a.Value.any == nil
}
