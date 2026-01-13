// 版权所有 2023 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

//go:build wasip1

package os

import (
	"syscall"
	"unsafe"
)

// https://github.com/WebAssembly/WASI/blob/main/legacy/preview1/docs.md#-dirent-record
const sizeOfDirent = 24

func direntIno(buf []byte) (uint64, bool) {
	return readInt(buf, unsafe.Offsetof(syscall.Dirent{}.Ino), unsafe.Sizeof(syscall.Dirent{}.Ino))
}

func direntReclen(buf []byte) (uint64, bool) {
	namelen, ok := direntNamlen(buf)
	return sizeOfDirent + namelen, ok
}

func direntNamlen(buf []byte) (uint64, bool) {
	return readInt(buf, unsafe.Offsetof(syscall.Dirent{}.Namlen), unsafe.Sizeof(syscall.Dirent{}.Namlen))
}

func direntType(buf []byte) FileMode {
	off := unsafe.Offsetof(syscall.Dirent{}.Type)
	if off >= uintptr(len(buf)) {
		return ^FileMode(0) // unknown
	}
	switch syscall.Filetype(buf[off]) {
	case syscall.FILETYPE_BLOCK_DEVICE:
		return ModeDevice
	case syscall.FILETYPE_CHARACTER_DEVICE:
		return ModeDevice | ModeCharDevice
	case syscall.FILETYPE_DIRECTORY:
		return ModeDir
	case syscall.FILETYPE_REGULAR_FILE:
		return 0
	case syscall.FILETYPE_SOCKET_DGRAM:
		return ModeSocket
	case syscall.FILETYPE_SOCKET_STREAM:
		return ModeSocket
	case syscall.FILETYPE_SYMBOLIC_LINK:
		return ModeSymlink
	}
	return ^FileMode(0) // unknown
}
