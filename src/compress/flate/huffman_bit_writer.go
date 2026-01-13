// 版权所有 2009 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package flate

import (
	"io"
)

const (
	// 最大的偏移码。
	offsetCodeCount = 30

	// 用于标记块结尾的特殊码。
	endBlockMarker = 256

	// 第一个长度码。
	lengthCodesStart = 257

	// 码生成码的数量。
	codegenCodeCount = 19
	badCode          = 255

	// bufferFlushSize 表示缓冲区大小，
	// 超过此大小字节将刷新到写入器。
	// 最好是 6 的倍数，因为我们在对缓冲区的每次写入之间累积 6 个字节。
	bufferFlushSize = 240

	// bufferSize 是实际的输出字节缓冲区大小。
	// 必须有额外的空间来进行刷新，
	// 刷新可能包含最多 8 个字节。
	bufferSize = bufferFlushSize + 8
)

// 长度码 X - LENGTH_CODES_START 所需的额外位数。
var lengthExtraBits = []int8{
	/* 257 */ 0, 0, 0,
	/* 260 */ 0, 0, 0, 0, 0, 1, 1, 1, 1, 2,
	/* 270 */ 2, 2, 2, 3, 3, 3, 3, 4, 4, 4,
	/* 280 */ 4, 5, 5, 5, 5, 0,
}

// 长度码 X - LENGTH_CODES_START 表示的长度。
var lengthBase = []uint32{
	0, 1, 2, 3, 4, 5, 6, 7, 8, 10,
	12, 14, 16, 20, 24, 28, 32, 40, 48, 56,
	64, 80, 96, 112, 128, 160, 192, 224, 255,
}

// 偏移码词的额外位。
var offsetExtraBits = []int8{
	0, 0, 0, 0, 1, 1, 2, 2, 3, 3,
	4, 4, 5, 5, 6, 6, 7, 7, 8, 8,
	9, 9, 10, 10, 11, 11, 12, 12, 13, 13,
}

var offsetBase = []uint32{
	0x000000, 0x000001, 0x000002, 0x000003, 0x000004,
	0x000006, 0x000008, 0x00000c, 0x000010, 0x000018,
	0x000020, 0x000030, 0x000040, 0x000060, 0x000080,
	0x0000c0, 0x000100, 0x000180, 0x000200, 0x000300,
	0x000400, 0x000600, 0x000800, 0x000c00, 0x001000,
	0x001800, 0x002000, 0x003000, 0x004000, 0x006000,
}

// 写入码生成码大小的奇特顺序。
var codegenOrder = []uint32{16, 17, 18, 0, 8, 7, 9, 6, 10, 5, 11, 4, 12, 3, 13, 2, 14, 1, 15}

type huffmanBitWriter struct {
	// writer 是底层写入器。
	// 不要直接使用它；使用 write 方法，它确保
	// Write 错误是粘性的。
	writer io.Writer

	// 等待写入的数据是 bytes[0:nbytes]
	// 然后是 bits 的最低 nbits。数据总是顺序地写入
	// 到字节数组。
	bits            uint64
	nbits           uint
	bytes           [bufferSize]byte
	codegenFreq     [codegenCodeCount]int32
	nbytes          int
	literalFreq     []int32
	offsetFreq      []int32
	codegen         []uint8
	literalEncoding *huffmanEncoder
	offsetEncoding  *huffmanEncoder
	codegenEncoding *huffmanEncoder
	err             error
}

func newHuffmanBitWriter(w io.Writer) *huffmanBitWriter {
	return &huffmanBitWriter{
		writer:          w,
		literalFreq:     make([]int32, maxNumLit),
		offsetFreq:      make([]int32, offsetCodeCount),
		codegen:         make([]uint8, maxNumLit+offsetCodeCount+1),
		literalEncoding: newHuffmanEncoder(maxNumLit),
		codegenEncoding: newHuffmanEncoder(codegenCodeCount),
		offsetEncoding:  newHuffmanEncoder(offsetCodeCount),
	}
}

