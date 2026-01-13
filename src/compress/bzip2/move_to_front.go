// 版权所有 2011 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package bzip2

// moveToFrontDecoder 实现了一个移动到前端列表。这种列表是将包含重复元素的
// 字符串转换为包含许多小值数字的字符串的有效方法，适合熵编码。它的工作原理是
// 从一个初始符号列表开始，通过符号在列表中的索引来引用符号。当一个符号被引用时，
// 它会被移动到列表的前面。因此，重复的符号最终会被编码为许多零，
// 因为在第一次访问后，该符号将位于列表的前面。
type moveToFrontDecoder []byte

// newMTFDecoder 创建一个带有显式初始符号列表的移动到前端解码器。
func newMTFDecoder(symbols []byte) moveToFrontDecoder {
	if len(symbols) > 256 {
		panic("too many symbols")
	}
	return moveToFrontDecoder(symbols)
}

// newMTFDecoderWithRange 创建一个初始符号列表为 0...n-1 的移动到前端解码器。
func newMTFDecoderWithRange(n int) moveToFrontDecoder {
	if n > 256 {
		panic("newMTFDecoderWithRange: cannot have > 256 symbols")
	}

	m := make([]byte, n)
	for i := 0; i < n; i++ {
		m[i] = byte(i)
	}
	return moveToFrontDecoder(m)
}

func (m moveToFrontDecoder) Decode(n int) (b byte) {
	// 使用简单的复制实现移动到前端。这种方法在基准测试中优于更复杂的方法，
	// 可能是因为它在单个缓存行内具有高引用局部性
	//（大多数移动到前端操作的 n < 64）。
	b = m[n]
	copy(m[1:], m[:n])
	m[0] = b
	return
}

// First 返回列表前端的符号。
func (m moveToFrontDecoder) First() byte {
	return m[0]
}
