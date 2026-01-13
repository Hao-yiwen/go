// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Scan 的类型转换。

package sql

import (
	"bytes"
	"database/sql/driver"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"time"
	"unicode"
	"unicode/utf8"
	_ "unsafe" // for linkname
)

var errNilPtr = errors.New("destination pointer is nil") // 嵌入在描述性错误中

func describeNamedValue(nv *driver.NamedValue) string {
	if len(nv.Name) == 0 {
		return fmt.Sprintf("$%d", nv.Ordinal)
	}
	return fmt.Sprintf("with name %q", nv.Name)
}

func validateNamedValueName(name string) error {
	if len(name) == 0 {
		return nil
	}
	r, _ := utf8.DecodeRuneInString(name)
	if unicode.IsLetter(r) {
		return nil
	}
	return fmt.Errorf("name %q does not begin with a letter", name)
}

// ccChecker 封装 driver.ColumnConverter，允许其用作 NamedValueChecker。
// 如果驱动程序 ColumnConverter 不存在，则 NamedValueChecker 将返回 driver.ErrSkip。
type ccChecker struct {
	cci  driver.ColumnConverter
	want int
}

func (c ccChecker) CheckNamedValue(nv *driver.NamedValue) error {
	if c.cci == nil {
		return driver.ErrSkip
	}
	// 列转换器不应被调用于任何意外的索引。
	// 最终错误将在参数转换器循环中抛出。
	index := nv.Ordinal - 1
	if c.want >= 0 && c.want <= index {
		return nil
	}

	// 首先，查看值本身是否知道如何将自己转换为驱动程序类型。
	// 例如，NullString 结构变成字符串或 nil。
	if vr, ok := nv.Value.(driver.Valuer); ok {
		sv, err := callValuerValue(vr)
		if err != nil {
			return err
		}
		if !driver.IsValue(sv) {
			return fmt.Errorf("non-subset type %T returned from Value", sv)
		}
		nv.Value = sv
	}

	// 其次，要求列进行理智检查。例如，驱动程序可能使用此方法来确保
	// 插入到 16 位整数字段的 int64 值在范围内（在被截断之前），
	// 或者 nil 不能进入 NOT NULL 列中，然后在网络上获取相同的错误。
	var err error
	arg := nv.Value
	nv.Value, err = c.cci.ColumnConverter(index).ConvertValue(arg)
	if err != nil {
		return err
	}
	if !driver.IsValue(nv.Value) {
		return fmt.Errorf("driver ColumnConverter error converted %T to unsupported type %T", arg, nv.Value)
	}
	return nil
}

// defaultCheckNamedValue 封装默认 ColumnConverter 以具有与 driver.NamedValueChecker
// 接口中 CheckNamedValue 相同的函数签名。
func defaultCheckNamedValue(nv *driver.NamedValue) (err error) {
	nv.Value, err = driver.DefaultParameterConverter.ConvertValue(nv.Value)
	return err
}

