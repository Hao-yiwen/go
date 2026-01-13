// 版权所有 2010 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// Package rand 实现了密码学安全的
// 随机数生成器。
package rand

import (
	"crypto/internal/boring"
	"crypto/internal/fips140/drbg"
	"crypto/internal/rand"
	"io"
	_ "unsafe"

	// 确保从 testing/cryptotest 到
	// crypto/internal/rand.SetTestingReader 的 go:linkname 有效。
	_ "crypto/internal/rand"
)

// Reader 是密码学安全的随机数生成器的全局共享实例。它对并发使用是安全的。
//
//   - 在 Linux、FreeBSD、Dragonfly 和 Solaris 上，Reader 使用 getrandom(2)。
//   - 在传统 Linux (< 3.17) 上，Reader 首次使用时打开 /dev/urandom。
//   - 在 macOS、iOS 和 OpenBSD 上，Reader 使用 arc4random_buf(3)。
//   - 在 NetBSD 上，Reader 使用 kern.arandom sysctl。
//   - 在 Windows 上，Reader 使用 ProcessPrng API。
//   - 在 js/wasm 上，Reader 使用 Web Crypto API。
//   - 在 wasip1/wasm 上，Reader 使用 random_get。
//
// 在 FIPS 140-3 模式中，输出通过 SP 800-90A Rev. 1
// 确定性随机位生成器（DRBG）。
var Reader io.Reader = rand.Reader

// fatal 是 [runtime.fatal]，通过 linkname 推送。
//
//go:linkname fatal
func fatal(string)

// Read 用密码学安全的随机字节填充 b。它永远不会返回
// 错误，并且总是完全填充 b。
//
// Read 在 [Reader] 上调用 [io.ReadFull]，如果返回
// 错误，则不可逆转地崩溃程序。默认 Reader 使用的操作系统 API 被
// 记录为在除传统 Linux 系统外从不返回错误。
func Read(b []byte) (n int, err error) {
	// 我们不希望 b 逃逸到堆，但逃逸分析无法看到
	// 通过潜在的重写 Reader，所以我们特别处理默认
	// 情况，这样我们可以保持不逃逸，而在一般情况下，我们读入
	// 堆缓冲区并从中复制。
	if rand.IsDefaultReader(Reader) {
		if boring.Enabled {
			_, err = io.ReadFull(boring.RandReader, b)
		} else {
			drbg.Read(b)
		}
	} else {
		bb := make([]byte, len(b))
		_, err = io.ReadFull(Reader, bb)
		copy(b, bb)
	}
	if err != nil {
		fatal("crypto/rand: failed to read random data (see https://go.dev/issue/66821): " + err.Error())
		panic("unreachable") // To be sure.
	}
	return len(b), nil
}
