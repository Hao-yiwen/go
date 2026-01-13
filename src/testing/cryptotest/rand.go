// 版权所有 2025 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// Package cryptotest 提供确定性随机源测试。
package cryptotest

import (
	cryptorand "crypto/rand"
	"internal/byteorder"
	"io"
	mathrand "math/rand/v2"
	"sync"
	"testing"

	// 导入 unsafe 和 crypto/rand，后者导入 crypto/internal/rand，
	// 用于 crypto/internal/rand.SetTestingReader go:linkname。
	_ "crypto/rand"
	_ "unsafe"
)

//go:linkname randSetTestingReader crypto/internal/rand.SetTestingReader
func randSetTestingReader(r io.Reader)

//go:linkname testingCheckParallel testing.checkParallel
func testingCheckParallel(t *testing.T)

// SetGlobalRandom 为测试 t 的持续时间设置全局确定性密码随机源。
// 它影响 crypto/rand 和 crypto/... 包中的所有隐式密码随机源。
//
// SetGlobalRandom 可以在同一个测试中多次调用以重置
// 随机流或更改种子。
//
// 因为 SetGlobalRandom 影响整个进程，它不能在
// 并行测试或具有并行祖先的测试中使用。
//
// 请注意，密码算法使用随机性的方式通常未指定，
// 可能会随时间变化。因此，如果测试期望从
// 密码函数获得特定输出，即使它使用
// SetGlobalRandom，也可能在未来失败。
//
// 针对 Go Cryptographic Module v1.0.0 构建时不支持 SetGlobalRandom
// (即当 [crypto/fips140.Version] 返回 "v1.0.0" 时)。
func SetGlobalRandom(t *testing.T, seed uint64) {
	if t == nil {
		panic("cryptotest: SetGlobalRandom called with a nil *testing.T")
	}
	if !testing.Testing() {
		panic("cryptotest: SetGlobalRandom used in a non-test binary")
	}
	testingCheckParallel(t)

	var s [32]byte
	byteorder.LEPutUint64(s[:8], seed)
	r := &lockedReader{r: mathrand.NewChaCha8(s)}

	randSetTestingReader(r)
	previous := cryptorand.Reader
	cryptorand.Reader = r
	t.Cleanup(func() {
		cryptorand.Reader = previous
		randSetTestingReader(nil)
	})
}

type lockedReader struct {
	sync.Mutex
	r *mathrand.ChaCha8
}

func (lr *lockedReader) Read(b []byte) (n int, err error) {
	lr.Lock()
	defer lr.Unlock()
	return lr.r.Read(b)
}
