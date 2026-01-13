// 版权所有 2017 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package crc32

import (
	"unsafe"
)

const (
	vecMinLen    = 16
	vecAlignMask = 15 // 对齐到 16 字节
	crcIEEE      = 1
	crcCast      = 2
)

//go:noescape
func ppc64SlicingUpdateBy8(crc uint32, table8 *slicing8Table, p []byte) uint32

// 此函数要求缓冲区 16 字节对齐且长度 > 16 字节。
//
//go:noescape
func vectorCrc32(crc uint32, poly uint32, p []byte) uint32

var archCastagnoliTable8 *slicing8Table

func archInitCastagnoli() {
	archCastagnoliTable8 = slicingMakeTable(Castagnoli)
}

func archUpdateCastagnoli(crc uint32, p []byte) uint32 {
	if len(p) >= 4*vecMinLen {
		// 如果未对齐，则先处理初始的未对齐字节

		if uint64(uintptr(unsafe.Pointer(&p[0])))&uint64(vecAlignMask) != 0 {
			align := uint64(uintptr(unsafe.Pointer(&p[0]))) & uint64(vecAlignMask)
			newlen := vecMinLen - align
			crc = ppc64SlicingUpdateBy8(crc, archCastagnoliTable8, p[:newlen])
			p = p[newlen:]
		}
		// 现在 p 应该已经对齐了
		aligned := len(p) & ^vecAlignMask
		crc = vectorCrc32(crc, crcCast, p[:aligned])
		p = p[aligned:]
	}
	if len(p) == 0 {
		return crc
	}
	return ppc64SlicingUpdateBy8(crc, archCastagnoliTable8, p)
}

func archAvailableIEEE() bool {
	return true
}
func archAvailableCastagnoli() bool {
	return true
}

var archIeeeTable8 *slicing8Table

func archInitIEEE() {
	// 对于小缓冲区，我们仍然使用 slicing-by-8。
	archIeeeTable8 = slicingMakeTable(IEEE)
}

// archUpdateIEEE 使用 vectorizedIEEE 计算 p 的校验和。
func archUpdateIEEE(crc uint32, p []byte) uint32 {

	// 检查是否应使用向量代码。如果未对齐，则先处理
	// 到对齐字节为止的那些字节。

	if len(p) >= 4*vecMinLen {
		if uint64(uintptr(unsafe.Pointer(&p[0])))&uint64(vecAlignMask) != 0 {
			align := uint64(uintptr(unsafe.Pointer(&p[0]))) & uint64(vecAlignMask)
			newlen := vecMinLen - align
			crc = ppc64SlicingUpdateBy8(crc, archIeeeTable8, p[:newlen])
			p = p[newlen:]
		}
		aligned := len(p) & ^vecAlignMask
		crc = vectorCrc32(crc, crcIEEE, p[:aligned])
		p = p[aligned:]
	}
	if len(p) == 0 {
		return crc
	}
	return ppc64SlicingUpdateBy8(crc, archIeeeTable8, p)
}
