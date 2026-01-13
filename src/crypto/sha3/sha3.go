// 版权所有 2024 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// Package sha3 实现了 FIPS 202 中定义的 SHA-3 哈希算法和 SHAKE 可扩展
// 输出函数。
package sha3

import (
	"crypto"
	"crypto/internal/fips140/sha3"
	"hash"
	_ "unsafe"
)

func init() {
	crypto.RegisterHash(crypto.SHA3_224, func() hash.Hash { return New224() })
	crypto.RegisterHash(crypto.SHA3_256, func() hash.Hash { return New256() })
	crypto.RegisterHash(crypto.SHA3_384, func() hash.Hash { return New384() })
	crypto.RegisterHash(crypto.SHA3_512, func() hash.Hash { return New512() })
}

// Sum224 返回数据的 SHA3-224 哈希。
func Sum224(data []byte) [28]byte {
	var out [28]byte
	h := sha3.New224()
	h.Write(data)
	h.Sum(out[:0])
	return out
}

// Sum256 返回数据的 SHA3-256 哈希。
func Sum256(data []byte) [32]byte {
	var out [32]byte
	h := sha3.New256()
	h.Write(data)
	h.Sum(out[:0])
	return out
}

// Sum384 返回数据的 SHA3-384 哈希。
func Sum384(data []byte) [48]byte {
	var out [48]byte
	h := sha3.New384()
	h.Write(data)
	h.Sum(out[:0])
	return out
}

// Sum512 返回数据的 SHA3-512 哈希。
func Sum512(data []byte) [64]byte {
	var out [64]byte
	h := sha3.New512()
	h.Write(data)
	h.Sum(out[:0])
	return out
}

// SumSHAKE128 对数据应用 SHAKE128 可扩展输出函数，并
// 返回给定字节长度的输出。
func SumSHAKE128(data []byte, length int) []byte {
	// 为最多 256 位的输出分配到调用者的堆栈。
	out := make([]byte, 32)
	return sumSHAKE128(out, data, length)
}

func sumSHAKE128(out, data []byte, length int) []byte {
	if len(out) < length {
		out = make([]byte, length)
	} else {
		out = out[:length]
	}
	h := sha3.NewShake128()
	h.Write(data)
	h.Read(out)
	return out
}

// SumSHAKE256 对数据应用 SHAKE256 可扩展输出函数，并
// 返回给定字节长度的输出。
func SumSHAKE256(data []byte, length int) []byte {
	// 为最多 512 位的输出分配到调用者的堆栈。
	out := make([]byte, 64)
	return sumSHAKE256(out, data, length)
}

func sumSHAKE256(out, data []byte, length int) []byte {
	if len(out) < length {
		out = make([]byte, length)
	} else {
		out = out[:length]
	}
	h := sha3.NewShake256()
	h.Write(data)
	h.Read(out)
	return out
}

// SHA3 是 SHA-3 哈希的实例。它实现了 [hash.Hash]。
// 零值是可用的 SHA3-256 哈希。
type SHA3 struct {
	s sha3.Digest
}

//go:linkname fips140hash_sha3Unwrap crypto/internal/fips140hash.sha3Unwrap
func fips140hash_sha3Unwrap(sha3 *SHA3) *sha3.Digest {
	return &sha3.s
}

// New224 创建一个新的 SHA3-224 哈希。
func New224() *SHA3 {
	return &SHA3{*sha3.New224()}
}

// New256 创建一个新的 SHA3-256 哈希。
func New256() *SHA3 {
	return &SHA3{*sha3.New256()}
}

// New384 创建一个新的 SHA3-384 哈希。
func New384() *SHA3 {
	return &SHA3{*sha3.New384()}
}

// New512 创建一个新的 SHA3-512 哈希。
func New512() *SHA3 {
	return &SHA3{*sha3.New512()}
}

func (s *SHA3) init() {
	if s.s.Size() == 0 {
		*s = *New256()
	}
}

// Write 将更多数据吸收到哈希的状态中。
func (s *SHA3) Write(p []byte) (n int, err error) {
	s.init()
	return s.s.Write(p)
}

// Sum 将当前哈希追加到 b 并返回结果切片。
func (s *SHA3) Sum(b []byte) []byte {
	s.init()
	return s.s.Sum(b)
}

