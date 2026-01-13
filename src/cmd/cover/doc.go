// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

/*
Cover 是用于分析由 'go test -coverprofile=cover.out' 生成的
覆盖率配置文件的程序。

Cover 也被 'go test -cover' 使用，以便用
注释重写源代码来跟踪每个函数的哪些部分被执行
（这被称为"检测"）。Cover 可以在"传统模式"下
一次在一个 Go 源文件上运行，或者当由 Go 工具调用时
它将一次处理单个包中的所有源文件
（包范围检测通过 "-pkgcfg" 选项启用）。

生成检测代码时，cover 工具通过研究源代码
计算近似基本块信息。因此它比
二进制重写覆盖率工具更可移植，但功能
也略逊一筹。例如，它不探查 && 和 || 表达式内部，
并可能被具有多个函数
字面量的单个语句轻微混淆。

计算使用 cgo 的包的覆盖率时，cover 工具
必须应用于 cgo 预处理的输出，而不是输入，
因为 cover 删除了对 cgo 重要的注释。

有关使用信息，请查看：

	go help testflag
	go tool cover -help
*/
package main
