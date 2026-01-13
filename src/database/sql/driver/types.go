// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package driver

import (
	"fmt"
	"reflect"
	"strconv"
	"time"
)

// ValueConverter 是提供 ConvertValue 方法的接口。
//
// driver 包提供了 ValueConverter 的各种实现，以提供
// 驱动程序之间转换的一致实现。ValueConverters 有多种用途：
//
//   - 从 sql 包提供的 [Value] 类型转换
//     为数据库表的特定列类型，并确保
//     适配，例如确保特定 int64 适配于
//     表的 uint16 列。
//
//   - 将从数据库给定的值转换为
//     驱动程序 [Value] 类型之一。
//
//   - 由 [database/sql] 包，用于从驱动程序的 [Value] 类型
//     转换为用户在扫描中的类型。
type ValueConverter interface {
	// ConvertValue 将值转换为驱动程序值。
	ConvertValue(v any) (Value, error)
}

// Valuer 是提供 Value 方法的接口。
//
// [Value] 方法返回的错误由 database/sql 包包装。
// 这允许调用者在执行操作后使用 [errors.Is] 进行精确错误处理，
// 如 [database/sql.Query]、[database/sql.Exec] 或 [database/sql.QueryRow]。
//
// 实现 Valuer 接口的类型能够将自己
// 转换为驱动程序 [Value]。
type Valuer interface {
	// Value 返回驱动程序值。
	// Value 不能恐慌。
	Value() (Value, error)
}

// Bool 是 [ValueConverter]，将输入值转换为 bool。
//
// 转换规则是：
//   - 布尔值未更改地返回
//   - 对于整数类型，
//     1 是真
//     0 是假，
//     其他整数是错误
//   - 对于字符串和 []byte，与 [strconv.ParseBool] 相同的规则
//   - 所有其他类型都是错误
var Bool boolType

type boolType struct{}

var _ ValueConverter = boolType{}

func (boolType) String() string { return "Bool" }

func (boolType) ConvertValue(src any) (Value, error) {
	switch s := src.(type) {
	case bool:
		return s, nil
	case string:
		b, err := strconv.ParseBool(s)
		if err != nil {
			return nil, fmt.Errorf("sql/driver: couldn't convert %q into type bool", s)
		}
		return b, nil
	case []byte:
		b, err := strconv.ParseBool(string(s))
		if err != nil {
			return nil, fmt.Errorf("sql/driver: couldn't convert %q into type bool", s)
		}
		return b, nil
	}

	sv := reflect.ValueOf(src)
	switch sv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		iv := sv.Int()
		if iv == 1 || iv == 0 {
			return iv == 1, nil
		}
		return nil, fmt.Errorf("sql/driver: couldn't convert %d into type bool", iv)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		uv := sv.Uint()
		if uv == 1 || uv == 0 {
			return uv == 1, nil
		}
		return nil, fmt.Errorf("sql/driver: couldn't convert %d into type bool", uv)
	}

	return nil, fmt.Errorf("sql/driver: couldn't convert %v (%T) into type bool", src, src)
}

// Int32 是 [ValueConverter]，将输入值转换为 int64，
// 尊重 int32 值的限制。
var Int32 int32Type

type int32Type struct{}

var _ ValueConverter = int32Type{}

func (int32Type) ConvertValue(v any) (Value, error) {
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		i64 := rv.Int()
		if i64 > (1<<31)-1 || i64 < -(1<<31) {
			return nil, fmt.Errorf("sql/driver: value %d overflows int32", v)
		}
		return i64, nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		u64 := rv.Uint()
		if u64 > (1<<31)-1 {
			return nil, fmt.Errorf("sql/driver: value %d overflows int32", v)
		}
		return int64(u64), nil
	case reflect.String:
		i, err := strconv.Atoi(rv.String())
		if err != nil {
			return nil, fmt.Errorf("sql/driver: value %q can't be converted to int32", v)
		}
		return int64(i), nil
	}
	return nil, fmt.Errorf("sql/driver: unsupported value %v (type %T) converting to int32", v, v)
}

// String 是 [ValueConverter]，将其输入转换为字符串。
// 如果值已经是字符串或 []byte，则未更改。
// 如果值是另一种类型，则进行字符串转换
// 使用 fmt.Sprintf("%v", v)。
var String stringType

type stringType struct{}

func (stringType) ConvertValue(v any) (Value, error) {
	switch v.(type) {
	case string, []byte:
		return v, nil
	}
	return fmt.Sprintf("%v", v), nil
}

// Null 是实现 [ValueConverter] 的类型，通过允许 nil
// 值但将其他情况委托给另一个 [ValueConverter]。
type Null struct {
	Converter ValueConverter
}

