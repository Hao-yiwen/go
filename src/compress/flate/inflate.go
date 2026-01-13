// 版权所有 2009 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// Package flate 实现 RFC 1951 中描述的 DEFLATE 压缩数据格式。
// [compress/gzip] 和 [compress/zlib] 包实现对基于 DEFLATE 的文件格式的访问。
package flate

import (
	"bufio"
	"io"
	"math/bits"
	"strconv"
	"sync"
)

const (
	maxCodeLen = 16 // Huffman 码的最大长度
	// 接下来的三个数字来自 RFC 第 3.2.7 节，
	// 附加第 3.2.5 节中的条款，暗示距离码 30 和 31 永远不应出现在压缩数据中。
	maxNumLit  = 286
	maxNumDist = 30
	numCodes   = 19 // Huffman 元码中的码数
)

// 仅在首次使用时初始化 fixedHuffmanDecoder 一次。
var fixedOnce sync.Once
var fixedHuffmanDecoder huffmanDecoder

// CorruptInputError 报告在给定偏移量处存在损坏的输入。
type CorruptInputError int64

func (e CorruptInputError) Error() string {
	return "flate: corrupt input before offset " + strconv.FormatInt(int64(e), 10)
}

// InternalError 报告 flate 代码本身中的错误。
type InternalError string

func (e InternalError) Error() string { return "flate: internal error: " + string(e) }

// ReadError 报告在读取输入时遇到的错误。
//
// 已弃用：不再返回。
type ReadError struct {
	Offset int64 // 发生错误的字节偏移量
	Err    error // 底层 Read 返回的错误
}

func (e *ReadError) Error() string {
	return "flate: read error at offset " + strconv.FormatInt(e.Offset, 10) + ": " + e.Err.Error()
}

// WriteError 报告在写入输出时遇到的错误。
//
// 已弃用：不再返回。
type WriteError struct {
	Offset int64 // 发生错误的字节偏移量
	Err    error // 底层 Write 返回的错误
}

func (e *WriteError) Error() string {
	return "flate: write error at offset " + strconv.FormatInt(e.Offset, 10) + ": " + e.Err.Error()
}

// Resetter 重置由 [NewReader] 或 [NewReaderDict] 返回的 ReadCloser
// 以切换到新的底层 [Reader]。这允许重用 ReadCloser 而不是分配新的。
type Resetter interface {
	// Reset 丢弃任何缓冲数据并重置 Resetter，就像它是用给定的 reader 新初始化的一样。
	Reset(r io.Reader, dict []byte) error
}

// 解码 Huffman 表的数据结构基于 zlib。有一个固定位宽的查找表（huffmanChunkBits），
// 对于小于表宽度的码，有多个条目（每个尾随位的组合具有相同的值）。
// 对于大于表宽度的码，表包含指向溢出表的链接。
// 链接表中每个条目的宽度是最大码大小减去块宽度。
//
// 注意，即使没有填充所有位，你也可以在表中进行查找。
// 由于额外的位是零，并且 DEFLATE Huffman 码具有较短码在较长码之前的属性，
// 结果中的位长度估计是实际位数的下界。
//
// 见以下内容：
//	https://github.com/madler/zlib/raw/master/doc/algorithm.txt

// chunk & 15 是位数
// chunk >> 4 是值，包括表链接

const (
	huffmanChunkBits  = 9
	huffmanNumChunks  = 1 << huffmanChunkBits
	huffmanCountMask  = 15
	huffmanValueShift = 4
)

type huffmanDecoder struct {
	min      int                      // 最小码长
	chunks   [huffmanNumChunks]uint32 // 如上所述的块
	links    [][]uint32               // 溢出链接
	linkMask uint32                   // 链接表宽度的掩码
}

