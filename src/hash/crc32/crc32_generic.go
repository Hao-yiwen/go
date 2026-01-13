// 版权所有 2011 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// 本文件包含不特定于任何架构且不使用硬件加速的 CRC32 算法。
//
// 简单（且慢速）的 CRC32 实现仅使用 256*4 字节的表。
//
// slicing-by-8 算法是一个更快的实现，使用更大的表（8*256*4 字节）。

package crc32

import "internal/byteorder"

// simpleMakeTable 为指定的多项式分配并构造一个 Table。
// 该表适用于简单算法（simpleUpdate）。
func simpleMakeTable(poly uint32) *Table {
	t := new(Table)
	simplePopulateTable(poly, t)
	return t
}

// simplePopulateTable 为指定的多项式构造一个 Table，
// 适用于 simpleUpdate。
func simplePopulateTable(poly uint32, t *Table) {
	for i := 0; i < 256; i++ {
		crc := uint32(i)
		for j := 0; j < 8; j++ {
			if crc&1 == 1 {
				crc = (crc >> 1) ^ poly
			} else {
				crc >>= 1
			}
		}
		t[i] = crc
	}
}

// simpleUpdate 使用简单算法更新 CRC，
// 给定一个之前使用 simpleMakeTable 计算的表。
func simpleUpdate(crc uint32, tab *Table, p []byte) uint32 {
	crc = ^crc
	for _, v := range p {
		crc = tab[byte(crc)^v] ^ (crc >> 8)
	}
	return ^crc
}

// 当负载 >= 此值时使用 slicing-by-8。
const slicing8Cutoff = 16

// slicing8Table 是 8 个 Table 的数组，由 slicing-by-8 算法使用。
type slicing8Table [8]Table

// slicingMakeTable 为指定的多项式构造一个 slicing8Table。
// 该表适用于 slicing-by-8 算法（slicingUpdate）。
func slicingMakeTable(poly uint32) *slicing8Table {
	t := new(slicing8Table)
	simplePopulateTable(poly, &t[0])
	for i := 0; i < 256; i++ {
		crc := t[0][i]
		for j := 1; j < 8; j++ {
			crc = t[0][crc&0xFF] ^ (crc >> 8)
			t[j][i] = crc
		}
	}
	return t
}

// slicingUpdate 使用 slicing-by-8 算法更新 CRC，
// 给定一个之前使用 slicingMakeTable 计算的表。
func slicingUpdate(crc uint32, tab *slicing8Table, p []byte) uint32 {
	if len(p) >= slicing8Cutoff {
		crc = ^crc
		for len(p) > 8 {
			crc ^= byteorder.LEUint32(p)
			crc = tab[0][p[7]] ^ tab[1][p[6]] ^ tab[2][p[5]] ^ tab[3][p[4]] ^
				tab[4][crc>>24] ^ tab[5][(crc>>16)&0xFF] ^
				tab[6][(crc>>8)&0xFF] ^ tab[7][crc&0xFF]
			p = p[8:]
		}
		crc = ^crc
	}
	if len(p) == 0 {
		return crc
	}
	return simpleUpdate(crc, &tab[0], p)
}
