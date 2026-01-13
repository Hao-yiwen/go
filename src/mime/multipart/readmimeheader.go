// 版权所有 2023 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package multipart

import (
	"net/textproto"
	_ "unsafe" // 用于 go:linkname
)

// readMIMEHeader 在包 [net/textproto] 中定义。
//
//go:linkname readMIMEHeader net/textproto.readMIMEHeader
func readMIMEHeader(r *textproto.Reader, maxMemory, maxHeaders int64) (textproto.MIMEHeader, error)
