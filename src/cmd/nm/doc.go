// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Nm 列出对象文件、存档或可执行文件定义或使用的符号。
//
// 用法：
//
//	go tool nm [options] file...
//
// 默认输出每行打印一个符号，有三个空格分隔的
// 字段，给出地址（十六进制）、类型（一个字符）和
// 符号的名称。类型如下：
//
//	T	文本（代码）段符号
//	t	静态文本段符号
//	R	只读数据段符号
//	r	静态只读数据段符号
//	D	数据段符号
//	d	静态数据段符号
//	B	bss 段符号
//	b	静态 bss 段符号
//	C	常数地址
//	U	引用但未定义的符号
//
// 按照既定惯例，对于未定义的
// 符号（类型 U），地址被省略。
//
// 选项控制打印输出：
//
//	-n
//		-sort address (numeric) 的别名，
//		与其他 nm 命令兼容
//	-size
//		在地址和类型之间以十进制打印符号大小
//	-sort {address,name,none,size}
//		按给定顺序排序输出（默认 name）
//		size 从最大到最小排序
//	-type
//		在名称后打印符号类型
package main
