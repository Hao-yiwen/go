// 版权所有 2015 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

/*
Asm，通常作为 "go tool asm" 调用，将源文件汇编成以参数源文件的基本名加 .o 后缀
命名的目标文件。然后可以将目标文件与其他目标文件组合成包归档文件。

# 命令行

用法：

	go tool asm [flags] file

指定的文件必须是 Go 汇编文件。
所有目标操作系统和架构使用相同的汇编器。
GOOS 和 GOARCH 环境变量设置所需的目标。

标志：

	-D name[=value]
		使用可选的简单值预定义符号名称。
		可以重复使用以定义多个符号。
	-I dir1 -I dir2
		在查阅 $GOROOT/pkg/$GOOS_$GOARCH 之后，
		在 dir1、dir2 等目录中搜索 #include 文件。
	-S
		打印汇编代码和机器码。
	-V
		打印汇编器版本并退出。
	-debug
		在解析时转储指令。
	-dynlink
		支持引用在其他共享库中定义的 Go 符号。
	-e
		报告的错误数量没有限制。
	-gensymabis
		将符号 ABI 信息写入输出文件。不进行汇编。
	-o file
		将输出写入文件。默认情况下，/a/b/c/foo.s 的输出为 foo.o。
	-p pkgpath
		将预期的包导入路径设置为 pkgpath。
	-shared
		生成可以链接到共享库的代码。
	-spectre list
		启用列表中的 spectre 缓解措施（all、ret）。
	-trimpath prefix
		从记录的源文件路径中移除前缀。
	-v
		打印调试输出。

输入语言：

汇编器对所有架构使用大致相同的语法，
主要的变化与寻址模式有关。输入通过一个简化的 C 预处理器处理，
该预处理器实现了 #include、#define、#ifdef/endif，但不支持 #if 或 ##。

更多信息，请参见 https://golang.org/doc/asm。
*/
package main
