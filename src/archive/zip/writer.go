// 版权所有 2011 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package zip

import (
	"bufio"
	"encoding/binary"
	"errors"
	"hash"
	"hash/crc32"
	"io"
	"io/fs"
	"strings"
	"unicode/utf8"
)

var (
	errLongName  = errors.New("zip: FileHeader.Name too long")
	errLongExtra = errors.New("zip: FileHeader.Extra too long")
)

// Writer 实现了一个 zip 文件写入器。
type Writer struct {
	cw          *countWriter
	dir         []*header
	last        *fileWriter
	closed      bool
	compressors map[uint16]Compressor
	comment     string

	// testHookCloseSizeOffset 如果非 nil，则在 Close 时调用
	// 中央目录的大小和偏移量。
	testHookCloseSizeOffset func(size, offset uint64)
}

type header struct {
	*FileHeader
	offset uint64
	raw    bool
}

// NewWriter 返回一个新的 [Writer]，向 w 写入 zip 文件。
func NewWriter(w io.Writer) *Writer {
	return &Writer{cw: &countWriter{w: bufio.NewWriter(w)}}
}

// SetOffset 设置 zip 数据在基础写入器中的开始偏移量。
// 它应在 zip 数据附加到现有文件（例如二进制可执行文件）时使用。
// 它必须在写入任何数据之前调用。
func (w *Writer) SetOffset(n int64) {
	if w.cw.count != 0 {
		panic("zip: SetOffset called after data was written")
	}
	w.cw.count = n
}

// Flush 将任何缓冲的数据刷新到基础写入器。
// 通常不需要调用 Flush；调用 Close 就足够了。
func (w *Writer) Flush() error {
	return w.cw.w.(*bufio.Writer).Flush()
}

// SetComment 设置中央目录末尾的注释字段。
// 它只能在 [Writer.Close] 之前调用。
func (w *Writer) SetComment(comment string) error {
	if len(comment) > uint16max {
		return errors.New("zip: Writer.Comment too long")
	}
	w.comment = comment
	return nil
}

// Close 通过写入中央目录完成 zip 文件的写入。
// 它不关闭基础写入器。
func (w *Writer) Close() error {
	if w.last != nil && !w.last.closed {
		if err := w.last.close(); err != nil {
			return err
		}
		w.last = nil
	}
	if w.closed {
		return errors.New("zip: writer closed twice")
	}
	w.closed = true

	// 写入中央目录
	start := w.cw.count
	for _, h := range w.dir {
		var buf [directoryHeaderLen]byte
		b := writeBuf(buf[:])
		b.uint32(uint32(directoryHeaderSignature))
		b.uint16(h.CreatorVersion)
		b.uint16(h.ReaderVersion)
		b.uint16(h.Flags)
		b.uint16(h.Method)
		b.uint16(h.ModifiedTime)
		b.uint16(h.ModifiedDate)
		b.uint32(h.CRC32)
		if h.isZip64() || h.offset >= uint32max {
			// 该文件需要 zip64 头部。在两个 32 位大小字段中存储 maxint
			//（以及稍后的偏移量）来表示应使用 zip64 额外头部。
			b.uint32(uint32max) // 压缩大小
			b.uint32(uint32max) // 未压缩大小

			// 将 zip64 额外块附加到 Extra
			var buf [28]byte // 2x uint16 + 3x uint64
			eb := writeBuf(buf[:])
			eb.uint16(zip64ExtraID)
			eb.uint16(24) // size = 3x uint64
			eb.uint64(h.UncompressedSize64)
			eb.uint64(h.CompressedSize64)
			eb.uint64(h.offset)
			h.Extra = append(h.Extra, buf[:]...)
		} else {
			b.uint32(h.CompressedSize)
			b.uint32(h.UncompressedSize)
		}

		b.uint16(uint16(len(h.Name)))
		b.uint16(uint16(len(h.Extra)))
		b.uint16(uint16(len(h.Comment)))
		b = b[4:] // 跳过磁盘号开始和内部文件属性（2x uint16）
		b.uint32(h.ExternalAttrs)
		if h.offset > uint32max {
			b.uint32(uint32max)
		} else {
			b.uint32(uint32(h.offset))
		}
		if _, err := w.cw.Write(buf[:]); err != nil {
			return err
		}
		if _, err := io.WriteString(w.cw, h.Name); err != nil {
			return err
		}
		if _, err := w.cw.Write(h.Extra); err != nil {
			return err
		}
		if _, err := io.WriteString(w.cw, h.Comment); err != nil {
			return err
		}
	}
	end := w.cw.count

	records := uint64(len(w.dir))
	size := uint64(end - start)
	offset := uint64(start)

	if f := w.testHookCloseSizeOffset; f != nil {
		f(size, offset)
	}

	if records >= uint16max || size >= uint32max || offset >= uint32max {
		var buf [directory64EndLen + directory64LocLen]byte
		b := writeBuf(buf[:])

		// zip64 中央目录末尾记录
		b.uint32(directory64EndSignature)
		b.uint64(directory64EndLen - 12) // 长度减去签名（uint32）和长度字段（uint64）
		b.uint16(zipVersion45)           // 创建版本
		b.uint16(zipVersion45)           // 提取所需版本
		b.uint32(0)                      // 此磁盘号
		b.uint32(0)                      // 包含中央目录开始的磁盘号
		b.uint64(records)                // 此磁盘上中央目录中的条目总数
		b.uint64(records)                // 中央目录中的条目总数
		b.uint64(size)                   // 中央目录的大小
		b.uint64(offset)                 // 中央目录相对于起始磁盘号的开始偏移量

		// zip64 中央目录末尾定位器
		b.uint32(directory64LocSignature)
		b.uint32(0)           // 包含 zip64 中央目录末尾的磁盘号
		b.uint64(uint64(end)) // zip64 中央目录末尾记录的相对偏移量
		b.uint32(1)           // 磁盘总数

		if _, err := w.cw.Write(buf[:]); err != nil {
			return err
		}

		// 在常规末尾记录中存储最大值以表示
		// 应改用 zip64 值
		records = uint16max
		size = uint32max
		offset = uint32max
	}

	// 写入末尾记录
	var buf [directoryEndLen]byte
	b := writeBuf(buf[:])
	b.uint32(uint32(directoryEndSignature))
	b = b[4:]                        // 跳过磁盘号和首个磁盘号（2x uint16）
	b.uint16(uint16(records))        // 此磁盘上的条目数
	b.uint16(uint16(records))        // 条目总数
	b.uint32(uint32(size))           // 目录大小
	b.uint32(uint32(offset))         // 目录开始
	b.uint16(uint16(len(w.comment))) // EOCD 注释的字节大小
	if _, err := w.cw.Write(buf[:]); err != nil {
		return err
	}
	if _, err := io.WriteString(w.cw, w.comment); err != nil {
		return err
	}

	return w.cw.w.(*bufio.Writer).Flush()
}

