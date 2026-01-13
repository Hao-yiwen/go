// 版权所有 2011 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// Package bzip2 实现 bzip2 解压缩。
package bzip2

import "io"

// bzip2 没有 RFC 规范。我参考了 Wikipedia 页面并进行了大量猜测：
// https://en.wikipedia.org/wiki/Bzip2
// pyflate 的源代码对调试很有帮助：
// http://www.paul.sladen.org/projects/pyflate

// 当发现 bzip2 数据在语法上无效时，会返回 StructuralError。
type StructuralError string

func (s StructuralError) Error() string {
	return "bzip2 data invalid: " + string(s)
}

// reader 解压缩 bzip2 压缩数据。
type reader struct {
	br           bitReader
	fileCRC      uint32
	blockCRC     uint32
	wantBlockCRC uint32
	setupDone    bool // 如果我们已经解析了 bzip2 头部，则为 true。
	eof          bool
	blockSize    int       // 块大小（以字节为单位），即 900 * 1000。
	c            [256]uint // 用于逆 BWT 的 "C" 数组。
	tt           []uint32  // 镜像 bzip2 源代码中的 "tt" 数组，并在高 24 位中包含 P 数组。
	tPos         uint32    // tt 中下一个输出字节的索引。

	preRLE      []uint32 // 包含仍需处理的 RLE 数据。
	preRLEUsed  int      // 已使用的 preRLE 条目数。
	lastByte    int      // 看到的最后一个字节值。
	byteRepeats uint     // 看到的 lastByte 重复次数。
	repeats     uint     // 要输出的 lastByte 副本数。
}

// NewReader 返回一个 [io.Reader]，它从 r 解压缩 bzip2 数据。
// 如果 r 没有同时实现 [io.ByteReader]，
// 解压缩器可能会从 r 读取比必要更多的数据。
func NewReader(r io.Reader) io.Reader {
	bz2 := new(reader)
	bz2.br = newBitReader(r)
	return bz2
}

const bzip2FileMagic = 0x425a // "BZ"
const bzip2BlockMagic = 0x314159265359
const bzip2FinalMagic = 0x177245385090

// setup 解析 bzip2 头部。
func (bz2 *reader) setup(needMagic bool) error {
	br := &bz2.br

	if needMagic {
		magic := br.ReadBits(16)
		if magic != bzip2FileMagic {
			return StructuralError("bad magic value")
		}
	}

	t := br.ReadBits(8)
	if t != 'h' {
		return StructuralError("non-Huffman entropy encoding")
	}

	level := br.ReadBits(8)
	if level < '1' || level > '9' {
		return StructuralError("invalid compression level")
	}

	bz2.fileCRC = 0
	bz2.blockSize = 100 * 1000 * (level - '0')
	if bz2.blockSize > len(bz2.tt) {
		bz2.tt = make([]uint32, bz2.blockSize)
	}
	return nil
}

func (bz2 *reader) Read(buf []byte) (n int, err error) {
	if bz2.eof {
		return 0, io.EOF
	}

	if !bz2.setupDone {
		err = bz2.setup(true)
		brErr := bz2.br.Err()
		if brErr != nil {
			err = brErr
		}
		if err != nil {
			return 0, err
		}
		bz2.setupDone = true
	}

	n, err = bz2.read(buf)
	brErr := bz2.br.Err()
	if brErr != nil {
		err = brErr
	}
	return
}

