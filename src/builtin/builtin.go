// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

/*
Package builtin 为 Go 的预声明标识符提供文档。
此处文档化的项目实际上不在 builtin 包中，
但此处的描述允许 godoc 为语言的特殊标识符
呈现文档。
*/
package builtin

import "cmp"

// bool 是布尔值的集合，true 和 false。
type bool bool

// true 和 false 是两个无类型的布尔值。
const (
	true  = 0 == 0 // 无类型 bool。
	false = 0 != 0 // 无类型 bool。
)

// uint8 是所有无符号 8 位整数的集合。
// 范围：0 到 255。
type uint8 uint8

// uint16 是所有无符号 16 位整数的集合。
// 范围：0 到 65535。
type uint16 uint16

// uint32 是所有无符号 32 位整数的集合。
// 范围：0 到 4294967295。
type uint32 uint32

// uint64 是所有无符号 64 位整数的集合。
// 范围：0 到 18446744073709551615。
type uint64 uint64

// int8 是所有有符号 8 位整数的集合。
// 范围：-128 到 127。
type int8 int8

// int16 是所有有符号 16 位整数的集合。
// 范围：-32768 到 32767。
type int16 int16

// int32 是所有有符号 32 位整数的集合。
// 范围：-2147483648 到 2147483647。
type int32 int32

// int64 是所有有符号 64 位整数的集合。
// 范围：-9223372036854775808 到 9223372036854775807。
type int64 int64

// float32 是所有 IEEE 754 32 位浮点数的集合。
type float32 float32

// float64 是所有 IEEE 754 64 位浮点数的集合。
type float64 float64

// complex64 是所有具有 float32 实部和
// 虚部的复数的集合。
type complex64 complex64

// complex128 是所有具有 float64 实部和
// 虚部的复数的集合。
type complex128 complex128

// string 是所有 8 位字节字符串的集合，通常但不一定
// 代表 UTF-8 编码的文本。字符串可能为空，但
// 不为 nil。string 类型的值是不可变的。
type string string

// int 是一种有符号整数类型，大小至少为 32 位。它是一个
// 不同的类型，但不是别名，比如 int32。
type int int

// uint 是一种无符号整数类型，大小至少为 32 位。它是一个
// 不同的类型，但不是别名，比如 uint32。
type uint uint

// uintptr 是一种整数类型，足够大以容纳
// 任何指针的位模式。
type uintptr uintptr

// byte 是 uint8 的别名，在各方面都等同于 uint8。它是
// 按惯例用来区分字节值和 8 位无符号
// 整数值的。
type byte = uint8

// rune 是 int32 的别名，在各方面都等同于 int32。它是
// 按惯例用来区分字符值和整数值的。
type rune = int32

// any 是 interface{} 的别名，在各方面都等同于 interface{}。
type any = interface{}

// comparable 是一个由所有可比较的类型实现的接口
//（布尔值、数字、字符串、指针、通道、可比较类型的数组、
// 其字段都是可比较类型的结构体）。
// comparable 接口只能用作类型参数约束，
// 不能用作变量的类型。
type comparable interface{ comparable }

// iota 是一个预声明的标识符，代表无类型整数序号
// 数（通常为括号包含的）const 声明中
// 当前 const 规范。它是零索引的。
const iota = 0 // 无类型 int。

// nil 是一个预声明的标识符，代表
// 指针、通道、func、接口、map 或切片类型的零值。
var nil Type // Type 必须是指针、通道、func、接口、map 或切片类型

// Type 仅用于文档目的而存在。它是任何
// Go 类型的占位符，但对于任何给定的函数
// 调用代表相同的类型。
type Type int

// Type1 仅用于文档目的而存在。它是任何
// Go 类型的占位符，但对于任何给定的函数
// 调用代表相同的类型。
type Type1 int

// IntegerType 仅用于文档目的而存在。它是任何
// 整数类型的占位符：int、uint、int8 等。
type IntegerType int

