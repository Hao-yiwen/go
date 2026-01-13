// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

/*
Package flag 实现命令行标志解析。

# 用法

使用 [flag.String]、[Bool]、[Int] 等定义标志。

这声明一个整数标志 -n，存储在指针 nFlag 中，类型为 *int：

	import "flag"
	var nFlag = flag.Int("n", 1234, "help message for flag n")

如果需要，您可以使用 Var() 函数将标志绑定到变量。

	var flagvar int
	func init() {
		flag.IntVar(&flagvar, "flagname", 1234, "help message for flagname")
	}

或者您可以创建满足 Value 接口的自定义标志（使用
指针接收器），并通过以下方式将其与标志解析耦合：

	flag.Var(&flagVal, "name", "help message for flagname")

对于此类标志，默认值只是变量的初始值。

定义所有标志后，调用

	flag.Parse()

将命令行解析为已定义的标志。

然后可以直接使用标志。如果您使用标志本身，
它们都是指针；如果绑定到变量，它们就是值。

	fmt.Println("ip has value ", *ip)
	fmt.Println("flagvar has value ", flagvar)

解析后，标志后面的参数可用作
切片 [flag.Args] 或单独使用 [flag.Arg](i)。
参数从 0 到 [flag.NArg]-1 进行索引。

# 命令行标志语法

允许以下形式：

	-flag
	--flag   // 也允许双破折号
	-flag=x
	-flag x  // 仅用于非布尔标志

可以使用一条或两条破折号；它们是等价的。
最后一种形式不允许用于布尔标志，因为
命令的含义

	cmd -x *

其中 * 是 Unix shell 通配符，如果存在名为 0、false 等的文件，则会更改。
您必须使用 -flag=false 形式来关闭
布尔标志。

标志解析在第一个非标志参数之前停止
（"-" 是非标志参数）或在终止符 "--" 之后停止。

整数标志接受 1234、0664、0x1234 并且可能是负数。
布尔标志可能是：

	1, 0, t, f, T, F, true, false, TRUE, FALSE, True, False

持续时间标志接受对 time.ParseDuration 有效的任何输入。

命令行标志的默认集由
顶级函数控制。[FlagSet] 类型允许定义
独立的标志集，例如在命令行界面中
实现子命令。[FlagSet] 的方法与
命令行标志集的顶级函数类似。
*/
package flag

import (
	"encoding"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"
)

// ErrHelp 是在调用 -help 或 -h 标志时返回的错误
// 但未定义此类标志。
var ErrHelp = errors.New("flag: help requested")

// errParse 是在 Set 标志值解析失败时返回的，例如为 Int 提供了无效的整数。
// 然后它通过 failf 进行包装以提供更多信息。
var errParse = errors.New("parse error")

// errRange 是在 Set 标志值超出范围时返回的。
// 然后它通过 failf 进行包装以提供更多信息。
var errRange = errors.New("value out of range")

func numError(err error) error {
	ne, ok := err.(*strconv.NumError)
	if !ok {
		return err
	}
	if ne.Err == strconv.ErrSyntax {
		return errParse
	}
	if ne.Err == strconv.ErrRange {
		return errRange
	}
	return err
}

// -- bool 值
type boolValue bool

func newBoolValue(val bool, p *bool) *boolValue {
	*p = val
	return (*boolValue)(p)
}

func (b *boolValue) Set(s string) error {
	v, err := strconv.ParseBool(s)
	if err != nil {
		err = errParse
	}
	*b = boolValue(v)
	return err
}

func (b *boolValue) Get() any { return bool(*b) }

func (b *boolValue) String() string { return strconv.FormatBool(bool(*b)) }

func (b *boolValue) IsBoolFlag() bool { return true }

// 可选接口，用于指示可以在不
// 提供 "=value" 文本的情况下提供的布尔标志
type boolFlag interface {
	Value
	IsBoolFlag() bool
}

// -- int 值
type intValue int

func newIntValue(val int, p *int) *intValue {
	*p = val
	return (*intValue)(p)
}

func (i *intValue) Set(s string) error {
	v, err := strconv.ParseInt(s, 0, strconv.IntSize)
	if err != nil {
		err = numError(err)
	}
	*i = intValue(v)
	return err
}

func (i *intValue) Get() any { return int(*i) }

func (i *intValue) String() string { return strconv.Itoa(int(*i)) }

// -- int64 值
type int64Value int64

func newInt64Value(val int64, p *int64) *int64Value {
	*p = val
	return (*int64Value)(p)
}

func (i *int64Value) Set(s string) error {
	v, err := strconv.ParseInt(s, 0, 64)
	if err != nil {
		err = numError(err)
	}
	*i = int64Value(v)
	return err
}

func (i *int64Value) Get() any { return int64(*i) }

func (i *int64Value) String() string { return strconv.FormatInt(int64(*i), 10) }

// -- uint 值
type uintValue uint

func newUintValue(val uint, p *uint) *uintValue {
	*p = val
	return (*uintValue)(p)
}

func (i *uintValue) Set(s string) error {
	v, err := strconv.ParseUint(s, 0, strconv.IntSize)
	if err != nil {
		err = numError(err)
	}
	*i = uintValue(v)
	return err
}

