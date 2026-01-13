// 版权所有 2011 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

//go:build !android && cgo && !netgo

package net

/*
#include <netdb.h>
*/
import "C"

// 注意(rsc): 理论上，在标志中包含或不包含 AI_ADDRCONFIG
// 的论据大致平衡（它在 IPv4 系统上只包含 IPv4 结果，
// IPv6 同理），但实际上设置它会导致 getaddrinfo 在 Linux 上
// 返回错误的规范名称。所以绝对要排除它。
const cgoAddrInfoFlags = C.AI_CANONNAME | C.AI_V4MAPPED | C.AI_ALL