// 从码长数组初始化 Huffman 解码表。
// 在此函数之后，h 保证被初始化为完整的树（即既不过度订阅也不欠订阅）。
// 例外是退化情况，其中树只有一个长度为 1 的单个符号。允许空树。
func (h *huffmanDecoder) init(lengths []int) bool {
	// Sanity 在 Huffman 表构建期间启用额外的运行时测试。
	// 它旨在开发期间使用，以补充当前的临时单元测试。
	const sanity = false

	if h.min != 0 {
		*h = huffmanDecoder{}
	}

	// 计算每个长度的码数，计算最小和最大长度。
	var count [maxCodeLen]int
	var min, max int
	for _, n := range lengths {
		if n == 0 {
			continue
		}
		if min == 0 || n < min {
			min = n
		}
		if n > max {
			max = n
		}
		count[n]++
	}

	// 空树。如果使用该树，decompressor.huffSym 函数稍后会失败。
	// 从技术上讲，空树仅对 HDIST 树有效，而不是 HCLEN 和 HLIT 树。
	// 然而，具有空 HCLEN 树的流保证会失败，因为它将尝试使用该树来解码
	// HLIT 和 HDIST 树的码。类似地，空 HLIT 树保证稍后会失败，
	// 因为压缩数据部分必须至少由一个符号（块结束标记）组成。
	if max == 0 {
		return true
	}

	code := 0
	var nextcode [maxCodeLen]int
	for i := min; i <= max; i++ {
		code <<= 1
		nextcode[i] = code
		code += count[i]
	}

	// 检查编码是否完整（即我们已分配所有 2 的 max 次方种可能的位序列）。
	// 例外：为了与 zlib 兼容，我们还需要接受退化的单码编码。
	// 另见 TestDegenerateHuffmanCoding。
	if code != 1<<uint(max) && !(code == 1 && max == 1) {
		return false
	}

	h.min = min
	if max > huffmanChunkBits {
		numLinks := 1 << (uint(max) - huffmanChunkBits)
		h.linkMask = uint32(numLinks - 1)

		// 创建链接表
		link := nextcode[huffmanChunkBits+1] >> 1
		h.links = make([][]uint32, huffmanNumChunks-link)
		for j := uint(link); j < huffmanNumChunks; j++ {
			reverse := int(bits.Reverse16(uint16(j)))
			reverse >>= uint(16 - huffmanChunkBits)
			off := j - uint(link)
			if sanity && h.chunks[reverse] != 0 {
				panic("impossible: overwriting existing chunk")
			}
			h.chunks[reverse] = uint32(off<<huffmanValueShift | (huffmanChunkBits + 1))
			h.links[off] = make([]uint32, numLinks)
		}
	}

	for i, n := range lengths {
		if n == 0 {
			continue
		}
		code := nextcode[n]
		nextcode[n]++
		chunk := uint32(i<<huffmanValueShift | n)
		reverse := int(bits.Reverse16(uint16(code)))
		reverse >>= uint(16 - n)
		if n <= huffmanChunkBits {
			for off := reverse; off < len(h.chunks); off += 1 << uint(n) {
				// 我们永远不需要覆盖现有的块。
				// 此外，0 永远不是有效的块，因为低 4 位 "count" 位应该在 1 到 15 之间。
				if sanity && h.chunks[off] != 0 {
					panic("impossible: overwriting existing chunk")
				}
				h.chunks[off] = chunk
			}
		} else {
			j := reverse & (huffmanNumChunks - 1)
			if sanity && h.chunks[j]&huffmanCountMask != huffmanChunkBits+1 {
				// 更长的码应该已经与上面的链接表关联。
				panic("impossible: not an indirect chunk")
			}
			value := h.chunks[j] >> huffmanValueShift
			linktab := h.links[value]
			reverse >>= huffmanChunkBits
			for off := reverse; off < len(linktab); off += 1 << uint(n-huffmanChunkBits) {
				if sanity && linktab[off] != 0 {
					panic("impossible: overwriting existing chunk")
				}
				linktab[off] = chunk
			}
		}
	}

	if sanity {
		// 上面我们已经检查过我们从未覆盖现有条目。
		// 这里我们另外检查我们是否完全填充了表。
		for i, chunk := range h.chunks {
			if chunk == 0 {
				// 作为例外，在退化的单码情况下，
				// 我们允许奇数块缺失。
				if code == 1 && i%2 == 1 {
					continue
				}
				panic("impossible: missing chunk")
			}
		}
		for _, linktab := range h.links {
			for _, chunk := range linktab {
				if chunk == 0 {
					panic("impossible: missing chunk")
				}
			}
		}
	}

	return true
}

