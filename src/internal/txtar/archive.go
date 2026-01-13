// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package txtar 实现一个简单的基于文本的文件存档格式。
//
// 格式的目标是：
//
//   - 足够简单，可以手动创建和编辑。
//   - 能够存储描述 go 命令测试用例的文本文件树。
//   - 在 git 历史和代码审查中显示得很好。
//
// 不包括的目标：成为完全通用的存档格式，
// 存储二进制数据、存储文件模式、存储特殊文件（如
// 符号链接）等。
//
// # Txtar 格式
//
// txtar 存档是零行或多行注释，然后是一系列文件条目。
// 每个文件条目以形式为 "-- 文件名 --" 的文件标记行开始
// 后面跟零行或多行文件内容行，组成文件数据。
// 注释或文件内容在下一个文件标记行处结束。
// 文件标记行必须以三字节序列 "-- " 开始
// 并以三字节序列 " --" 结束，但括起来的
// 文件名可以被额外的空白字符包围，
// 所有这些都会被去掉。
//
// 如果 txtar 文件在最后一行缺少尾部换行符，
// 解析器应该认为存在最终换行符。
//
// txtar 存档中没有可能的语法错误。
package txtar

import (
	"bytes"
	"fmt"
	"os"
	"strings"
)

// An Archive 是文件的集合。
type Archive struct {
	Comment []byte
	Files   []File
}

// A File 是存档中的单个文件。
type File struct {
	Name string // 文件名 ("foo/bar.txt")
	Data []byte // 文件的文本内容
}

// Format 返回 Archive 的序列化形式。
// 假设 Archive 数据结构格式良好：
// a.Comment 和所有 a.File[i].Data 不包含文件标记行，
// 且所有 a.File[i].Name 都非空。
func Format(a *Archive) []byte {
	var buf bytes.Buffer
	buf.Write(fixNL(a.Comment))
	for _, f := range a.Files {
		fmt.Fprintf(&buf, "-- %s --\n", f.Name)
		buf.Write(fixNL(f.Data))
	}
	return buf.Bytes()
}

// ParseFile 将命名文件解析为存档。
func ParseFile(file string) (*Archive, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	return Parse(data), nil
}

// Parse 解析 Archive 的序列化形式。
// 返回的 Archive 保存数据的切片。
func Parse(data []byte) *Archive {
	a := new(Archive)
	var name string
	a.Comment, name, data = findFileMarker(data)
	for name != "" {
		f := File{name, nil}
		f.Data, name, data = findFileMarker(data)
		a.Files = append(a.Files, f)
	}
	return a
}

var (
	newlineMarker = []byte("\n-- ")
	marker        = []byte("-- ")
	markerEnd     = []byte(" --")
)

// findFileMarker 在数据中找到下一个文件标记，
// 提取文件名，并返回标记前的数据、
// 文件名和标记后的数据。
// 如果没有下一个标记，findFileMarker 返回 before = fixNL(data), name = "", after = nil。
func findFileMarker(data []byte) (before []byte, name string, after []byte) {
	var i int
	for {
		if name, after = isMarker(data[i:]); name != "" {
			return data[:i], name, after
		}
		j := bytes.Index(data[i:], newlineMarker)
		if j < 0 {
			return fixNL(data), "", nil
		}
		i += j + 1 // positioned at start of new possible marker
	}
}

// isMarker 检查数据是否以文件标记行开始。
// 如果是，它返回行中的名称和行后的数据。
// 否则它返回 name == "" 和未指定的 after。
func isMarker(data []byte) (name string, after []byte) {
	if !bytes.HasPrefix(data, marker) {
		return "", nil
	}
	if i := bytes.IndexByte(data, '\n'); i >= 0 {
		data, after = data[:i], data[i+1:]
	}
	if !(bytes.HasSuffix(data, markerEnd) && len(data) >= len(marker)+len(markerEnd)) {
		return "", nil
	}
	return strings.TrimSpace(string(data[len(marker) : len(data)-len(markerEnd)])), after
}

// 如果数据为空或以 \n 结尾，fixNL 返回数据。
// 否则 fixNL 返回由添加了最终 \n 的数据组成的新切片。
func fixNL(data []byte) []byte {
	if len(data) == 0 || data[len(data)-1] == '\n' {
		return data
	}
	d := make([]byte, len(data)+1)
	copy(d, data)
	d[len(data)] = '\n'
	return d
}