func (i *uintValue) Get() any { return uint(*i) }

func (i *uintValue) String() string { return strconv.FormatUint(uint64(*i), 10) }

// -- uint64 值
type uint64Value uint64

func newUint64Value(val uint64, p *uint64) *uint64Value {
	*p = val
	return (*uint64Value)(p)
}

func (i *uint64Value) Set(s string) error {
	v, err := strconv.ParseUint(s, 0, 64)
	if err != nil {
		err = numError(err)
	}
	*i = uint64Value(v)
	return err
}

func (i *uint64Value) Get() any { return uint64(*i) }

func (i *uint64Value) String() string { return strconv.FormatUint(uint64(*i), 10) }

// -- string 值
type stringValue string

func newStringValue(val string, p *string) *stringValue {
	*p = val
	return (*stringValue)(p)
}

func (s *stringValue) Set(val string) error {
	*s = stringValue(val)
	return nil
}

func (s *stringValue) Get() any { return string(*s) }

func (s *stringValue) String() string { return string(*s) }

// -- float64 值
type float64Value float64

func newFloat64Value(val float64, p *float64) *float64Value {
	*p = val
	return (*float64Value)(p)
}

func (f *float64Value) Set(s string) error {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		err = numError(err)
	}
	*f = float64Value(v)
	return err
}

func (f *float64Value) Get() any { return float64(*f) }

func (f *float64Value) String() string { return strconv.FormatFloat(float64(*f), 'g', -1, 64) }

// -- time.Duration 值
type durationValue time.Duration

func newDurationValue(val time.Duration, p *time.Duration) *durationValue {
	*p = val
	return (*durationValue)(p)
}

func (d *durationValue) Set(s string) error {
	v, err := time.ParseDuration(s)
	if err != nil {
		err = errParse
	}
	*d = durationValue(v)
	return err
}

func (d *durationValue) Get() any { return time.Duration(*d) }

func (d *durationValue) String() string { return (*time.Duration)(d).String() }

// -- encoding.TextUnmarshaler 值
type textValue struct{ p encoding.TextUnmarshaler }

func newTextValue(val encoding.TextMarshaler, p encoding.TextUnmarshaler) textValue {
	ptrVal := reflect.ValueOf(p)
	if ptrVal.Kind() != reflect.Ptr {
		panic("variable value type must be a pointer")
	}
	defVal := reflect.ValueOf(val)
	if defVal.Kind() == reflect.Ptr {
		defVal = defVal.Elem()
	}
	if defVal.Type() != ptrVal.Type().Elem() {
		panic(fmt.Sprintf("default type does not match variable type: %v != %v", defVal.Type(), ptrVal.Type().Elem()))
	}
	ptrVal.Elem().Set(defVal)
	return textValue{p}
}

func (v textValue) Set(s string) error {
	return v.p.UnmarshalText([]byte(s))
}

func (v textValue) Get() any {
	return v.p
}

func (v textValue) String() string {
	if m, ok := v.p.(encoding.TextMarshaler); ok {
		if b, err := m.MarshalText(); err == nil {
			return string(b)
		}
	}
	return ""
}

// -- func 值
type funcValue func(string) error

func (f funcValue) Set(s string) error { return f(s) }

func (f funcValue) String() string { return "" }

// -- boolFunc 值
type boolFuncValue func(string) error

func (f boolFuncValue) Set(s string) error { return f(s) }

func (f boolFuncValue) String() string { return "" }

func (f boolFuncValue) IsBoolFlag() bool { return true }

// Value 是存储在标志中的动态值的接口。
// （默认值表示为字符串。）
//
// 如果 Value 有一个返回 true 的 IsBoolFlag() bool 方法，
// 命令行解析器将 -name 等同于 -name=true
// 而不是使用下一个命令行参数。
//
// Set 为每个存在的标志调用一次，按命令行顺序调用。
// flag 包可以使用零值接收器调用 [String] 方法，
// 例如 nil 指针。
type Value interface {
	String() string
	Set(string) error
}

// Getter 是一个接口，允许检索 [Value] 的内容。
// 它包装了 [Value] 接口，而不是它的一部分，因为它
// 在 Go 1 之后出现，需要兼容性规则。该包提供的所有 [Value] 类型
// 都满足 [Getter] 接口，除了 [Func] 使用的类型。
type Getter interface {
	Value
	Get() any
}

// ErrorHandling 定义了当解析失败时 [FlagSet.Parse] 的行为。
type ErrorHandling int

// 这些常量在解析失败时导致 [FlagSet.Parse] 表现如下所述。
const (
	ContinueOnError ErrorHandling = iota // 返回描述性错误。
	ExitOnError                          // 调用 os.Exit(2) 或对于 -h/-help 调用 Exit(0)。
	PanicOnError                         // 使用描述性错误调用 panic。
)

