// 版权所有 2012 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package p1

import (
	ptwo "p2"
)

const (
	ConstChase2 = constChase // 前向声明到未导出标识符
	constChase  = AIsLowerA  // 前向声明到导出标识符

	// Deprecated: 请使用 B。
	A         = 1
	a         = 11
	A64 int64 = 1

	AIsLowerA = a // 之前已声明
)

const (
	ConversionConst = MyInt(5)
)

// 来自函数调用的变量。
var (
	V = ptwo.F()
	// Deprecated: 请使用 WError。
	VError = BarE()
	V1     = Bar1(1, 2, 3)
	V2     = ptwo.G()
)

// 带有类型转换的变量：
var (
	StrConv  = string("foo")
	ByteConv = []byte("foo")
)

var ChecksumError = ptwo.NewError("gzip checksum error")

const B0 = 2
const StrConst = "foo"
const FloatConst = 1.5

type myInt int

type MyInt int

type Time struct{}

type S struct {
	// Deprecated: 请使用 PublicTime。
	Public     *int
	private    *int
	PublicTime Time
}

// Deprecated: 请使用 URI。
type URL struct{}

type EmbedURLPtr struct {
	*URL
}

type S2 struct {
	// Deprecated: 请使用 T。
	S
	Extra bool
}

var X0 int64

var (
	Y int
	X I
)

type Namer interface {
	Name() string
}

type I interface {
	Namer
	ptwo.Twoer
	Set(name string, balance int64)
	// Deprecated: 请使用 GetNamed。
	Get(string) int64
	GetNamed(string) (balance int64)
	private()
}

type Public interface {
	X()
	Y()
}

// Deprecated: 请使用 Unexported。
type Private interface {
	X()
	y()
}

type Error interface {
	error
	Temporary() bool
}

func (myInt) privateTypeMethod()           {}
func (myInt) CapitalMethodUnexportedType() {}

// Deprecated: 请使用 TMethod。
func (s *S2) SMethod(x int8, y int16, z int64) {}

type s struct{}

func (s) method()
func (s) Method()

func (S) StructValueMethod()
func (ignored S) StructValueMethodNamedRecv()

func (s *S2) unexported(x int8, y int16, z int64) {}

func Bar(x int8, y int16, z int64)                  {}
func Bar1(x int8, y int16, z int64) uint64          {}
func Bar2(x int8, y int16, z int64) (uint8, uint64) {}
func BarE() Error                                   {}

func unexported(x int8, y int16, z int64) {}

func TakesFunc(f func(dontWantName int) int)

type Codec struct {
	Func func(x int, y int) (z int)
}

type SI struct {
	I int
}

var SIVal = SI{}
var SIPtr = &SI{}
var SIPtr2 *SI

type T struct {
	common
}

type B struct {
	common
}

type common struct {
	i int
}

type TPtrUnexported struct {
	*common
}

type TPtrExported struct {
	*Embedded
}

type FuncType func(x, y int, s string) (b *B, err error)

type Embedded struct{}

func PlainFunc(x, y int, s string) (b *B, err error)

func (*Embedded) OnEmbedded() {}

func (*T) JustOnT()             {}
func (*B) JustOnB()             {}
func (*common) OnBothTandBPtr() {}
func (common) OnBothTandBVal()  {}

type EmbedSelector struct {
	Time
}

const (
	foo          = "foo"
	foo2  string = "foo2"
	truth        = foo == "foo" || foo2 == "foo2"
)

func ellipsis(...string) {}

func Now() Time {
	var now Time
	return now
}

var x = &S{
	Public:     nil,
	private:    nil,
	PublicTime: Now(),
}

var parenExpr = (1 + 5)

var funcLit = func() {}

var m map[string]int

var chanVar chan int

var ifaceVar any = 5

var assertVar = ifaceVar.(int)

var indexVar = m["foo"]

var Byte byte
var ByteFunc func(byte) rune

type ByteStruct struct {
	B byte
	R rune
}