func (w *huffmanBitWriter) reset(writer io.Writer) {
	w.writer = writer
	w.bits, w.nbits, w.nbytes, w.err = 0, 0, 0, nil
}

func (w *huffmanBitWriter) flush() {
	if w.err != nil {
		w.nbits = 0
		return
	}
	n := w.nbytes
	for w.nbits != 0 {
		w.bytes[n] = byte(w.bits)
		w.bits >>= 8
		if w.nbits > 8 { // 避免下溢
			w.nbits -= 8
		} else {
			w.nbits = 0
		}
		n++
	}
	w.bits = 0
	w.write(w.bytes[:n])
	w.nbytes = 0
}

func (w *huffmanBitWriter) write(b []byte) {
	if w.err != nil {
		return
	}
	_, w.err = w.writer.Write(b)
}

func (w *huffmanBitWriter) writeBits(b int32, nb uint) {
	if w.err != nil {
		return
	}
	w.bits |= uint64(b) << w.nbits
	w.nbits += nb
	if w.nbits >= 48 {
		bits := w.bits
		w.bits >>= 48
		w.nbits -= 48
		n := w.nbytes
		bytes := w.bytes[n : n+6]
		bytes[0] = byte(bits)
		bytes[1] = byte(bits >> 8)
		bytes[2] = byte(bits >> 16)
		bytes[3] = byte(bits >> 24)
		bytes[4] = byte(bits >> 32)
		bytes[5] = byte(bits >> 40)
		n += 6
		if n >= bufferFlushSize {
			w.write(w.bytes[:n])
			n = 0
		}
		w.nbytes = n
	}
}

func (w *huffmanBitWriter) writeBytes(bytes []byte) {
	if w.err != nil {
		return
	}
	n := w.nbytes
	if w.nbits&7 != 0 {
		w.err = InternalError("writeBytes with unfinished bits")
		return
	}
	for w.nbits != 0 {
		w.bytes[n] = byte(w.bits)
		w.bits >>= 8
		w.nbits -= 8
		n++
	}
	if n != 0 {
		w.write(w.bytes[:n])
	}
	w.nbytes = 0
	w.write(bytes)
}

// RFC 1951 3.2.7 指定了一种特殊的游程长度编码来指定
// 字面量和偏移长度数组（它们连接成单个数组）。
// 此方法生成该游程长度编码。
//
// 结果写入 codegen 数组，并且每个码的频率
// 写入 codegenFreq 数组。
// 码 0-15 是单字节码。码 16-18 后面跟着额外的信息。
// 码 badCode 是结束标记
//
//	numLiterals      literalEncoding 中的字面量数
//	numOffsets       offsetEncoding 中的偏移数
//	litenc, offenc   要使用的字面量和偏移编码器
func (w *huffmanBitWriter) generateCodegen(numLiterals int, numOffsets int, litEnc, offEnc *huffmanEncoder) {
	clear(w.codegenFreq[:])
	// 注意，我们将 codegen 既用作临时变量来保存频率的副本，
	// 也用作放置结果的地方。这很好，因为输出总是
	// 比迄今为止使用的输入更短。
	codegen := w.codegen // 缓存
	// 将连接的码大小复制到 codegen。在末尾放置标记。
	cgnl := codegen[:numLiterals]
	for i := range cgnl {
		cgnl[i] = uint8(litEnc.codes[i].len)
	}

	cgnl = codegen[numLiterals : numLiterals+numOffsets]
	for i := range cgnl {
		cgnl[i] = uint8(offEnc.codes[i].len)
	}
	codegen[numLiterals+numOffsets] = badCode

	size := codegen[0]
	count := 1
	outIndex := 0
	for inIndex := 1; size != badCode; inIndex++ {
		// 不变量：我们看到了"count"份"size"，
		// 但还没有为它们生成输出。
		nextSize := codegen[inIndex]
		if nextSize == size {
			count++
			continue
		}
		// 我们需要生成表示"count"个"size"的 codegen。
		if size != 0 {
			codegen[outIndex] = size
			outIndex++
			w.codegenFreq[size]++
			count--
			for count >= 3 {
				n := 6
				if n > count {
					n = count
				}
				codegen[outIndex] = 16
				outIndex++
				codegen[outIndex] = uint8(n - 3)
				outIndex++
				w.codegenFreq[16]++
				count -= n
			}
		} else {
			for count >= 11 {
				n := 138
				if n > count {
					n = count
				}
				codegen[outIndex] = 18
				outIndex++
				codegen[outIndex] = uint8(n - 11)
				outIndex++
				w.codegenFreq[18]++
				count -= n
			}
			if count >= 3 {
				// count >= 3 && count <= 10
				codegen[outIndex] = 17
				outIndex++
				codegen[outIndex] = uint8(count - 3)
				outIndex++
				w.codegenFreq[17]++
				count = 0
			}
		}
		count--
		for ; count >= 0; count-- {
			codegen[outIndex] = size
			outIndex++
			w.codegenFreq[size]++
		}
		// 为下一次循环设置不变量。
		size = nextSize
		count = 1
	}
	// 表示 codegen 结尾的标记。
	codegen[outIndex] = badCode
}

