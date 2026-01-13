// 版权所有 2020 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package maphash_test

import (
	"fmt"
	"hash/maphash"
)

func Example() {
	// 零值 Hash 是有效的且可以直接使用；
	// 不需要设置初始种子。
	var h maphash.Hash

	// 向哈希添加字符串，并打印当前哈希值。
	h.WriteString("hello, ")
	fmt.Printf("%#x\n", h.Sum64())

	// 追加额外数据（以字节数组形式）。
	h.Write([]byte{'w', 'o', 'r', 'l', 'd'})
	fmt.Printf("%#x\n", h.Sum64())

	// Reset 丢弃之前添加到 Hash 的所有数据，
	// 但不改变其种子。
	h.Reset()

	// 使用 SetSeed 创建一个新的 Hash h2，
	// 它的行为与 h 相同。
	var h2 maphash.Hash
	h2.SetSeed(h.Seed())

	h.WriteString("same")
	h2.WriteString("same")
	fmt.Printf("%#x == %#x\n", h.Sum64(), h2.Sum64())
}
