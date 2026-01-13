// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

/*
Compile 通常通过 ``go tool compile'' 调用，编译单个 Go 包
该包由命令行上指定的文件组成。然后它写入一个单一的
对象文件，其名称根据第一个源文件的基名加 .o 后缀。
对象文件随后可以与其他对象合并到包存档中
或直接传递给链接器（``go tool link''）。如果使用 -pack 调用，编译器
直接写入存档，绕过中间对象文件。

生成的文件包含有关包导出的符号的类型信息
以及包从其他包导入的符号使用的类型。因此，
在编译包 P 的客户端 C 时，不需要
读取 P 的依赖项的文件，只需读取 P 的编译输出。

# 命令行

用法：

	go tool compile [flags] file...

指定的文件必须是 Go 源文件，且都属于同一包。
对所有目标操作系统和架构使用相同的编译器。
GOOS 和 GOARCH 环境变量设置所需的目标。

标志：

	-D path
		为本地导入设置相对路径。
	-I dir1 -I dir2
		在 dir1, dir2 等中搜索导入的包，
		在咨询 $GOROOT/pkg/$GOOS_$GOARCH 之后。
	-L
		在错误消息中显示完整文件路径。
	-N
		禁用优化。
	-S
		将汇编列表打印到标准输出（仅代码）。
	-S -S
		将汇编列表打印到标准输出（代码和数据）。
	-V
		打印编译器版本并退出。
	-asmhdr file
		将汇编头写入文件。
	-asan
		插入对 C/C++ 地址清理程序的调用。
	-buildid id
		在导出元数据中记录 id 作为构建 id。
	-blockprofile file
		将编译的块配置文件写入文件。
	-c int
		编译期间的并发数。设置为 1 表示无并发（默认值为 1）。
	-complete
		假设包没有非 Go 组件。
	-cpuprofile file
		将编译的 CPU 配置文件写入文件。
	-dynlink
		允许对共享库中 Go 符号的引用（实验性）。
	-e
		删除报告错误数量的限制（默认限制为 10）。
	-embedcfg file
		从文件读取 go:embed 配置。
		如果使用任何 //go:embed 指令，这是必需的。
		该文件是将模式映射到文件名列表
		和文件名映射到完整路径名的 JSON 文件。
	-goversion string
		指定运行时的必需 go 工具版本。
		当运行时 go 版本与 goversion 不匹配时退出。
	-h
		在检测到第一个错误时使用堆栈跟踪停止。
	-importcfg file
		从文件读取导入配置。
		在文件中，设置 importmap、packagefile 以指定导入解析。
	-installsuffix suffix
		在 $GOROOT/pkg/$GOOS_$GOARCH_suffix 中查找包
		而不是 $GOROOT/pkg/$GOOS_$GOARCH。
	-l
		禁用内联。
	-lang version
		设置要编译的语言版本，如 -lang=go1.12。
		默认为当前版本。
	-linkobj file
		将链接器特定对象写入文件，并将编译器特定
		对象写入通常输出文件（由 -o 指定）。
		没有此标志，-o 输出是链接器和编译器输入的组合。
	-m
		打印优化决策。较高的值或重复
		会产生更多细节。
	-memprofile file
		将编译的内存配置文件写入文件。
	-memprofilerate rate
		将编译的 runtime.MemProfileRate 设置为 rate。
	-msan
		插入对 C/C++ 内存清理程序的调用。
	-mutexprofile file
		将编译的互斥体配置文件写入文件。
	-nolocalimports
		禁止本地（相对）导入。
	-o file
		将对象写入文件（默认 file.o 或使用 -pack 时 file.a）。
	-p path
		为正在编译的代码设置预期的包导入路径，
		并诊断会导致循环依赖的导入。
	-pack
		写入包（存档）文件而不是对象文件
	-race
		使用启用的竞态检测器编译。
	-s
		警告可以简化的复合字面量。
	-shared
		生成可以链接到共享库的代码。
	-spectre list
		在列表中启用 spectre 缓解措施（all、index、ret）。
	-traceprofile file
		将执行跟踪写入文件。
	-trimpath prefix
		从记录的源文件路径中删除前缀。

与调试信息相关的标志：

	-dwarf
		生成 DWARF 符号。
	-dwarflocationlists
		在优化模式下向 DWARF 添加位置列表。
	-gendwarfinl int
		生成 DWARF 内联信息记录（默认 2）。

用于调试编译器本身的标志：

	-E
		调试符号导出。
	-K
		调试缺失的行号。
	-d list
		打印列表中项目的调试信息。尝试 -d help 获取更多信息。
	-live
		调试活跃度分析。
	-v
		增加调试冗长度。
	-%
		调试非静态初始化程序。
	-W
		类型检查后调试解析树。
	-f
		调试堆栈帧。
	-i
		调试行号堆栈。
	-j
		调试运行时初始化变量。
	-r
		调试生成的包装器。
	-w
		调试类型检查。

# 编译器指令

编译器以注释形式接受指令。
每个指令必须放在其自己的行上，在注释之前
只允许前导空格和制表符，注释开头和指令名称之间
必须没有空格，以区别于常规注释。
不了解指令约定或特定
指令的工具可以像任何其他注释一样跳过指令。

除了作为历史特殊情况的 line 指令外，
所有其他编译器指令都是
//go:name 的形式，指示它们由 Go 工具链定义。
*/
// # 行指令
//
// 行指令有几种形式：
//
// 	//line :line
// 	//line :line:col
// 	//line filename:line
// 	//line filename:line:col
// 	/*line :line*/
// 	/*line :line:col*/
// 	/*line filename:line*/
// 	/*line filename:line:col*/
//
// 为了被识别为行指令，注释必须以
// //line 或 /*line 开头，后跟一个空格，且必须包含至少一个冒号。
// //line 形式必须在行首开始。
// 行指令指定紧随其后的字符的源位置
// 注释来自指定的文件、行和列：
// 对于 //line 注释，这是下一行的第一个字符，
// 对于 /*line 注释，这是紧跟在结束 */ 之后的字符位置。
// 如果没有给出文件名，当也没有列号时，记录的文件名为空；
// 否则它是最近记录的文件名（实际文件名或由
// 前一个行指令指定的文件名）。
// 如果行指令不指定列号，列是"未知的"，直到
// 下一个指令，编译器不报告该范围的列号。
// 行指令文本从后向前解析：首先从指令文本中剥离
// 尾部的 :ddd（如果 ddd 是有效的数字 > 0）。然后以相同方式
// 剥离第二个 :ddd（如果有效）。之前的任何内容都被视为文件名
// （可能包括空格和冒号）。无效的行或列值被报告为错误。
//
// 示例：
//
//	//line foo.go:10      文件名是 foo.go，下一行的行号是 10
//	//line C:foo.go:10    冒号在文件名中是允许的，这里文件名是 C:foo.go，行是 10
//	//line  a:100 :10     空格在文件名中是允许的，这里文件名是 " a:100 "（不含引号）
//	/*line :10:20*/x      x 的位置在当前文件中，行号为 10，列号为 20
//	/*line foo: 10 */     此注释被识别为无效行指令（行号周围有额外空格）
//
// 行指令通常出现在机器生成的代码中，以便编译器和调试器
// 可以报告生成器原始输入中的位置。
/*
# 函数指令

函数指令适用于紧随其后的 Go 函数。

	//go:noescape

//go:noescape 指令必须后跟函数声明，不带
函数体（意味着函数的实现不是用 Go 编写的）。
它指定该函数不允许作为
参数传递的任何指针逃逸到堆或从该函数返回的值中。
此信息可在编译器对调用该函数的 Go 代码进行
逃逸分析期间使用。

	//go:uintptrescapes

//go:uintptrescapes 指令必须后跟函数声明。
它指定该函数的 uintptr 参数可能是已
转换为 uintptr 的指针值，必须在堆上且在调用期间
保持活跃，即使从类型来看似乎该
对象在调用期间不再需要。从指针到
uintptr 的转换必须出现在对该函数的任何调用的参数列表中。此
指令对于某些低级系统调用实现是必要的，
否则应避免使用。

	//go:noinline

//go:noinline 指令必须后跟函数声明。
它指定对该函数的调用不应被内联，覆盖
编译器的常规优化规则。这通常仅在调试特殊运行时函数或
编译器时需要。

	//go:norace

//go:norace 指令必须后跟函数声明。
它指定该函数的内存访问必须被
竞态检测器忽略。这最常用于在
不安全调用竞态检测器运行时的情况下调用的低级代码中。

	//go:nosplit

//go:nosplit 指令必须后跟函数声明。
它指定该函数必须省略其通常的堆栈溢出检查。
这最常被低级运行时代码使用，在
不安全抢占调用 goroutine 的情况下调用。
在低级运行时代码之外使用此指令是不安全的，
因为它允许 nosplit 函数覆盖堆栈末端，
导致内存损坏和任意程序故障。

# 链接名称指令

	//go:linkname localname [importpath.name]

//go:linkname 指令通常在以 ``localname`` 命名的 var 或 func
声明之前，尽管其位置不
改变其效果。
此指令确定用于 Go var 或
func 声明的对象文件符号，允许两个 Go 符号别名为相同的
对象文件符号，从而使一个包能够访问
另一个包中的符号，即使这会违反通常的
未导出声明封装或甚至类型安全。
因此，它仅在导入了 "unsafe" 的文件中启用。

它可能用于两种场景。假设包 upper
导入包 lower，可能间接。在第一种情况下，
包 lower 定义一个符号，其对象文件名属于
包 upper。两个包都包含一个 linkname 指令：包
lower 使用双参数形式，包 upper 使用
单参数形式。在下面的示例中，lower.f 是函数
upper.g 的别名：

    package upper
    import _ "unsafe"
    //go:linkname g
    func g()

    package lower
    import _ "unsafe"
    //go:linkname f upper.g
    func f() { ... }

包 upper 中的 linkname 指令抑制缺少函数体的常规错误。
（该检查也可以通过在包中包含 .s 文件（即使是空的）来抑制。）

在第二种情况下，包 upper 单方面为
包 lower 中的符号创建别名。在下面的示例中，upper.g 是函数
lower.f 的别名。

    package upper
    import _ "unsafe"
    //go:linkname g lower.f
    func g()

    package lower
    func f() { ... }

lower.f 的声明也可能有一个带有
单个参数 f 的 linkname 指令。这是可选的，但有助于
提醒读者该函数从包外部访问。

# WebAssembly 指令

	//go:wasmimport importmodule importname

//go:wasmimport 指令仅限于 wasm，必须后跟一个
不带函数体的函数声明。
它指定该函数由通过 ``importmodule'' 和 ``importname'' 标识的
wasm 模块提供。例如，

	//go:wasmimport a_module f
	func g()

导致 g 引用来自模块 a_module 的 WebAssembly 函数 f。

	//go:wasmexport exportname

//go:wasmexport 指令仅限于 wasm，必须后跟一个
函数定义。
它指定该函数作为 ``exportname'' 导出到 wasm 主机。
例如，

	//go:wasmexport h
	func hWasm() { ... }

使 Go 函数 hWasm 在此 WebAssembly 模块外作为 h 可用。

对于 go:wasmimport 和 go:wasmexport，
Go 函数的参数和返回值的类型根据
以下表格转换为 Wasm：

    Go 类型         Wasm 类型
    bool            i32
    int32, uint32   i32
    int64, uint64   i64
    float32         f32
    float64         f64
    unsafe.Pointer  i32
    pointer         i32 (下面有更多限制)
    string          (i32, i32) (仅允许作为参数，不作为结果)

编译器禁止任何其他参数类型。

对于指针类型，其元素类型必须是 bool、int8、uint8、int16、uint16、
int32、uint32、int64、uint64、float32、float64，或者是一个
数组，其元素类型是允许的指针元素类型，或者是一个结构体，
如果非空，则嵌入 [structs.HostLayout]，
并仅包含类型为允许的指针元素类型的字段。
*/
package main