func (n Null) ConvertValue(v any) (Value, error) {
	if v == nil {
		return nil, nil
	}
	return n.Converter.ConvertValue(v)
}

// NotNull 是实现 [ValueConverter] 的类型，通过禁止 nil
// 值但将其他情况委托给另一个 [ValueConverter]。
type NotNull struct {
	Converter ValueConverter
}

func (n NotNull) ConvertValue(v any) (Value, error) {
	if v == nil {
		return nil, fmt.Errorf("nil value not allowed")
	}
	return n.Converter.ConvertValue(v)
}

// IsValue 报告 v 是否是有效的 [Value] 参数类型。
func IsValue(v any) bool {
	if v == nil {
		return true
	}
	switch v.(type) {
	case []byte, bool, float64, int64, string, time.Time:
		return true
	case decimalDecompose:
		return true
	}
	return false
}

// IsScanValue 等价于 [IsValue]。
// 它存在是为了兼容性。
func IsScanValue(v any) bool {
	return IsValue(v)
}

// DefaultParameterConverter 是默认的 [ValueConverter] 实现，
// 在 [Stmt] 没有实现 [ColumnConverter] 时使用。
//
// DefaultParameterConverter 在 IsValue(arg) 时直接返回其参数。
// 否则，如果参数实现 [Valuer]，则使用其
// Value 方法返回 [Value]。作为后备，提供的
// 参数的底层类型用于将其转换为 [Value]：
// 底层整数类型转换为 int64，浮点数转换为 float64，
// bool、string 和 []byte 转换为它们自己。如果参数是 nil
// 指针，defaultConverter.ConvertValue 返回 nil [Value]。
// 如果参数是非 nil 指针，它被取消引用并且
// 递归调用 defaultConverter.ConvertValue。其他类型
// 是错误。
var DefaultParameterConverter defaultConverter

type defaultConverter struct{}

var _ ValueConverter = defaultConverter{}

var valuerReflectType = reflect.TypeFor[Valuer]()

// callValuerValue 返回 vr.Value()，有一个例外：
// 如果 vr.Value 是指针类型上的自动生成的方法，并且
// 指针为 nil，它会在 panicwrap 方法中在运行时出现恐慌。
// 像对待 nil 一样对待它。
// 问题 8415。
//
// 这是为了让人们可以在值类型上实现 driver.Value，
// 并且仍然使用 nil 指针来表示这些类型意味着 nil/NULL，就像
// string/*string。
//
// 此函数在 database/sql 包中镜像。
func callValuerValue(vr Valuer) (v Value, err error) {
	if rv := reflect.ValueOf(vr); rv.Kind() == reflect.Pointer &&
		rv.IsNil() &&
		rv.Type().Elem().Implements(valuerReflectType) {
		return nil, nil
	}
	return vr.Value()
}

func (defaultConverter) ConvertValue(v any) (Value, error) {
	if IsValue(v) {
		return v, nil
	}

	switch vr := v.(type) {
	case Valuer:
		sv, err := callValuerValue(vr)
		if err != nil {
			return nil, err
		}
		if !IsValue(sv) {
			return nil, fmt.Errorf("non-Value type %T returned from Value", sv)
		}
		return sv, nil

	// 现在，继续优先使用 Valuer 接口而不是十进制分解接口。
	case decimalDecompose:
		return vr, nil
	}

	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Pointer:
		// 间接指针
		if rv.IsNil() {
			return nil, nil
		} else {
			return defaultConverter{}.ConvertValue(rv.Elem().Interface())
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return rv.Int(), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32:
		return int64(rv.Uint()), nil
	case reflect.Uint64:
		u64 := rv.Uint()
		if u64 >= 1<<63 {
			return nil, fmt.Errorf("uint64 values with high bit set are not supported")
		}
		return int64(u64), nil
	case reflect.Float32, reflect.Float64:
		return rv.Float(), nil
	case reflect.Bool:
		return rv.Bool(), nil
	case reflect.Slice:
		ek := rv.Type().Elem().Kind()
		if ek == reflect.Uint8 {
			return rv.Bytes(), nil
		}
		return nil, fmt.Errorf("unsupported type %T, a slice of %s", v, ek)
	case reflect.String:
		return rv.String(), nil
	}
	return nil, fmt.Errorf("unsupported type %T, a %s", v, rv.Kind())
}

type decimalDecompose interface {
	// Decompose 以部分的方式返回内部十进制状态。
	// 如果提供的 buf 有足够的容量，buf 可能作为系数返回，
	// 值设置和长度设置为适当。
	Decompose(buf []byte) (form byte, negative bool, coefficient []byte, exponent int32)
}