// Create 使用提供的名称将文件添加到 zip 文件。
// 它返回一个应写入文件内容的 [Writer]。
// 文件内容将使用 [Deflate] 方法压缩。
// 名称必须是相对路径：它不能以驱动器号（例如 C:）或前导斜杠开头，
// 仅允许正斜杠。要创建目录而不是文件，请向名称添加尾部斜杠。
// 重复的名称不会覆盖以前的条目，而是附加到 zip 文件。
// 文件的内容必须在下一次调用 [Writer.Create]、[Writer.CreateHeader]
// 或 [Writer.Close] 之前写入 [io.Writer]。
func (w *Writer) Create(name string) (io.Writer, error) {
	header := &FileHeader{
		Name:   name,
		Method: Deflate,
	}
	return w.CreateHeader(header)
}

// detectUTF8 报告 s 是否是有效的 UTF-8 字符串，
// 以及字符串是否必须被视为 UTF-8 编码
//（即与 CP-437、ASCII 或任何其他常见编码不兼容）。
func detectUTF8(s string) (valid, require bool) {
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		i += size
		// 官方上，ZIP 使用 CP-437，但许多读取器使用系统的
		// 本地字符编码。大多数编码与大量的 CP-437 子集兼容，
		// 而 CP-437 本身是类似 ASCII 的。
		//
		// 禁止 0x7e 和 0x5c，因为 EUC-KR 和 Shift-JIS 将这些
		// 字符替换为本地化的货币和上划线字符。
		if r < 0x20 || r > 0x7d || r == 0x5c {
			if !utf8.ValidRune(r) || (r == utf8.RuneError && size == 1) {
				return false, false
			}
			require = true
		}
	}
	return true, require
}

// prepare 执行 CreateHeader 和 CreateRaw 开始时所需的簿记操作。
func (w *Writer) prepare(fh *FileHeader) error {
	if w.last != nil && !w.last.closed {
		if err := w.last.close(); err != nil {
			return err
		}
	}
	if len(w.dir) > 0 && w.dir[len(w.dir)-1].FileHeader == fh {
		// See https://golang.org/issue/11144 confusion.
		return errors.New("archive/zip: invalid duplicate FileHeader")
	}
	return nil
}

