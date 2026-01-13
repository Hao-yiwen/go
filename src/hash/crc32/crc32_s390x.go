// 版权所有 2016 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package crc32

import "internal/cpu"

const (
	vxMinLen    = 64
	vxAlignMask = 15 // 对齐到 16 字节
)

// hasVX 报告机器是否安装并启用了 z/Architecture 向量功能。
var hasVX = cpu.S390X.HasVX

// vectorizedCastagnoli 使用向量指令实现 CRC32。
// 它在 crc32_s390x.s 中定义。
//
//go:noescape
func vectorizedCastagnoli(crc uint32, p []byte) uint32

// vectorizedIEEE 使用向量指令实现 CRC32。
// 它在 crc32_s390x.s 中定义。
//
//go:noescape
func vectorizedIEEE(crc uint32, p []byte) uint32

func archAvailableCastagnoli() bool {
	return hasVX
}

var archCastagnoliTable8 *slicing8Table

func archInitCastagnoli() {
	if !hasVX {
		panic("not available")
	}
	// 对于小缓冲区，我们仍然使用 slicing-by-8。
	archCastagnoliTable8 = slicingMakeTable(Castagnoli)
}

// archUpdateCastagnoli 使用 vectorizedCastagnoli 计算 p 的校验和。
func archUpdateCastagnoli(crc uint32, p []byte) uint32 {
	if !hasVX {
		panic("not available")
	}
	// 如果数据长度超过阈值，则使用向量化函数。
	if len(p) >= vxMinLen {
		aligned := len(p) & ^vxAlignMask
		crc = vectorizedCastagnoli(crc, p[:aligned])
		p = p[aligned:]
	}
	if len(p) == 0 {
		return crc
	}
	return slicingUpdate(crc, archCastagnoliTable8, p)
}

func archAvailableIEEE() bool {
	return hasVX
}

var archIeeeTable8 *slicing8Table

func archInitIEEE() {
	if !hasVX {
		panic("not available")
	}
	// 对于小缓冲区，我们仍然使用 slicing-by-8。
	archIeeeTable8 = slicingMakeTable(IEEE)
}

// archUpdateIEEE 使用 vectorizedIEEE 计算 p 的校验和。
func archUpdateIEEE(crc uint32, p []byte) uint32 {
	if !hasVX {
		panic("not available")
	}
	// 如果数据长度超过阈值，则使用向量化函数。
	if len(p) >= vxMinLen {
		aligned := len(p) & ^vxAlignMask
		crc = vectorizedIEEE(crc, p[:aligned])
		p = p[aligned:]
	}
	if len(p) == 0 {
		return crc
	}
	return slicingUpdate(crc, archIeeeTable8, p)
}