func (bz2 *reader) readFromBlock(buf []byte) int {
	// bzip2 是一个基于块的压缩器，但它有一个游程长度编码（RLE）预处理步骤。
	// 基于块的特性意味着我们可以预分配固定大小的缓冲区并重用它们。
	// 然而，RLE 预处理需要分配巨大的缓冲区来存储最大扩展。
	// 因此，我们一次性处理所有块，除了 RLE，我们按需解压缩。
	n := 0
	for (bz2.repeats > 0 || bz2.preRLEUsed < len(bz2.preRLE)) && n < len(buf) {
		// 我们有待处理的 RLE 数据。

		// 游程长度编码的工作原理如下：
		// 任何四个相等字节的序列后面都跟着一个长度字节，
		// 它包含要包含的该字节的重复次数。（重复次数可以为零。）
		// 因为我们是按需解压缩的，所以状态保存在 reader 对象中。

		if bz2.repeats > 0 {
			buf[n] = byte(bz2.lastByte)
			n++
			bz2.repeats--
			if bz2.repeats == 0 {
				bz2.lastByte = -1
			}
			continue
		}

		bz2.tPos = bz2.preRLE[bz2.tPos]
		b := byte(bz2.tPos)
		bz2.tPos >>= 8
		bz2.preRLEUsed++

		if bz2.byteRepeats == 3 {
			bz2.repeats = uint(b)
			bz2.byteRepeats = 0
			continue
		}

		if bz2.lastByte == int(b) {
			bz2.byteRepeats++
		} else {
			bz2.byteRepeats = 0
		}
		bz2.lastByte = int(b)

		buf[n] = b
		n++
	}

	return n
}

func (bz2 *reader) read(buf []byte) (int, error) {
	for {
		n := bz2.readFromBlock(buf)
		if n > 0 || len(buf) == 0 {
			bz2.blockCRC = updateCRC(bz2.blockCRC, buf[:n])
			return n, nil
		}

		// 块结束。检查 CRC。
		if bz2.blockCRC != bz2.wantBlockCRC {
			bz2.br.err = StructuralError("block checksum mismatch")
			return 0, bz2.br.err
		}

		// 查找下一个块。
		br := &bz2.br
		switch br.ReadBits64(48) {
		default:
			return 0, StructuralError("bad magic value found")

		case bzip2BlockMagic:
			// 块的开始。
			err := bz2.readBlock()
			if err != nil {
				return 0, err
			}

		case bzip2FinalMagic:
			// 检查文件结束 CRC。
			wantFileCRC := uint32(br.ReadBits64(32))
			if br.err != nil {
				return 0, br.err
			}
			if bz2.fileCRC != wantFileCRC {
				br.err = StructuralError("file checksum mismatch")
				return 0, br.err
			}

			// 跳到字节边界。
			// 是否有文件连接到这个文件？
			// 它会以 BZ 开头。
			if br.bits%8 != 0 {
				br.ReadBits(br.bits % 8)
			}
			b, err := br.r.ReadByte()
			if err == io.EOF {
				br.err = io.EOF
				bz2.eof = true
				return 0, io.EOF
			}
			if err != nil {
				br.err = err
				return 0, err
			}
			z, err := br.r.ReadByte()
			if err != nil {
				if err == io.EOF {
					err = io.ErrUnexpectedEOF
				}
				br.err = err
				return 0, err
			}
			if b != 'B' || z != 'Z' {
				return 0, StructuralError("bad magic value in continuation file")
			}
			if err := bz2.setup(false); err != nil {
				return 0, err
			}
		}
	}
}

