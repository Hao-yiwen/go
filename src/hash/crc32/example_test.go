// 版权所有 2015 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package crc32_test

import (
	"fmt"
	"hash/crc32"
)

func ExampleMakeTable() {
	// 在此包中，CRC 多项式以反转表示法表示，
	// 即 LSB 优先表示法。
	//
	// LSB 优先表示法是一个 n 位的十六进制数，其中
	// 最高有效位表示 x⁰ 的系数，最低有效位
	// 表示 xⁿ⁻¹ 的系数（xⁿ 的系数是隐含的）。
	//
	// 例如，CRC32-Q 由以下多项式定义：
	//	x³²+ x³¹+ x²⁴+ x²²+ x¹⁶+ x¹⁴+ x⁸+ x⁷+ x⁵+ x³+ x¹+ x⁰
	// 其反转表示法为 0b11010101100000101000001010000001，因此
	// 应该传递给 MakeTable 的值是 0xD5828281。
	crc32q := crc32.MakeTable(0xD5828281)
	fmt.Printf("%08x\n", crc32.Checksum([]byte("Hello world"), crc32q))
	// Output:
	// 2964d064
}