// FlagSet 代表一组已定义的标志。FlagSet 的零值
// 没有名字，有 [ContinueOnError] 错误处理。
//
// [Flag] 名称必须在 FlagSet 内唯一。尝试定义一个名称
// 已经在使用中的标志会导致 panic。
type FlagSet struct {
	// Usage 是在解析标志时发生错误时调用的函数。
	// 该字段是一个函数（不是方法），可以更改为指向
	// 自定义错误处理器。Usage 被调用后发生的事情取决于
	// ErrorHandling 设置；对于命令行，这默认为
	// ExitOnError，它在调用 Usage 后退出程序。
	Usage func()

	name          string
	parsed        bool
	actual        map[string]*Flag
	formal        map[string]*Flag
	args          []string // 标志后的参数
	errorHandling ErrorHandling
	output        io.Writer         // nil 表示 stderr；使用 Output() 访问器
	undef         map[string]string // Set 时不存在的标志
}

// Flag 代表标志的状态。
type Flag struct {
	Name     string // 如命令行中出现的名称
	Usage    string // 帮助消息
	Value    Value  // 设置的值
	DefValue string // 默认值（作为文本）；用于用法消息
}

// sortFlags 按字典序排序顺序将标志作为切片返回。
func sortFlags(flags map[string]*Flag) []*Flag {
	result := make([]*Flag, len(flags))
	i := 0
	for _, f := range flags {
		result[i] = f
		i++
	}
	slices.SortFunc(result, func(a, b *Flag) int {
		return strings.Compare(a.Name, b.Name)
	})
	return result
}

// Output 返回用法和错误消息的目标。如果
// 输出未设置或设置为 nil，则返回 [os.Stderr]。
func (f *FlagSet) Output() io.Writer {
	if f.output == nil {
		return os.Stderr
	}
	return f.output
}

// Name 返回标志集的名称。
func (f *FlagSet) Name() string {
	return f.name
}

// ErrorHandling 返回标志集的错误处理行为。
func (f *FlagSet) ErrorHandling() ErrorHandling {
	return f.errorHandling
}

// SetOutput 设置用法和错误消息的目标。
// 如果输出为 nil，则使用 [os.Stderr]。
func (f *FlagSet) SetOutput(output io.Writer) {
	f.output = output
}

// VisitAll 按字典序顺序访问标志，为每个标志调用 fn。
// 它访问所有标志，包括未设置的标志。
func (f *FlagSet) VisitAll(fn func(*Flag)) {
	for _, flag := range sortFlags(f.formal) {
		fn(flag)
	}
}

// VisitAll 按字典序顺序访问命令行标志，为每个标志调用
// fn。它访问所有标志，包括未设置的标志。
func VisitAll(fn func(*Flag)) {
	CommandLine.VisitAll(fn)
}

// Visit 按字典序顺序访问标志，为每个标志调用 fn。
// 它仅访问已设置的标志。
func (f *FlagSet) Visit(fn func(*Flag)) {
	for _, flag := range sortFlags(f.actual) {
		fn(flag)
	}
}

// Visit 按字典序顺序访问命令行标志，为每个标志调用 fn。
// 它仅访问已设置的标志。
func Visit(fn func(*Flag)) {
	CommandLine.Visit(fn)
}

// Lookup 返回指定标志的 [Flag] 结构，如果不存在则返回 nil。
func (f *FlagSet) Lookup(name string) *Flag {
	return f.formal[name]
}

// Lookup 返回指定命令行标志的 [Flag] 结构，
// 如果不存在则返回 nil。
func Lookup(name string) *Flag {
	return CommandLine.formal[name]
}

// Set 设置指定标志的值。
func (f *FlagSet) Set(name, value string) error {
	return f.set(name, value)
}
func (f *FlagSet) set(name, value string) error {
	flag, ok := f.formal[name]
	if !ok {
		// 记住一个未定义的标志正在被设置。
		// 在这种情况下我们返回一个错误，但此外如果
		// 随后定义了该标志，我们想在
		// 定义点 panic。
		// 这是一个问题，当定义和
		// Set 调用都在 init 代码中，并且无论什么
		// 原因 init 代码改变求值顺序时发生。
		// 参见 issue 57411。
		_, file, line, ok := runtime.Caller(2)
		if !ok {
			file = "?"
			line = 0
		}
		if f.undef == nil {
			f.undef = map[string]string{}
		}
		f.undef[name] = fmt.Sprintf("%s:%d", file, line)

		return fmt.Errorf("no such flag -%v", name)
	}
	err := flag.Value.Set(value)
	if err != nil {
		return err
	}
	if f.actual == nil {
		f.actual = make(map[string]*Flag)
	}
	f.actual[name] = flag
	return nil
}

// Set 设置指定命令行标志的值。
func Set(name, value string) error {
	return CommandLine.set(name, value)
}

// isZeroValue 确定字符串是否代表标志的零
// 值。
func isZeroValue(flag *Flag, value string) (ok bool, err error) {
	// 构建标志的 Value 类型的零值，并查看是否
	// 调用其 String 方法的结果等于传入的值。
	// 这有效，除非 Value 类型本身是接口类型。
	typ := reflect.TypeOf(flag.Value)
	var z reflect.Value
	if typ.Kind() == reflect.Pointer {
		z = reflect.New(typ.Elem())
	} else {
		z = reflect.Zero(typ)
	}
	// 捕获调用 String 方法时的 panic，这不应该阻止
	// 用法消息被打印，但我们应该向
	// 用户报告，以便他们知道修复他们的代码。
	defer func() {
		if e := recover(); e != nil {
			if typ.Kind() == reflect.Pointer {
				typ = typ.Elem()
			}
			err = fmt.Errorf("panic calling String method on zero %v for flag %s: %v", typ, flag.Name, e)
		}
	}()
	return value == z.Interface().(Value).String(), nil
}