// [NewReader] 所需的实际读取接口。
// 如果传入的 [io.Reader] 没有 ReadByte，
// [NewReader] 将引入自己的缓冲。
type Reader interface {
	io.Reader
	io.ByteReader
}

// 解压缩状态。
type decompressor struct {
	// 输入源。
	r       Reader
	rBuf    *bufio.Reader // 如果提供的 io.Reader 未实现 io.ByteReader 则创建
	roffset int64

	// 输入位，在 b 的顶部。
	b  uint32
	nb uint

	// 用于字面量/长度、距离的 Huffman 解码器。
	h1, h2 huffmanDecoder

	// 用于定义 Huffman 码的长度数组。
	bits     *[maxNumLit + maxNumDist]int
	codebits *[numCodes]int

	// 输出历史记录，缓冲区。
	dict dictDecoder

	// 临时缓冲区（避免重复分配）。
	buf [4]byte

	// 解压缩的下一步，以及解压缩状态。
	step      func(*decompressor)
	stepState int
	final     bool
	err       error
	toRead    []byte
	hl, hd    *huffmanDecoder
	copyLen   int
	copyDist  int
}

func (f *decompressor) nextBlock() {
	for f.nb < 1+2 {
		if f.err = f.moreBits(); f.err != nil {
			return
		}
	}
	f.final = f.b&1 == 1
	f.b >>= 1
	typ := f.b & 3
	f.b >>= 2
	f.nb -= 1 + 2
	switch typ {
	case 0:
		f.dataBlock()
	case 1:
		// compressed, fixed Huffman tables
		f.hl = &fixedHuffmanDecoder
		f.hd = nil
		f.huffmanBlock()
	case 2:
		// compressed, dynamic Huffman tables
		if f.err = f.readHuffman(); f.err != nil {
			break
		}
		f.hl = &f.h1
		f.hd = &f.h2
		f.huffmanBlock()
	default:
		// 3 is reserved.
		f.err = CorruptInputError(f.roffset)
	}
}

func (f *decompressor) Read(b []byte) (int, error) {
	for {
		if len(f.toRead) > 0 {
			n := copy(b, f.toRead)
			f.toRead = f.toRead[n:]
			if len(f.toRead) == 0 {
				return n, f.err
			}
			return n, nil
		}
		if f.err != nil {
			return 0, f.err
		}
		f.step(f)
		if f.err != nil && len(f.toRead) == 0 {
			f.toRead = f.dict.readFlush() // Flush what's left in case of error
		}
	}
}

func (f *decompressor) Close() error {
	if f.err == io.EOF {
		return nil
	}
	return f.err
}

// RFC 1951 section 3.2.7.
// Compression with dynamic Huffman codes

var codeOrder = [...]int{16, 17, 18, 0, 8, 7, 9, 6, 10, 5, 11, 4, 12, 3, 13, 2, 14, 1, 15}

