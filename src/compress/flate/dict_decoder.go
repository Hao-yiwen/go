// 版权所有 2016 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package flate

// dictDecoder 实现了 LZ77 滑动字典，用于解压缩。
// LZ77 通过两种形式的命令序列对数据进行解压缩：
//
//   - 字面量插入：一个或多个符号的运行按原样插入到数据流中。
//     对于单个符号，通过 writeByte 方法完成；对于多个符号，
//     通过 writeSlice/writeMark 的组合完成。
//     任何有效的流必须以字面量插入开始（如果未使用预设字典）。
//
//   - 向后复制：一个或多个符号的运行从先前发出的数据中复制。
//     向后复制为元组 (dist, length)，其中 dist 确定从流中向后复制多远，
//     length 确定要复制多少字节。注意，长度可以大于距离。
//     由于 LZ77 使用向前复制，这种情况用于对重复的符号运行执行游程长度编码。
//     writeCopy 和 tryWriteCopy 用于实现此命令。
//
// 出于性能原因，此实现对参数几乎不执行健全性检查。
// 因此，为每个方法调用记录的不变量必须得到遵守。
type dictDecoder struct {
	hist []byte // 滑动窗口历史记录

	// 不变量：0 <= rdPos <= wrPos <= len(hist)
	wrPos int  // 缓冲区中的当前输出位置
	rdPos int  // 已发出 hist[:rdPos]
	full  bool // 是否已写入完整的窗口长度？
}

// init 初始化 dictDecoder 以具有给定大小的滑动窗口字典。
// 如果提供了预设字典，它将使用 dict 的内容初始化字典。
func (dd *dictDecoder) init(size int, dict []byte) {
	*dd = dictDecoder{hist: dd.hist}

	if cap(dd.hist) < size {
		dd.hist = make([]byte, size)
	}
	dd.hist = dd.hist[:size]

	if len(dict) > len(dd.hist) {
		dict = dict[len(dict)-len(dd.hist):]
	}
	dd.wrPos = copy(dd.hist, dict)
	if dd.wrPos == len(dd.hist) {
		dd.wrPos = 0
		dd.full = true
	}
	dd.rdPos = dd.wrPos
}

// histSize 报告字典中的总历史数据量。
func (dd *dictDecoder) histSize() int {
	if dd.full {
		return len(dd.hist)
	}
	return dd.wrPos
}

// availRead 报告可以通过 readFlush 刷新的字节数。
func (dd *dictDecoder) availRead() int {
	return dd.wrPos - dd.rdPos
}

// availWrite 报告可用的输出缓冲区空间量。
func (dd *dictDecoder) availWrite() int {
	return len(dd.hist) - dd.wrPos
}

// writeSlice 返回可用缓冲区的切片以写入数据。
//
// 将保持此不变量：len(s) <= availWrite()
func (dd *dictDecoder) writeSlice() []byte {
	return dd.hist[dd.wrPos:]
}

// writeMark 将写入器指针向前推进 cnt。
//
// 必须保持此不变量：0 <= cnt <= availWrite()
func (dd *dictDecoder) writeMark(cnt int) {
	dd.wrPos += cnt
}

// writeByte 将单个字节写入字典。
//
// 必须保持此不变量：0 < availWrite()
func (dd *dictDecoder) writeByte(c byte) {
	dd.hist[dd.wrPos] = c
	dd.wrPos++
}

// writeCopy 将给定 (dist, length) 处的字符串复制到输出。
// 如果输出缓冲区中的可用空间太小，返回复制的字节数，
// 可能少于请求的长度。
//
// 必须保持此不变量：0 < dist <= histSize()
func (dd *dictDecoder) writeCopy(dist, length int) int {
	dstBase := dd.wrPos
	dstPos := dstBase
	srcPos := dstPos - dist
	endPos := dstPos + length
	if endPos > len(dd.hist) {
		endPos = len(dd.hist)
	}

	// 复制目标位置之后的非重叠部分。
	//
	// 此部分是非重叠的，因为此部分的复制长度总是小于或等于向后距离。
	// 这可能发生在距离指代缓冲区中换行的数据时。
	// 因此，这里执行向后复制；即源中复制前的确切字节放在目标中。
	if srcPos < 0 {
		srcPos += len(dd.hist)
		dstPos += copy(dd.hist[dstPos:endPos], dd.hist[srcPos:])
		srcPos = 0
	}

	// 复制目标位置之前可能重叠的部分。
	//
	// 如果此部分的复制长度大于向后距离，此部分可能重叠。
	// LZ77 允许这样做，以便重复的字符串可以使用 (dist, length) 对简洁地表示。
	// 因此，这里执行向前复制；即复制的字节可能取决于
	// 当复制进行时目标中产生的字节。这在功能上等同于以下内容：
	//
	//	for i := 0; i < endPos-dstPos; i++ {
	//		dd.hist[dstPos+i] = dd.hist[srcPos+i]
	//	}
	//	dstPos = endPos
	//
	for dstPos < endPos {
		dstPos += copy(dd.hist[dstPos:endPos], dd.hist[srcPos:dstPos])
	}

	dd.wrPos = dstPos
	return dstPos - dstBase
}

// tryWriteCopy 尝试将给定 (distance, length) 处的字符串复制到输出。
// 此专用版本针对短距离进行了优化。
//
// 此方法旨在为性能原因而内联。
//
// 必须保持此不变量：0 < dist <= histSize()
func (dd *dictDecoder) tryWriteCopy(dist, length int) int {
	dstPos := dd.wrPos
	endPos := dstPos + length
	if dstPos < dist || endPos > len(dd.hist) {
		return 0
	}
	dstBase := dstPos
	srcPos := dstPos - dist

	// 复制目标位置之前可能重叠的部分。
	for dstPos < endPos {
		dstPos += copy(dd.hist[dstPos:endPos], dd.hist[srcPos:dstPos])
	}

	dd.wrPos = dstPos
	return dstPos - dstBase
}

// readFlush 返回准备好发出给用户的历史缓冲区的切片。
// readFlush 返回的数据必须在调用任何其他 dictDecoder 方法之前完全消耗。
func (dd *dictDecoder) readFlush() []byte {
	toRead := dd.hist[dd.rdPos:dd.wrPos]
	dd.rdPos = dd.wrPos
	if dd.wrPos == len(dd.hist) {
		dd.wrPos, dd.rdPos = 0, 0
		dd.full = true
	}
	return toRead
}