// driverArgsConnLocked 将来自 Stmt.Exec 和 Stmt.Query 调用者的参数转换为驱动程序值。
//
// 如果没有可用的语句，语句 ds 可能为 nil。
//
// ci 必须被锁定。
func driverArgsConnLocked(ci driver.Conn, ds *driverStmt, args []any) ([]driver.NamedValue, error) {
	nvargs := make([]driver.NamedValue, len(args))

	// -1 表示驱动程序不知道如何计算占位符的数量，
	// 所以我们不会在此进行理智检查，而是让驱动程序处理错误。
	want := -1

	var si driver.Stmt
	var cc ccChecker
	if ds != nil {
		si = ds.si
		want = ds.si.NumInput()
		cc.want = want
	}

	// 从一开始检查所有类型的接口。
	// 驱动程序可以选择对特殊参数类型使用 NamedValueChecker，
	// 然后返回 driver.ErrSkip 将其传递给列转换器。
	nvc, ok := si.(driver.NamedValueChecker)
	if !ok {
		nvc, _ = ci.(driver.NamedValueChecker)
	}
	cci, ok := si.(driver.ColumnConverter)
	if ok {
		cc.cci = cci
	}

	// 遍历所有参数，检查每一个。
	// 如果没有返回错误，只需增加索引并继续。
	// 但是，如果返回 driver.ErrRemoveArgument，则参数不包含在查询参数列表中。
	var err error
	var n int
	for _, arg := range args {
		nv := &nvargs[n]
		if np, ok := arg.(NamedArg); ok {
			if err = validateNamedValueName(np.Name); err != nil {
				return nil, err
			}
			arg = np.Value
			nv.Name = np.Name
		}
		nv.Ordinal = n + 1
		nv.Value = arg

		// 检查序列有四条路线：
		// A: 1. 默认
		// B: 1. NamedValueChecker 2. 列转换器 3. 默认
		// C: 1. NamedValueChecker 3. 默认
		// D: 1. 列转换器 2. 默认
		//
		// 列转换器被调用的唯一时间是首先
		// 或在 NamedValueConverter 之后。如果首先在
		// nextCheck 标签之前处理。因此对于重复尝试，仅当
		// 选择 NamedValueConverter 时，列转换器
		// 应该在重试中使用。
		checker := defaultCheckNamedValue
		nextCC := false
		switch {
		case nvc != nil:
			nextCC = cci != nil
			checker = nvc.CheckNamedValue
		case cci != nil:
			checker = cc.CheckNamedValue
		}

	nextCheck:
		err = checker(nv)
		switch err {
		case nil:
			n++
			continue
		case driver.ErrRemoveArgument:
			nvargs = nvargs[:len(nvargs)-1]
			continue
		case driver.ErrSkip:
			if nextCC {
				nextCC = false
				checker = cc.CheckNamedValue
			} else {
				checker = defaultCheckNamedValue
			}
			goto nextCheck
		default:
			return nil, fmt.Errorf("sql: converting argument %s type: %w", describeNamedValue(nv), err)
		}
	}

	// 转换后检查参数长度以允许省略参数。
	if want != -1 && len(nvargs) != want {
		return nil, fmt.Errorf("sql: expected %d arguments, got %d", want, len(nvargs))
	}

	return nvargs, nil
}

// convertAssign 与 convertAssignRows 相同，但没有可选的行参数。
//
// convertAssign 应该是内部细节，
// 但被广泛使用的包使用 linkname 访问它。
// 名义上的羞愧堂成员包括：
//   - ariga.io/entcache
//
// 不要删除或改变类型签名。
// 请参见 go.dev/issue/67401。
//
//go:linkname convertAssign
func convertAssign(dest, src any) error {
	return convertAssignRows(dest, src, nil)
}