// UnquoteUsage 从标志的用法
// 字符串中提取反引号括起的名称并返回它和未引用的用法。
// 给定 "a `name` to show" 它返回 ("name", "a name to show")。
// 如果没有反引号，该名称是标志值类型的有根据的猜测，
// 或者如果标志是布尔的则为空字符串。
func UnquoteUsage(flag *Flag) (name string, usage string) {
	// 查找反引号括起的名称，但避免 strings 包。
	usage = flag.Usage
	for i := 0; i < len(usage); i++ {
		if usage[i] == '`' {
			for j := i + 1; j < len(usage); j++ {
				if usage[j] == '`' {
					name = usage[i+1 : j]
					usage = usage[:i] + name + usage[j+1:]
					return name, usage
				}
			}
			break // 只有一个反引号；使用类型名称。
		}
	}
	// 没有显式名称，因此使用类型（如果我们能找到的话）。
	name = "value"
	switch fv := flag.Value.(type) {
	case boolFlag:
		if fv.IsBoolFlag() {
			name = ""
		}
	case *durationValue:
		name = "duration"
	case *float64Value:
		name = "float"
	case *intValue, *int64Value:
		name = "int"
	case *stringValue:
		name = "string"
	case *uintValue, *uint64Value:
		name = "uint"
	}
	return
}

// PrintDefaults 将所有已定义的命令行标志的默认值打印到
// 标准错误（除非另有配置）。查看
// 全局函数 PrintDefaults 的文档以获取更多信息。
func (f *FlagSet) PrintDefaults() {
	var isZeroValueErrs []error
	f.VisitAll(func(flag *Flag) {
		var b strings.Builder
		fmt.Fprintf(&b, "  -%s", flag.Name) // - 前两个空格；参见接下来的两个注释。
		name, usage := UnquoteUsage(flag)
		if len(name) > 0 {
			b.WriteString(" ")
			b.WriteString(name)
		}
		// 单个 ASCII 字母的布尔标志非常常见，我们
		// 特别对待它们，将其用法放在同一行上。
		if b.Len() <= 4 { // space, space, '-', 'x'.
			b.WriteString("\t")
		} else {
			// tab 前四个空格可以触发良好的对齐
			// 用于 4 空格和 8 空格制表位。
			b.WriteString("\n    \t")
		}
		b.WriteString(strings.ReplaceAll(usage, "\n", "\n    \t"))

		// 仅当默认值与该标志类型的零值不同时打印默认值。
		if isZero, err := isZeroValue(flag, flag.DefValue); err != nil {
			isZeroValueErrs = append(isZeroValueErrs, err)
		} else if !isZero {
			if _, ok := flag.Value.(*stringValue); ok {
				// 在值上加引号
				fmt.Fprintf(&b, " (default %q)", flag.DefValue)
			} else {
				fmt.Fprintf(&b, " (default %v)", flag.DefValue)
			}
		}
		fmt.Fprint(f.Output(), b.String(), "\n")
	})
	// 如果在任何零 flag.Values 上调用 String 触发了 panic，打印
	// 完整默认值集之后的消息，以便程序员
	// 知道修复 panic。
	if errs := isZeroValueErrs; len(errs) > 0 {
		fmt.Fprintln(f.Output())
		for _, err := range errs {
			fmt.Fprintln(f.Output(), err)
		}
	}
}

// PrintDefaults 将一条用法消息打印到标准错误（除非另有配置），
// 显示所有已定义的
// 命令行标志的默认设置。
// 对于整数值标志 x，默认输出的形式为
//
//	-x int
//		usage-message-for-x (default 7)
//
// 对除了单字节名称的布尔标志外的任何标志，用法消息将
// 出现在单独的行上。对于布尔标志，类型被
// 省略，如果标志名称是一个字节，用法消息
// 出现在同一行上。如果
// 默认值是该类型的零值，则省略括号内的默认值。列出的类型（这里是 int）
// 可以通过在标志的用法
// 字符串中放置反引号括起的名称来更改；消息中的第一项
// 被视为要在消息中显示的参数名称，反引号
// 在显示时从消息中剥离。例如，给定
//
//	flag.String("I", "", "search `directory` for include files")
//
// 输出将为
//
//	-I directory
//		search directory for include files.
//
// 要更改标志消息的目标，请调用 [CommandLine].SetOutput。
func PrintDefaults() {
	CommandLine.PrintDefaults()
}

// defaultUsage 是打印用法消息的默认函数。
func (f *FlagSet) defaultUsage() {
	if f.name == "" {
		fmt.Fprintf(f.Output(), "Usage:\n")
	} else {
		fmt.Fprintf(f.Output(), "Usage of %s:\n", f.name)
	}
	f.PrintDefaults()
}

