// 版权所有 2025 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// scannerhooks 包定义 nonexported channels between parser and scanner.
// Ideally this package could be eliminated by adding API to scanner.
package scannerhooks

import "go/token"

var StringEnd func(scanner any) token.Pos