// FloatType 仅用于文档目的而存在。它是任何
// 浮点类型的占位符：float32 或 float64。
type FloatType float32

// ComplexType 仅用于文档目的而存在。它是任何
// 复数类型的占位符：complex64 或 complex128。
type ComplexType complex64

// append 内置函数将元素追加到切片的末尾。如果
// 它有足够的容量，目标切片会被重新切片来容纳
// 新元素。如果没有，将分配一个新的基础数组。
// Append 返回更新后的切片。因此，有必要存储
// append 的结果，通常是在保持切片本身的变量中：
//
//	slice = append(slice, elem1, elem2)
//	slice = append(slice, anotherSlice...)
//
// 作为特殊情况，向字节切片追加字符串是合法的，像这样：
//
//	slice = append([]byte("hello "), "world"...)
func append(slice []Type, elems ...Type) []Type

// copy 内置函数将元素从源切片复制到
// 目标切片。（作为特殊情况，它也会将字节从
// 字符串复制到字节切片。）源和目标可能重叠。Copy
// 返回复制的元素数，这将是
// len(src) 和 len(dst) 的最小值。
func copy(dst, src []Type) int

// delete 内置函数从映射中删除指定的键
//（m[key]）对应的元素。如果 m 是 nil 或没有这样的元素，delete
// 是空操作。
func delete(m map[Type]Type1, key Type)

// len 内置函数根据 v 的类型返回其长度：
//
//   - 数组：v 中的元素个数。
//   - 数组指针：*v 中的元素个数（即使 v 是 nil）。
//   - 切片或映射：v 中的元素个数；如果 v 是 nil，len(v) 为零。
//   - 字符串：v 中的字节数。
//   - 通道：通道缓冲区中队列中的元素个数（未读）；
//     如果 v 是 nil，len(v) 为零。
//
// 对于某些参数，例如字符串字面量或简单的数组表达式，
// 结果可以是常量。有关详细信息，请参阅 Go 语言规范的
// "长度和容量"部分。
func len(v Type) int

// cap 内置函数根据 v 的类型返回其容量：
//
//   - 数组：v 中的元素个数（与 len(v) 相同）。
//   - 数组指针：*v 中的元素个数（与 len(v) 相同）。
//   - 切片：重新切片时切片可以达到的最大长度；
//     如果 v 是 nil，cap(v) 为零。
//   - 通道：通道缓冲区容量，以元素为单位；
//     如果 v 是 nil，cap(v) 为零。
//
// 对于某些参数，例如简单的数组表达式，结果可以是
// 常量。有关详细信息，请参阅 Go 语言规范的
// "长度和容量"部分。
func cap(v Type) int

// make 内置函数分配并初始化类型为 slice、map 或 chan（仅限这些）
// 的对象。与 new 一样，第一个参数是类型，而不是值。
// 与 new 不同，make 的返回类型与其参数的类型相同，而不是指向它的指针。
// 结果的规范取决于类型：
//
//   - 切片：大小指定长度。切片的容量是
//     等于其长度。可以提供第二个整数参数来
//     指定不同的容量；它必须不小于
//     长度。例如，make([]int, 0, 10) 分配一个大小为 10 的基础数组
//     并返回长度为 0、容量为 10 的切片，
//     由该基础数组支持。
//   - 映射：分配一个空映射，具有足够的空间来保存
//     指定数量的元素。可以省略大小，在这种情况下，
//     分配一个小的起始大小。
//   - 通道：通道的缓冲区用指定的
//     缓冲区容量初始化。如果为零或省略大小，通道是
//     无缓冲的。
func make(t Type, size ...IntegerType) Type

// max 内置函数返回固定数量的
// [cmp.Ordered] 类型参数中的最大值。必须至少有一个参数。
// 如果 T 是浮点类型且任何参数是 NaN，
// max 将返回 NaN。
func max[T cmp.Ordered](x T, y ...T) T

