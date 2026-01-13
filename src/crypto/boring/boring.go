// 版权所有 2020 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

//go:build boringcrypto

// boring 包公开仅在使用 Go+BoringCrypto 构建时可用的函数。
// 只要使用 Go+BoringCrypto 工具链，此包在所有目标上都可用。
// 使用 Enabled 函数来确定 BoringCrypto 核心是否实际在使用。
//
// 任何时候使用 Go+BoringCrypto 工具链时，"boringcrypto" 构建标记
// 都会被满足，这样应用程序就可以标记使用此包的文件。
package boring

import "crypto/internal/boring"

// Enabled 报告 BoringCrypto 是否处理受支持的加密操作。
func Enabled() bool {
	return boring.Enabled
}