// 注意：Usage 不仅仅是 defaultUsage(CommandLine)
// 因为它通过 godoc flag Usage 作为
// 示例，说明如何编写自己的用法函数。

// Usage 打印一条记录所有已定义的命令行标志的用法消息
// 到 [CommandLine] 的输出，默认情况下是 [os.Stderr]。
// 当解析标志时发生错误时调用。
// 该函数是一个变量，可以更改为指向自定义函数。
// 默认情况下，它打印一个简单的标头并调用 [PrintDefaults]；有关详细信息，请参阅
// 输出格式和控制方法，请参见 [PrintDefaults] 的文档。
// 自定义用法函数可以选择退出程序；默认情况下，
// 由于命令行的错误处理策略设置为
// [ExitOnError]，退出也会发生。
var Usage = func() {
	fmt.Fprintf(CommandLine.Output(), "Usage of %s:\n", os.Args[0])
	PrintDefaults()
}

// NFlag 返回已设置的标志数。
func (f *FlagSet) NFlag() int { return len(f.actual) }

// NFlag 返回已设置的命令行标志数。
func NFlag() int { return len(CommandLine.actual) }

// Arg 返回第 i 个参数。Arg(0) 是处理标志后的第一个剩余参数。
// 如果请求的元素不存在，Arg 返回空字符串。
func (f *FlagSet) Arg(i int) string {
	if i < 0 || i >= len(f.args) {
		return ""
	}
	return f.args[i]
}

// Arg 返回第 i 个命令行参数。Arg(0) 是处理标志后的第一个剩余参数。
// 如果请求的元素不存在，Arg 返回空字符串。
func Arg(i int) string {
	return CommandLine.Arg(i)
}

// NArg 是处理标志后剩余的参数数量。
func (f *FlagSet) NArg() int { return len(f.args) }

// NArg 是处理标志后剩余的参数数量。
func NArg() int { return len(CommandLine.args) }

// Args 返回非标志参数。
func (f *FlagSet) Args() []string { return f.args }

// Args 返回非标志命令行参数。
func Args() []string { return CommandLine.args }

// BoolVar 用指定的名称、默认值和用法字符串定义一个布尔标志。
// 参数 p 指向一个布尔变量，用于存储标志的值。
func (f *FlagSet) BoolVar(p *bool, name string, value bool, usage string) {
	f.Var(newBoolValue(value, p), name, usage)
}

// BoolVar 用指定的名称、默认值和用法字符串定义一个布尔标志。
// 参数 p 指向一个布尔变量，用于存储标志的值。
func BoolVar(p *bool, name string, value bool, usage string) {
	CommandLine.Var(newBoolValue(value, p), name, usage)
}

// Bool 用指定的名称、默认值和用法字符串定义一个布尔标志。
// 返回值是一个布尔变量的地址，该变量存储标志的值。
func (f *FlagSet) Bool(name string, value bool, usage string) *bool {
	p := new(bool)
	f.BoolVar(p, name, value, usage)
	return p
}

// Bool 用指定的名称、默认值和用法字符串定义一个布尔标志。
// 返回值是一个布尔变量的地址，该变量存储标志的值。
func Bool(name string, value bool, usage string) *bool {
	return CommandLine.Bool(name, value, usage)
}

// IntVar 用指定的名称、默认值和用法字符串定义一个 int 标志。
// 参数 p 指向一个 int 变量，用于存储标志的值。
func (f *FlagSet) IntVar(p *int, name string, value int, usage string) {
	f.Var(newIntValue(value, p), name, usage)
}

// IntVar 用指定的名称、默认值和用法字符串定义一个 int 标志。
// 参数 p 指向一个 int 变量，用于存储标志的值。
func IntVar(p *int, name string, value int, usage string) {
	CommandLine.Var(newIntValue(value, p), name, usage)
}

// Int 用指定的名称、默认值和用法字符串定义一个 int 标志。
// 返回值是一个 int 变量的地址，该变量存储标志的值。
func (f *FlagSet) Int(name string, value int, usage string) *int {
	p := new(int)
	f.IntVar(p, name, value, usage)
	return p
}

// Int 用指定的名称、默认值和用法字符串定义一个 int 标志。
// 返回值是一个 int 变量的地址，该变量存储标志的值。
func Int(name string, value int, usage string) *int {
	return CommandLine.Int(name, value, usage)
}

// Int64Var 用指定的名称、默认值和用法字符串定义一个 int64 标志。
// 参数 p 指向一个 int64 变量，用于存储标志的值。
func (f *FlagSet) Int64Var(p *int64, name string, value int64, usage string) {
	f.Var(newInt64Value(value, p), name, usage)
}

// Int64Var 用指定的名称、默认值和用法字符串定义一个 int64 标志。
// 参数 p 指向一个 int64 变量，用于存储标志的值。
func Int64Var(p *int64, name string, value int64, usage string) {
	CommandLine.Var(newInt64Value(value, p), name, usage)
}

// Int64 用指定的名称、默认值和用法字符串定义一个 int64 标志。
// 返回值是一个 int64 变量的地址，该变量存储标志的值。
func (f *FlagSet) Int64(name string, value int64, usage string) *int64 {
	p := new(int64)
	f.Int64Var(p, name, value, usage)
	return p
}