// dynamicSize 返回动态编码数据的大小（以位为单位）。
func (w *huffmanBitWriter) dynamicSize(litEnc, offEnc *huffmanEncoder, extraBits int) (size, numCodegens int) {
	numCodegens = len(w.codegenFreq)
	for numCodegens > 4 && w.codegenFreq[codegenOrder[numCodegens-1]] == 0 {
		numCodegens--
	}
	header := 3 + 5 + 5 + 4 + (3 * numCodegens) +
		w.codegenEncoding.bitLength(w.codegenFreq[:]) +
		int(w.codegenFreq[16])*2 +
		int(w.codegenFreq[17])*3 +
		int(w.codegenFreq[18])*7
	size = header +
		litEnc.bitLength(w.literalFreq) +
		offEnc.bitLength(w.offsetFreq) +
		extraBits

	return size, numCodegens
}

// fixedSize 返回动态编码数据的大小（以位为单位）。
func (w *huffmanBitWriter) fixedSize(extraBits int) int {
	return 3 +
		fixedLiteralEncoding.bitLength(w.literalFreq) +
		fixedOffsetEncoding.bitLength(w.offsetFreq) +
		extraBits
}

// storedSize 计算存储的大小，包括头部。
// 该函数返回大小（以位为单位）以及块是否适合单个块。
func (w *huffmanBitWriter) storedSize(in []byte) (int, bool) {
	if in == nil {
		return 0, false
	}
	if len(in) <= maxStoreBlockSize {
		return (len(in) + 5) * 8, true
	}
	return 0, false
}

func (w *huffmanBitWriter) writeCode(c hcode) {
	if w.err != nil {
		return
	}
	w.bits |= uint64(c.code) << w.nbits
	w.nbits += uint(c.len)
	if w.nbits >= 48 {
		bits := w.bits
		w.bits >>= 48
		w.nbits -= 48
		n := w.nbytes
		bytes := w.bytes[n : n+6]
		bytes[0] = byte(bits)
		bytes[1] = byte(bits >> 8)
		bytes[2] = byte(bits >> 16)
		bytes[3] = byte(bits >> 24)
		bytes[4] = byte(bits >> 32)
		bytes[5] = byte(bits >> 40)
		n += 6
		if n >= bufferFlushSize {
			w.write(w.bytes[:n])
			n = 0
		}
		w.nbytes = n
	}
}

