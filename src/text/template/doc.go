// 版权所有 2011 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

/*
Package template 实现了数据驱动的模板，用于生成文本输出。

要生成 HTML 输出，请参见 [html/template]，它与本包具有相同的接口，
但会自动保护 HTML 输出免受某些攻击。

模板通过将其应用于数据结构来执行。模板中的注解引用数据结构的元素
（通常是结构体的字段或映射中的键）来控制执行并派生要显示的值。
模板的执行会遍历结构并设置光标（用句点 '.' 表示，称为 "dot"）
为执行过程中结构中当前位置的值。

本包使用的安全模型假定模板作者是可信的。本包不会自动转义输出，
因此如果模板由不可信来源执行，向模板注入代码可能导致任意代码执行。

模板的输入文本是任意格式的 UTF-8 编码文本。
"动作"——数据求值或控制结构——由 "{{" 和 "}}" 分隔；
动作之外的所有文本都原样复制到输出。

一旦解析完成，模板可以安全地并行执行，但如果并行执行共享一个 Writer，
输出可能会交错。

这里有一个简单的示例，打印 "17 items are made of wool"。

	type Inventory struct {
		Material string
		Count    uint
	}
	sweaters := Inventory{"wool", 17}
	tmpl, err := template.New("test").Parse("{{.Count}} items are made of {{.Material}}")
	if err != nil { panic(err) }
	err = tmpl.Execute(os.Stdout, sweaters)
	if err != nil { panic(err) }

下面有更复杂的示例。

文本和空格

默认情况下，当模板执行时，动作之间的所有文本都按原样复制。
例如，上面示例中的字符串 " items are made of " 会在程序运行时
出现在标准输出上。

然而，为了帮助格式化模板源代码，如果动作的左分隔符（默认为 "{{"）
后面紧跟减号和空白字符，则紧接在前面的文本的所有尾部空白都会被修剪。
类似地，如果右分隔符（"}}"）前面有空白字符和减号，
则紧随其后的文本的所有前导空白都会被修剪。
在这些修剪标记中，空白字符必须存在：
"{{- 3}}" 类似于 "{{3}}" 但会修剪紧接在前面的文本，
而 "{{-3}}" 被解析为包含数字 -3 的动作。

例如，当执行源代码为

	"{{23 -}} < {{- 45}}"

的模板时，生成的输出将是

	"23<45"

对于这种修剪，空白字符的定义与 Go 中相同：
空格、水平制表符、回车符和换行符。

动作

以下是动作列表。"参数"和"管道"是数据求值，
在后面相应的章节中有详细定义。

*/
//	{{/* 注释 */}}
//	{{- /* 从前后文本中修剪空白的注释 */ -}}
//		注释；被丢弃。可以包含换行符。
//		注释不能嵌套，必须如此处所示在分隔符处开始和结束。
/*

	{{pipeline}}
		管道值的默认文本表示（与 fmt.Print 打印的相同）
		被复制到输出。

	{{if pipeline}} T1 {{end}}
		如果管道的值为空，则不生成输出；
		否则，执行 T1。空值包括 false、0、任何
		nil 指针或接口值，以及任何长度为零的数组、切片、映射或字符串。
		Dot 不受影响。

	{{if pipeline}} T1 {{else}} T0 {{end}}
		如果管道的值为空，则执行 T0；
		否则，执行 T1。Dot 不受影响。

	{{if pipeline}} T1 {{else if pipeline}} T0 {{end}}
		为了简化 if-else 链的外观，if 的 else 动作
		可以直接包含另一个 if；效果与以下写法完全相同
			{{if pipeline}} T1 {{else}}{{if pipeline}} T0 {{end}}{{end}}

	{{range pipeline}} T1 {{end}}
		管道的值必须是数组、切片、映射、iter.Seq、
		iter.Seq2、整数或通道。
		如果管道的值长度为零，则不输出任何内容；
		否则，dot 被设置为数组、切片或映射的连续元素，
		并执行 T1。如果值是映射且键是具有已定义顺序的基本类型，
		则元素将按排序后的键顺序访问。

	{{range pipeline}} T1 {{else}} T0 {{end}}
		管道的值必须是数组、切片、映射、iter.Seq、
		iter.Seq2、整数或通道。
		如果管道的值长度为零，dot 不受影响并执行 T0；
		否则，dot 被设置为数组、切片或映射的连续元素，并执行 T1。

	{{break}}
		最内层的 {{range pipeline}} 循环提前结束，
		停止当前迭代并跳过所有剩余迭代。

	{{continue}}
		最内层 {{range pipeline}} 循环的当前迭代停止，
		循环开始下一次迭代。

	{{template "name"}}
		使用 nil 数据执行具有指定名称的模板。

	{{template "name" pipeline}}
		使用 dot 设置为管道值来执行具有指定名称的模板。

	{{block "name" pipeline}} T1 {{end}}
		block 是定义模板的简写
			{{define "name"}} T1 {{end}}
		然后在原地执行它
			{{template "name" pipeline}}
		典型用法是定义一组根模板，
		然后通过重新定义其中的块模板来自定义。

	{{with pipeline}} T1 {{end}}
		如果管道的值为空，则不生成输出；
		否则，dot 被设置为管道的值并执行 T1。

	{{with pipeline}} T1 {{else}} T0 {{end}}
		如果管道的值为空，dot 不受影响并执行 T0；
		否则，dot 被设置为管道的值并执行 T1。

	{{with pipeline}} T1 {{else with pipeline}} T0 {{end}}
		为了简化 with-else 链的外观，with 的 else 动作
		可以直接包含另一个 with；效果与以下写法完全相同
			{{with pipeline}} T1 {{else}}{{with pipeline}} T0 {{end}}{{end}}


参数

参数是一个简单值，由以下之一表示。

	- Go 语法中的布尔、字符串、字符、整数、浮点数、虚数
	  或复数常量。这些行为类似于 Go 的无类型常量。
	  注意，与 Go 一样，大整数常量在赋值或传递给函数时
	  是否溢出可能取决于宿主机的 int 是 32 位还是 64 位。
	- 关键字 nil，表示无类型的 Go nil。
	- 字符 '.'（句点）：

		.

	  结果是 dot 的值。
	- 变量名，是一个（可能为空的）字母数字字符串，
	  前面带有美元符号，例如

		$piOver2

	  或

		$

	  结果是变量的值。
	  变量在下面描述。
	- 数据的字段名（数据必须是结构体），
	  前面带有句点，例如

		.Field

	  结果是字段的值。字段调用可以链接：

	    .Field1.Field2

	  字段也可以在变量上求值，包括链接：

	    $x.Field1.Field2
	- 数据的键名（数据必须是映射），
	  前面带有句点，例如

		.Key

	  结果是由键索引的映射元素值。
	  键调用可以链接并与字段组合到任意深度：

	    .Field1.Key1.Field2.Key2

	  虽然键必须是字母数字标识符，但与字段名不同，
	  它们不需要以大写字母开头。
	  键也可以在变量上求值，包括链接：

	    $x.key1.key2
	- 数据的无参方法名，前面带有句点，
	  例如

		.Method

	  结果是以 dot 作为接收者调用方法的值，即 dot.Method()。
	  这样的方法必须有一个返回值（任意类型）或两个返回值，
	  其中第二个是 error。如果有两个返回值且返回的 error 非 nil，
	  执行终止并向调用者返回 error 作为 Execute 的值。
	  方法调用可以链接并与字段和键组合到任意深度：

	    .Field1.Key1.Method1.Field2.Key2.Method2

	  方法也可以在变量上求值，包括链接：

	    $x.Method1.Field
	- 无参函数的名称，例如

		fun

	  结果是调用函数的值，即 fun()。返回类型和值的行为
	  与方法相同。函数和函数名在下面描述。
	- 上述之一的括号实例，用于分组。结果可以通过
	  字段或映射键调用来访问。

		print (.F1 arg1) (.F2 arg2)
		(.StructValuedMethod "arg").Field

参数可以求值为任何类型；如果它们是指针，实现会在需要时
自动间接引用到基础类型。
如果求值产生函数值，例如结构体的函数值字段，
函数不会自动调用，但它可以用作 if 动作等的真值。
要调用它，请使用下面定义的 call 函数。

管道

管道是可能链接的"命令"序列。命令是一个简单值（参数）
或函数或方法调用，可能带有多个参数：

	Argument
		结果是求值参数的值。
	.Method [Argument...]
		方法可以单独出现或作为链的最后一个元素，但与链中间的方法不同，
		它可以接受参数。
		结果是使用参数调用方法的值：
			dot.Method(Argument1, etc.)
	functionName [Argument...]
		结果是调用与名称关联的函数的值：
			function(Argument1, etc.)
		函数和函数名在下面描述。

管道可以通过用管道字符 '|' 分隔命令序列来"链接"。
在链式管道中，每个命令的结果作为下一个命令的最后一个参数传递。
管道中最后一个命令的输出是管道的值。

命令的输出将是一个值或两个值，其中第二个类型为 error。
如果第二个值存在且求值为非 nil，执行终止并向 Execute 的调用者返回错误。

变量

动作内的管道可以初始化一个变量来捕获结果。
初始化的语法是

	$variable := pipeline

其中 $variable 是变量的名称。声明变量的动作不产生输出。

先前声明的变量也可以赋值，使用语法

	$variable = pipeline

如果 "range" 动作初始化一个变量，该变量被设置为迭代的连续元素。
此外，"range" 可以声明两个变量，用逗号分隔：

	range $index, $element := pipeline

在这种情况下，$index 和 $element 分别被设置为数组/切片索引
或映射键和元素的连续值。注意，如果只有一个变量，
它被赋值为元素；这与 Go range 子句中的约定相反。

变量的作用域延伸到声明它的控制结构（"if"、"with" 或 "range"）
的 "end" 动作，或者如果没有这样的控制结构，则延伸到模板的末尾。
模板调用不从其调用点继承变量。

当执行开始时，$ 被设置为传递给 Execute 的数据参数，
即 dot 的初始值。

示例

以下是一些演示管道和变量的单行模板示例。
所有示例都产生带引号的单词 "output"：

	{{"\"output\""}}
		字符串常量。
	{{`"output"`}}
		原始字符串常量。
	{{printf "%q" "output"}}
		函数调用。
	{{"output" | printf "%q"}}
		最后一个参数来自前一个命令的函数调用。
	{{printf "%q" (print "out" "put")}}
		括号参数。
	{{"put" | printf "%s%s" "out" | printf "%q"}}
		更复杂的调用。
	{{"output" | printf "%s" | printf "%q"}}
		更长的链。
	{{with "output"}}{{printf "%q" .}}{{end}}
		使用 dot 的 with 动作。
	{{with $x := "output" | printf "%q"}}{{$x}}{{end}}
		创建并使用变量的 with 动作。
	{{with $x := "output"}}{{printf "%q" $x}}{{end}}
		在另一个动作中使用变量的 with 动作。
	{{with $x := "output"}}{{$x | printf "%q"}}{{end}}
		相同，但使用管道。

函数

在执行期间，函数在两个函数映射中查找：首先在模板中，
然后在全局函数映射中。默认情况下，模板中没有定义函数，
但可以使用 Funcs 方法添加它们。

预定义的全局函数命名如下。

	and
		通过返回第一个空参数或最后一个参数来返回其参数的布尔 AND。
		也就是说，"and x y" 的行为类似于 "if x then y else x"。
		求值从左到右遍历参数，并在确定结果时返回。
	call
		返回调用第一个参数（必须是函数）的结果，
		其余参数作为参数。因此 "call .X.Y 1 2" 在 Go 表示法中是
		dot.X.Y(1, 2)，其中 Y 是函数值字段、映射条目等。
		第一个参数必须是求值结果为函数类型的值
		（与 print 等预定义函数不同）。函数必须返回
		一个或两个结果值，第二个类型为 error。
		如果参数与函数不匹配或返回的错误值非 nil，执行停止。
	html
		返回其参数文本表示的转义 HTML 等价物。
		此函数在 html/template 中不可用，但有一些例外。
	index
		返回用后续参数索引第一个参数的结果。
		因此 "index x 1 2 3" 在 Go 语法中是 x[1][2][3]。
		每个被索引的项必须是映射、切片或数组。
	slice
		slice 返回用其余参数切片第一个参数的结果。
		因此 "slice x 1 2" 在 Go 语法中是 x[1:2]，
		而 "slice x" 是 x[:]，"slice x 1" 是 x[1:]，
		"slice x 1 2 3" 是 x[1:2:3]。
		第一个参数必须是字符串、切片或数组。
	js
		返回其参数文本表示的转义 JavaScript 等价物。
	len
		返回其参数的整数长度。
	not
		返回其单个参数的布尔否定。
	or
		通过返回第一个非空参数或最后一个参数来返回其参数的布尔 OR，
		也就是说，"or x y" 的行为类似于 "if x then x else y"。
		求值从左到右遍历参数，并在确定结果时返回。
	print
		fmt.Sprint 的别名
	printf
		fmt.Sprintf 的别名
	println
		fmt.Sprintln 的别名
	urlquery
		返回其参数文本表示的转义值，
		格式适合嵌入 URL 查询。
		此函数在 html/template 中不可用，但有一些例外。

布尔函数将任何零值视为 false，将非零值视为 true。

还有一组定义为函数的二元比较运算符：

	eq
		返回 arg1 == arg2 的布尔真值
	ne
		返回 arg1 != arg2 的布尔真值
	lt
		返回 arg1 < arg2 的布尔真值
	le
		返回 arg1 <= arg2 的布尔真值
	gt
		返回 arg1 > arg2 的布尔真值
	ge
		返回 arg1 >= arg2 的布尔真值

为了更简单的多路相等测试，eq（仅限 eq）接受两个或更多参数，
并将第二个及后续参数与第一个比较，实际上返回

	arg1==arg2 || arg1==arg3 || arg1==arg4 ...

（然而，与 Go 中的 || 不同，eq 是函数调用，
所有参数都会被求值。）

比较函数适用于 Go 定义为可比较的任何类型的值。
对于整数等基本类型，规则比较宽松：
大小和确切类型被忽略，因此任何整数值（有符号或无符号）
都可以与任何其他整数值比较。（比较的是算术值，
而不是位模式，因此所有负整数都小于所有无符号整数。）
然而，像往常一样，不能将 int 与 float32 等进行比较。

关联模板

每个模板在创建时由一个字符串命名。此外，每个模板
与零个或多个其他模板关联，可以按名称调用它们；
这种关联是传递的，形成模板的命名空间。

模板可以使用模板调用来实例化另一个关联模板；
请参阅上面 "template" 动作的解释。名称必须是
与包含调用的模板关联的模板的名称。

嵌套模板定义

在解析模板时，可以定义另一个模板并将其与正在解析的模板关联。
模板定义必须出现在模板的顶层，就像 Go 程序中的全局变量一样。

这种定义的语法是用 "define" 和 "end" 动作包围每个模板声明。

define 动作通过提供字符串常量来命名正在创建的模板。
这里是一个简单的示例：

	{{define "T1"}}ONE{{end}}
	{{define "T2"}}TWO{{end}}
	{{define "T3"}}{{template "T1"}} {{template "T2"}}{{end}}
	{{template "T3"}}

这定义了两个模板 T1 和 T2，以及第三个模板 T3，
它在执行时调用另外两个。最后它调用 T3。
如果执行，此模板将产生文本

	ONE TWO

根据构造，模板只能驻留在一个关联中。如果需要从多个关联中
访问一个模板，模板定义必须被解析多次以创建不同的 *Template 值，
或者必须使用 [Template.Clone] 或 [Template.AddParseTree] 复制。

Parse 可以被多次调用来组装各种关联模板；
请参阅 [ParseFiles]、[ParseGlob]、[Template.ParseFiles] 和 [Template.ParseGlob]
了解解析存储在文件中的相关模板的简单方法。

模板可以直接执行或通过 [Template.ExecuteTemplate] 执行，
后者执行按名称标识的关联模板。要调用上面的示例，我们可以写

	err := tmpl.Execute(os.Stdout, "no data needed")
	if err != nil {
		log.Fatalf("execution failed: %s", err)
	}

或者按名称显式调用特定模板，

	err := tmpl.ExecuteTemplate(os.Stdout, "T2", "no data needed")
	if err != nil {
		log.Fatalf("execution failed: %s", err)
	}

*/
package template
