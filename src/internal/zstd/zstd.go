// 版权所有 2023 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// Package zstd 为 zstd 流提供解压器，
// 在 RFC 8878 中描述。它不支持字典。
package zstd

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// fuzzing 是一个模糊测试器钩子，在模糊测试时设置为真。
// 这用于拒绝与 zstd 不匹配的情况。
var fuzzing = false

// Reader 实现 [io.Reader] 来读取 zstd 压缩流。
type Reader struct {
	// 基础 Reader。
	r io.Reader

	// 我们是否已读取帧头。
	// 当缓冲区为空时这很有意义。
	// 如果为真，我们期望看到一个新块。
	sawFrameHeader bool

	// 当前帧是否需要校验和。
	hasChecksum bool

	// 我们是否至少读取了一帧。
	readOneFrame bool

	// 如果帧大小未知，则为真。
	frameSizeUnknown bool

	// 当前帧中未压缩字节的数量。
	// 如果 frameSizeUnknown 为真，则此值无效。
	remainingFrameSize uint64

	// 从 r 读取的字节数，到当前块的开始，用于错误报告。
	blockOffset int64

	// 缓冲的解压缩数据。
	buffer []byte
	// 缓冲区中的当前读取偏移量。
	off int

	// 当前重复偏移量。
	repeatedOffset1 uint32
	repeatedOffset2 uint32
	repeatedOffset3 uint32

	// 用于压缩字面的当前哈夫曼树。
	huffmanTable     []uint16
	huffmanTableBits int

	// 用于后向引用的窗口。
	window window

	// 用于保存压缩块的缓冲区。
	compressedBuf []byte

	// 字面的缓冲区。
	literals []byte

	// 序列解码 FSE 表。
	seqTables    [3][]fseBaselineEntry
	seqTableBits [3]uint8

	// 序列解码 FSE 表的缓冲区。
	seqTableBuffers [3][]fseBaselineEntry

	// 为小读取保留的临时空间，以避免分配。
	scratch [16]byte

	// 用于读取 FSE 的临时表。仅临时有效。
	fseScratch []fseEntry

	// 用于校验和计算。
	checksum xxhash64
}

// NewReader 创建一个新的 Reader，从给定的 Reader 解压数据。
func NewReader(input io.Reader) *Reader {
	r := new(Reader)
	r.Reset(input)
	return r
}

// Reset 丢弃当前状态并从 r 开始读取新流。
// 这允许重用 Reader 而不是分配新的。
func (r *Reader) Reset(input io.Reader) {
	r.r = input

	// 保留了几个字段以避免分配。
	// 其他字段在使用前始终设置。
	r.sawFrameHeader = false
	r.hasChecksum = false
	r.readOneFrame = false
	r.frameSizeUnknown = false
	r.remainingFrameSize = 0
	r.blockOffset = 0
	r.buffer = r.buffer[:0]
	r.off = 0
	// repeatedOffset1
	// repeatedOffset2
	// repeatedOffset3
	// huffmanTable
	// huffmanTableBits
	// window
	// compressedBuf
	// literals
	// seqTables
	// seqTableBits
	// seqTableBuffers
	// scratch
	// fseScratch
}

// Read 实现 [io.Reader]。
func (r *Reader) Read(p []byte) (int, error) {
	if err := r.refillIfNeeded(); err != nil {
		return 0, err
	}
	n := copy(p, r.buffer[r.off:])
	r.off += n
	return n, nil
}

// ReadByte 实现 [io.ByteReader]。
func (r *Reader) ReadByte() (byte, error) {
	if err := r.refillIfNeeded(); err != nil {
		return 0, err
	}
	ret := r.buffer[r.off]
	r.off++
	return ret, nil
}

// refillIfNeeded 在必要时读取下一个块。
func (r *Reader) refillIfNeeded() error {
	for r.off >= len(r.buffer) {
		if err := r.refill(); err != nil {
			return err
		}
		r.off = 0
	}
	return nil
}