// convertAssignRows 将 src 中的值复制到 dest，如果可能的话进行转换。
// 如果复制会导致信息丢失，则返回错误。dest 应该是指针类型。
// 如果传入行，行将用作从 driver.Rows 转换为 *Rows 的任何游标值的父级。
func convertAssignRows(dest, src any, rows *Rows) error {
	// 常见情况，无需反射。
	switch s := src.(type) {
	case string:
		switch d := dest.(type) {
		case *string:
			if d == nil {
				return errNilPtr
			}
			*d = s
			return nil
		case *[]byte:
			if d == nil {
				return errNilPtr
			}
			*d = []byte(s)
			return nil
		case *RawBytes:
			if d == nil {
				return errNilPtr
			}
			*d = rows.setrawbuf(append(rows.rawbuf(), s...))
			return nil
		}
	case []byte:
		switch d := dest.(type) {
		case *string:
			if d == nil {
				return errNilPtr
			}
			*d = string(s)
			return nil
		case *any:
			if d == nil {
				return errNilPtr
			}
			*d = bytes.Clone(s)
			return nil
		case *[]byte:
			if d == nil {
				return errNilPtr
			}
			*d = bytes.Clone(s)
			return nil
		case *RawBytes:
			if d == nil {
				return errNilPtr
			}
			*d = s
			return nil
		}
	case time.Time:
		switch d := dest.(type) {
		case *time.Time:
			*d = s
			return nil
		case *string:
			*d = s.Format(time.RFC3339Nano)
			return nil
		case *[]byte:
			if d == nil {
				return errNilPtr
			}
			*d = s.AppendFormat(make([]byte, 0, len(time.RFC3339Nano)), time.RFC3339Nano)
			return nil
		case *RawBytes:
			if d == nil {
				return errNilPtr
			}
			*d = rows.setrawbuf(s.AppendFormat(rows.rawbuf(), time.RFC3339Nano))
			return nil
		}
	case decimalDecompose:
		switch d := dest.(type) {
		case decimalCompose:
			return d.Compose(s.Decompose(nil))
		}
	case nil:
		switch d := dest.(type) {
		case *any:
			if d == nil {
				return errNilPtr
			}
			*d = nil
			return nil
		case *[]byte:
			if d == nil {
				return errNilPtr
			}
			*d = nil
			return nil
		case *RawBytes:
			if d == nil {
				return errNilPtr
			}
			*d = nil
			return nil
		}
	// 驱动程序返回客户端可能遍历的游标。
	case driver.Rows:
		switch d := dest.(type) {
		case *Rows:
			if d == nil {
				return errNilPtr
			}
			if rows == nil {
				return errors.New("invalid context to convert cursor rows, missing parent *Rows")
			}
			*d = Rows{
				dc:          rows.dc,
				releaseConn: func(error) {},
				rowsi:       s,
			}
			// 链接取消函数。
			parentCancel := rows.cancel
			rows.cancel = func() {
				// 调用 Rows.cancel 时，closemu 也会被锁定。
				// 所以我们可以访问 rs.lasterr。
				d.close(rows.lasterr)
				if parentCancel != nil {
					parentCancel()
				}
			}
			return nil
		}
	}

	var sv reflect.Value

	switch d := dest.(type) {
	case *string:
		sv = reflect.ValueOf(src)
		switch sv.Kind() {
		case reflect.Bool,
			reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
			reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
			reflect.Float32, reflect.Float64:
			*d = asString(src)
			return nil
		}
	case *[]byte:
		sv = reflect.ValueOf(src)
		if b, ok := asBytes(nil, sv); ok {
			*d = b
			return nil
		}
	case *RawBytes:
		sv = reflect.ValueOf(src)
		if b, ok := asBytes(rows.rawbuf(), sv); ok {
			*d = rows.setrawbuf(b)
			return nil
		}
	case *bool:
		bv, err := driver.Bool.ConvertValue(src)
		if err == nil {
			*d = bv.(bool)
		}
		return err
	case *any:
		*d = src
		return nil
	}

	if scanner, ok := dest.(Scanner); ok {
		return scanner.Scan(src)
	}

	dpv := reflect.ValueOf(dest)
	if dpv.Kind() != reflect.Pointer {
		return errors.New("destination not a pointer")
	}
	if dpv.IsNil() {
		return errNilPtr
	}

	if !sv.IsValid() {
		sv = reflect.ValueOf(src)
	}

	dv := reflect.Indirect(dpv)
	if sv.IsValid() && sv.Type().AssignableTo(dv.Type()) {
		switch b := src.(type) {
		case []byte:
			dv.Set(reflect.ValueOf(bytes.Clone(b)))
		default:
			dv.Set(sv)
		}
		return nil
	}

	if dv.Kind() == sv.Kind() && sv.Type().ConvertibleTo(dv.Type()) {
		dv.Set(sv.Convert(dv.Type()))
		return nil
	}

	// 以下转换使用字符串值作为中间表示来在各种数字类型之间转换。
	//
	// 这也允许扫描到用户定义的类型，例如 "type Int int64"。
	// 为了对称性，也检查字符串目标类型。
	switch dv.Kind() {
	case reflect.Pointer:
		if src == nil {
			dv.SetZero()
			return nil
		}
		dv.Set(reflect.New(dv.Type().Elem()))
		return convertAssignRows(dv.Interface(), src, rows)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if src == nil {
			return fmt.Errorf("converting NULL to %s is unsupported", dv.Kind())
		}
		s := asString(src)
		i64, err := strconv.ParseInt(s, 10, dv.Type().Bits())
		if err != nil {
			err = strconvErr(err)
			return fmt.Errorf("converting driver.Value type %T (%q) to a %s: %v", src, s, dv.Kind(), err)
		}
		dv.SetInt(i64)
		return nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if src == nil {
			return fmt.Errorf("converting NULL to %s is unsupported", dv.Kind())
		}
		s := asString(src)
		u64, err := strconv.ParseUint(s, 10, dv.Type().Bits())
		if err != nil {
			err = strconvErr(err)
			return fmt.Errorf("converting driver.Value type %T (%q) to a %s: %v", src, s, dv.Kind(), err)
		}
		dv.SetUint(u64)
		return nil
	case reflect.Float32, reflect.Float64:
		if src == nil {
			return fmt.Errorf("converting NULL to %s is unsupported", dv.Kind())
		}
		s := asString(src)
		f64, err := strconv.ParseFloat(s, dv.Type().Bits())
		if err != nil {
			err = strconvErr(err)
			return fmt.Errorf("converting driver.Value type %T (%q) to a %s: %v", src, s, dv.Kind(), err)
		}
		dv.SetFloat(f64)
		return nil
	case reflect.String:
		if src == nil {
			return fmt.Errorf("converting NULL to %s is unsupported", dv.Kind())
		}
		switch v := src.(type) {
		case string:
			dv.SetString(v)
			return nil
		case []byte:
			dv.SetString(string(v))
			return nil
		}
	}

	return fmt.Errorf("unsupported Scan, storing driver.Value type %T into type %T", src, dest)
}

