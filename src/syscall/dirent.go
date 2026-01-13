// 版权所有 2009 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

//go:build unix || (js && wasm) || wasip1

package syscall

import (
	"internal/byteorder"
	"internal/goarch"
	"runtime"
	"unsafe"
)

// readInt 返回位于偏移量 off 处的 size 字节无符号整数（按本机字节序）。
func readInt(b []byte, off, size uintptr) (u uint64, ok bool) {
	if len(b) < int(off+size) {
		return 0, false
	}
	if goarch.BigEndian {
		return readIntBE(b[off:], size), true
	}
	return readIntLE(b[off:], size), true
}

func readIntBE(b []byte, size uintptr) uint64 {
	switch size {
	case 1:
		return uint64(b[0])
	case 2:
		return uint64(byteorder.BEUint16(b))
	case 4:
		return uint64(byteorder.BEUint32(b))
	case 8:
		return byteorder.BEUint64(b)
	default:
		panic("syscall: readInt with unsupported size")
	}
}

func readIntLE(b []byte, size uintptr) uint64 {
	switch size {
	case 1:
		return uint64(b[0])
	case 2:
		return uint64(byteorder.LEUint16(b))
	case 4:
		return uint64(byteorder.LEUint32(b))
	case 8:
		return byteorder.LEUint64(b)
	default:
		panic("syscall: readInt with unsupported size")
	}
}

// ParseDirent 解析 buf 中最多 max 个目录条目，
// 将名称追加到 names。它返回从 buf 中消耗的字节数、
// 添加到 names 的条目数以及新的 names 切片。
func ParseDirent(buf []byte, max int, names []string) (consumed int, count int, newnames []string) {
	origlen := len(buf)
	count = 0
	for max != 0 && len(buf) > 0 {
		reclen, ok := direntReclen(buf)
		if !ok || reclen > uint64(len(buf)) {
			return origlen, count, names
		}
		rec := buf[:reclen]
		buf = buf[reclen:]
		ino, ok := direntIno(rec)
		if !ok {
			break
		}
		// 关于为什么在 wasip1 和 linux 上排除此条件，
		// 请参阅 src/os/dir_unix.go。
		if ino == 0 && runtime.GOOS != "linux" && runtime.GOOS != "wasip1" { // 文件不在目录中。
			continue
		}
		const namoff = uint64(unsafe.Offsetof(Dirent{}.Name))
		namlen, ok := direntNamlen(rec)
		if !ok || namoff+namlen > uint64(len(rec)) {
			break
		}
		name := rec[namoff : namoff+namlen]
		for i, c := range name {
			if c == 0 {
				name = name[:i]
				break
			}
		}
		// 在分配字符串之前检查无用的名称。
		if string(name) == "." || string(name) == ".." {
			continue
		}
		max--
		count++
		names = append(names, string(name))
	}
	return origlen - len(buf), count, names
}
