// Copyright 2014 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

/*
 * Plan 9 a.out 常数和数据结构
 */

package plan9obj

// Plan 9 程序头。
type prog struct {
	Magic uint32 /* 魔术数字 */
	Text  uint32 /* 文本段的大小 */
	Data  uint32 /* 初始化数据的大小 */
	Bss   uint32 /* 未初始化数据的大小 */
	Syms  uint32 /* 符号表的大小 */
	Entry uint32 /* 入口点 */
	Spsz  uint32 /* pc/sp 偏移表的大小 */
	Pcsz  uint32 /* pc/行号表的大小 */
}

// Plan 9 符号表条目。
type sym struct {
	value uint64
	typ   byte
	name  []byte
}

const (
	Magic64 = 0x8000 // 64 位扩展头

	Magic386   = (4*11+0)*11 + 7
	MagicAMD64 = (4*26+0)*26 + 7 + Magic64
	MagicARM   = (4*20+0)*20 + 7
)