func strconvErr(err error) error {
	if ne, ok := err.(*strconv.NumError); ok {
		return ne.Err
	}
	return err
}

func asString(src any) string {
	switch v := src.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	}
	rv := reflect.ValueOf(src)
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(rv.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(rv.Uint(), 10)
	case reflect.Float64:
		return strconv.FormatFloat(rv.Float(), 'g', -1, 64)
	case reflect.Float32:
		return strconv.FormatFloat(rv.Float(), 'g', -1, 32)
	case reflect.Bool:
		return strconv.FormatBool(rv.Bool())
	}
	return fmt.Sprintf("%v", src)
}

func asBytes(buf []byte, rv reflect.Value) (b []byte, ok bool) {
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.AppendInt(buf, rv.Int(), 10), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.AppendUint(buf, rv.Uint(), 10), true
	case reflect.Float32:
		return strconv.AppendFloat(buf, rv.Float(), 'g', -1, 32), true
	case reflect.Float64:
		return strconv.AppendFloat(buf, rv.Float(), 'g', -1, 64), true
	case reflect.Bool:
		return strconv.AppendBool(buf, rv.Bool()), true
	case reflect.String:
		s := rv.String()
		return append(buf, s...), true
	}
	return
}

var valuerReflectType = reflect.TypeFor[driver.Valuer]()

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
// 此函数在 database/sql/driver 包中镜像。
func callValuerValue(vr driver.Valuer) (v driver.Value, err error) {
	if rv := reflect.ValueOf(vr); rv.Kind() == reflect.Pointer &&
		rv.IsNil() &&
		rv.Type().Elem().Implements(valuerReflectType) {
		return nil, nil
	}
	return vr.Value()
}

// decimal 组成或分解十进制值为各个部分，反之亦然。
// 有四个部分：布尔负标志、三种可能状态的形式字节
// （finite=0, infinite=1, NaN=2）、基数为 2 的大端整数
// 系数（也称为有效数字）作为 []byte，以及 int32 指数。
// 这些被组成为最终值为 "decimal = (neg) (form=finite) coefficient * 10 ^ exponent"。
// 零长度系数是零值。
// 大端整数系数首先存储最重要的字节（在 coefficient[0]）。
// 如果形式不是有限的，系数和指数应该被忽略。
// 对于任何形式，negative 参数可能被设置为 true，尽管实现不需要
// 在非有限形式中尊重负参数。
//
// 实现可能选择在零或 NaN 值上将负参数设置为 true，
// 但不区分负和正的实现
// 零或 NaN 值应该忽略负参数而不出现错误。
// 如果实现不支持无穷大，它可能会在没有错误的情况下转换为 NaN。
// 如果设置的值大于实现支持的值，
// 必须返回错误。
// 如果尝试设置 NaN 或无穷大而两者都不支持，实现必须返回错误。
//
// NOTE(kardianos)：这是一个实验性接口。请参见 https://golang.org/issue/30870
type decimal interface {
	decimalDecompose
	decimalCompose
}

type decimalDecompose interface {
	// Decompose 以部分的方式返回内部十进制状态。
	// 如果提供的 buf 有足够的容量，buf 可能作为系数返回，
	// 值设置和长度设置为适当。
	Decompose(buf []byte) (form byte, negative bool, coefficient []byte, exponent int32)
}

type decimalCompose interface {
	// Compose 从各个部分设置内部十进制值。如果无法表示该值，
	// 则应返回错误。
	Compose(form byte, negative bool, coefficient []byte, exponent int32) error
}
