// Copyright 2014 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

/*
Pack 是传统 Unix ar 工具的简单版本。
它仅实现 Go 所需的操作。

用法：

	go tool pack op file.a [name...]

Pack 将操作应用于存档，使用名称作为操作的参数。

操作 op 由以下字母之一给出：

	c	将文件（来自文件系统）附加到新存档
	p	从存档打印文件
	r	将文件（来自文件系统）附加到存档
	t	列出存档中的文件
	x	从存档中提取文件

c 命令的存档参数必须不存在或是
有效的存档文件，将在添加新条目前清除。如果
文件存在但不是存档，这是一个错误。

对于 p、t 和 x 命令，在命令行上列出没有名称
导致操作应用于存档中的所有文件。

与 Unix ar 相比，r 操作总是附加到存档，
即使具有给定名称的文件已在存档中存在。这样
pack 的 r 操作更像 Unix ar 的 rq 操作。

在操作中添加字母 v，如 pv 或 rv，启用详细操作：
对于 c 和 r 命令，在添加文件时打印名称。
对于 p 命令，每个文件都以名称为前缀，单独一行。
对于 t 命令，列表包括额外的文件元数据。
对于 x 命令，在提取文件时打印名称。
*/
package main
