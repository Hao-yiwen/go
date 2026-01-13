// 版权所有 2011 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// AMD64 特定的硬件加速 CRC32 算法。有关每个架构特定文件
// 实现的接口描述，请参阅 crc32.go。

package crc32

import (
	"internal/cpu"
	"unsafe"
)

// 用于汇编的 internal/cpu 记录的偏移量。
const (
	offsetX86HasAVX512VPCLMULQDQL = unsafe.Offsetof(cpu.X86.HasAVX512VPCLMULQDQ)
)

// 本文件包含调用 SSE 4.2 版本的 Castagnoli 和 IEEE CRC 的代码。

// castagnoliSSE42 在 crc32_amd64.s 中定义，使用 SSE 4.2 CRC32 指令。
//
//go:noescape
func castagnoliSSE42(crc uint32, p []byte) uint32

// castagnoliSSE42Triple 在 crc32_amd64.s 中定义，使用 SSE 4.2 CRC32 指令。
//
//go:noescape
func castagnoliSSE42Triple(
	crcA, crcB, crcC uint32,
	a, b, c []byte,
	rounds uint32,
) (retA uint32, retB uint32, retC uint32)

// ieeeCLMUL 在 crc_amd64.s 中定义，使用 PCLMULQDQ 指令和 SSE 4.1。
//
//go:noescape
func ieeeCLMUL(crc uint32, p []byte) uint32

const castagnoliK1 = 168
const castagnoliK2 = 1344

type sse42Table [4]Table

var castagnoliSSE42TableK1 *sse42Table
var castagnoliSSE42TableK2 *sse42Table

func archAvailableCastagnoli() bool {
	return cpu.X86.HasSSE42
}

func archInitCastagnoli() {
	if !cpu.X86.HasSSE42 {
		panic("arch-specific Castagnoli not available")
	}
	castagnoliSSE42TableK1 = new(sse42Table)
	castagnoliSSE42TableK2 = new(sse42Table)
	// 参见 updateCastagnoli 中的描述。
	//    t[0][i] = CRC(i000, O)
	//    t[1][i] = CRC(0i00, O)
	//    t[2][i] = CRC(00i0, O)
	//    t[3][i] = CRC(000i, O)
	// 其中 O 是 K 个零的序列。
	var tmp [castagnoliK2]byte
	for b := 0; b < 4; b++ {
		for i := 0; i < 256; i++ {
			val := uint32(i) << uint32(b*8)
			castagnoliSSE42TableK1[b][i] = castagnoliSSE42(val, tmp[:castagnoliK1])
			castagnoliSSE42TableK2[b][i] = castagnoliSSE42(val, tmp[:])
		}
	}
}

// castagnoliShift 使用给定的初始 crc 值计算 K1 或 K2 个零
//（取决于给定的表）的 CRC32-C。这对应于 updateCastagnoli
// 描述中的 CRC(crc, O)。
func castagnoliShift(table *sse42Table, crc uint32) uint32 {
	return table[3][crc>>24] ^
		table[2][(crc>>16)&0xFF] ^
		table[1][(crc>>8)&0xFF] ^
		table[0][crc&0xFF]
}