// CreateHeader 使用提供的 [FileHeader] 将文件添加到 zip 归档，
// 用于文件元数据。[Writer] 获取 fh 的所有权并可能改变
// 其字段。调用者在调用 [Writer.CreateHeader] 后不得修改 fh。
//
// 这返回一个应写入文件内容的 [Writer]。
// 文件的内容必须在下一次调用 [Writer.Create]、[Writer.CreateHeader]、
// [Writer.CreateRaw] 或 [Writer.Close] 之前写入 io.Writer。
func (w *Writer) CreateHeader(fh *FileHeader) (io.Writer, error) {
	if err := w.prepare(fh); err != nil {
		return nil, err
	}

	// ZIP 格式在字符编码方面处于一个悲哀的状态。
	// 官方上，名称和注释字段应该使用 CP-437 进行编码
	//（大部分与 ASCII 兼容），除非设置了 UTF-8 标志位。
	// 但是，存在一些问题：
	//
	//	* 许多 ZIP 读取器仍然不支持 UTF-8。
	//	* 如果清除了 UTF-8 标志，几个读取器只是将
	//	名称和注释字段解释为本地系统编码。
	//
	// 为了避免破坏不支持 UTF-8 的读取器，
	// 如果字符串是 CP-437 兼容的，我们避免设置 UTF-8 标志。
	// 但是，如果字符串需要多字节 UTF-8 编码并且是
	// 有效的 UTF-8 字符串，则我们设置 UTF-8 位。
	//
	// 对于用户明确想要指定编码为 UTF-8 的情况，
	// 他们需要自己设置标志位。
	utf8Valid1, utf8Require1 := detectUTF8(fh.Name)
	utf8Valid2, utf8Require2 := detectUTF8(fh.Comment)
	switch {
	case fh.NonUTF8:
		fh.Flags &^= 0x800
	case (utf8Require1 || utf8Require2) && (utf8Valid1 && utf8Valid2):
		fh.Flags |= 0x800
	}

	fh.CreatorVersion = fh.CreatorVersion&0xff00 | zipVersion20 // 保留兼容性字节
	fh.ReaderVersion = zipVersion20

	// 如果设置了 Modified，则优先于 MS-DOS 时间戳字段。
	if !fh.Modified.IsZero() {
		// 与 FileHeader.SetModTime 方法相反，我们故意
		// 不转换为 UTC，因为我们假设用户打算使用
		// 指定的时区对日期进行编码。用户可能需要这种控制
		// 因为许多遗留 ZIP 读取器根据本地时区
		// 解释时间戳。
		//
		// The timezone is only non-UTC if a user directly sets the Modified
		// field directly themselves. All other approaches sets UTC.
		fh.ModifiedDate, fh.ModifiedTime = timeToMsDosTime(fh.Modified)

		// Use "extended timestamp" format since this is what Info-ZIP uses.
		// Nearly every major ZIP implementation uses a different format,
		// but at least most seem to be able to understand the other formats.
		//
		// This format happens to be identical for both local and central header
		// if modification time is the only timestamp being encoded.
		var mbuf [9]byte // 2*SizeOf(uint16) + SizeOf(uint8) + SizeOf(uint32)
		mt := uint32(fh.Modified.Unix())
		eb := writeBuf(mbuf[:])
		eb.uint16(extTimeExtraID)
		eb.uint16(5)  // Size: SizeOf(uint8) + SizeOf(uint32)
		eb.uint8(1)   // Flags: ModTime
		eb.uint32(mt) // ModTime
		fh.Extra = append(fh.Extra, mbuf[:]...)
	}

	var (
		ow io.Writer
		fw *fileWriter
	)
	h := &header{
		FileHeader: fh,
		offset:     uint64(w.cw.count),
	}

	if strings.HasSuffix(fh.Name, "/") {
		// Set the compression method to Store to ensure data length is truly zero,
		// which the writeHeader method always encodes for the size fields.
		// This is necessary as most compression formats have non-zero lengths
		// even when compressing an empty string.
		fh.Method = Store
		fh.Flags &^= 0x8 // we will not write a data descriptor

		// Explicitly clear sizes as they have no meaning for directories.
		fh.CompressedSize = 0
		fh.CompressedSize64 = 0
		fh.UncompressedSize = 0
		fh.UncompressedSize64 = 0

		ow = dirWriter{}
	} else {
		fh.Flags |= 0x8 // we will write a data descriptor

		fw = &fileWriter{
			zipw:      w.cw,
			compCount: &countWriter{w: w.cw},
			crc32:     crc32.NewIEEE(),
		}
		comp := w.compressor(fh.Method)
		if comp == nil {
			return nil, ErrAlgorithm
		}
		var err error
		fw.comp, err = comp(fw.compCount)
		if err != nil {
			return nil, err
		}
		fw.rawCount = &countWriter{w: fw.comp}
		fw.header = h
		ow = fw
	}
	w.dir = append(w.dir, h)
	if err := writeHeader(w.cw, h); err != nil {
		return nil, err
	}
	// If we're creating a directory, fw is nil.
	w.last = fw
	return ow, nil
}

