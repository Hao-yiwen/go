// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

/*
Package fmt 实现了格式化输入输出，函数类似于 C 的 printf 和 scanf。格式"动词"源自 C 的语法但更简洁。

# 打印

有四类打印函数，根据输出目标定义。
[Print]、[Println] 和 [Printf] 写入到 [os.Stdout]；
[Sprint]、[Sprintln] 和 [Sprintf] 返回一个字符串；
[Fprint]、[Fprintln] 和 [Fprintf] 写入到 [io.Writer]；以及
[Append]、[Appendln] 和 [Appendf] 将输出追加到字节切片。

每个系列中的函数根据名称末尾进行格式化。
Print、Sprint、Fprint 和 Append 为每个参数使用默认格式，
当都不是字符串时在操作数之间添加空格。
Println、Sprintln、Fprintln 和 Appendln 总是添加空格并追加换行符。
Printf、Sprintf、Fprintf 和 Appendf 使用一系列"动词"来控制格式化。

动词：

通用：

	%v	默认格式的值
		打印结构体时，加号标志 (%+v) 添加字段名
	%#v	值的 Go 语法表示
		（浮点无穷大和 NaN 打印为 ±Inf 和 NaN）
	%T	值的类型的 Go 语法表示
	%%	字面上的百分号；不消耗任何值

布尔值：

	%t	单词 true 或 false

整数：

	%b	二进制
	%c	由对应 Unicode 码点表示的字符
	%d	十进制
	%o	八进制
	%O	八进制，带 0o 前缀
	%q	单引号字符文字，用 Go 语法安全转义
	%x	十六进制，a-f 使用小写字母
	%X	十六进制，A-F 使用大写字母
	%U	Unicode 格式：U+1234；与 "U+%04X" 相同

浮点数和复数成分：

	%b	无小数的科学计数法，指数是 2 的幂，
		采用 strconv.FormatFloat 的 'b' 格式方式，
		例如 -123456p-78
	%e	科学计数法，例如 -1.234456e+78
	%E	科学计数法，例如 -1.234456E+78
	%f	小数点但无指数，例如 123.456
	%F	%f 的同义词
	%g	大指数时用 %e，否则用 %f。精度详见下文。
	%G	大指数时用 %E，否则用 %F
	%x	十六进制表示法（带十进制 2 的幂指数），例如 -0x1.23abcp+20
	%X	大写十六进制表示法，例如 -0X1.23ABCP+20

	指数总是十进制整数。
	对于 %b 之外的格式，指数至少是两位数字。

字符串和字节切片（用这些动词等效处理）：

	%s	字符串或切片的未解释字节
	%q	双引号字符串，用 Go 语法安全转义
	%x	十六进制，小写，每字节两个字符
	%X	十六进制，大写，每字节两个字符

切片：

	%p	第 0 个元素的地址，十六进制表示法，带前导 0x

指针：

	%p	十六进制表示法，带前导 0x
	%b、%d、%o、%x 和 %X 动词也适用于指针，
	格式化值就像它是整数一样。

%v 的默认格式是：

	bool:                    %t
	int、int8 等：          %d
	uint、uint8 等：        %d，%#v 打印时用 %#x
	float32、complex64 等：%g
	string:                  %s
	chan:                    %p
	pointer:                 %p

对于复合对象，元素使用这些规则递归打印，
布局如下：

	struct:             {field0 field1 ...}
	array、slice:       [elem0 elem1 ...]
	maps:               map[key1:value1 key2:value2 ...]
	指向上述的指针：   &{}、&[]、&map[]

宽度由紧接在动词前的可选十进制数指定。
如果不存在，宽度就是表示该值所需的任何值。
精度在（可选的）宽度之后由句号后跟十进制数指定。
如果没有句号，则使用默认精度。
没有跟随数字的句号指定精度为零。
示例：

	%f     默认宽度，默认精度
	%9f    宽度 9，默认精度
	%.2f   默认宽度，精度 2
	%9.2f  宽度 9，精度 2
	%9.f   宽度 9，精度 0

宽度和精度以 Unicode 码点的单位测量，
即符文。（这不同于 C 的 printf，其中
单位总是以字节为单位。）标志中的任何一个或两个
可以替换为字符 '*'，导致它们的值从
下一个操作数（前面的那个要格式化的）获取，
该操作数必须是 int 类型。

对于大多数值，宽度是输出的最小符文数，
如果必要的话用空格填充格式化形式。

但对于字符串、字节切片和字节数组，精度
限制了要格式化的输入的长度（不是输出的大小），
如果必要的话截断。通常以符文为单位测量，
但对于这些类型以 %x 或 %X 格式格式化时，
以字节为单位。

对于浮点值，宽度设置字段的最小宽度，
精度设置小数点后的位数（如果合适），
除了对于 %g/%G 精度设置有效数字的最大数量（尾随零被移除）。
例如，给定 12.345，格式 %6.3f 打印 12.345，
而 %.3g 打印 12.3。%e、%f 和 %#g 的默认精度是 6；
对于 %g，它是唯一标识该值所需的最小位数。

对于复数，宽度和精度独立地应用于两个
分量，结果被括起，所以 %f 应用到
1.2+3.4i 会产生 (1.200000+3.400000i)。

使用 %q 格式化单个整数码点或符文字符串（类型 []rune）时，
无效的 Unicode 码点被更改为 Unicode 替换
字符 U+FFFD，如 [strconv.QuoteRune] 中一样。

其他标志：

	'+'	总是为数值打印符号；
		保证 %q 的 ASCII 专用输出 (%+q)
	'-'	在右边而不是左边用空格填充（左对齐字段）
	'#'	备用格式：为二进制添加前导 0b (%#b)、为八进制添加 0 (%#o)、
		为十六进制添加 0x 或 0X (%#x 或 %#X)；为 %p 抑制 0x (%#p)；
		对于 %q，如果 [strconv.CanBackquote]
		返回 true，打印原始（反引号）字符串；
		总是为 %e、%E、%f、%F、%g 和 %G 打印小数点；
		不要移除 %g 和 %G 的尾随零；
		如果字符可打印，对于 %U 写 e.g. U+0078 'x'（%#U）
	' '	（空格）为数字中省略的符号留一个空格（% d）；
		在十六进制打印字符串或切片的字节之间放入空格（% x、% X）
	'0'	用前导零而不是空格填充；
		对于数字，这会在符号后移动填充

不期望标志的动词会忽略标志。
例如，没有备用十进制格式，所以 %#d 和 %d
的行为完全相同。

对于每个类似 Printf 的函数，还有一个 Print 函数
不取格式，相当于对每个
操作数说 %v。另一个变体 Println 在
操作数之间插入空格并追加换行符。

无论动词如何，如果操作数是接口值，
使用内部具体值，而不是接口本身。
因此：

	var i interface{} = 23
	fmt.Printf("%v\n", i)

会打印 23。

除了使用动词 %T 和 %p 打印时外，特殊
格式化考虑适用于实现
某些接口的操作数。按应用顺序：

1. If the operand is a [reflect.Value], the operand is replaced by the
concrete value that it holds, and printing continues with the next rule.

2. If an operand implements the [Formatter] interface, it will
be invoked. In this case the interpretation of verbs and flags is
controlled by that implementation.

3. If the %v verb is used with the # flag (%#v) and the operand
implements the [GoStringer] interface, that will be invoked.

If the format (which is implicitly %v for [Println] etc.) is valid
for a string (%s %q %x %X), or is %v but not %#v,
the following two rules apply:

4. If an operand implements the error interface, the Error method
will be invoked to convert the object to a string, which will then
be formatted as required by the verb (if any).

5. If an operand implements method String() string, that method
will be invoked to convert the object to a string, which will then
be formatted as required by the verb (if any).

For compound operands such as slices and structs, the format
applies to the elements of each operand, recursively, not to the
operand as a whole. Thus %q will quote each element of a slice
of strings, and %6.2f will control formatting for each element
of a floating-point array.

However, when printing a byte slice with a string-like verb
(%s %q %x %X), it is treated identically to a string, as a single item.

To avoid recursion in cases such as

	type X string
	func (x X) String() string { return Sprintf("<%s>", x) }

convert the value before recurring:

	func (x X) String() string { return Sprintf("<%s>", string(x)) }

Infinite recursion can also be triggered by self-referential data
structures, such as a slice that contains itself as an element, if
that type has a String method. Such pathologies are rare, however,
and the package does not protect against them.

When printing a struct, fmt cannot and therefore does not invoke
formatting methods such as Error or String on unexported fields.

# Explicit argument indexes

In [Printf], [Sprintf], [Fprintf], and [Appendf], the default behavior is for each
formatting verb to format successive arguments passed in the call.
However, the notation [n] immediately before the verb indicates that the
nth one-indexed argument is to be formatted instead. The same notation
before a '*' for a width or precision selects the argument index holding
the value. After processing a bracketed expression [n], subsequent verbs
will use arguments n+1, n+2, etc. unless otherwise directed.

For example,

	fmt.Sprintf("%[2]d %[1]d\n", 11, 22)

will yield "22 11", while

	fmt.Sprintf("%[3]*.[2]*[1]f", 12.0, 2, 6)

equivalent to

	fmt.Sprintf("%6.2f", 12.0)

will yield " 12.00". Because an explicit index affects subsequent verbs,
this notation can be used to print the same values multiple times
by resetting the index for the first argument to be repeated:

	fmt.Sprintf("%d %d %#[1]x %#x", 16, 17)

will yield "16 17 0x10 0x11".

# Format errors

If an invalid argument is given for a verb, such as providing
a string to %d, the generated string will contain a
description of the problem, as in these examples:

	Wrong type or unknown verb: %!verb(type=value)
		Printf("%d", "hi"):        %!d(string=hi)
	Too many arguments: %!(EXTRA type=value)
		Printf("hi", "guys"):      hi%!(EXTRA string=guys)
	Too few arguments: %!verb(MISSING)
		Printf("hi%d"):            hi%!d(MISSING)
	Non-int for width or precision: %!(BADWIDTH) or %!(BADPREC)
		Printf("%*s", 4.5, "hi"):  %!(BADWIDTH)hi
		Printf("%.*s", 4.5, "hi"): %!(BADPREC)hi
	Invalid or invalid use of argument index: %!(BADINDEX)
		Printf("%*[2]d", 7):       %!d(BADINDEX)
		Printf("%.[2]d", 7):       %!d(BADINDEX)

All errors begin with the string "%!" followed sometimes
by a single character (the verb) and end with a parenthesized
description.

If an Error or String method triggers a panic when called by a
print routine, the fmt package reformats the error message
from the panic, decorating it with an indication that it came
through the fmt package.  For example, if a String method
calls panic("bad"), the resulting formatted message will look
like

	%!s(PANIC=bad)

The %!s just shows the print verb in use when the failure
occurred. If the panic is caused by a nil receiver to an Error,
String, or GoString method, however, the output is the undecorated
string, "<nil>".

# Scanning

An analogous set of functions scans formatted text to yield
values.  [Scan], [Scanf] and [Scanln] read from [os.Stdin]; [Fscan],
[Fscanf] and [Fscanln] read from a specified [io.Reader]; [Sscan],
[Sscanf] and [Sscanln] read from an argument string.

[Scan], [Fscan], [Sscan] treat newlines in the input as spaces.

[Scanln], [Fscanln] and [Sscanln] stop scanning at a newline and
require that the items be followed by a newline or EOF.

[Scanf], [Fscanf], and [Sscanf] parse the arguments according to a
format string, analogous to that of [Printf]. In the text that
follows, 'space' means any Unicode whitespace character
except newline.

In the format string, a verb introduced by the % character
consumes and parses input; these verbs are described in more
detail below. A character other than %, space, or newline in
the format consumes exactly that input character, which must
be present. A newline with zero or more spaces before it in
the format string consumes zero or more spaces in the input
followed by a single newline or the end of the input. A space
following a newline in the format string consumes zero or more
spaces in the input. Otherwise, any run of one or more spaces
in the format string consumes as many spaces as possible in
the input. Unless the run of spaces in the format string
appears adjacent to a newline, the run must consume at least
one space from the input or find the end of the input.

The handling of spaces and newlines differs from that of C's
scanf family: in C, newlines are treated as any other space,
and it is never an error when a run of spaces in the format
string finds no spaces to consume in the input.

The verbs behave analogously to those of [Printf].
For example, %x will scan an integer as a hexadecimal number,
and %v will scan the default representation format for the value.
The [Printf] verbs %p and %T and the flags # and + are not implemented.
For floating-point and complex values, all valid formatting verbs
(%b %e %E %f %F %g %G %x %X and %v) are equivalent and accept
both decimal and hexadecimal notation (for example: "2.3e+7", "0x4.5p-8")
and digit-separating underscores (for example: "3.14159_26535_89793").

Input processed by verbs is implicitly space-delimited: the
implementation of every verb except %c starts by discarding
leading spaces from the remaining input, and the %s verb
(and %v reading into a string) stops consuming input at the first
space or newline character.

The familiar base-setting prefixes 0b (binary), 0o and 0 (octal),
and 0x (hexadecimal) are accepted when scanning integers
without a format or with the %v verb, as are digit-separating
underscores.

Width is interpreted in the input text but there is no
syntax for scanning with a precision (no %5.2f, just %5f).
If width is provided, it applies after leading spaces are
trimmed and specifies the maximum number of runes to read
to satisfy the verb. For example,

	Sscanf(" 1234567 ", "%5s%d", &s, &i)

will set s to "12345" and i to 67 while

	Sscanf(" 12 34 567 ", "%5s%d", &s, &i)

will set s to "12" and i to 34.

In all the scanning functions, a carriage return followed
immediately by a newline is treated as a plain newline
(\r\n means the same as \n).

In all the scanning functions, if an operand implements method
[Scan] (that is, it implements the [Scanner] interface) that
method will be used to scan the text for that operand.  Also,
if the number of arguments scanned is less than the number of
arguments provided, an error is returned.

All arguments to be scanned must be either pointers to basic
types or implementations of the [Scanner] interface.

Like [Scanf] and [Fscanf], [Sscanf] need not consume its entire input.
There is no way to recover how much of the input string [Sscanf] used.

Note: [Fscan] etc. can read one character (rune) past the input
they return, which means that a loop calling a scan routine
may skip some of the input.  This is usually a problem only
when there is no space between input values.  If the reader
provided to [Fscan] implements ReadRune, that method will be used
to read characters.  If the reader also implements UnreadRune,
that method will be used to save the character and successive
calls will not lose data.  To attach ReadRune and UnreadRune
methods to a reader without that capability, use
[bufio.NewReader].
*/
package fmt