// readBlock 读取一个 bzip2 块。魔数应该已经被消耗掉了。
func (bz2 *reader) readBlock() (err error) {
	br := &bz2.br
	bz2.wantBlockCRC = uint32(br.ReadBits64(32)) // 跳过校验和。TODO: 如果我们能弄清楚它是什么，就检查它。
	bz2.blockCRC = 0
	bz2.fileCRC = (bz2.fileCRC<<1 | bz2.fileCRC>>31) ^ bz2.wantBlockCRC
	randomized := br.ReadBits(1)
	if randomized != 0 {
		return StructuralError("deprecated randomized files")
	}
	origPtr := uint(br.ReadBits(24))

	// 如果块中未使用每个字节值（即它是文本），则符号集会减少。
	// 使用的符号存储为两级 16x16 位图。
	symbolRangeUsedBitmap := br.ReadBits(16)
	symbolPresent := make([]bool, 256)
	numSymbols := 0
	for symRange := uint(0); symRange < 16; symRange++ {
		if symbolRangeUsedBitmap&(1<<(15-symRange)) != 0 {
			bits := br.ReadBits(16)
			for symbol := uint(0); symbol < 16; symbol++ {
				if bits&(1<<(15-symbol)) != 0 {
					symbolPresent[16*symRange+symbol] = true
					numSymbols++
				}
			}
		}
	}

	if numSymbols == 0 {
		// 必须有一个 EOF 符号。
		return StructuralError("no symbols in input")
	}

	// 一个块使用两到六个不同的 Huffman 树。
	numHuffmanTrees := br.ReadBits(3)
	if numHuffmanTrees < 2 || numHuffmanTrees > 6 {
		return StructuralError("invalid number of Huffman trees")
	}

	// Huffman 树可以每 50 个符号切换一次，所以有一个树索引列表
	// 告诉我们每 50 个符号块使用哪棵树。
	numSelectors := br.ReadBits(15)
	treeIndexes := make([]uint8, numSelectors)

	// 树索引经过移动到前端（MTF）变换，并存储为一元数。
	mtfTreeDecoder := newMTFDecoderWithRange(numHuffmanTrees)
	for i := range treeIndexes {
		c := 0
		for {
			inc := br.ReadBits(1)
			if inc == 0 {
				break
			}
			c++
		}
		if c >= numHuffmanTrees {
			return StructuralError("tree index too large")
		}
		treeIndexes[i] = mtfTreeDecoder.Decode(c)
	}

	// 移动到前端变换的符号列表取自之前解码的符号位图。
	symbols := make([]byte, numSymbols)
	nextSymbol := 0
	for i := 0; i < 256; i++ {
		if symbolPresent[i] {
			symbols[nextSymbol] = byte(i)
			nextSymbol++
		}
	}
	mtf := newMTFDecoder(symbols)

	numSymbols += 2 // 为 RUNA 和 RUNB 符号预留
	huffmanTrees := make([]huffmanTree, numHuffmanTrees)

	// 现在我们解码每棵树的码长数组。
	lengths := make([]uint8, numSymbols)
	for i := range huffmanTrees {
		// 码长从 5 位基值进行差分编码。
		length := br.ReadBits(5)
		for j := range lengths {
			for {
				if length < 1 || length > 20 {
					return StructuralError("Huffman length out of range")
				}
				if !br.ReadBit() {
					break
				}
				if br.ReadBit() {
					length--
				} else {
					length++
				}
			}
			lengths[j] = uint8(length)
		}
		huffmanTrees[i], err = newHuffmanTree(lengths)
		if err != nil {
			return err
		}
	}

	selectorIndex := 1 // 下一个要使用的树索引
	if len(treeIndexes) == 0 {
		return StructuralError("no tree selectors given")
	}
	if int(treeIndexes[0]) >= len(huffmanTrees) {
		return StructuralError("tree selector out of range")
	}
	currentHuffmanTree := huffmanTrees[treeIndexes[0]]
	bufIndex := 0 // 索引 bz2.buf，即输出缓冲区。
	// 移动到前端变换的输出是游程长度编码的，
	// 我们将解码合并到 Huffman 解析循环中。
	// 这两个变量累积重复计数。详情请参阅 Wikipedia 页面。
	repeat := 0
	repeatPower := 0

	// "C" 数组（用于逆 BWT）需要初始化为零。
	clear(bz2.c[:])

	decoded := 0 // 计算当前树解码的符号数。
	for {
		if decoded == 50 {
			if selectorIndex >= numSelectors {
				return StructuralError("insufficient selector indices for number of symbols")
			}
			if int(treeIndexes[selectorIndex]) >= len(huffmanTrees) {
				return StructuralError("tree selector out of range")
			}
			currentHuffmanTree = huffmanTrees[treeIndexes[selectorIndex]]
			selectorIndex++
			decoded = 0
		}

		v := currentHuffmanTree.Decode(br)
		decoded++

		if v < 2 {
			// 这是 RUNA 或 RUNB 符号。
			if repeat == 0 {
				repeatPower = 1
			}
			repeat += repeatPower << v
			repeatPower <<= 1

			// 这个 200 万的限制来自 bzip2 源代码。
			// 它防止 repeat 溢出。
			if repeat > 2*1024*1024 {
				return StructuralError("repeat count too large")
			}
			continue
		}

		if repeat > 0 {
			// 我们已经解码了一个完整的游程长度，所以需要复制最后一个输出符号。
			if repeat > bz2.blockSize-bufIndex {
				return StructuralError("repeats past end of block")
			}
			for i := 0; i < repeat; i++ {
				b := mtf.First()
				bz2.tt[bufIndex] = uint32(b)
				bz2.c[b]++
				bufIndex++
			}
			repeat = 0
		}

		if int(v) == numSymbols-1 {
			// 这是 EOF 符号。因为它总是在移动到前端列表的末尾，
			// 并且永远不会被移动到前面，所以它有这个唯一的值。
			break
		}

		// 由于两个元符号（RUNA 和 RUNB）的值为 0 和 1，
		// 人们会期望传递 |v-2| 给 MTF 解码器。
		// 然而，MTF 列表的前端永远不会被引用为 0，
		// 它总是以游程长度 1 被引用。因此 0 不需要被编码，
		// 我们在下一行使用 |v-1|。
		b := mtf.Decode(int(v - 1))
		if bufIndex >= bz2.blockSize {
			return StructuralError("data exceeds block size")
		}
		bz2.tt[bufIndex] = uint32(b)
		bz2.c[b]++
		bufIndex++
	}

	if origPtr >= uint(bufIndex) {
		return StructuralError("origPtr out of bounds")
	}

	// 我们已经完成了熵解码。现在我们可以执行逆 BWT 并设置 RLE 缓冲区。
	bz2.preRLE = bz2.tt[:bufIndex]
	bz2.preRLEUsed = 0
	bz2.tPos = inverseBWT(bz2.preRLE, origPtr, bz2.c[:])
	bz2.lastByte = -1
	bz2.byteRepeats = 0
	bz2.repeats = 0

	return nil
}