// 将动态 Huffman 块的头写入输出流。
//
//	numLiterals  codegen 中指定的字面量数
//	numOffsets   codegen 中指定的偏移数
//	numCodegens  codegen 中使用的 codegens 数
func (w *huffmanBitWriter) writeDynamicHeader(numLiterals int, numOffsets int, numCodegens int, isEof bool) {
	if w.err != nil {
		return
	}
	var firstBits int32 = 4
	if isEof {
		firstBits = 5
	}
	w.writeBits(firstBits, 3)
	w.writeBits(int32(numLiterals-257), 5)
	w.writeBits(int32(numOffsets-1), 5)
	w.writeBits(int32(numCodegens-4), 4)

	for i := 0; i < numCodegens; i++ {
		value := uint(w.codegenEncoding.codes[codegenOrder[i]].len)
		w.writeBits(int32(value), 3)
	}

	i := 0
	for {
		var codeWord int = int(w.codegen[i])
		i++
		if codeWord == badCode {
			break
		}
		w.writeCode(w.codegenEncoding.codes[uint32(codeWord)])

		switch codeWord {
		case 16:
			w.writeBits(int32(w.codegen[i]), 2)
			i++
		case 17:
			w.writeBits(int32(w.codegen[i]), 3)
			i++
		case 18:
			w.writeBits(int32(w.codegen[i]), 7)
			i++
		}
	}
}

func (w *huffmanBitWriter) writeStoredHeader(length int, isEof bool) {
	if w.err != nil {
		return
	}
	var flag int32
	if isEof {
		flag = 1
	}
	w.writeBits(flag, 3)
	w.flush()
	w.writeBits(int32(length), 16)
	w.writeBits(int32(^uint16(length)), 16)
}

func (w *huffmanBitWriter) writeFixedHeader(isEof bool) {
	if w.err != nil {
		return
	}
	// Indicate that we are a fixed Huffman block
	var value int32 = 2
	if isEof {
		value = 3
	}
	w.writeBits(value, 3)
}

// writeBlock 将用最小编码写入一个token块。
// 可以提供原始输入，如果 Huffman 编码数据
// 大于原始字节，数据将作为存储块写入。
// 如果输入为nil，token将始终进行 Huffman 编码。
func (w *huffmanBitWriter) writeBlock(tokens []token, eof bool, input []byte) {
	if w.err != nil {
		return
	}

	tokens = append(tokens, endBlockMarker)
	numLiterals, numOffsets := w.indexTokens(tokens)

	var extraBits int
	storedSize, storable := w.storedSize(input)
	if storable {
		// 我们只在需要将这两个编码与存储编码进行比较时，
		// 才费力计算偏移字段长度所需的额外位的成本
		// （这对固定和动态编码都是相同的）。
		for lengthCode := lengthCodesStart + 8; lengthCode < numLiterals; lengthCode++ {
			// 前八个长度码的额外大小 = 0。
			extraBits += int(w.literalFreq[lengthCode]) * int(lengthExtraBits[lengthCode-lengthCodesStart])
		}
		for offsetCode := 4; offsetCode < numOffsets; offsetCode++ {
			// 前四个偏移码的额外大小 = 0。
			extraBits += int(w.offsetFreq[offsetCode]) * int(offsetExtraBits[offsetCode])
		}
	}

	// 计算最小码。
	// 固定 Huffman 基线。
	var literalEncoding = fixedLiteralEncoding
	var offsetEncoding = fixedOffsetEncoding
	var size = w.fixedSize(extraBits)

	// 动态 Huffman？
	var numCodegens int

	// 生成 codegen 和 codegenFrequencies，表示如何编码
	// literalEncoding 和 offsetEncoding。
	w.generateCodegen(numLiterals, numOffsets, w.literalEncoding, w.offsetEncoding)
	w.codegenEncoding.generate(w.codegenFreq[:], 7)
	dynamicSize, numCodegens := w.dynamicSize(w.literalEncoding, w.offsetEncoding, extraBits)

	if dynamicSize < size {
		size = dynamicSize
		literalEncoding = w.literalEncoding
		offsetEncoding = w.offsetEncoding
	}

	// 存储字节？
	if storable && storedSize < size {
		w.writeStoredHeader(len(input), eof)
		w.writeBytes(input)
		return
	}

	// Huffman。
	if literalEncoding == fixedLiteralEncoding {
		w.writeFixedHeader(eof)
	} else {
		w.writeDynamicHeader(numLiterals, numOffsets, numCodegens, eof)
	}

	// 写入token。
	w.writeTokens(tokens, literalEncoding.codes, offsetEncoding.codes)
}