// Int64 用指定的名称、默认值和用法字符串定义一个 int64 标志。
// 返回值是一个 int64 变量的地址，该变量存储标志的值。
func Int64(name string, value int64, usage string) *int64 {
	return CommandLine.Int64(name, value, usage)
}

// UintVar defines a uint flag with specified name, default value, and usage string.
// The argument p points to a uint variable in which to store the value of the flag.
func (f *FlagSet) UintVar(p *uint, name string, value uint, usage string) {
	f.Var(newUintValue(value, p), name, usage)
}

// UintVar defines a uint flag with specified name, default value, and usage string.
// The argument p points to a uint variable in which to store the value of the flag.
func UintVar(p *uint, name string, value uint, usage string) {
	CommandLine.Var(newUintValue(value, p), name, usage)
}

// Uint defines a uint flag with specified name, default value, and usage string.
// The return value is the address of a uint variable that stores the value of the flag.
func (f *FlagSet) Uint(name string, value uint, usage string) *uint {
	p := new(uint)
	f.UintVar(p, name, value, usage)
	return p
}

// Uint defines a uint flag with specified name, default value, and usage string.
// The return value is the address of a uint variable that stores the value of the flag.
func Uint(name string, value uint, usage string) *uint {
	return CommandLine.Uint(name, value, usage)
}

// Uint64Var defines a uint64 flag with specified name, default value, and usage string.
// The argument p points to a uint64 variable in which to store the value of the flag.
func (f *FlagSet) Uint64Var(p *uint64, name string, value uint64, usage string) {
	f.Var(newUint64Value(value, p), name, usage)
}

// Uint64Var defines a uint64 flag with specified name, default value, and usage string.
// The argument p points to a uint64 variable in which to store the value of the flag.
func Uint64Var(p *uint64, name string, value uint64, usage string) {
	CommandLine.Var(newUint64Value(value, p), name, usage)
}

// Uint64 defines a uint64 flag with specified name, default value, and usage string.
// The return value is the address of a uint64 variable that stores the value of the flag.
func (f *FlagSet) Uint64(name string, value uint64, usage string) *uint64 {
	p := new(uint64)
	f.Uint64Var(p, name, value, usage)
	return p
}

// Uint64 defines a uint64 flag with specified name, default value, and usage string.
// The return value is the address of a uint64 variable that stores the value of the flag.
func Uint64(name string, value uint64, usage string) *uint64 {
	return CommandLine.Uint64(name, value, usage)
}

// StringVar defines a string flag with specified name, default value, and usage string.
// The argument p points to a string variable in which to store the value of the flag.
func (f *FlagSet) StringVar(p *string, name string, value string, usage string) {
	f.Var(newStringValue(value, p), name, usage)
}

// StringVar defines a string flag with specified name, default value, and usage string.
// The argument p points to a string variable in which to store the value of the flag.
func StringVar(p *string, name string, value string, usage string) {
	CommandLine.Var(newStringValue(value, p), name, usage)
}

// String defines a string flag with specified name, default value, and usage string.
// The return value is the address of a string variable that stores the value of the flag.
func (f *FlagSet) String(name string, value string, usage string) *string {
	p := new(string)
	f.StringVar(p, name, value, usage)
	return p
}

// String defines a string flag with specified name, default value, and usage string.
// The return value is the address of a string variable that stores the value of the flag.
func String(name string, value string, usage string) *string {
	return CommandLine.String(name, value, usage)
}

// Float64Var defines a float64 flag with specified name, default value, and usage string.
// The argument p points to a float64 variable in which to store the value of the flag.
func (f *FlagSet) Float64Var(p *float64, name string, value float64, usage string) {
	f.Var(newFloat64Value(value, p), name, usage)
}

// Float64Var defines a float64 flag with specified name, default value, and usage string.
// The argument p points to a float64 variable in which to store the value of the flag.
func Float64Var(p *float64, name string, value float64, usage string) {
	CommandLine.Var(newFloat64Value(value, p), name, usage)
}

// Float64 defines a float64 flag with specified name, default value, and usage string.
// The return value is the address of a float64 variable that stores the value of the flag.
func (f *FlagSet) Float64(name string, value float64, usage string) *float64 {
	p := new(float64)
	f.Float64Var(p, name, value, usage)
	return p
}

// Float64 defines a float64 flag with specified name, default value, and usage string.
// The return value is the address of a float64 variable that stores the value of the flag.
func Float64(name string, value float64, usage string) *float64 {
	return CommandLine.Float64(name, value, usage)
}

// DurationVar defines a time.Duration flag with specified name, default value, and usage string.
// The argument p points to a time.Duration variable in which to store the value of the flag.
// The flag accepts a value acceptable to time.ParseDuration.
func (f *FlagSet) DurationVar(p *time.Duration, name string, value time.Duration, usage string) {
	f.Var(newDurationValue(value, p), name, usage)
}

// DurationVar defines a time.Duration flag with specified name, default value, and usage string.
// The argument p points to a time.Duration variable in which to store the value of the flag.
// The flag accepts a value acceptable to time.ParseDuration.
func DurationVar(p *time.Duration, name string, value time.Duration, usage string) {
	CommandLine.Var(newDurationValue(value, p), name, usage)
}