func writeHeader(w io.Writer, h *header) error {
	const maxUint16 = 1<<16 - 1
	if len(h.Name) > maxUint16 {
		return errLongName
	}
	if len(h.Extra) > maxUint16 {
		return errLongExtra
	}

	var buf [fileHeaderLen]byte
	b := writeBuf(buf[:])
	b.uint32(uint32(fileHeaderSignature))
	b.uint16(h.ReaderVersion)
	b.uint16(h.Flags)
	b.uint16(h.Method)
	b.uint16(h.ModifiedTime)
	b.uint16(h.ModifiedDate)
	// In raw mode (caller does the compression), the values are either
	// written here or in the trailing data descriptor based on the header
	// flags.
	if h.raw && !h.hasDataDescriptor() {
		b.uint32(h.CRC32)
		b.uint32(uint32(min(h.CompressedSize64, uint32max)))
		b.uint32(uint32(min(h.UncompressedSize64, uint32max)))
	} else {
		// When this package handle the compression, these values are
		// always written to the trailing data descriptor.
		b.uint32(0) // crc32
		b.uint32(0) // compressed size
		b.uint32(0) // uncompressed size
	}
	b.uint16(uint16(len(h.Name)))
	b.uint16(uint16(len(h.Extra)))
	if _, err := w.Write(buf[:]); err != nil {
		return err
	}
	if _, err := io.WriteString(w, h.Name); err != nil {
		return err
	}
	_, err := w.Write(h.Extra)
	return err
}

// CreateRaw 使用提供的 [FileHeader] 将文件添加到 zip 归档，
// 并返回应写入文件内容的 [Writer]。文件的内容
// 必须在下一次调用 [Writer.Create]、[Writer.CreateHeader]、
// [Writer.CreateRaw] 或 [Writer.Close] 之前写入 io.Writer。
//
// 与 [Writer.CreateHeader] 相反，传递给 Writer 的字节不会被压缩。
//
// CreateRaw 的参数存储在 w 中。如果参数是来自
// 从内存中数据创建的 [Reader] 获得的 [File] 中
// 嵌入的 [FileHeader] 的指针，则 w 将引用该内存的所有内容。
func (w *Writer) CreateRaw(fh *FileHeader) (io.Writer, error) {
	if err := w.prepare(fh); err != nil {
		return nil, err
	}

	fh.CompressedSize = uint32(min(fh.CompressedSize64, uint32max))
	fh.UncompressedSize = uint32(min(fh.UncompressedSize64, uint32max))

	h := &header{
		FileHeader: fh,
		offset:     uint64(w.cw.count),
		raw:        true,
	}
	w.dir = append(w.dir, h)
	if err := writeHeader(w.cw, h); err != nil {
		return nil, err
	}

	if strings.HasSuffix(fh.Name, "/") {
		w.last = nil
		return dirWriter{}, nil
	}

	fw := &fileWriter{
		header: h,
		zipw:   w.cw,
	}
	w.last = fw
	return fw, nil
}

// Copy 将文件 f（从 [Reader] 获得）复制到 w。
// 它直接复制原始形式，绕过解压缩、压缩和验证。
func (w *Writer) Copy(f *File) error {
	r, err := f.OpenRaw()
	if err != nil {
		return err
	}
	// Copy the FileHeader so w doesn't store a pointer to the data
	// of f's entire archive. See #65499.
	fh := f.FileHeader
	fw, err := w.CreateRaw(&fh)
	if err != nil {
		return err
	}
	_, err = io.Copy(fw, r)
	return err
}

// RegisterCompressor 为特定的方法 ID 注册或覆盖自定义压缩器。
// 如果未找到给定方法的压缩器，[Writer] 将
// 默认在包级别查找压缩器。
func (w *Writer) RegisterCompressor(method uint16, comp Compressor) {
	if w.compressors == nil {
		w.compressors = make(map[uint16]Compressor)
	}
	w.compressors[method] = comp
}