// min 内置函数返回固定数量的
// [cmp.Ordered] 类型参数中的最小值。必须至少有一个参数。
// 如果 T 是浮点类型且任何参数是 NaN，
// min 将返回 NaN。
func min[T cmp.Ordered](x T, y ...T) T

// new 内置函数分配内存。第一个参数是类型，
// 不是值，返回的值是指向新分配的
// 该类型零值的指针。
func new(Type) *Type

// complex 内置函数从两个浮点值构造复数值。
// 实部和虚部的大小必须相同，
// float32 或 float64（或可分配给它们），返回值
// 将是相应的复数类型（float32 为 complex64，
// float64 为 complex128）。
func complex(r, i FloatType) ComplexType

// real 内置函数返回复数 c 的实部。
// 返回值将是与 c 的类型对应的浮点类型。
func real(c ComplexType) FloatType

// imag 内置函数返回复数 c 的虚部。
// 返回值将是与 c 的类型对应的浮点类型。
func imag(c ComplexType) FloatType

// clear 内置函数清空映射和切片。
// 对于映射，clear 删除所有条目，导致空映射。
// 对于切片，clear 将切片长度的所有元素
// 设置为各自元素类型的零值。如果参数
// 类型是类型参数，类型参数的类型集必须
// 仅包含映射或切片类型，clear 执行由
// 类型参数隐示的操作。如果 t 是 nil，clear 是空操作。
func clear[T ~[]Type | ~map[Type]Type1](t T)

// close 内置函数关闭一个通道，该通道必须是
// 双向或仅发送。它应该仅由发送方执行，
// 永不由接收方执行，并且具有在接收到
// 最后一个发送值后关闭通道的效果。在接收到
// 来自已关闭通道 c 的最后一个值后，任何来自 c 的接收将
// 成功而不阻塞，返回通道元素的零值。形式
//
//	x, ok := <-c
//
// 对于已关闭且为空的通道，也将 ok 设置为 false。
func close(c chan<- Type)

// panic 内置函数停止当前 goroutine 的正常执行。
// 当函数 F 调用 panic 时，F 的正常执行立即停止。
// 任何由 F 延迟执行的函数都按通常的方式运行，然后 F 返回给其调用者。
// 对于调用者 G，对 F 的调用然后表现为对 panic 的调用，
// 终止 G 的执行并运行任何延迟函数。这将继续直到
// 执行中 goroutine 的所有函数都停止，顺序相反。
// 此时，程序使用非零退出代码终止。
// 这个终止序列称为 panicking，可以由内置函数 recover 控制。
//
// 从 Go 1.21 开始，使用 nil 接口值或无类型 nil
// 调用 panic 会导致运行时错误（不同的 panic）。
// GODEBUG 设置 panicnil=1 禁用运行时错误。
func panic(v any)

// recover 内置函数允许程序管理 panicking goroutine 的行为。
// 在延迟函数内执行对 recover 的调用（但不是由它调用的任何函数）
// 通过恢复正常执行来停止 panicking 序列，并检索
// 传递给 panic 调用的错误值。如果在延迟函数外调用
// recover，它将不会停止 panicking 序列。
// 在这种情况下，或当 goroutine 不在 panicking 时，
// recover 返回 nil。
//
// 在 Go 1.21 之前，如果使用 nil 参数调用 panic，
// recover 也会返回 nil。有关详细信息，请参阅 [panic]。
func recover() any

// print 内置函数以实现特定的方式格式化其参数
// 并将结果写入标准错误。
// Print 对于引导和调试很有用；不保证
// 它会保留在语言中。
func print(args ...Type)

// println 内置函数以实现特定的方式格式化其参数
// 并将结果写入标准错误。
// 总是在参数之间添加空格并附加换行符。
// Println 对于引导和调试很有用；不保证
// 它会保留在语言中。
func println(args ...Type)

// error 内置接口类型是代表错误条件的约定接口，
// 其中 nil 值代表无错误。
type error interface {
	Error() string
}
