// 版权所有 2023 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

//go:build !purego

package maphash

import (
	"internal/abi"
	"internal/runtime/maps"
	"unsafe"
)

const purego = false

//go:linkname runtime_rand runtime.rand
func runtime_rand() uint64

//go:linkname runtime_memhash runtime.memhash
//go:noescape
func runtime_memhash(p unsafe.Pointer, seed, s uintptr) uintptr

func rthash(buf []byte, seed uint64) uint64 {
	if len(buf) == 0 {
		return seed
	}
	len := len(buf)
	// 运行时哈希器只能处理 uintptr。对于 64 位
	// 架构，我们直接使用哈希器。否则，
	// 我们对低 32 位和高 32 位使用两个并行的哈希器。
	if maps.Use64BitHash {
		return uint64(runtime_memhash(unsafe.Pointer(&buf[0]), uintptr(seed), uintptr(len)))
	}
	lo := runtime_memhash(unsafe.Pointer(&buf[0]), uintptr(uint32(seed)), uintptr(len))
	hi := runtime_memhash(unsafe.Pointer(&buf[0]), uintptr(seed>>32), uintptr(len))
	return uint64(hi)<<32 | uint64(lo)
}

func rthashString(s string, state uint64) uint64 {
	buf := unsafe.Slice(unsafe.StringData(s), len(s))
	return rthash(buf, state)
}

func randUint64() uint64 {
	return runtime_rand()
}

func comparableHash[T comparable](v T, seed Seed) uint64 {
	s := seed.s
	var m map[T]struct{}
	mTyp := abi.TypeOf(m)
	hasher := (*abi.MapType)(unsafe.Pointer(mTyp)).Hasher
	if maps.Use64BitHash {
		return uint64(hasher(abi.NoEscape(unsafe.Pointer(&v)), uintptr(s)))
	}
	lo := hasher(abi.NoEscape(unsafe.Pointer(&v)), uintptr(uint32(s)))
	hi := hasher(abi.NoEscape(unsafe.Pointer(&v)), uintptr(s>>32))
	return uint64(hi)<<32 | uint64(lo)
}

func writeComparable[T comparable](h *Hash, v T) {
	h.state.s = comparableHash(v, h.state)
}
