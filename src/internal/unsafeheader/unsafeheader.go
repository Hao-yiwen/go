// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package unsafeheader 包含 Go 运行时的切片
// 和字符串实现的头声明。
//
// 此包允许无法导入"reflect"的包使用经过测试
// 等价于 reflect.SliceHeader 和 reflect.StringHeader 的类型。
package unsafeheader

import (
	"unsafe"
)

// Slice 是切片的运行时表示。
// 它不能安全地或可移植地使用，其表示可能在
// 更高版本中更改。
//
// 与 reflect.SliceHeader 不同，其 Data 字段足以保证
// 它引用的数据不会被垃圾回收。
type Slice struct {
	Data unsafe.Pointer
	Len  int
	Cap  int
}

// String 是字符串的运行时表示。
// 它不能安全地或可移植地使用，其表示可能在
// 更高版本中更改。
//
// 与 reflect.StringHeader 不同，其 Data 字段足以保证
// 它引用的数据不会被垃圾回收。
type String struct {
	Data unsafe.Pointer
	Len  int
}
