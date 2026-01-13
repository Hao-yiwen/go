// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

/*
Gofmt 格式化 Go 程序。
它使用制表符进行缩进，使用空格进行对齐。
对齐假设编辑器使用固定宽度字体。

没有显式路径，它处理标准输入。给定一个文件，
它操作该文件；给定一个目录，它递归地操作
该目录中的所有 .go 文件。（以句号开头的文件被忽略。）
默认情况下，gofmt 将重新格式化的源代码打印到标准输出。

用法：

	gofmt [flags] [path ...]

标志是：

	-d
		不将重新格式化的源代码打印到标准输出。
		如果文件的格式与 gofmt 的不同，打印差异
		到标准输出。
	-e
		打印所有（包括虚假的）错误。
	-l
		不将重新格式化的源代码打印到标准输出。
		如果文件的格式与 gofmt 的不同，打印其名称
		到标准输出。
	-r rule
		在重新格式化前应用重写规则到源代码。
	-s
		尝试简化代码（在应用重写规则后，如果有的话）。
	-w
		不将重新格式化的源代码打印到标准输出。
		如果文件的格式与 gofmt 的不同，用 gofmt 的版本覆盖它。
		如果在覆盖期间发生错误，原始文件将从自动备份中恢复。

调试支持：

	-cpuprofile filename
		将 cpu 配置文件写入指定文件。

使用 -r 标志指定的重写规则必须是以下形式的字符串：

	pattern -> replacement

pattern 和 replacement 都必须是有效的 Go 表达式。
在模式中，单字符小写标识符作为
通配符匹配任意子表达式；这些表达式
将被替换为替换中的相同标识符。

当 gofmt 从标准输入读取时，它接受完整的 Go 程序
或程序片段。程序片段必须是语法上
有效的声明列表、语句列表或表达式。在格式化
此类片段时，gofmt 保留前导缩进以及前导
和尾随空格，以便 Go 程序的各个部分可以
通过管道传送到 gofmt 来格式化。

# 示例

要检查文件中的不必要括号：

	gofmt -r '(a) -> a' -l *.go

要删除括号：

	gofmt -r '(a) -> a' -w *.go

要将包树从显式切片上界转换为隐式上界：

	gofmt -r 'α[β:len(α)] -> α[β:]' -w $GOROOT/src

# simplify 命令

使用 -s 调用时，gofmt 将在可能的地方进行以下源代码转换。

	数组、切片或映射复合字面量的形式：
		[]T{T{}, T{}}
	将被简化为：
		[]T{{}, {}}

	切片表达式的形式：
		s[a:len(s)]
	将被简化为：
		s[a:]

	范围的形式：
		for x, _ = range v {...}
	将被简化为：
		for x = range v {...}

	范围的形式：
		for _ = range v {...}
	将被简化为：
		for range v {...}

这可能导致与早期 Go 版本不兼容的更改。
*/
package main

// BUG(rsc): -r 的实现有点慢。
// BUG(gri): 如果 -w 失败，恢复的原始文件可能没有某些
// 原始文件属性。
