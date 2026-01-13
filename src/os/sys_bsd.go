// 版权所有 2009 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

//go:build darwin || dragonfly || freebsd || (js && wasm) || netbsd || openbsd || wasip1

package os

import "syscall"

func hostname() (name string, err error) {
	name, err = syscall.Sysctl("kern.hostname")
	if err != nil {
		return "", NewSyscallError("sysctl kern.hostname", err)
	}
	return name, nil
}