func (f *decompressor) readHuffman() error {
	// HLIT[5], HDIST[5], HCLEN[4].
	for f.nb < 5+5+4 {
		if err := f.moreBits(); err != nil {
			return err
		}
	}
	nlit := int(f.b&0x1F) + 257
	if nlit > maxNumLit {
		return CorruptInputError(f.roffset)
	}
	f.b >>= 5
	ndist := int(f.b&0x1F) + 1
	if ndist > maxNumDist {
		return CorruptInputError(f.roffset)
	}
	f.b >>= 5
	nclen := int(f.b&0xF) + 4
	// numCodes is 19, so nclen is always valid.
	f.b >>= 4
	f.nb -= 5 + 5 + 4

	// (HCLEN+4)*3 bits: code lengths in the magic codeOrder order.
	for i := 0; i < nclen; i++ {
		for f.nb < 3 {
			if err := f.moreBits(); err != nil {
				return err
			}
		}
		f.codebits[codeOrder[i]] = int(f.b & 0x7)
		f.b >>= 3
		f.nb -= 3
	}
	for i := nclen; i < len(codeOrder); i++ {
		f.codebits[codeOrder[i]] = 0
	}
	if !f.h1.init(f.codebits[0:]) {
		return CorruptInputError(f.roffset)
	}

	// HLIT + 257 code lengths, HDIST + 1 code lengths,
	// using the code length Huffman code.
	for i, n := 0, nlit+ndist; i < n; {
		x, err := f.huffSym(&f.h1)
		if err != nil {
			return err
		}
		if x < 16 {
			// Actual length.
			f.bits[i] = x
			i++
			continue
		}
		// Repeat previous length or zero.
		var rep int
		var nb uint
		var b int
		switch x {
		default:
			return InternalError("unexpected length code")
		case 16:
			rep = 3
			nb = 2
			if i == 0 {
				return CorruptInputError(f.roffset)
			}
			b = f.bits[i-1]
		case 17:
			rep = 3
			nb = 3
			b = 0
		case 18:
			rep = 11
			nb = 7
			b = 0
		}
		for f.nb < nb {
			if err := f.moreBits(); err != nil {
				return err
			}
		}
		rep += int(f.b & uint32(1<<nb-1))
		f.b >>= nb
		f.nb -= nb
		if i+rep > n {
			return CorruptInputError(f.roffset)
		}
		for j := 0; j < rep; j++ {
			f.bits[i] = b
			i++
		}
	}

	if !f.h1.init(f.bits[0:nlit]) || !f.h2.init(f.bits[nlit:nlit+ndist]) {
		return CorruptInputError(f.roffset)
	}

	// As an optimization, we can initialize the min bits to read at a time
	// for the HLIT tree to the length of the EOB marker since we know that
	// every block must terminate with one. This preserves the property that
	// we never read any extra bytes after the end of the DEFLATE stream.
	if f.h1.min < f.bits[endBlockMarker] {
		f.h1.min = f.bits[endBlockMarker]
	}

	return nil
}

// Decode a single Huffman block from f.
// hl and hd are the Huffman states for the lit/length values
// and the distance values, respectively. If hd == nil, using the
// fixed distance encoding associated with fixed Huffman blocks.
func (f *decompressor) huffmanBlock() {
	const (
		stateInit = iota // Zero value must be stateInit
		stateDict
	)

	switch f.stepState {
	case stateInit:
		goto readLiteral
	case stateDict:
		goto copyHistory
	}

readLiteral:
	// Read literal and/or (length, distance) according to RFC section 3.2.3.
	{
		v, err := f.huffSym(f.hl)
		if err != nil {
			f.err = err
			return
		}
		var n uint // number of bits extra
		var length int
		switch {
		case v < 256:
			f.dict.writeByte(byte(v))
			if f.dict.availWrite() == 0 {
				f.toRead = f.dict.readFlush()
				f.step = (*decompressor).huffmanBlock
				f.stepState = stateInit
				return
			}
			goto readLiteral
		case v == 256:
			f.finishBlock()
			return
		// otherwise, reference to older data
		case v < 265:
			length = v - (257 - 3)
			n = 0
		case v < 269:
			length = v*2 - (265*2 - 11)
			n = 1
		case v < 273:
			length = v*4 - (269*4 - 19)
			n = 2
		case v < 277:
			length = v*8 - (273*8 - 35)
			n = 3
		case v < 281:
			length = v*16 - (277*16 - 67)
			n = 4
		case v < 285:
			length = v*32 - (281*32 - 131)
			n = 5
		case v < maxNumLit:
			length = 258
			n = 0
		default:
			f.err = CorruptInputError(f.roffset)
			return
		}
		if n > 0 {
			for f.nb < n {
				if err = f.moreBits(); err != nil {
					f.err = err
					return
				}
			}
			length += int(f.b & uint32(1<<n-1))
			f.b >>= n
			f.nb -= n
		}

		var dist int
		if f.hd == nil {
			for f.nb < 5 {
				if err = f.moreBits(); err != nil {
					f.err = err
					return
				}
			}
			dist = int(bits.Reverse8(uint8(f.b & 0x1F << 3)))
			f.b >>= 5
			f.nb -= 5
		} else {
			if dist, err = f.huffSym(f.hd); err != nil {
				f.err = err
				return
			}
		}

		switch {
		case dist < 4:
			dist++
		case dist < maxNumDist:
			nb := uint(dist-2) >> 1
			// have 1 bit in bottom of dist, need nb more.
			extra := (dist & 1) << nb
			for f.nb < nb {
				if err = f.moreBits(); err != nil {
					f.err = err
					return
				}
			}
			extra |= int(f.b & uint32(1<<nb-1))
			f.b >>= nb
			f.nb -= nb
			dist = 1<<(nb+1) + 1 + extra
		default:
			f.err = CorruptInputError(f.roffset)
			return
		}

		// No check on length; encoding can be prescient.
		if dist > f.dict.histSize() {
			f.err = CorruptInputError(f.roffset)
			return
		}

		f.copyLen, f.copyDist = length, dist
		goto copyHistory
	}

copyHistory:
	// Perform a backwards copy according to RFC section 3.2.3.
	{
		cnt := f.dict.tryWriteCopy(f.copyDist, f.copyLen)
		if cnt == 0 {
			cnt = f.dict.writeCopy(f.copyDist, f.copyLen)
		}
		f.copyLen -= cnt

		if f.dict.availWrite() == 0 || f.copyLen > 0 {
			f.toRead = f.dict.readFlush()
			f.step = (*decompressor).huffmanBlock // We need to continue this work
			f.stepState = stateDict
			return
		}
		goto readLiteral
	}
}

