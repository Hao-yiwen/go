// 版权所有 2015 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

//go:build cgo && !netgo && (aix || dragonfly || freebsd || netbsd || openbsd)

package net

/*
#include <sys/types.h>
#include <sys/socket.h>

#include <netinet/in.h>
*/
import "C"

import (
	"syscall"
	"unsafe"
)

func cgoSockaddrInet4(ip IP) *C.struct_sockaddr {
	sa := syscall.RawSockaddrInet4{Len: syscall.SizeofSockaddrInet4, Family: syscall.AF_INET}
	copy(sa.Addr[:], ip)
	return (*C.struct_sockaddr)(unsafe.Pointer(&sa))
}

func cgoSockaddrInet6(ip IP, zone int) *C.struct_sockaddr {
	sa := syscall.RawSockaddrInet6{Len: syscall.SizeofSockaddrInet6, Family: syscall.AF_INET6, Scope_id: uint32(zone)}
	copy(sa.Addr[:], ip)
	return (*C.struct_sockaddr)(unsafe.Pointer(&sa))
}
