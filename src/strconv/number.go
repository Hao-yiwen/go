// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package strconv

import (
	"errors"
	"internal/strconv"
	"internal/stringslite"
)

// IntSize 是 int 或 uint 值的位大小。
const IntSize = strconv.IntSize

// ParseBool 返回字符串表示的布尔值。
// 它接受 1, t, T, TRUE, true, True, 0, f, F, FALSE, false, False。
// 任何其他值都会返回错误。
func ParseBool(str string) (bool, error) {
	x, err := strconv.ParseBool(str)
	if err != nil {
		return x, toError("ParseBool", str, 0, 0, err)
	}
	return x, nil
}

// FormatBool 根据 b 的值返回 "true" 或 "false"。
func FormatBool(b bool) string {
	return strconv.FormatBool(b)
}

// AppendBool 根据 b 的值将 "true" 或 "false" 追加到 dst，
// 并返回扩展后的缓冲区。
func AppendBool(dst []byte, b bool) []byte {
	return strconv.AppendBool(dst, b)
}

// ParseComplex 将字符串 s 转换为复数，
// 精度由 bitSize 指定：complex64 为 64，complex128 为 128。
// 当 bitSize=64 时，结果仍然是 complex128 类型，但它将
// 可转换为 complex64 而不改变其值。
//
// 由 s 表示的数字必须是 N、Ni 或 N±Ni 的形式，其中 N 是
// 由 [ParseFloat] 识别的浮点数，i 是虚数部分。
// 如果第二个 N 是无符号的，则在两个分量之间需要 + 号
// 如 ± 所示。如果第二个 N 是 NaN，则只接受 + 号。
// 该形式可能被括号括起，不能包含任何空格。
// 生成的复数由 ParseFloat 转换的两个分量组成。
//
// ParseComplex 返回的错误具有具体类型 [*NumError]
// 并包括 err.Num = s。
//
// 如果 s 在语法上不是良好形式的，ParseComplex 返回 err.Err = ErrSyntax。
//
// 如果 s 在语法上是良好形式的，但任一分量距离
// 给定分量大小的最大浮点数超过 1/2 ULP，
// ParseComplex 返回 err.Err = ErrRange，c = ±Inf 对于相应的分量。
func ParseComplex(s string, bitSize int) (complex128, error) {
	x, err := strconv.ParseComplex(s, bitSize)
	if err != nil {
		return x, toError("ParseComplex", s, 0, bitSize, err)
	}
	return x, nil
}

// ParseFloat 将字符串 s 转换为浮点数，
// 精度由 bitSize 指定：float32 为 32，float64 为 64。
// 当 bitSize=32 时，结果仍然是 float64 类型，但它将
// 可转换为 float32 而不改变其值。
//
// ParseFloat 接受由 Go 语法定义的十进制和十六进制浮点数
// 如 [浮点字面量]。如果 s 是良好形式的且接近有效的浮点数，
// ParseFloat 返回使用 IEEE754 无偏舍入进行舍入的最近的浮点数。
// （解析十六进制浮点值仅在十六进制表示中的位数超过
// 尾数所能容纳的位数时进行舍入。）
//
// ParseFloat 返回的错误具有具体类型 *NumError
// 并包括 err.Num = s。
//
// 如果 s 在语法上不是良好形式的，ParseFloat 返回 err.Err = ErrSyntax。
//
// 如果 s 在语法上是良好形式的，但距离
// 给定大小的最大浮点数超过 1/2 ULP，
// ParseFloat 返回 f = ±Inf，err.Err = ErrRange。
//
// ParseFloat 识别字符串 "NaN" 以及（可能带符号的）字符串 "Inf" 和 "Infinity"
// 作为各自的特殊浮点值。匹配时不区分大小写。
//
// [浮点字面量]: https://go.dev/ref/spec#Floating-point_literals
func ParseFloat(s string, bitSize int) (float64, error) {
	x, err := strconv.ParseFloat(s, bitSize)
	if err != nil {
		return x, toError("ParseFloat", s, 0, bitSize, err)
	}
	return x, nil
}

