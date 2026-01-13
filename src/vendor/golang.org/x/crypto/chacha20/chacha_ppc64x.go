// 版权所有 2019 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

//go:build gc && !purego && (ppc64 || ppc64le)

package chacha20

const bufSize = 256

//go:noescape
func chaCha20_ctr32_vsx(out, inp *byte, len int, key *[8]uint32, counter *uint32)

func (c *Cipher) xorKeyStreamBlocks(dst, src []byte) {
	chaCha20_ctr32_vsx(&dst[0], &src[0], len(src), &c.key, &c.counter)
}
