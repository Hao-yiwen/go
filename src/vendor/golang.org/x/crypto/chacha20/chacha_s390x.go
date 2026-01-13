// 版权所有 2018 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

//go:build gc && !purego

package chacha20

import "golang.org/x/sys/cpu"

var haveAsm = cpu.S390X.HasVX

const bufSize = 256

// xorKeyStreamVX 是 XORKeyStream 的汇编实现。只有在向量设施可用时才能调用。
// 实现在 asm_s390x.s 中。
//
//go:noescape
func xorKeyStreamVX(dst, src []byte, key *[8]uint32, nonce *[3]uint32, counter *uint32)

func (c *Cipher) xorKeyStreamBlocks(dst, src []byte) {
	if cpu.S390X.HasVX {
		xorKeyStreamVX(dst, src, &c.key, &c.nonce, &c.counter)
	} else {
		c.xorKeyStreamBlocksGeneric(dst, src)
	}
}