// ParseUint 类似于 [ParseInt] 但用于无符号数字。
//
// 不允许符号前缀。
func ParseUint(s string, base int, bitSize int) (uint64, error) {
	x, err := strconv.ParseUint(s, base, bitSize)
	if err != nil {
		return x, toError("ParseUint", s, base, bitSize, err)
	}
	return x, nil
}

// ParseInt 在给定的进制（0、2 到 36）和
// 位大小（0 到 64）中解释字符串 s，并返回相应的值 i。
//
// 字符串可能以前导符号开始："+" 或 "-"。
//
// 如果 base 参数为 0，则真实的进制由字符串的
// 前缀（如果存在符号，则在符号之后）隐示：
// 2 表示 "0b"，8 表示 "0" 或 "0o"，
// 16 表示 "0x"，否则为 10。此外，仅当参数 base 为 0 时，
// 允许下划线字符，如 Go 语法所定义的
// [整数字面量]。
//
// bitSize 参数指定整数类型
// 结果必须适应该类型。位大小 0、8、16、32 和 64
// 分别对应 int、int8、int16、int32 和 int64。
// 如果 bitSize 小于 0 或大于 64，则返回错误。
//
// ParseInt 返回的错误具有具体类型 [*NumError]
// 并包括 err.Num = s。如果 s 为空或包含无效的
// 数字，err.Err = [ErrSyntax]，返回值为 0；
// 如果对应 s 的值无法由给定大小的有符号
// 整数表示，err.Err = [ErrRange]，返回值为
// 相应位大小和符号的最大幅度整数。
//
// [整数字面量]: https://go.dev/ref/spec#Integer_literals
func ParseInt(s string, base int, bitSize int) (i int64, err error) {
	x, err := strconv.ParseInt(s, base, bitSize)
	if err != nil {
		return x, toError("ParseInt", s, base, bitSize, err)
	}
	return x, nil
}

// Atoi 等价于 ParseInt(s, 10, 0)，转换为 int 类型。
func Atoi(s string) (int, error) {
	x, err := strconv.Atoi(s)
	if err != nil {
		return x, toError("Atoi", s, 0, 0, err)
	}
	return strconv.Atoi(s)
}

// FormatComplex 将复数 c 转换为 (a+bi) 形式的字符串，
// 其中 a 和 b 分别是实部和虚部，
// 根据格式 fmt 和精度 prec 进行格式化。
//
// 格式 fmt 和精度 prec 与 [FormatFloat] 中的意义相同。
// 它假设原始值是从 bitSize 位的复数值获得的，进行舍入，
// 对于 complex64 必须是 64，对于 complex128 必须是 128。
func FormatComplex(c complex128, fmt byte, prec, bitSize int) string {
	return strconv.FormatComplex(c, fmt, prec, bitSize)
}

// FormatFloat 将浮点数 f 转换为字符串，
// 根据格式 fmt 和精度 prec。它假设原始值是从
// bitSize 位的浮点值获得的，进行舍入
// （32 位用于 float32，64 位用于 float64）。
//
// 格式 fmt 是以下之一
//   - 'b'（-ddddp±ddd，二进制指数），
//   - 'e'（-d.dddde±dd，十进制指数），
//   - 'E'（-d.ddddE±dd，十进制指数），
//   - 'f'（-ddd.dddd，无指数），
//   - 'g'（大指数用 'e'，否则用 'f'），
//   - 'G'（大指数用 'E'，否则用 'f'），
//   - 'x'（-0xd.ddddp±ddd，十六进制分数和二进制指数），或
//   - 'X'（-0Xd.ddddP±ddd，十六进制分数和二进制指数）。
//
// 精度 prec 控制由 'e'、'E'、'f'、'g'、'G'、'x' 和 'X' 格式
// 打印的数字数量（不包括指数）。
// 对于 'e'、'E'、'f'、'x' 和 'X'，是小数点后的数字数。
// 对于 'g' 和 'G'，是有效数字的最大数量（尾随
// 零被删除）。
// 特殊精度 -1 使用最少数量的必要数字
// 以使得 ParseFloat 返回 f 的精确值。
// 指数写为十进制整数；
// 对于除 'b' 外的所有格式，它至少有两位。
func FormatFloat(f float64, fmt byte, prec, bitSize int) string {
	return strconv.FormatFloat(f, fmt, prec, bitSize)
}

