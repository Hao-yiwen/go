// 版权所有 2024 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

//go:build plan9

package os

import (
	"internal/filepathlite"
)

func checkPathEscapes(r *Root, name string) error {
	if r.root.closed.Load() {
		return ErrClosed
	}
	if !filepathlite.IsLocal(name) {
		return errPathEscapes
	}
	return nil
}

func checkPathEscapesLstat(r *Root, name string) error {
	return checkPathEscapes(r, name)
}
