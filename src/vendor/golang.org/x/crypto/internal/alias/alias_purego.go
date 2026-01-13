// 版权所有 2018 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

//go:build purego

// alias 包实现内存别名测试。
package alias

// 这是基于 reflect 的 Google App Engine 标准变体，
// 因为 unsafe 包和 cgo 是不允许的。

import "reflect"

// AnyOverlap 报告 x 和 y 是否在任何（不一定对应的）索引处共享内存。
// 超出切片长度的内存被忽略。
func AnyOverlap(x, y []byte) bool {
	return len(x) > 0 && len(y) > 0 &&
		reflect.ValueOf(&x[0]).Pointer() <= reflect.ValueOf(&y[len(y)-1]).Pointer() &&
		reflect.ValueOf(&y[0]).Pointer() <= reflect.ValueOf(&x[len(x)-1]).Pointer()
}

// InexactOverlap 报告 x 和 y 是否在任何非对应索引处共享内存。
// 超出切片长度的内存被忽略。注意 x 和 y 可以有不同的长度，
// 仍然没有任何不精确重叠。
//
// InexactOverlap 可用于实现 crypto/cipher AEAD、Block、BlockMode
// 和 Stream 接口的要求。
func InexactOverlap(x, y []byte) bool {
	if len(x) == 0 || len(y) == 0 || &x[0] == &y[0] {
		return false
	}
	return AnyOverlap(x, y)
}
