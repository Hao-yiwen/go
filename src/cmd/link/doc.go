// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

/*
Link 通常作为 "go tool link" 调用，读取包 main 的 Go 存档或对象，
以及其依赖项，并将它们组合
成可执行二进制文件。

# 命令行

用法：

	go tool link [flags] main.a

标志：

	-B note
		添加使用 ELF 时的 ELF_NT_GNU_BUILD_ID 注释。
		该值应以 0x 开头，为偶数个十六进制数字。
		或者，你可以传递 "gobuildid" 以从
		Go 构建 ID 派生 GNU 构建 ID。
	-E entry
		设置入口符号名称。
	-H type
		设置可执行格式类型。
		默认格式从 GOOS 和 GOARCH 推断。
		在 Windows 上，-H windowsgui 写入 "GUI 二进制文件" 而不是 "控制台二进制文件"。
	-I interpreter
		设置要使用的 ELF 动态链接器。
	-L dir1 -L dir2
		在 dir1, dir2 等中搜索导入的包，
		在咨询 $GOROOT/pkg/$GOOS_$GOARCH 之后。
	-R quantum
		设置地址舍入量子。
	-T address
		设置文本符号的起始地址。
	-V
		打印链接器版本并退出。
	-X importpath.name=value
		将 importpath 中名为 name 的字符串变量的值设置为 value。
		这仅在变量在源代码中声明为未初始化
		或初始化为常量字符串表达式时有效。如果初始化程序
		进行函数调用或引用其他变量，-X 将不起作用。
		注意，在 Go 1.5 之前，此选项需要两个单独的参数。
	-asan
		与 C/C++ 地址清理程序支持链接。
	-aslr
		在 Windows 上为 buildmode=c-shared 启用 ASLR（默认 true）。
	-bindnow
		标记动态链接的 ELF 对象以供立即函数绑定（默认 false）。
	-buildid id
		记录 id 作为 Go 工具链构建 id。
	-buildmode mode
		设置构建模式（默认 exe）。
	-c
		转储调用图。
	-checklinkname=value
		如果值为 0，允许所有 go:linkname 指令。
		如果值为 1（默认值），仅允许已知的一组广泛使用的
		链接名称。
	-compressdwarf
		如果可能，压缩 DWARF（默认 true）。
	-cpuprofile file
		将 CPU 配置文件写入文件。
	-d
		禁用动态可执行文件的生成。
		发出的代码在两种情况下都相同；该选项
		仅控制是否包含动态头。
		动态头默认启用，即使没有
		对动态库的引用，因为许多常见的
		系统工具现在假设头的存在。
	-dumpdep
		转储符号依赖关系图。
	-e
		对报告的错误数量没有限制。
	-extar ar
		设置外部存档程序（默认 "ar"）。
		仅用于 -buildmode=c-archive。
	-extld linker
		设置外部链接器（默认 "clang" 或 "gcc"）。
	-extldflags flags
		设置要传递给外部链接器的以空格分隔的标志。
	-f
		忽略链接存档中的版本不匹配。
	-funcalign N
		将函数对齐设置为 N 字节
	-g
		禁用 Go 包数据检查。
	-importcfg file
		从文件读取导入配置。
		在文件中，设置 packagefile、packageshlib 以指定导入解析。
	-installsuffix suffix
		在 $GOROOT/pkg/$GOOS_$GOARCH_suffix 中查找包
		而不是 $GOROOT/pkg/$GOOS_$GOARCH。
	-k symbol
		设置字段跟踪符号。设置 GOEXPERIMENT=fieldtrack 时使用此标志。
	-libgcc file
		设置编译器支持库的名称。
		这仅在内部链接模式下使用。
		如果未设置，默认值来自运行编译器，
		该编译器可以由 -extld 选项设置。
		设置为 "none" 以不使用支持库。
	-linkmode mode
		设置链接模式（internal、external、auto）。
		这设置 cmd/cgo/doc.go 中描述的链接模式。
	-linkshared
		与已安装的 Go 共享库链接（实验性）。
	-memprofile file
		将内存配置文件写入文件。
	-memprofilerate rate
		将 runtime.MemProfileRate 设置为 rate。
	-msan
		与 C/C++ 内存清理程序支持链接。
	-o file
		将输出写入文件（默认 a.out，或在 Windows 上为 a.out.exe）。
	-pluginpath path
		用于前缀导出的插件符号的路径名。
	-r dir1:dir2:...
		设置 ELF 动态链接器搜索路径。
	-race
		与竞态检测库链接。
	-s
		省略符号表和调试信息。
		暗示 -w 标志，可以用 -w=0 否定。
	-tmpdir dir
		将临时文件写入 dir。
		临时文件仅在外部链接模式下使用。
	-v
		打印链接器操作的跟踪。
	-w
		省略 DWARF 符号表。
*/
package main