// Reset 将哈希重置为其初始状态。
func (s *SHA3) Reset() {
	s.init()
	s.s.Reset()
}

// Size 返回 Sum 将产生的字节数。
func (s *SHA3) Size() int {
	s.init()
	return s.s.Size()
}

// BlockSize 返回哈希的速率。
func (s *SHA3) BlockSize() int {
	s.init()
	return s.s.BlockSize()
}

// MarshalBinary 实现了 [encoding.BinaryMarshaler]。
func (s *SHA3) MarshalBinary() ([]byte, error) {
	s.init()
	return s.s.MarshalBinary()
}

// AppendBinary 实现了 [encoding.BinaryAppender]。
func (s *SHA3) AppendBinary(p []byte) ([]byte, error) {
	s.init()
	return s.s.AppendBinary(p)
}

// UnmarshalBinary 实现了 [encoding.BinaryUnmarshaler]。
func (s *SHA3) UnmarshalBinary(data []byte) error {
	s.init()
	return s.s.UnmarshalBinary(data)
}

// Clone 实现了 [hash.Cloner]。
func (d *SHA3) Clone() (hash.Cloner, error) {
	r := *d
	return &r, nil
}

// SHAKE 是 SHAKE 可扩展输出函数的实例。
// 零值是可用的 SHAKE256 哈希。
type SHAKE struct {
	s sha3.SHAKE
}

func (s *SHAKE) init() {
	if s.s.Size() == 0 {
		*s = *NewSHAKE256()
	}
}

// NewSHAKE128 创建一个新的 SHAKE128 XOF。
func NewSHAKE128() *SHAKE {
	return &SHAKE{*sha3.NewShake128()}
}

// NewSHAKE256 创建一个新的 SHAKE256 XOF。
func NewSHAKE256() *SHAKE {
	return &SHAKE{*sha3.NewShake256()}
}

// NewCSHAKE128 创建一个新的 cSHAKE128 XOF。
//
// N 用于定义基于 cSHAKE 的函数，当需要纯 cSHAKE 时可以为空。
// S 是用于域分离的自定义字节字符串。当 N 和 S 都为空时，
// 这等价于 NewSHAKE128。
func NewCSHAKE128(N, S []byte) *SHAKE {
	return &SHAKE{*sha3.NewCShake128(N, S)}
}

// NewCSHAKE256 创建一个新的 cSHAKE256 XOF。
//
// N 用于定义基于 cSHAKE 的函数，当需要纯 cSHAKE 时可以为空。
// S 是用于域分离的自定义字节字符串。当 N 和 S 都为空时，
// 这等价于 NewSHAKE256。
func NewCSHAKE256(N, S []byte) *SHAKE {
	return &SHAKE{*sha3.NewCShake256(N, S)}
}

// Write 将更多数据吸收到 XOF 的状态中。
//
// 如果已读取任何输出，则会 panic。
func (s *SHAKE) Write(p []byte) (n int, err error) {
	s.init()
	return s.s.Write(p)
}

// Read 从 XOF 中挤出更多输出。
//
// 在 Read 调用后的任何 Write 调用都会 panic。
func (s *SHAKE) Read(p []byte) (n int, err error) {
	s.init()
	return s.s.Read(p)
}

// Reset 将 XOF 重置为其初始状态。
func (s *SHAKE) Reset() {
	s.init()
	s.s.Reset()
}

// BlockSize 返回 XOF 的速率。
func (s *SHAKE) BlockSize() int {
	s.init()
	return s.s.BlockSize()
}

// MarshalBinary 实现了 [encoding.BinaryMarshaler]。
func (s *SHAKE) MarshalBinary() ([]byte, error) {
	s.init()
	return s.s.MarshalBinary()
}

// AppendBinary 实现了 [encoding.BinaryAppender]。
func (s *SHAKE) AppendBinary(p []byte) ([]byte, error) {
	s.init()
	return s.s.AppendBinary(p)
}

// UnmarshalBinary 实现了 [encoding.BinaryUnmarshaler]。
func (s *SHAKE) UnmarshalBinary(data []byte) error {
	s.init()
	return s.s.UnmarshalBinary(data)
}
