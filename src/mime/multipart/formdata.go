// 版权所有 2011 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package multipart

import (
	"bytes"
	"errors"
	"internal/godebug"
	"io"
	"math"
	"net/textproto"
	"os"
	"strconv"
)

// ErrMessageTooLarge 在消息表单数据太大无法处理时由 ReadForm 返回。
var ErrMessageTooLarge = errors.New("multipart: message too large")

// TODO(adg,bradfitz): 找到一种方法将此处的 DoS 防护策略与 http 包的 ParseForm 统一。

// ReadForm 解析整个 multipart 消息，其 part 的 Content-Disposition 为 "form-data"。
// 它在内存中存储最多 maxMemory 字节 + 10MB（为非文件 part 保留）。
// 无法存储在内存中的文件 part 将存储在磁盘的临时文件中。
// 如果所有非文件 part 无法存储在内存中，则返回 [ErrMessageTooLarge]。
func (r *Reader) ReadForm(maxMemory int64) (*Form, error) {
	return r.readForm(maxMemory)
}

var (
	multipartfiles    = godebug.New("#multipartfiles") // TODO: document and remove #
	multipartmaxparts = godebug.New("multipartmaxparts")
)

func (r *Reader) readForm(maxMemory int64) (_ *Form, err error) {
	form := &Form{make(map[string][]string), make(map[string][]*FileHeader)}
	var (
		file    *os.File
		fileOff int64
	)
	numDiskFiles := 0
	combineFiles := true
	if multipartfiles.Value() == "distinct" {
		combineFiles = false
		// multipartfiles.IncNonDefault() // TODO: uncomment after documenting
	}
	maxParts := 1000
	if s := multipartmaxparts.Value(); s != "" {
		if v, err := strconv.Atoi(s); err == nil && v >= 0 {
			maxParts = v
			multipartmaxparts.IncNonDefault()
		}
	}
	maxHeaders := maxMIMEHeaders()

	defer func() {
		if file != nil {
			if cerr := file.Close(); err == nil {
				err = cerr
			}
		}
		if combineFiles && numDiskFiles > 1 {
			for _, fhs := range form.File {
				for _, fh := range fhs {
					fh.tmpshared = true
				}
			}
		}
		if err != nil {
			form.RemoveAll()
			if file != nil {
				os.Remove(file.Name())
			}
		}
	}()

	// maxFileMemoryBytes 是我们将在内存中存储的文件数据的最大字节数。
	// 超过此限制的数据将写入磁盘。
	// 此限制严格适用于内容，而不是元数据（文件名、MIME 头等），
	// 因为元数据始终存储在内存中，而不是磁盘上。
	//
	// maxMemoryBytes 是我们将在内存中存储的最大字节数，包括文件内容、
	// 非文件 part 值、元数据和 map 条目开销。
	//
	// 我们在 maxMemoryBytes 中为非文件数据额外保留 10 MB。
	//
	// 这些参数之间的关系，以及添加到 maxMemory 的过大且不可配置的 10 MB，
	// 是不幸的，但在文档化的 API 约束内难以更改。
	maxFileMemoryBytes := maxMemory
	if maxFileMemoryBytes == math.MaxInt64 {
		maxFileMemoryBytes--
	}
	maxMemoryBytes := maxMemory + int64(10<<20)
	if maxMemoryBytes <= 0 {
		if maxMemory < 0 {
			maxMemoryBytes = 0
		} else {
			maxMemoryBytes = math.MaxInt64
		}
	}
	var copyBuf []byte
	for {
		p, err := r.nextPart(false, maxMemoryBytes, maxHeaders)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if maxParts <= 0 {
			return nil, ErrMessageTooLarge
		}
		maxParts--

		name := p.FormName()
		if name == "" {
			continue
		}
		filename := p.FileName()

		// 同一键的多个值（一个 map 条目，较长的切片）比不同键的相同数量的值
		// （多个 map 条目）更便宜，但使用一致的每个值开销成本更简单。
		const mapEntryOverhead = 200
		maxMemoryBytes -= int64(len(name))
		maxMemoryBytes -= mapEntryOverhead
		if maxMemoryBytes < 0 {
			// 实际上我们不可能走到这条路径，因为 nextPart 已经拒绝了过大的 MIME 头。
			// 无论如何还是检查一下。
			return nil, ErrMessageTooLarge
		}

		var b bytes.Buffer

		if filename == "" {
			// 值，作为字符串存储在内存中
			n, err := io.CopyN(&b, p, maxMemoryBytes+1)
			if err != nil && err != io.EOF {
				return nil, err
			}
			maxMemoryBytes -= n
			if maxMemoryBytes < 0 {
				return nil, ErrMessageTooLarge
			}
			form.Value[name] = append(form.Value[name], b.String())
			continue
		}

		// 文件，存储在内存或磁盘上
		const fileHeaderSize = 100
		maxMemoryBytes -= mimeHeaderSize(p.Header)
		maxMemoryBytes -= mapEntryOverhead
		maxMemoryBytes -= fileHeaderSize
		if maxMemoryBytes < 0 {
			return nil, ErrMessageTooLarge
		}
		for _, v := range p.Header {
			maxHeaders -= int64(len(v))
		}
		fh := &FileHeader{
			Filename: filename,
			Header:   p.Header,
		}
		n, err := io.CopyN(&b, p, maxFileMemoryBytes+1)
		if err != nil && err != io.EOF {
			return nil, err
		}
		if n > maxFileMemoryBytes {
			if file == nil {
				file, err = os.CreateTemp(r.tempDir, "multipart-")
				if err != nil {
					return nil, err
				}
			}
			numDiskFiles++
			if _, err := file.Write(b.Bytes()); err != nil {
				return nil, err
			}
			if copyBuf == nil {
				copyBuf = make([]byte, 32*1024) // 与 io.Copy 使用相同的缓冲区大小
			}
			// 如果让 io.Copy 使用 os.File.ReadFrom，它会分配自己的复制缓冲区。
			type writerOnly struct{ io.Writer }
			remainingSize, err := io.CopyBuffer(writerOnly{file}, p, copyBuf)
			if err != nil {
				return nil, err
			}
			fh.tmpfile = file.Name()
			fh.Size = int64(b.Len()) + remainingSize
			fh.tmpoff = fileOff
			fileOff += fh.Size
			if !combineFiles {
				if err := file.Close(); err != nil {
					return nil, err
				}
				file = nil
			}
		} else {
			fh.content = b.Bytes()
			fh.Size = int64(len(fh.content))
			maxFileMemoryBytes -= n
			maxMemoryBytes -= n
		}
		form.File[name] = append(form.File[name], fh)
	}

	return form, nil
}

