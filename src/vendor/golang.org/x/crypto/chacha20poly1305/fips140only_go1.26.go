// 版权所有 2025 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

//go:build go1.26

package chacha20poly1305

import "crypto/fips140"

func fips140Enforced() bool { return fips140.Enforced() }
