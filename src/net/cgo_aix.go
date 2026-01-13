// 版权所有 2019 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

//go:build cgo && !netgo

package net

/*
#include <sys/types.h>
#include <sys/socket.h>

#include <netdb.h>
*/
import "C"

import "unsafe"

const cgoAddrInfoFlags = C.AI_CANONNAME

func cgoNameinfoPTR(b []byte, sa *C.struct_sockaddr, salen C.socklen_t) (int, error) {
	gerrno, err := C.getnameinfo(sa, C.size_t(salen), (*C.char)(unsafe.Pointer(&b[0])), C.size_t(len(b)), nil, 0, C.NI_NAMEREQD)
	return int(gerrno), err
}