// Duration defines a time.Duration flag with specified name, default value, and usage string.
// The return value is the address of a time.Duration variable that stores the value of the flag.
// The flag accepts a value acceptable to time.ParseDuration.
func (f *FlagSet) Duration(name string, value time.Duration, usage string) *time.Duration {
	p := new(time.Duration)
	f.DurationVar(p, name, value, usage)
	return p
}

// Duration defines a time.Duration flag with specified name, default value, and usage string.
// The return value is the address of a time.Duration variable that stores the value of the flag.
// The flag accepts a value acceptable to time.ParseDuration.
func Duration(name string, value time.Duration, usage string) *time.Duration {
	return CommandLine.Duration(name, value, usage)
}

// TextVar defines a flag with a specified name, default value, and usage string.
// The argument p must be a pointer to a variable that will hold the value
// of the flag, and p must implement encoding.TextUnmarshaler.
// If the flag is used, the flag value will be passed to p's UnmarshalText method.
// The type of the default value must be the same as the type of p.
func (f *FlagSet) TextVar(p encoding.TextUnmarshaler, name string, value encoding.TextMarshaler, usage string) {
	f.Var(newTextValue(value, p), name, usage)
}

// TextVar defines a flag with a specified name, default value, and usage string.
// The argument p must be a pointer to a variable that will hold the value
// of the flag, and p must implement encoding.TextUnmarshaler.
// If the flag is used, the flag value will be passed to p's UnmarshalText method.
// The type of the default value must be the same as the type of p.
func TextVar(p encoding.TextUnmarshaler, name string, value encoding.TextMarshaler, usage string) {
	CommandLine.Var(newTextValue(value, p), name, usage)
}

// Func defines a flag with the specified name and usage string.
// Each time the flag is seen, fn is called with the value of the flag.
// If fn returns a non-nil error, it will be treated as a flag value parsing error.
func (f *FlagSet) Func(name, usage string, fn func(string) error) {
	f.Var(funcValue(fn), name, usage)
}

// Func defines a flag with the specified name and usage string.
// Each time the flag is seen, fn is called with the value of the flag.
// If fn returns a non-nil error, it will be treated as a flag value parsing error.
func Func(name, usage string, fn func(string) error) {
	CommandLine.Func(name, usage, fn)
}

// BoolFunc defines a flag with the specified name and usage string without requiring values.
// Each time the flag is seen, fn is called with the value of the flag.
// If fn returns a non-nil error, it will be treated as a flag value parsing error.
func (f *FlagSet) BoolFunc(name, usage string, fn func(string) error) {
	f.Var(boolFuncValue(fn), name, usage)
}

// BoolFunc defines a flag with the specified name and usage string without requiring values.
// Each time the flag is seen, fn is called with the value of the flag.
// If fn returns a non-nil error, it will be treated as a flag value parsing error.
func BoolFunc(name, usage string, fn func(string) error) {
	CommandLine.BoolFunc(name, usage, fn)
}

// Var defines a flag with the specified name and usage string. The type and
// value of the flag are represented by the first argument, of type [Value], which
// typically holds a user-defined implementation of [Value]. For instance, the
// caller could create a flag that turns a comma-separated string into a slice
// of strings by giving the slice the methods of [Value]; in particular, [Set] would
// decompose the comma-separated string into the slice.
func (f *FlagSet) Var(value Value, name string, usage string) {
	// Flag must not begin "-" or contain "=".
	if strings.HasPrefix(name, "-") {
		panic(f.sprintf("flag %q begins with -", name))
	} else if strings.Contains(name, "=") {
		panic(f.sprintf("flag %q contains =", name))
	}

	// Remember the default value as a string; it won't change.
	flag := &Flag{name, usage, value, value.String()}
	_, alreadythere := f.formal[name]
	if alreadythere {
		var msg string
		if f.name == "" {
			msg = f.sprintf("flag redefined: %s", name)
		} else {
			msg = f.sprintf("%s flag redefined: %s", f.name, name)
		}
		panic(msg) // Happens only if flags are declared with identical names
	}
	if pos := f.undef[name]; pos != "" {
		panic(fmt.Sprintf("flag %s set at %s before being defined", name, pos))
	}
	if f.formal == nil {
		f.formal = make(map[string]*Flag)
	}
	f.formal[name] = flag
}

// Var defines a flag with the specified name and usage string. The type and
// value of the flag are represented by the first argument, of type [Value], which
// typically holds a user-defined implementation of [Value]. For instance, the
// caller could create a flag that turns a comma-separated string into a slice
// of strings by giving the slice the methods of [Value]; in particular, [Set] would
// decompose the comma-separated string into the slice.
func Var(value Value, name string, usage string) {
	CommandLine.Var(value, name, usage)
}

// sprintf formats the message, prints it to output, and returns it.
func (f *FlagSet) sprintf(format string, a ...any) string {
	msg := fmt.Sprintf(format, a...)
	fmt.Fprintln(f.Output(), msg)
	return msg
}

// failf prints to standard error a formatted error and usage message and
// returns the error.
func (f *FlagSet) failf(format string, a ...any) error {
	msg := f.sprintf(format, a...)
	f.usage()
	return errors.New(msg)
}