func mimeHeaderSize(h textproto.MIMEHeader) (size int64) {
	size = 400
	for k, vs := range h {
		size += int64(len(k))
		size += 200 // map entry overhead
		for _, v := range vs {
			size += int64(len(v))
		}
	}
	return size
}

// Form 是解析后的 multipart 表单。
// 其 File part 存储在内存或磁盘上，可通过 [*FileHeader] 的 Open 方法访问。
// 其 Value part 存储为字符串。
// 两者都以字段名为键。
type Form struct {
	Value map[string][]string
	File  map[string][]*FileHeader
}

// RemoveAll 删除与 [Form] 关联的所有临时文件。
func (f *Form) RemoveAll() error {
	var err error
	for _, fhs := range f.File {
		for _, fh := range fhs {
			if fh.tmpfile != "" {
				e := os.Remove(fh.tmpfile)
				if e != nil && !errors.Is(e, os.ErrNotExist) && err == nil {
					err = e
				}
			}
		}
	}
	return err
}

// FileHeader 描述 multipart 请求的文件 part。
type FileHeader struct {
	Filename string
	Header   textproto.MIMEHeader
	Size     int64

	content   []byte
	tmpfile   string
	tmpoff    int64
	tmpshared bool
}

// Open 打开并返回 [FileHeader] 关联的 File。
func (fh *FileHeader) Open() (File, error) {
	if b := fh.content; b != nil {
		r := io.NewSectionReader(bytes.NewReader(b), 0, int64(len(b)))
		return sectionReadCloser{r, nil}, nil
	}
	if fh.tmpshared {
		f, err := os.Open(fh.tmpfile)
		if err != nil {
			return nil, err
		}
		r := io.NewSectionReader(f, fh.tmpoff, fh.Size)
		return sectionReadCloser{r, f}, nil
	}
	return os.Open(fh.tmpfile)
}

// File 是访问 multipart 消息的文件 part 的接口。
// 其内容可能存储在内存或磁盘上。
// 如果存储在磁盘上，File 的底层具体类型将是 *os.File。
type File interface {
	io.Reader
	io.ReaderAt
	io.Seeker
	io.Closer
}

// 辅助类型，用于将 []byte 转换为 File

type sectionReadCloser struct {
	*io.SectionReader
	io.Closer
}

func (rc sectionReadCloser) Close() error {
	if rc.Closer != nil {
		return rc.Closer.Close()
	}
	return nil
}