// writeBlockDynamic 使用动态 Huffman 表编码一个块。
// 如果使用的符号具有不均衡的直方图分布，应使用此方法。
// 如果提供了输入，且压缩节省少于输入大小的 1/16，块将被存储。
func (w *huffmanBitWriter) writeBlockDynamic(tokens []token, eof bool, input []byte) {
	if w.err != nil {
		return
	}

	tokens = append(tokens, endBlockMarker)
	numLiterals, numOffsets := w.indexTokens(tokens)

	// 生成 codegen 和 codegenFrequencies，表示如何编码
	// literalEncoding 和 offsetEncoding。
	w.generateCodegen(numLiterals, numOffsets, w.literalEncoding, w.offsetEncoding)
	w.codegenEncoding.generate(w.codegenFreq[:], 7)
	size, numCodegens := w.dynamicSize(w.literalEncoding, w.offsetEncoding, 0)

	// 存储字节，如果我们没有获得合理的改进。
	if ssize, storable := w.storedSize(input); storable && ssize < (size+size>>4) {
		w.writeStoredHeader(len(input), eof)
		w.writeBytes(input)
		return
	}

	// 写入 Huffman 表。
	w.writeDynamicHeader(numLiterals, numOffsets, numCodegens, eof)

	// 写入token。
	w.writeTokens(tokens, w.literalEncoding.codes, w.offsetEncoding.codes)
}

// indexTokens 对token切片进行索引，并更新
// literalFreq 和 offsetFreq，并生成 literalEncoding
// 和 offsetEncoding。
// 返回字面量和偏移token的数量。
func (w *huffmanBitWriter) indexTokens(tokens []token) (numLiterals, numOffsets int) {
	clear(w.literalFreq)
	clear(w.offsetFreq)

	for _, t := range tokens {
		if t < matchType {
			w.literalFreq[t.literal()]++
			continue
		}
		length := t.length()
		offset := t.offset()
		w.literalFreq[lengthCodesStart+lengthCode(length)]++
		w.offsetFreq[offsetCode(offset)]++
	}

	// get the number of literals
	numLiterals = len(w.literalFreq)
	for w.literalFreq[numLiterals-1] == 0 {
		numLiterals--
	}
	// get the number of offsets
	numOffsets = len(w.offsetFreq)
	for numOffsets > 0 && w.offsetFreq[numOffsets-1] == 0 {
		numOffsets--
	}
	if numOffsets == 0 {
		// 我们没有找到单个匹配。如果我们想使用动态编码，
		// 我们应该至少计算一个偏移，以确保偏移 Huffman 树可以编码。
		w.offsetFreq[0] = 1
		numOffsets = 1
	}
	w.literalEncoding.generate(w.literalFreq, 15)
	w.offsetEncoding.generate(w.offsetFreq, 15)
	return
}