// AddFS 将来自 fs.FS 的文件添加到归档。
// 它从文件系统的根开始遍历目录树，
// 使用压缩将每个文件添加到 zip，同时保持目录结构。
func (w *Writer) AddFS(fsys fs.FS) error {
	return fs.WalkDir(fsys, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if name == "." {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !d.IsDir() && !info.Mode().IsRegular() {
			return errors.New("zip: cannot add non-regular file")
		}
		h, err := FileInfoHeader(info)
		if err != nil {
			return err
		}
		h.Name = name
		if d.IsDir() {
			h.Name += "/"
		}
		h.Method = Deflate
		fw, err := w.CreateHeader(h)
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		f, err := fsys.Open(name)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(fw, f)
		return err
	})
}

func (w *Writer) compressor(method uint16) Compressor {
	comp := w.compressors[method]
	if comp == nil {
		comp = compressor(method)
	}
	return comp
}

type dirWriter struct{}

func (dirWriter) Write(b []byte) (int, error) {
	if len(b) == 0 {
		return 0, nil
	}
	return 0, errors.New("zip: write to directory")
}

type fileWriter struct {
	*header
	zipw      io.Writer
	rawCount  *countWriter
	comp      io.WriteCloser
	compCount *countWriter
	crc32     hash.Hash32
	closed    bool
}

func (w *fileWriter) Write(p []byte) (int, error) {
	if w.closed {
		return 0, errors.New("zip: write to closed file")
	}
	if w.raw {
		return w.zipw.Write(p)
	}
	w.crc32.Write(p)
	return w.rawCount.Write(p)
}

func (w *fileWriter) close() error {
	if w.closed {
		return errors.New("zip: file closed twice")
	}
	w.closed = true
	if w.raw {
		return w.writeDataDescriptor()
	}
	if err := w.comp.Close(); err != nil {
		return err
	}

	// update FileHeader
	fh := w.header.FileHeader
	fh.CRC32 = w.crc32.Sum32()
	fh.CompressedSize64 = uint64(w.compCount.count)
	fh.UncompressedSize64 = uint64(w.rawCount.count)

	if fh.isZip64() {
		fh.CompressedSize = uint32max
		fh.UncompressedSize = uint32max
		fh.ReaderVersion = zipVersion45 // requires 4.5 - File uses ZIP64 format extensions
	} else {
		fh.CompressedSize = uint32(fh.CompressedSize64)
		fh.UncompressedSize = uint32(fh.UncompressedSize64)
	}

	return w.writeDataDescriptor()
}

func (w *fileWriter) writeDataDescriptor() error {
	if !w.hasDataDescriptor() {
		return nil
	}
	// Write data descriptor. This is more complicated than one would
	// think, see e.g. comments in zipfile.c:putextended() and
	// https://bugs.openjdk.org/browse/JDK-7073588.
	// The approach here is to write 8 byte sizes if needed without
	// adding a zip64 extra in the local header (too late anyway).
	var buf []byte
	if w.isZip64() {
		buf = make([]byte, dataDescriptor64Len)
	} else {
		buf = make([]byte, dataDescriptorLen)
	}
	b := writeBuf(buf)
	b.uint32(dataDescriptorSignature) // de-facto standard, required by OS X
	b.uint32(w.CRC32)
	if w.isZip64() {
		b.uint64(w.CompressedSize64)
		b.uint64(w.UncompressedSize64)
	} else {
		b.uint32(w.CompressedSize)
		b.uint32(w.UncompressedSize)
	}
	_, err := w.zipw.Write(buf)
	return err
}

type countWriter struct {
	w     io.Writer
	count int64
}

func (w *countWriter) Write(p []byte) (int, error) {
	n, err := w.w.Write(p)
	w.count += int64(n)
	return n, err
}

type nopCloser struct {
	io.Writer
}

func (w nopCloser) Close() error {
	return nil
}

type writeBuf []byte

func (b *writeBuf) uint8(v uint8) {
	(*b)[0] = v
	*b = (*b)[1:]
}

func (b *writeBuf) uint16(v uint16) {
	binary.LittleEndian.PutUint16(*b, v)
	*b = (*b)[2:]
}

func (b *writeBuf) uint32(v uint32) {
	binary.LittleEndian.PutUint32(*b, v)
	*b = (*b)[4:]
}

func (b *writeBuf) uint64(v uint64) {
	binary.LittleEndian.PutUint64(*b, v)
	*b = (*b)[8:]
}