// refill 读取并解压下一个块。
func (r *Reader) refill() error {
	if !r.sawFrameHeader {
		if err := r.readFrameHeader(); err != nil {
			return err
		}
	}
	return r.readBlock()
}

// readFrameHeader 读取帧头并准备读取块。
func (r *Reader) readFrameHeader() error {
retry:
	relativeOffset := 0

	// 读取魔数。RFC 3.1.1。
	if _, err := io.ReadFull(r.r, r.scratch[:4]); err != nil {
		// 我们要求流至少包含一帧。
		if err == io.EOF && !r.readOneFrame {
			err = io.ErrUnexpectedEOF
		}
		return r.wrapError(relativeOffset, err)
	}

	if magic := binary.LittleEndian.Uint32(r.scratch[:4]); magic != 0xfd2fb528 {
		if magic >= 0x184d2a50 && magic <= 0x184d2a5f {
			// 这是一个可跳过的帧。
			r.blockOffset += int64(relativeOffset) + 4
			if err := r.skipFrame(); err != nil {
				return err
			}
			r.readOneFrame = true
			goto retry
		}

		return r.makeError(relativeOffset, "invalid magic number")
	}

	relativeOffset += 4

	// 读取 Frame_Header_Descriptor。RFC 3.1.1.1.1。
	if _, err := io.ReadFull(r.r, r.scratch[:1]); err != nil {
		return r.wrapNonEOFError(relativeOffset, err)
	}
	descriptor := r.scratch[0]

	singleSegment := descriptor&(1<<5) != 0

	fcsFieldSize := 1 << (descriptor >> 6)
	if fcsFieldSize == 1 && !singleSegment {
		fcsFieldSize = 0
	}

	var windowDescriptorSize int
	if singleSegment {
		windowDescriptorSize = 0
	} else {
		windowDescriptorSize = 1
	}

	if descriptor&(1<<3) != 0 {
		return r.makeError(relativeOffset, "reserved bit set in frame header descriptor")
	}

	r.hasChecksum = descriptor&(1<<2) != 0
	if r.hasChecksum {
		r.checksum.reset()
	}

	// Dictionary_ID_Flag。RFC 3.1.1.1.1.6。
	dictionaryIdSize := 0
	if dictIdFlag := descriptor & 3; dictIdFlag != 0 {
		dictionaryIdSize = 1 << (dictIdFlag - 1)
	}

	relativeOffset++

	headerSize := windowDescriptorSize + dictionaryIdSize + fcsFieldSize

	if _, err := io.ReadFull(r.r, r.scratch[:headerSize]); err != nil {
		return r.wrapNonEOFError(relativeOffset, err)
	}

	// 找出我们需要为后向引用保留的最大数据量。
	var windowSize uint64
	if !singleSegment {
		// 窗口描述符。RFC 3.1.1.1.2。
		windowDescriptor := r.scratch[0]
		exponent := uint64(windowDescriptor >> 3)
		mantissa := uint64(windowDescriptor & 7)
		windowLog := exponent + 10
		windowBase := uint64(1) << windowLog
		windowAdd := (windowBase / 8) * mantissa
		windowSize = windowBase + windowAdd

		// 默认 zstd 对窗口大小设置限制。
		if fuzzing && (windowLog > 31 || windowSize > 1<<27) {
			return r.makeError(relativeOffset, "windowSize too large")
		}
	}

	// Dictionary_ID。RFC 3.1.1.1.3。
	if dictionaryIdSize != 0 {
		dictionaryId := r.scratch[windowDescriptorSize : windowDescriptorSize+dictionaryIdSize]
		// 仅允许零 Dictionary ID。
		for _, b := range dictionaryId {
			if b != 0 {
				return r.makeError(relativeOffset, "dictionaries are not supported")
			}
		}
	}

	// Frame_Content_Size。RFC 3.1.1.1.4。
	r.frameSizeUnknown = false
	r.remainingFrameSize = 0
	fb := r.scratch[windowDescriptorSize+dictionaryIdSize:]
	switch fcsFieldSize {
	case 0:
		r.frameSizeUnknown = true
	case 1:
		r.remainingFrameSize = uint64(fb[0])
	case 2:
		r.remainingFrameSize = 256 + uint64(binary.LittleEndian.Uint16(fb))
	case 4:
		r.remainingFrameSize = uint64(binary.LittleEndian.Uint32(fb))
	case 8:
		r.remainingFrameSize = binary.LittleEndian.Uint64(fb)
	default:
		panic("unreachable")
	}

	// RFC 3.1.1.1.2.
	// 当设置 Single_Segment_Flag 时，Window_Descriptor 不存在。
	// 在这种情况下，Window_Size 是 Frame_Content_Size。
	if singleSegment {
		windowSize = r.remainingFrameSize
	}

	// RFC 8878 3.1.1.1.1.2. 允许我们在窗口大小上设置 8M 最大值。
	const maxWindowSize = 8 << 20
	if windowSize > maxWindowSize {
		windowSize = maxWindowSize
	}

	relativeOffset += headerSize

	r.sawFrameHeader = true
	r.readOneFrame = true
	r.blockOffset += int64(relativeOffset)

	// 准备从帧中读取块。
	r.repeatedOffset1 = 1
	r.repeatedOffset2 = 4
	r.repeatedOffset3 = 8
	r.huffmanTableBits = 0
	r.window.reset(int(windowSize))
	r.seqTables[0] = nil
	r.seqTables[1] = nil
	r.seqTables[2] = nil

	return nil
}

