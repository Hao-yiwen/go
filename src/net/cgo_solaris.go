// 版权所有 2015 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

//go:build cgo && !netgo

package net

/*
#cgo LDFLAGS: -lsocket -lnsl
#include <netdb.h>
*/
import "C"

const cgoAddrInfoFlags = C.AI_CANONNAME | C.AI_V4MAPPED | C.AI_ALL