// writeTokens 将token切片写入输出。
// 必须提供字面量和偏移编码的码。
func (w *huffmanBitWriter) writeTokens(tokens []token, leCodes, oeCodes []hcode) {
	if w.err != nil {
		return
	}
	for _, t := range tokens {
		if t < matchType {
			w.writeCode(leCodes[t.literal()])
			continue
		}
		// 写入长度
		length := t.length()
		lengthCode := lengthCode(length)
		w.writeCode(leCodes[lengthCode+lengthCodesStart])
		extraLengthBits := uint(lengthExtraBits[lengthCode])
		if extraLengthBits > 0 {
			extraLength := int32(length - lengthBase[lengthCode])
			w.writeBits(extraLength, extraLengthBits)
		}
		// 写入偏移
		offset := t.offset()
		offsetCode := offsetCode(offset)
		w.writeCode(oeCodes[offsetCode])
		extraOffsetBits := uint(offsetExtraBits[offsetCode])
		if extraOffsetBits > 0 {
			extraOffset := int32(offset - offsetBase[offsetCode])
			w.writeBits(extraOffset, extraOffsetBits)
		}
	}
}

// huffOffset 是用于仅 Huffman 编码的静态偏移编码器。
// 可以重用，因为我们不会编码偏移值。
var huffOffset *huffmanEncoder

func init() {
	offsetFreq := make([]int32, offsetCodeCount)
	offsetFreq[0] = 1
	huffOffset = newHuffmanEncoder(offsetCodeCount)
	huffOffset.generate(offsetFreq, 15)
}

// writeBlockHuff 将字节块编码为 Huffman 编码的字面量
// 或如果压缩收益很小，则编码为未压缩字节。
func (w *huffmanBitWriter) writeBlockHuff(eof bool, input []byte) {
	if w.err != nil {
		return
	}

	// 清除直方图
	clear(w.literalFreq)

	// 将所有内容添加为字面量
	histogram(input, w.literalFreq)

	w.literalFreq[endBlockMarker] = 1

	const numLiterals = endBlockMarker + 1
	w.offsetFreq[0] = 1
	const numOffsets = 1

	w.literalEncoding.generate(w.literalFreq, 15)

	// 计算最小码。
	// 始终使用动态 Huffman 或存储
	var numCodegens int

	// 生成 codegen 和 codegenFrequencies，表示如何编码
	// literalEncoding 和 offsetEncoding。
	w.generateCodegen(numLiterals, numOffsets, w.literalEncoding, huffOffset)
	w.codegenEncoding.generate(w.codegenFreq[:], 7)
	size, numCodegens := w.dynamicSize(w.literalEncoding, huffOffset, 0)

	// 存储字节，如果我们没有获得合理的改进。
	if ssize, storable := w.storedSize(input); storable && ssize < (size+size>>4) {
		w.writeStoredHeader(len(input), eof)
		w.writeBytes(input)
		return
	}

	// Huffman。
	w.writeDynamicHeader(numLiterals, numOffsets, numCodegens, eof)
	encoding := w.literalEncoding.codes[:257]
	n := w.nbytes
	for _, t := range input {
		// 内联位写入，~30% 加速
		c := encoding[t]
		w.bits |= uint64(c.code) << w.nbits
		w.nbits += uint(c.len)
		if w.nbits < 48 {
			continue
		}
		// 存储 6 字节
		bits := w.bits
		w.bits >>= 48
		w.nbits -= 48
		bytes := w.bytes[n : n+6]
		bytes[0] = byte(bits)
		bytes[1] = byte(bits >> 8)
		bytes[2] = byte(bits >> 16)
		bytes[3] = byte(bits >> 24)
		bytes[4] = byte(bits >> 32)
		bytes[5] = byte(bits >> 40)
		n += 6
		if n < bufferFlushSize {
			continue
		}
		w.write(w.bytes[:n])
		if w.err != nil {
			return // 在写入失败时提前返回
		}
		n = 0
	}
	w.nbytes = n
	w.writeCode(encoding[endBlockMarker])
}

// histogram 将 b 的直方图累积到 h 中。
//
// len(h) 必须 >= 256，h 的所有元素必须为零。
func histogram(b []byte, h []int32) {
	h = h[:256]
	for _, t := range b {
		h[t]++
	}
}