// skipFrame 跳过一个可跳过的帧。RFC 3.1.2。
func (r *Reader) skipFrame() error {
	relativeOffset := 0

	if _, err := io.ReadFull(r.r, r.scratch[:4]); err != nil {
		return r.wrapNonEOFError(relativeOffset, err)
	}

	relativeOffset += 4

	size := binary.LittleEndian.Uint32(r.scratch[:4])
	if size == 0 {
		r.blockOffset += int64(relativeOffset)
		return nil
	}

	if seeker, ok := r.r.(io.Seeker); ok {
		r.blockOffset += int64(relativeOffset)
		// Seeker 的实现并不总是检测无效偏移，
		// 因此通过与末尾比较来检查新偏移是否有效。
		prev, err := seeker.Seek(0, io.SeekCurrent)
		if err != nil {
			return r.wrapError(0, err)
		}
		end, err := seeker.Seek(0, io.SeekEnd)
		if err != nil {
			return r.wrapError(0, err)
		}
		if prev > end-int64(size) {
			r.blockOffset += end - prev
			return r.makeEOFError(0)
		}

		// 新偏移有效，所以寻求到它。
		_, err = seeker.Seek(prev+int64(size), io.SeekStart)
		if err != nil {
			return r.wrapError(0, err)
		}
		r.blockOffset += int64(size)
		return nil
	}

	n, err := io.CopyN(io.Discard, r.r, int64(size))
	relativeOffset += int(n)
	if err != nil {
		return r.wrapNonEOFError(relativeOffset, err)
	}
	r.blockOffset += int64(relativeOffset)
	return nil
}

