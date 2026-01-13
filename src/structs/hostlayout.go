// Copyright 2024 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package structs

// HostLayout 将结构体标记为使用主机内存布局。具有 HostLayout 类型字段的结构体
// 将按照主机的期望在内存中进行布局，通常遵循主机的 C ABI。
//
// HostLayout 不会影响包含结构体中任何其他结构体类型字段的布局，
// 也不会影响包含标记为主机布局的结构体的结构体的布局。
//
// 按惯例，HostLayout 应该用作名为"_"的字段的类型，
// 并放在结构体类型定义的开始处。
type HostLayout struct {
	_ hostLayout // 防止与纯 struct{} 的意外转换
}

// 我们在导出的类型中使用一个未导出的类型来给标记类型本身，
// 而不仅仅是其名称，在类型系统中一个可识别的身份。这样做的主要结果是用户可以给
// 该类型一个新的名称，它仍将具有相同的属性，例如，
//
//	type HL structs.HostLayout
//
// 它也防止了 struct{} 到命名标记类型的无意转换。
type hostLayout struct {
}
