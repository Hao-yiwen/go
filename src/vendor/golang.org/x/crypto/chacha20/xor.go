// 版权所有 2018 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package chacha20

import "runtime"

// 具有快速未对齐 32 位小端序访问的平台。
const unaligned = runtime.GOARCH == "386" ||
	runtime.GOARCH == "amd64" ||
	runtime.GOARCH == "arm64" ||
	runtime.GOARCH == "ppc64le" ||
	runtime.GOARCH == "s390x"

// addXor 从 src 读取小端序 uint32，将其与 (a + b) 进行异或，
// 并将结果以小端序字节顺序放入 dst。
func addXor(dst, src []byte, a, b uint32) {
	_, _ = src[3], dst[3] // 边界检查消除提示
	if unaligned {
		// 编译器应该将此代码优化为
		// 32 位未对齐小端序加载和存储。
		// TODO: 一旦编译器能可靠地处理下面的通用代码，就删除此代码。
		// 详见 issue #25111。
		v := uint32(src[0])
		v |= uint32(src[1]) << 8
		v |= uint32(src[2]) << 16
		v |= uint32(src[3]) << 24
		v ^= a + b
		dst[0] = byte(v)
		dst[1] = byte(v >> 8)
		dst[2] = byte(v >> 16)
		dst[3] = byte(v >> 24)
	} else {
		a += b
		dst[0] = src[0] ^ byte(a)
		dst[1] = src[1] ^ byte(a>>8)
		dst[2] = src[2] ^ byte(a>>16)
		dst[3] = src[3] ^ byte(a>>24)
	}
}