// Copy a single uncompressed data block from input to output.
func (f *decompressor) dataBlock() {
	// Uncompressed.
	// Discard current half-byte.
	f.nb = 0
	f.b = 0

	// Length then ones-complement of length.
	nr, err := io.ReadFull(f.r, f.buf[0:4])
	f.roffset += int64(nr)
	if err != nil {
		f.err = noEOF(err)
		return
	}
	n := int(f.buf[0]) | int(f.buf[1])<<8
	nn := int(f.buf[2]) | int(f.buf[3])<<8
	if uint16(nn) != uint16(^n) {
		f.err = CorruptInputError(f.roffset)
		return
	}

	if n == 0 {
		f.toRead = f.dict.readFlush()
		f.finishBlock()
		return
	}

	f.copyLen = n
	f.copyData()
}

// copyData copies f.copyLen bytes from the underlying reader into f.hist.
// It pauses for reads when f.hist is full.
func (f *decompressor) copyData() {
	buf := f.dict.writeSlice()
	if len(buf) > f.copyLen {
		buf = buf[:f.copyLen]
	}

	cnt, err := io.ReadFull(f.r, buf)
	f.roffset += int64(cnt)
	f.copyLen -= cnt
	f.dict.writeMark(cnt)
	if err != nil {
		f.err = noEOF(err)
		return
	}

	if f.dict.availWrite() == 0 || f.copyLen > 0 {
		f.toRead = f.dict.readFlush()
		f.step = (*decompressor).copyData
		return
	}
	f.finishBlock()
}

func (f *decompressor) finishBlock() {
	if f.final {
		if f.dict.availRead() > 0 {
			f.toRead = f.dict.readFlush()
		}
		f.err = io.EOF
	}
	f.step = (*decompressor).nextBlock
}

// noEOF returns err, unless err == io.EOF, in which case it returns io.ErrUnexpectedEOF.
func noEOF(e error) error {
	if e == io.EOF {
		return io.ErrUnexpectedEOF
	}
	return e
}

func (f *decompressor) moreBits() error {
	c, err := f.r.ReadByte()
	if err != nil {
		return noEOF(err)
	}
	f.roffset++
	f.b |= uint32(c) << f.nb
	f.nb += 8
	return nil
}

