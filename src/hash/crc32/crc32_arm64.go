// 版权所有 2017 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// ARM64 特定的硬件加速 CRC32 算法。有关每个架构特定文件
// 实现的接口描述，请参阅 crc32.go。

package crc32

import "internal/cpu"

func castagnoliUpdate(crc uint32, p []byte) uint32
func ieeeUpdate(crc uint32, p []byte) uint32

func archAvailableCastagnoli() bool {
	return cpu.ARM64.HasCRC32
}

func archInitCastagnoli() {
	if !cpu.ARM64.HasCRC32 {
		panic("arch-specific crc32 instruction for Castagnoli not available")
	}
}

func archUpdateCastagnoli(crc uint32, p []byte) uint32 {
	if !cpu.ARM64.HasCRC32 {
		panic("arch-specific crc32 instruction for Castagnoli not available")
	}

	return ^castagnoliUpdate(^crc, p)
}

func archAvailableIEEE() bool {
	return cpu.ARM64.HasCRC32
}

func archInitIEEE() {
	if !cpu.ARM64.HasCRC32 {
		panic("arch-specific crc32 instruction for IEEE not available")
	}
}

func archUpdateIEEE(crc uint32, p []byte) uint32 {
	if !cpu.ARM64.HasCRC32 {
		panic("arch-specific crc32 instruction for IEEE not available")
	}

	return ^ieeeUpdate(^crc, p)
}