func archUpdateCastagnoli(crc uint32, p []byte) uint32 {
	if !cpu.X86.HasSSE42 {
		panic("not available")
	}

	// 此方法灵感来自 Intel 白皮书中的算法：
	//    "Fast CRC Computation for iSCSI Polynomial Using CRC32 Instruction"
	// 使用了相同的将缓冲区分成三部分的策略，但合并计算不同；
	// 完整的推导在下面解释。
	//
	// -- 基本思想 --
	//
	// CRC32 指令（在 SSE4.2 中可用）一次可以处理 8 字节。
	// 在最近的 Intel 架构中，该指令需要 3 个周期；
	// 但是，如果三条指令不相互依赖，处理器可以流水线执行多达三条指令。
	//
	// 粗略地说，这意味着我们可以在大约相同的时间内处理三个缓冲区，
	// 与处理一个缓冲区的时间相同。
	//
	// 因此，思路是将缓冲区分成三部分，分别对三部分进行 CRC 计算，
	// 然后合并结果。
	//
	// 合并结果需要预计算的表，因此我们必须选择一个固定的缓冲区长度来优化。
	// 长度越长，速度越快；但只有长于此长度的缓冲区才会使用此优化。
	// 我们选择两个阈值并为两者计算表：
	//  - 一个约为 512：168*3=504
	//  - 一个约为 4KB：1344*3=4032
	//
	// -- 具体细节 --
	//
	// 设 CRC(I, X) 为序列 X 的非反转 CRC32-C（初始非反转 CRC 为 I）。
	// 此函数具有以下属性：
	//   (a) CRC(I, AB) = CRC(CRC(I, A), B)
	//   (b) CRC(I, A xor B) = CRC(I, A) xor CRC(0, B)
	//
	// 假设我们想计算 CRC(I, ABC)，其中 A、B、C 是三个各 K 字节的序列，
	// K 是固定常数。设 O 为 K 个零字节的序列。
	//
	// CRC(I, ABC) = CRC(I, ABO xor C)
	//             = CRC(I, ABO) xor CRC(0, C)
	//             = CRC(CRC(I, AB), O) xor CRC(0, C)
	//             = CRC(CRC(I, AO xor B), O) xor CRC(0, C)
	//             = CRC(CRC(I, AO) xor CRC(0, B), O) xor CRC(0, C)
	//             = CRC(CRC(CRC(I, A), O) xor CRC(0, B), O) xor CRC(0, C)
	//
	// castagnoliSSE42Triple 函数可以高效地计算 CRC(I, A)、CRC(0, B)
	// 和 CRC(0, C)。我们只需要找到一种方法，在给定 4 字节初始值 uvwx 的情况下
	// 快速计算 CRC(uvwx, O)。我们可以预计算这些值；由于我们不能有一个 32 位表，
	// 我们将其分解为四个 8 位表：
	//
	//    CRC(uvwx, O) = CRC(u000, O) xor
	//                   CRC(0v00, O) xor
	//                   CRC(00w0, O) xor
	//                   CRC(000x, O)
	//
	// 我们可以为所有 8 位值计算对应于四个项的表。

	crc = ^crc

	// 如果缓冲区足够长以使用优化，处理前几个字节
	// 以将缓冲区对齐到 8 字节边界（如有必要）。
	if len(p) >= castagnoliK1*3 {
		delta := int(uintptr(unsafe.Pointer(&p[0])) & 7)
		if delta != 0 {
			delta = 8 - delta
			crc = castagnoliSSE42(crc, p[:delta])
			p = p[delta:]
		}
	}

	// 每次处理 3*K2。
	for len(p) >= castagnoliK2*3 {
		// 计算 CRC(I, A)、CRC(0, B) 和 CRC(0, C)。
		crcA, crcB, crcC := castagnoliSSE42Triple(
			crc, 0, 0,
			p, p[castagnoliK2:], p[castagnoliK2*2:],
			castagnoliK2/24)

		// CRC(I, AB) = CRC(CRC(I, A), O) xor CRC(0, B)
		crcAB := castagnoliShift(castagnoliSSE42TableK2, crcA) ^ crcB
		// CRC(I, ABC) = CRC(CRC(I, AB), O) xor CRC(0, C)
		crc = castagnoliShift(castagnoliSSE42TableK2, crcAB) ^ crcC
		p = p[castagnoliK2*3:]
	}

	// 每次处理 3*K1。
	for len(p) >= castagnoliK1*3 {
		// 计算 CRC(I, A)、CRC(0, B) 和 CRC(0, C)。
		crcA, crcB, crcC := castagnoliSSE42Triple(
			crc, 0, 0,
			p, p[castagnoliK1:], p[castagnoliK1*2:],
			castagnoliK1/24)

		// CRC(I, AB) = CRC(CRC(I, A), O) xor CRC(0, B)
		crcAB := castagnoliShift(castagnoliSSE42TableK1, crcA) ^ crcB
		// CRC(I, ABC) = CRC(CRC(I, AB), O) xor CRC(0, C)
		crc = castagnoliShift(castagnoliSSE42TableK1, crcAB) ^ crcC
		p = p[castagnoliK1*3:]
	}

	// 对剩余部分使用简单实现。
	crc = castagnoliSSE42(crc, p)
	return ^crc
}

func archAvailableIEEE() bool {
	return cpu.X86.HasPCLMULQDQ && cpu.X86.HasSSE41
}

var archIeeeTable8 *slicing8Table

func archInitIEEE() {
	if !cpu.X86.HasPCLMULQDQ || !cpu.X86.HasSSE41 {
		panic("not available")
	}
	// 对于小缓冲区，我们仍然使用 slicing-by-8。
	archIeeeTable8 = slicingMakeTable(IEEE)
}

func archUpdateIEEE(crc uint32, p []byte) uint32 {
	if !cpu.X86.HasPCLMULQDQ || !cpu.X86.HasSSE41 {
		panic("not available")
	}

	if len(p) >= 64 {
		left := len(p) & 15
		do := len(p) - left
		crc = ^ieeeCLMUL(^crc, p[:do])
		p = p[do:]
	}
	if len(p) == 0 {
		return crc
	}
	return slicingUpdate(crc, archIeeeTable8, p)
}