// Read the next Huffman-encoded symbol from f according to h.
func (f *decompressor) huffSym(h *huffmanDecoder) (int, error) {
	// Since a huffmanDecoder can be empty or be composed of a degenerate tree
	// with single element, huffSym must error on these two edge cases. In both
	// cases, the chunks slice will be 0 for the invalid sequence, leading it
	// satisfy the n == 0 check below.
	n := uint(h.min)
	// Optimization. Compiler isn't smart enough to keep f.b,f.nb in registers,
	// but is smart enough to keep local variables in registers, so use nb and b,
	// inline call to moreBits and reassign b,nb back to f on return.
	nb, b := f.nb, f.b
	for {
		for nb < n {
			c, err := f.r.ReadByte()
			if err != nil {
				f.b = b
				f.nb = nb
				return 0, noEOF(err)
			}
			f.roffset++
			b |= uint32(c) << (nb & 31)
			nb += 8
		}
		chunk := h.chunks[b&(huffmanNumChunks-1)]
		n = uint(chunk & huffmanCountMask)
		if n > huffmanChunkBits {
			chunk = h.links[chunk>>huffmanValueShift][(b>>huffmanChunkBits)&h.linkMask]
			n = uint(chunk & huffmanCountMask)
		}
		if n <= nb {
			if n == 0 {
				f.b = b
				f.nb = nb
				f.err = CorruptInputError(f.roffset)
				return 0, f.err
			}
			f.b = b >> (n & 31)
			f.nb = nb - n
			return int(chunk >> huffmanValueShift), nil
		}
	}
}

func (f *decompressor) makeReader(r io.Reader) {
	if rr, ok := r.(Reader); ok {
		f.rBuf = nil
		f.r = rr
		return
	}
	// Reuse rBuf if possible. Invariant: rBuf is always created (and owned) by decompressor.
	if f.rBuf != nil {
		f.rBuf.Reset(r)
	} else {
		// bufio.NewReader will not return r, as r does not implement flate.Reader, so it is not bufio.Reader.
		f.rBuf = bufio.NewReader(r)
	}
	f.r = f.rBuf
}

func fixedHuffmanDecoderInit() {
	fixedOnce.Do(func() {
		// These come from the RFC section 3.2.6.
		var bits [288]int
		for i := 0; i < 144; i++ {
			bits[i] = 8
		}
		for i := 144; i < 256; i++ {
			bits[i] = 9
		}
		for i := 256; i < 280; i++ {
			bits[i] = 7
		}
		for i := 280; i < 288; i++ {
			bits[i] = 8
		}
		fixedHuffmanDecoder.init(bits[:])
	})
}

func (f *decompressor) Reset(r io.Reader, dict []byte) error {
	*f = decompressor{
		rBuf:     f.rBuf,
		bits:     f.bits,
		codebits: f.codebits,
		dict:     f.dict,
		step:     (*decompressor).nextBlock,
	}
	f.makeReader(r)
	f.dict.init(maxMatchOffset, dict)
	return nil
}

// NewReader returns a new ReadCloser that can be used
// to read the uncompressed version of r.
// If r does not also implement [io.ByteReader],
// the decompressor may read more data than necessary from r.
// The reader returns [io.EOF] after the final block in the DEFLATE stream has
// been encountered. Any trailing data after the final block is ignored.
//
// The [io.ReadCloser] returned by NewReader also implements [Resetter].
func NewReader(r io.Reader) io.ReadCloser {
	fixedHuffmanDecoderInit()

	var f decompressor
	f.makeReader(r)
	f.bits = new([maxNumLit + maxNumDist]int)
	f.codebits = new([numCodes]int)
	f.step = (*decompressor).nextBlock
	f.dict.init(maxMatchOffset, nil)
	return &f
}

// NewReaderDict is like [NewReader] but initializes the reader
// with a preset dictionary. The returned reader behaves as if
// the uncompressed data stream started with the given dictionary,
// which has already been read. NewReaderDict is typically used
// to read data compressed by [NewWriterDict].
//
// The ReadCloser returned by NewReaderDict also implements [Resetter].
func NewReaderDict(r io.Reader, dict []byte) io.ReadCloser {
	fixedHuffmanDecoderInit()

	var f decompressor
	f.makeReader(r)
	f.bits = new([maxNumLit + maxNumDist]int)
	f.codebits = new([numCodes]int)
	f.step = (*decompressor).nextBlock
	f.dict.init(maxMatchOffset, dict)
	return &f
}