// usage calls the Usage method for the flag set if one is specified,
// or the appropriate default usage function otherwise.
func (f *FlagSet) usage() {
	if f.Usage == nil {
		f.defaultUsage()
	} else {
		f.Usage()
	}
}

// parseOne parses one flag. It reports whether a flag was seen.
func (f *FlagSet) parseOne() (bool, error) {
	if len(f.args) == 0 {
		return false, nil
	}
	s := f.args[0]
	if len(s) < 2 || s[0] != '-' {
		return false, nil
	}
	numMinuses := 1
	if s[1] == '-' {
		numMinuses++
		if len(s) == 2 { // "--" terminates the flags
			f.args = f.args[1:]
			return false, nil
		}
	}
	name := s[numMinuses:]
	if len(name) == 0 || name[0] == '-' || name[0] == '=' {
		return false, f.failf("bad flag syntax: %s", s)
	}

	// it's a flag. does it have an argument?
	f.args = f.args[1:]
	hasValue := false
	value := ""
	for i := 1; i < len(name); i++ { // equals cannot be first
		if name[i] == '=' {
			value = name[i+1:]
			hasValue = true
			name = name[0:i]
			break
		}
	}

	flag, ok := f.formal[name]
	if !ok {
		if name == "help" || name == "h" { // special case for nice help message.
			f.usage()
			return false, ErrHelp
		}
		return false, f.failf("flag provided but not defined: -%s", name)
	}

	if fv, ok := flag.Value.(boolFlag); ok && fv.IsBoolFlag() { // special case: doesn't need an arg
		if hasValue {
			if err := fv.Set(value); err != nil {
				return false, f.failf("invalid boolean value %q for -%s: %v", value, name, err)
			}
		} else {
			if err := fv.Set("true"); err != nil {
				return false, f.failf("invalid boolean flag %s: %v", name, err)
			}
		}
	} else {
		// It must have a value, which might be the next argument.
		if !hasValue && len(f.args) > 0 {
			// value is the next arg
			hasValue = true
			value, f.args = f.args[0], f.args[1:]
		}
		if !hasValue {
			return false, f.failf("flag needs an argument: -%s", name)
		}
		if err := flag.Value.Set(value); err != nil {
			return false, f.failf("invalid value %q for flag -%s: %v", value, name, err)
		}
	}
	if f.actual == nil {
		f.actual = make(map[string]*Flag)
	}
	f.actual[name] = flag
	return true, nil
}

// Parse parses flag definitions from the argument list, which should not
// include the command name. Must be called after all flags in the [FlagSet]
// are defined and before flags are accessed by the program.
// The return value will be [ErrHelp] if -help or -h were set but not defined.
func (f *FlagSet) Parse(arguments []string) error {
	f.parsed = true
	f.args = arguments
	for {
		seen, err := f.parseOne()
		if seen {
			continue
		}
		if err == nil {
			break
		}
		switch f.errorHandling {
		case ContinueOnError:
			return err
		case ExitOnError:
			if err == ErrHelp {
				os.Exit(0)
			}
			os.Exit(2)
		case PanicOnError:
			panic(err)
		}
	}
	return nil
}

// Parsed reports whether f.Parse has been called.
func (f *FlagSet) Parsed() bool {
	return f.parsed
}

// Parse parses the command-line flags from [os.Args][1:]. Must be called
// after all flags are defined and before flags are accessed by the program.
func Parse() {
	// Ignore errors; CommandLine is set for ExitOnError.
	CommandLine.Parse(os.Args[1:])
}

// Parsed reports whether the command-line flags have been parsed.
func Parsed() bool {
	return CommandLine.Parsed()
}

// CommandLine is the default set of command-line flags, parsed from [os.Args].
// The top-level functions such as [BoolVar], [Arg], and so on are wrappers for the
// methods of CommandLine.
var CommandLine *FlagSet

func init() {
	// It's possible for execl to hand us an empty os.Args.
	if len(os.Args) == 0 {
		CommandLine = NewFlagSet("", ExitOnError)
	} else {
		CommandLine = NewFlagSet(os.Args[0], ExitOnError)
	}

	// Override generic FlagSet default Usage with call to global Usage.
	// Note: This is not CommandLine.Usage = Usage,
	// because we want any eventual call to use any updated value of Usage,
	// not the value it has when this line is run.
	CommandLine.Usage = commandLineUsage
}

func commandLineUsage() {
	Usage()
}

// NewFlagSet returns a new, empty flag set with the specified name and
// error handling property. If the name is not empty, it will be printed
// in the default usage message and in error messages.
func NewFlagSet(name string, errorHandling ErrorHandling) *FlagSet {
	f := &FlagSet{
		name:          name,
		errorHandling: errorHandling,
	}
	f.Usage = f.defaultUsage
	return f
}

// Init sets the name and error handling property for a flag set.
// By default, the zero [FlagSet] uses an empty name and the
// [ContinueOnError] error handling policy.
func (f *FlagSet) Init(name string, errorHandling ErrorHandling) {
	f.name = name
	f.errorHandling = errorHandling
}