// inverseBWT 实现逆 Burrows-Wheeler 变换，如
// http://www.hpl.hp.com/techreports/Compaq-DEC/SRC-RR-124.pdf 第 4.2 节所述。
// 在那篇文档中，origPtr 被称为 "I"，c 是第一次遍历数据后的 "C" 数组。
// 它在这里作为参数是因为我们将第一次遍历与 Huffman 解码合并了。
//
// 这也实现了 bzip2 源代码中的"单数组"方法，它将输出（仍然是打乱的）
// 留在 tt 的低 8 位中，下一个字节的索引在高 24 位中。
// 返回第一个字节的索引。
func inverseBWT(tt []uint32, origPtr uint, c []uint) uint32 {
	sum := uint(0)
	for i := 0; i < 256; i++ {
		sum += c[i]
		c[i] = sum - c[i]
	}

	for i := range tt {
		b := tt[i] & 0xff
		tt[c[b]] |= uint32(i) << 8
		c[b]++
	}

	return tt[origPtr] >> 8
}

// 这是一个标准的 CRC32，类似于 hash/crc32，只是所有的移位都是反向的，
// 导致输入中的位以与通常顺序相反的方式处理。

var crctab [256]uint32

func init() {
	const poly = 0x04C11DB7
	for i := range crctab {
		crc := uint32(i) << 24
		for j := 0; j < 8; j++ {
			if crc&0x80000000 != 0 {
				crc = (crc << 1) ^ poly
			} else {
				crc <<= 1
			}
		}
		crctab[i] = crc
	}
}

// updateCRC 更新 crc 值以包含 b 中的数据。
// 初始值为 0。
func updateCRC(val uint32, b []byte) uint32 {
	crc := ^val
	for _, v := range b {
		crc = crctab[byte(crc>>24)^v] ^ (crc << 8)
	}
	return ^crc
}