// readBlock 从一帧中读取下一个块。
func (r *Reader) readBlock() error {
	relativeOffset := 0

	// 读取 Block_Header。RFC 3.1.1.2。
	if _, err := io.ReadFull(r.r, r.scratch[:3]); err != nil {
		return r.wrapNonEOFError(relativeOffset, err)
	}

	relativeOffset += 3

	header := uint32(r.scratch[0]) | (uint32(r.scratch[1]) << 8) | (uint32(r.scratch[2]) << 16)

	lastBlock := header&1 != 0
	blockType := (header >> 1) & 3
	blockSize := int(header >> 3)

	// 最大块大小是窗口大小和 128K 中的较小者。
	// 我们不记录单段帧的窗口大小，
	// 所以只使用 128K。RFC 3.1.1.2.3，3.1.1.2.4。
	if blockSize > 128<<10 || (r.window.size > 0 && blockSize > r.window.size) {
		return r.makeError(relativeOffset, "block size too large")
	}

	// 处理不同的块类型。RFC 3.1.1.2.2。
	switch blockType {
	case 0:
		r.setBufferSize(blockSize)
		if _, err := io.ReadFull(r.r, r.buffer); err != nil {
			return r.wrapNonEOFError(relativeOffset, err)
		}
		relativeOffset += blockSize
		r.blockOffset += int64(relativeOffset)
	case 1:
		r.setBufferSize(blockSize)
		if _, err := io.ReadFull(r.r, r.scratch[:1]); err != nil {
			return r.wrapNonEOFError(relativeOffset, err)
		}
		relativeOffset++
		v := r.scratch[0]
		for i := range r.buffer {
			r.buffer[i] = v
		}
		r.blockOffset += int64(relativeOffset)
	case 2:
		r.blockOffset += int64(relativeOffset)
		if err := r.compressedBlock(blockSize); err != nil {
			return err
		}
		r.blockOffset += int64(blockSize)
	case 3:
		return r.makeError(relativeOffset, "invalid block type")
	}

	if !r.frameSizeUnknown {
		if uint64(len(r.buffer)) > r.remainingFrameSize {
			return r.makeError(relativeOffset, "too many uncompressed bytes in frame")
		}
		r.remainingFrameSize -= uint64(len(r.buffer))
	}

	if r.hasChecksum {
		r.checksum.update(r.buffer)
	}

	if !lastBlock {
		r.window.save(r.buffer)
	} else {
		if !r.frameSizeUnknown && r.remainingFrameSize != 0 {
			return r.makeError(relativeOffset, "not enough uncompressed bytes for frame")
		}
		// 检查帧末尾的校验和。RFC 3.1.1。
		if r.hasChecksum {
			if _, err := io.ReadFull(r.r, r.scratch[:4]); err != nil {
				return r.wrapNonEOFError(0, err)
			}

			inputChecksum := binary.LittleEndian.Uint32(r.scratch[:4])
			dataChecksum := uint32(r.checksum.digest())
			if inputChecksum != dataChecksum {
				return r.wrapError(0, fmt.Errorf("invalid checksum: got %#x want %#x", dataChecksum, inputChecksum))
			}

			r.blockOffset += 4
		}
		r.sawFrameHeader = false
	}

	return nil
}

// setBufferSize sets the decompressed buffer size.
// When this is called the buffer is empty.
func (r *Reader) setBufferSize(size int) {
	if cap(r.buffer) < size {
		need := size - cap(r.buffer)
		r.buffer = append(r.buffer[:cap(r.buffer)], make([]byte, need)...)
	}
	r.buffer = r.buffer[:size]
}

// zstdError is an error while decompressing.
type zstdError struct {
	offset int64
	err    error
}

func (ze *zstdError) Error() string {
	return fmt.Sprintf("zstd decompression error at %d: %v", ze.offset, ze.err)
}

func (ze *zstdError) Unwrap() error {
	return ze.err
}

func (r *Reader) makeEOFError(off int) error {
	return r.wrapError(off, io.ErrUnexpectedEOF)
}

func (r *Reader) wrapNonEOFError(off int, err error) error {
	if err == io.EOF {
		err = io.ErrUnexpectedEOF
	}
	return r.wrapError(off, err)
}

func (r *Reader) makeError(off int, msg string) error {
	return r.wrapError(off, errors.New(msg))
}

func (r *Reader) wrapError(off int, err error) error {
	if err == io.EOF {
		return err
	}
	return &zstdError{r.blockOffset + int64(off), err}
}