// AppendFloat 将由 [FormatFloat] 生成的浮点数 f 的字符串形式
// 追加到 dst，并返回扩展后的缓冲区。
func AppendFloat(dst []byte, f float64, fmt byte, prec, bitSize int) []byte {
	return strconv.AppendFloat(dst, f, fmt, prec, bitSize)
}

// FormatUint 返回 i 在给定进制中的字符串表示，
// 对于 2 <= base <= 36。结果对于数字值 >= 10 使用小写字母 'a' 到 'z'。
func FormatUint(i uint64, base int) string {
	return strconv.FormatUint(i, base)
}

// FormatInt 返回 i 在给定进制中的字符串表示，
// 对于 2 <= base <= 36。结果对于数字值 >= 10 使用小写字母 'a' 到 'z'。
func FormatInt(i int64, base int) string {
	return strconv.FormatInt(i, base)
}

// Itoa 等价于 [FormatInt](int64(i), 10)。
func Itoa(i int) string {
	return strconv.Itoa(i)
}

// AppendInt 将由 [FormatInt] 生成的整数 i 的字符串形式
// 追加到 dst，并返回扩展后的缓冲区。
func AppendInt(dst []byte, i int64, base int) []byte {
	return strconv.AppendInt(dst, i, base)
}

// AppendUint 将由 [FormatUint] 生成的无符号整数 i 的字符串形式
// 追加到 dst，并返回扩展后的缓冲区。
func AppendUint(dst []byte, i uint64, base int) []byte {
	return strconv.AppendUint(dst, i, base)
}

// toError 将 internal/strconv.Error 转换为此包的 API 保证的错误。
func toError(fn, s string, base, bitSize int, err error) error {
	switch err {
	case strconv.ErrSyntax:
		return syntaxError(fn, s)
	case strconv.ErrRange:
		return rangeError(fn, s)
	case strconv.ErrBase:
		return baseError(fn, s, base)
	case strconv.ErrBitSize:
		return bitSizeError(fn, s, bitSize)
	}
	return err
}

// ErrRange 表示一个值超出目标类型的范围。
var ErrRange = errors.New("value out of range")

// ErrSyntax 表示一个值不具有目标类型的正确语法。
var ErrSyntax = errors.New("invalid syntax")

// NumError 记录一次失败的转换。
type NumError struct {
	Func string // 失败的函数（ParseBool、ParseInt、ParseUint、ParseFloat、ParseComplex）
	Num  string // 输入
	Err  error  // 转换失败的原因（如 ErrRange、ErrSyntax 等）
}

func (e *NumError) Error() string {
	return "strconv." + e.Func + ": " + "parsing " + Quote(e.Num) + ": " + e.Err.Error()
}

func (e *NumError) Unwrap() error { return e.Err }

// 所有 ParseXXX 函数都允许输入字符串逃逸到错误值。
// 这对 strconv.ParseXXX(string(b)) 调用造成问题，其中 b 是 []byte，
// 因为从 []byte 的转换必须在堆上分配一个字符串。
// 如果我们假设错误很少发生，那么我们可以通过首先复制来避免逃逸
// 输入回输出。这允许编译器为大多数 []byte 到 string
// 的转换调用 strconv.ParseXXX 而不需要堆分配，
// 因为它现在可以证明字符串不能逃逸 Parse。

func syntaxError(fn, str string) *NumError {
	return &NumError{fn, stringslite.Clone(str), ErrSyntax}
}

func rangeError(fn, str string) *NumError {
	return &NumError{fn, stringslite.Clone(str), ErrRange}
}

func baseError(fn, str string, base int) *NumError {
	return &NumError{fn, stringslite.Clone(str), errors.New("invalid base " + Itoa(base))}
}

func bitSizeError(fn, str string, bitSize int) *NumError {
	return &NumError{fn, stringslite.Clone(str), errors.New("invalid bit size " + Itoa(bitSize))}
}
