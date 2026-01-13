// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sql

import (
	"database/sql/driver"
	"fmt"
	"internal/asan"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

var someTime = time.Unix(123, 0)
var answer int64 = 42

type (
	userDefined       float64
	userDefinedSlice  []int
	userDefinedString string
)

type conversionTest struct {
	s, d any // 源和目的地

	// 以下是如果它们非零则使用
	wantint    int64
	wantuint   uint64
	wantstr    string
	wantbytes  []byte
	wantraw    RawBytes
	wantf32    float32
	wantf64    float64
	wanttime   time.Time
	wantbool   bool // 如果 d 是类型 *bool 时使用
	wanterr    string
	wantiface  any
	wantptr    *int64 // 如果非 nil，*d 的指向值必须等于 *wantptr
	wantnil    bool   // 如果为 true，*d 必须是 *int64(nil)
	wantusrdef userDefined
	wantusrstr userDefinedString
}

// 扫描到的目标变量。
var (
	scanstr    string
	scanbytes  []byte
	scanraw    RawBytes
	scanint    int
	scanuint8  uint8
	scanuint16 uint16
	scanbool   bool
	scanf32    float32
	scanf64    float64
	scantime   time.Time
	scanptr    *int64
	scaniface  any
)

func conversionTests() []conversionTest {
	// 返回一个新的实例进行测试，以便 "go test -count 2" 正常工作。
	return []conversionTest{
		// 精确转换（目标指针类型与源类型匹配）
		{s: "foo", d: &scanstr, wantstr: "foo"},
		{s: 123, d: &scanint, wantint: 123},
		{s: someTime, d: &scantime, wanttime: someTime},

		// 转换为字符串
		{s: "string", d: &scanstr, wantstr: "string"},
		{s: []byte("byteslice"), d: &scanstr, wantstr: "byteslice"},
		{s: 123, d: &scanstr, wantstr: "123"},
		{s: int8(123), d: &scanstr, wantstr: "123"},
		{s: int64(123), d: &scanstr, wantstr: "123"},
		{s: uint8(123), d: &scanstr, wantstr: "123"},
		{s: uint16(123), d: &scanstr, wantstr: "123"},
		{s: uint32(123), d: &scanstr, wantstr: "123"},
		{s: uint64(123), d: &scanstr, wantstr: "123"},
		{s: 1.5, d: &scanstr, wantstr: "1.5"},

		// 从 time.Time：
		{s: time.Unix(1, 0).UTC(), d: &scanstr, wantstr: "1970-01-01T00:00:01Z"},
		{s: time.Unix(1453874597, 0).In(time.FixedZone("here", -3600*8)), d: &scanstr, wantstr: "2016-01-26T22:03:17-08:00"},
		{s: time.Unix(1, 2).UTC(), d: &scanstr, wantstr: "1970-01-01T00:00:01.000000002Z"},
		{s: time.Time{}, d: &scanstr, wantstr: "0001-01-01T00:00:00Z"},
		{s: time.Unix(1, 2).UTC(), d: &scanbytes, wantbytes: []byte("1970-01-01T00:00:01.000000002Z")},
		{s: time.Unix(1, 2).UTC(), d: &scaniface, wantiface: time.Unix(1, 2).UTC()},

		// 转换为 []byte
		{s: nil, d: &scanbytes, wantbytes: nil},
		{s: "string", d: &scanbytes, wantbytes: []byte("string")},
		{s: []byte("byteslice"), d: &scanbytes, wantbytes: []byte("byteslice")},
		{s: 123, d: &scanbytes, wantbytes: []byte("123")},
		{s: int8(123), d: &scanbytes, wantbytes: []byte("123")},
		{s: int64(123), d: &scanbytes, wantbytes: []byte("123")},
		{s: uint8(123), d: &scanbytes, wantbytes: []byte("123")},
		{s: uint16(123), d: &scanbytes, wantbytes: []byte("123")},
		{s: uint32(123), d: &scanbytes, wantbytes: []byte("123")},
		{s: uint64(123), d: &scanbytes, wantbytes: []byte("123")},
		{s: 1.5, d: &scanbytes, wantbytes: []byte("1.5")},

		// 转换为 RawBytes
		{s: nil, d: &scanraw, wantraw: nil},
		{s: []byte("byteslice"), d: &scanraw, wantraw: RawBytes("byteslice")},
		{s: "string", d: &scanraw, wantraw: RawBytes("string")},
		{s: 123, d: &scanraw, wantraw: RawBytes("123")},
		{s: int8(123), d: &scanraw, wantraw: RawBytes("123")},
		{s: int64(123), d: &scanraw, wantraw: RawBytes("123")},
		{s: uint8(123), d: &scanraw, wantraw: RawBytes("123")},
		{s: uint16(123), d: &scanraw, wantraw: RawBytes("123")},
		{s: uint32(123), d: &scanraw, wantraw: RawBytes("123")},
		{s: uint64(123), d: &scanraw, wantraw: RawBytes("123")},
		{s: 1.5, d: &scanraw, wantraw: RawBytes("1.5")},
		// time.Time 已放在此处以检查 RawBytes 切片
		// 在调用 time.Time.AppendFormat 时是否被正确重置。
		{s: time.Unix(2, 5).UTC(), d: &scanraw, wantraw: RawBytes("1970-01-01T00:00:02.000000005Z")},

		// 字符串转整数
		{s: "255", d: &scanuint8, wantuint: 255},
		{s: "256", d: &scanuint8, wanterr: "converting driver.Value type string (\"256\") to a uint8: value out of range"},
		{s: "256", d: &scanuint16, wantuint: 256},
		{s: "-1", d: &scanint, wantint: -1},
		{s: "foo", d: &scanint, wanterr: "converting driver.Value type string (\"foo\") to a int: invalid syntax"},

		// int64 转为较小的整数
		{s: int64(5), d: &scanuint8, wantuint: 5},
		{s: int64(256), d: &scanuint8, wanterr: "converting driver.Value type int64 (\"256\") to a uint8: value out of range"},
		{s: int64(256), d: &scanuint16, wantuint: 256},
		{s: int64(65536), d: &scanuint16, wanterr: "converting driver.Value type int64 (\"65536\") to a uint16: value out of range"},

		// 真布尔值
		{s: true, d: &scanbool, wantbool: true},
		{s: "True", d: &scanbool, wantbool: true},
		{s: "TRUE", d: &scanbool, wantbool: true},
		{s: "1", d: &scanbool, wantbool: true},
		{s: 1, d: &scanbool, wantbool: true},
		{s: int64(1), d: &scanbool, wantbool: true},
		{s: uint16(1), d: &scanbool, wantbool: true},

		// 假布尔值
		{s: false, d: &scanbool, wantbool: false},
		{s: "false", d: &scanbool, wantbool: false},
		{s: "FALSE", d: &scanbool, wantbool: false},
		{s: "0", d: &scanbool, wantbool: false},
		{s: 0, d: &scanbool, wantbool: false},
		{s: int64(0), d: &scanbool, wantbool: false},
		{s: uint16(0), d: &scanbool, wantbool: false},

		// 非布尔值
		{s: "yup", d: &scanbool, wanterr: `sql/driver: couldn't convert "yup" into type bool`},
		{s: 2, d: &scanbool, wanterr: `sql/driver: couldn't convert 2 into type bool`},

		// 浮点数
		{s: float64(1.5), d: &scanf64, wantf64: float64(1.5)},
		{s: int64(1), d: &scanf64, wantf64: float64(1)},
		{s: float64(1.5), d: &scanf32, wantf32: float32(1.5)},
		{s: "1.5", d: &scanf32, wantf32: float32(1.5)},
		{s: "1.5", d: &scanf64, wantf64: float64(1.5)},

		// 指针
		{s: any(nil), d: &scanptr, wantnil: true},
		{s: int64(42), d: &scanptr, wantptr: &answer},

		// 转换为 interface{}
		{s: float64(1.5), d: &scaniface, wantiface: float64(1.5)},
		{s: int64(1), d: &scaniface, wantiface: int64(1)},
		{s: "str", d: &scaniface, wantiface: "str"},
		{s: []byte("byteslice"), d: &scaniface, wantiface: []byte("byteslice")},
		{s: true, d: &scaniface, wantiface: true},
		{s: nil, d: &scaniface},
		{s: []byte(nil), d: &scaniface, wantiface: []byte(nil)},

		// 转换为用户定义类型
		{s: 1.5, d: new(userDefined), wantusrdef: 1.5},
		{s: int64(123), d: new(userDefined), wantusrdef: 123},
		{s: "1.5", d: new(userDefined), wantusrdef: 1.5},
		{s: []byte{1, 2, 3}, d: new(userDefinedSlice), wanterr: `unsupported Scan, storing driver.Value type []uint8 into type *sql.userDefinedSlice`},
		{s: "str", d: new(userDefinedString), wantusrstr: "str"},

		// 其他错误
		{s: complex(1, 2), d: &scanstr, wanterr: `unsupported Scan, storing driver.Value type complex128 into type *string`},
	}
}

func intPtrValue(intptr any) any {
	return reflect.Indirect(reflect.Indirect(reflect.ValueOf(intptr))).Int()
}

func intValue(intptr any) int64 {
	return reflect.Indirect(reflect.ValueOf(intptr)).Int()
}

func uintValue(intptr any) uint64 {
	return reflect.Indirect(reflect.ValueOf(intptr)).Uint()
}

func float64Value(ptr any) float64 {
	return *(ptr.(*float64))
}

func float32Value(ptr any) float32 {
	return *(ptr.(*float32))
}

func timeValue(ptr any) time.Time {
	return *(ptr.(*time.Time))
}

func TestConversions(t *testing.T) {
	for n, ct := range conversionTests() {
		err := convertAssign(ct.d, ct.s)
		errstr := ""
		if err != nil {
			errstr = err.Error()
		}
		errf := func(format string, args ...any) {
			base := fmt.Sprintf("convertAssign #%d: for %v (%T) -> %T, ", n, ct.s, ct.s, ct.d)
			t.Errorf(base+format, args...)
		}
		if errstr != ct.wanterr {
			errf("got error %q, want error %q", errstr, ct.wanterr)
		}
		if ct.wantstr != "" && ct.wantstr != scanstr {
			errf("want string %q, got %q", ct.wantstr, scanstr)
		}
		if ct.wantbytes != nil && string(ct.wantbytes) != string(scanbytes) {
			errf("want byte %q, got %q", ct.wantbytes, scanbytes)
		}
		if ct.wantraw != nil && string(ct.wantraw) != string(scanraw) {
			errf("want RawBytes %q, got %q", ct.wantraw, scanraw)
		}
		if ct.wantint != 0 && ct.wantint != intValue(ct.d) {
			errf("want int %d, got %d", ct.wantint, intValue(ct.d))
		}
		if ct.wantuint != 0 && ct.wantuint != uintValue(ct.d) {
			errf("want uint %d, got %d", ct.wantuint, uintValue(ct.d))
		}
		if ct.wantf32 != 0 && ct.wantf32 != float32Value(ct.d) {
			errf("want float32 %v, got %v", ct.wantf32, float32Value(ct.d))
		}
		if ct.wantf64 != 0 && ct.wantf64 != float64Value(ct.d) {
			errf("want float32 %v, got %v", ct.wantf64, float64Value(ct.d))
		}
		if bp, boolTest := ct.d.(*bool); boolTest && *bp != ct.wantbool && ct.wanterr == "" {
			errf("want bool %v, got %v", ct.wantbool, *bp)
		}
		if !ct.wanttime.IsZero() && !ct.wanttime.Equal(timeValue(ct.d)) {
			errf("want time %v, got %v", ct.wanttime, timeValue(ct.d))
		}
		if ct.wantnil && *ct.d.(**int64) != nil {
			errf("want nil, got %v", intPtrValue(ct.d))
		}
		if ct.wantptr != nil {
			if *ct.d.(**int64) == nil {
				errf("want pointer to %v, got nil", *ct.wantptr)
			} else if *ct.wantptr != intPtrValue(ct.d) {
				errf("want pointer to %v, got %v", *ct.wantptr, intPtrValue(ct.d))
			}
		}
		if ifptr, ok := ct.d.(*any); ok {
			if !reflect.DeepEqual(ct.wantiface, scaniface) {
				errf("want interface %#v, got %#v", ct.wantiface, scaniface)
				continue
			}
			if srcBytes, ok := ct.s.([]byte); ok {
				dstBytes := (*ifptr).([]byte)
				if len(srcBytes) > 0 && &dstBytes[0] == &srcBytes[0] {
					errf("copy into interface{} didn't copy []byte data")
				}
			}
		}
		if ct.wantusrdef != 0 && ct.wantusrdef != *ct.d.(*userDefined) {
			errf("want userDefined %f, got %f", ct.wantusrdef, *ct.d.(*userDefined))
		}
		if len(ct.wantusrstr) != 0 && ct.wantusrstr != *ct.d.(*userDefinedString) {
			errf("want userDefined %q, got %q", ct.wantusrstr, *ct.d.(*userDefinedString))
		}
	}
}

func TestNullString(t *testing.T) {
	var ns NullString
	convertAssign(&ns, []byte("foo"))
	if !ns.Valid {
		t.Errorf("expecting not null")
	}
	if ns.String != "foo" {
		t.Errorf("expecting foo; got %q", ns.String)
	}
	convertAssign(&ns, nil)
	if ns.Valid {
		t.Errorf("expecting null on nil")
	}
	if ns.String != "" {
		t.Errorf("expecting blank on nil; got %q", ns.String)
	}
}

type valueConverterTest struct {
	c       driver.ValueConverter
	in, out any
	err     string
}

var valueConverterTests = []valueConverterTest{
	{driver.DefaultParameterConverter, NullString{"hi", true}, "hi", ""},
	{driver.DefaultParameterConverter, NullString{"", false}, nil, ""},
}

func TestValueConverters(t *testing.T) {
	for i, tt := range valueConverterTests {
		out, err := tt.c.ConvertValue(tt.in)
		goterr := ""
		if err != nil {
			goterr = err.Error()
		}
		if goterr != tt.err {
			t.Errorf("test %d: %T(%T(%v)) error = %q; want error = %q",
				i, tt.c, tt.in, tt.in, goterr, tt.err)
		}
		if tt.err != "" {
			continue
		}
		if !reflect.DeepEqual(out, tt.out) {
			t.Errorf("test %d: %T(%T(%v)) = %v (%T); want %v (%T)",
				i, tt.c, tt.in, tt.in, out, out, tt.out, tt.out)
		}
	}
}

// Tests that assigning to RawBytes doesn't allocate (and also works).
func TestRawBytesAllocs(t *testing.T) {
	var tests = []struct {
		name string
		in   any
		want string
	}{
		{"uint64", uint64(12345678), "12345678"},
		{"uint32", uint32(1234), "1234"},
		{"uint16", uint16(12), "12"},
		{"uint8", uint8(1), "1"},
		{"uint", uint(123), "123"},
		{"int", int(123), "123"},
		{"int8", int8(1), "1"},
		{"int16", int16(12), "12"},
		{"int32", int32(1234), "1234"},
		{"int64", int64(12345678), "12345678"},
		{"float32", float32(1.5), "1.5"},
		{"float64", float64(64), "64"},
		{"bool", false, "false"},
		{"time", time.Unix(2, 5).UTC(), "1970-01-01T00:00:02.000000005Z"},
	}
	if asan.Enabled {
		t.Skip("test allocates more with -asan; see #70079")
	}

	var buf RawBytes
	rows := &Rows{}
	test := func(name string, in any, want string) {
		if err := convertAssignRows(&buf, in, rows); err != nil {
			t.Fatalf("%s: convertAssign = %v", name, err)
		}
		match := len(buf) == len(want)
		if match {
			for i, b := range buf {
				if want[i] != b {
					match = false
					break
				}
			}
		}
		if !match {
			t.Fatalf("%s: got %q (len %d); want %q (len %d)", name, buf, len(buf), want, len(want))
		}
	}

	n := testing.AllocsPerRun(100, func() {
		for _, tt := range tests {
			rows.raw = rows.raw[:0]
			test(tt.name, tt.in, tt.want)
		}
	})

	// 下面的数字仅对 64 位接口字大小有效，
	// 和 gc。对于 32 位字，有更多的 convT2E allocs，
	// 对于 gccgo，目前只有指针进入接口数据。
	// 所以目前只关心 amd64 gc。
	measureAllocs := false
	switch runtime.GOARCH {
	case "amd64", "arm64":
		measureAllocs = runtime.Compiler == "gc"
	}

	if n > 0.5 && measureAllocs {
		t.Fatalf("allocs = %v; want 0", n)
	}

	// 这个涉及 convT2E 分配，string -> interface{}
	n = testing.AllocsPerRun(100, func() {
		test("string", "foo", "foo")
	})
	if n > 1.5 && measureAllocs {
		t.Fatalf("allocs = %v; want max 1", n)
	}
}

// https://golang.org/issues/13905
func TestUserDefinedBytes(t *testing.T) {
	type userDefinedBytes []byte
	var u userDefinedBytes
	v := []byte("foo")

	convertAssign(&u, v)
	if &u[0] == &v[0] {
		t.Fatal("userDefinedBytes 可能获得脏驱动程序内存")
	}
}

type Valuer_V string

func (v Valuer_V) Value() (driver.Value, error) {
	return strings.ToUpper(string(v)), nil
}

type Valuer_P string

func (p *Valuer_P) Value() (driver.Value, error) {
	if p == nil {
		return "nil-to-str", nil
	}
	return strings.ToUpper(string(*p)), nil
}

func TestDriverArgs(t *testing.T) {
	var nilValuerVPtr *Valuer_V
	var nilValuerPPtr *Valuer_P
	var nilStrPtr *string
	tests := []struct {
		args []any
		want []driver.NamedValue
	}{
		0: {
			args: []any{Valuer_V("foo")},
			want: []driver.NamedValue{
				{
					Ordinal: 1,
					Value:   "FOO",
				},
			},
		},
		1: {
			args: []any{nilValuerVPtr},
			want: []driver.NamedValue{
				{
					Ordinal: 1,
					Value:   nil,
				},
			},
		},
		2: {
			args: []any{nilValuerPPtr},
			want: []driver.NamedValue{
				{
					Ordinal: 1,
					Value:   "nil-to-str",
				},
			},
		},
		3: {
			args: []any{"plain-str"},
			want: []driver.NamedValue{
				{
					Ordinal: 1,
					Value:   "plain-str",
				},
			},
		},
		4: {
			args: []any{nilStrPtr},
			want: []driver.NamedValue{
				{
					Ordinal: 1,
					Value:   nil,
				},
			},
		},
	}
	for i, tt := range tests {
		ds := &driverStmt{Locker: &sync.Mutex{}, si: stubDriverStmt{nil}}
		got, err := driverArgsConnLocked(nil, ds, tt.args)
		if err != nil {
			t.Errorf("test[%d]: %v", i, err)
			continue
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("test[%d]: got %v, want %v", i, got, tt.want)
		}
	}
}

type dec struct {
	form        byte
	neg         bool
	coefficient [16]byte
	exponent    int32
}

func (d dec) Decompose(buf []byte) (form byte, negative bool, coefficient []byte, exponent int32) {
	coef := make([]byte, 16)
	copy(coef, d.coefficient[:])
	return d.form, d.neg, coef, d.exponent
}

func (d *dec) Compose(form byte, negative bool, coefficient []byte, exponent int32) error {
	switch form {
	default:
		return fmt.Errorf("unknown form %d", form)
	case 1, 2:
		d.form = form
		d.neg = negative
		return nil
	case 0:
	}
	d.form = form
	d.neg = negative
	d.exponent = exponent

	// This isn't strictly correct, as the extra bytes could be all zero,
	// ignore this for this test.
	if len(coefficient) > 16 {
		return fmt.Errorf("coefficient too large")
	}
	copy(d.coefficient[:], coefficient)

	return nil
}

type decFinite struct {
	neg         bool
	coefficient [16]byte
	exponent    int32
}

func (d decFinite) Decompose(buf []byte) (form byte, negative bool, coefficient []byte, exponent int32) {
	coef := make([]byte, 16)
	copy(coef, d.coefficient[:])
	return 0, d.neg, coef, d.exponent
}

func (d *decFinite) Compose(form byte, negative bool, coefficient []byte, exponent int32) error {
	switch form {
	default:
		return fmt.Errorf("unknown form %d", form)
	case 1, 2:
		return fmt.Errorf("unsupported form %d", form)
	case 0:
	}
	d.neg = negative
	d.exponent = exponent

	// This isn't strictly correct, as the extra bytes could be all zero,
	// ignore this for this test.
	if len(coefficient) > 16 {
		return fmt.Errorf("coefficient too large")
	}
	copy(d.coefficient[:], coefficient)

	return nil
}

func TestDecimal(t *testing.T) {
	list := []struct {
		name string
		in   decimalDecompose
		out  dec
		err  bool
	}{
		{name: "same", in: dec{exponent: -6}, out: dec{exponent: -6}},

		// 通过使用不同的类型来确保反射不用于分配值。
		{name: "diff", in: decFinite{exponent: -6}, out: dec{exponent: -6}},

		{name: "bad-form", in: dec{form: 200}, err: true},
	}
	for _, item := range list {
		t.Run(item.name, func(t *testing.T) {
			out := dec{}
			err := convertAssign(&out, item.in)
			if item.err {
				if err == nil {
					t.Fatalf("unexpected nil error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(out, item.out) {
				t.Fatalf("got %#v want %#v", out, item.out)
			}
		})
	}
}
