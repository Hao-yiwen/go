// 版权所有 2011 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// 此文件包含在 Unix 系统上调用的 cgo 函数的存根版本。
// 我们在以下情况下构建此文件：
// - 在 Unix 系统上使用 netgo 构建标签时
// - 在没有 cgo 解析器函数的 Unix 系统上
//   （Darwin 始终在 cgo_unix_syscall.go 中提供 cgo 函数）
// - 在 wasip1 上，cgo 永远不可用

//go:build (netgo && unix) || (unix && !cgo && !darwin) || js || wasip1

package net

import "context"

// cgoAvailable 设置为 false 表示此系统上
// cgo 解析器不可用。
const cgoAvailable = false

func cgoLookupHost(ctx context.Context, name string) (addrs []string, err error) {
	panic("cgo stub: cgo not available")
}

func cgoLookupPort(ctx context.Context, network, service string) (port int, err error) {
	panic("cgo stub: cgo not available")
}

func cgoLookupIP(ctx context.Context, network, name string) (addrs []IPAddr, err error) {
	panic("cgo stub: cgo not available")
}

func cgoLookupCNAME(ctx context.Context, name string) (cname string, err error) {
	panic("cgo stub: cgo not available")
}

func cgoLookupPTR(ctx context.Context, addr string) (ptrs []string, err error) {
	panic("cgo stub: cgo not available")
}
