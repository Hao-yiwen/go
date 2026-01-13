// 版权所有 2010 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。
//

/*
Package multipart 实现了 RFC 2046 定义的 MIME multipart 解析。

该实现足以满足 HTTP（RFC 2388）和主流浏览器生成的 multipart 主体的需求。

# 限制

为了防止恶意输入，本包对其处理的 MIME 数据大小设置了限制。

[Reader.NextPart] 和 [Reader.NextRawPart] 将每个 part 的头部数量限制为 10000，
[Reader.ReadForm] 将所有 FileHeaders 中的头部总数限制为 10000。
这些限制可以通过 GODEBUG=multipartmaxheaders=<values> 设置进行调整。

Reader.ReadForm 还将表单中的 part 数量限制为 1000。
此限制可以通过 GODEBUG=multipartmaxparts=<value> 设置进行调整。
*/
package multipart

import (
	"bufio"
	"bytes"
	"fmt"
	"internal/godebug"
	"io"
	"mime"
	"mime/quotedprintable"
	"net/textproto"
	"path/filepath"
	"strconv"
	"strings"
)

var emptyParams = make(map[string]string)

// 这个常量至少需要为 76 才能使本包正常工作。
// 这是因为 \r\n--separator_of_len_70- 会填满缓冲区，
// 从中消费单个字节将不安全。
const peekBufferSize = 4096

// Part 表示 multipart 主体中的单个部分。
type Part struct {
	// 主体的头部（如果有），键的规范化方式与 Go http.Request 头部相同。
	// 例如，"foo-bar" 会变为 "Foo-Bar"
	Header textproto.MIMEHeader

	mr *Reader

	disposition       string
	dispositionParams map[string]string

	// r 要么是直接从 mr 读取的 reader，要么是这种 reader 的包装器，
	// 用于解码 Content-Transfer-Encoding
	r io.Reader

	n       int   // 在 mr.bufReader 中等待的已知数据字节数
	total   int64 // 已读取的总数据字节数
	err     error // 当 n == 0 时返回的错误
	readErr error // 从 mr.bufReader 观察到的读取错误
}

// FormName 如果 p 的 Content-Disposition 类型为 "form-data"，则返回 name 参数。
// 否则返回空字符串。
func (p *Part) FormName() string {
	// 有关 Content-Disposition 值格式的 EBNF，参见 https://tools.ietf.org/html/rfc2183 第 2 节。
	if p.dispositionParams == nil {
		p.parseContentDisposition()
	}
	if p.disposition != "form-data" {
		return ""
	}
	return p.dispositionParams["name"]
}

// FileName 返回 [Part] 的 Content-Disposition 头的 filename 参数。
// 如果不为空，filename 在返回前会通过 filepath.Base（与平台相关）处理。
func (p *Part) FileName() string {
	if p.dispositionParams == nil {
		p.parseContentDisposition()
	}
	filename := p.dispositionParams["filename"]
	if filename == "" {
		return ""
	}
	// RFC 7578 第 4.2 节要求，如果提供了 filename，则不得使用目录路径信息。
	return filepath.Base(filename)
}

func (p *Part) parseContentDisposition() {
	v := p.Header.Get("Content-Disposition")
	var err error
	p.disposition, p.dispositionParams, err = mime.ParseMediaType(v)
	if err != nil {
		p.dispositionParams = emptyParams
	}
}

// NewReader 创建一个新的 multipart [Reader]，使用给定的 MIME boundary 从 r 读取。
//
// boundary 通常从消息的 "Content-Type" 头的 "boundary" 参数获取。
// 使用 [mime.ParseMediaType] 来解析此类头。
func NewReader(r io.Reader, boundary string) *Reader {
	b := []byte("\r\n--" + boundary + "--")
	return &Reader{
		bufReader:        bufio.NewReaderSize(&stickyErrorReader{r: r}, peekBufferSize),
		nl:               b[:2],
		nlDashBoundary:   b[:len(b)-2],
		dashBoundaryDash: b[2:],
		dashBoundary:     b[2 : len(b)-2],
	}
}

// stickyErrorReader 是一个 io.Reader，一旦遇到错误就不再调用底层 Reader 的 Read。
// （io.Reader 接口的契约对错误后的 Read 调用返回值没有任何承诺，
// 但本包确实在错误后执行多次 Read）
type stickyErrorReader struct {
	r   io.Reader
	err error
}

func (r *stickyErrorReader) Read(p []byte) (n int, _ error) {
	if r.err != nil {
		return 0, r.err
	}
	n, r.err = r.r.Read(p)
	return n, r.err
}

func newPart(mr *Reader, rawPart bool, maxMIMEHeaderSize, maxMIMEHeaders int64) (*Part, error) {
	bp := &Part{
		Header: make(map[string][]string),
		mr:     mr,
	}
	if err := bp.populateHeaders(maxMIMEHeaderSize, maxMIMEHeaders); err != nil {
		return nil, err
	}
	bp.r = partReader{bp}

	// rawPart 用于在 Part.NextPart 和 Part.NextRawPart 之间切换。
	if !rawPart {
		const cte = "Content-Transfer-Encoding"
		if strings.EqualFold(bp.Header.Get(cte), "quoted-printable") {
			bp.Header.Del(cte)
			bp.r = quotedprintable.NewReader(bp.r)
		}
	}
	return bp, nil
}

func (p *Part) populateHeaders(maxMIMEHeaderSize, maxMIMEHeaders int64) error {
	r := textproto.NewReader(p.mr.bufReader)
	header, err := readMIMEHeader(r, maxMIMEHeaderSize, maxMIMEHeaders)
	if err == nil {
		p.Header = header
	}
	// TODO: 向 net/textproto 添加可区分的错误。
	if err != nil && err.Error() == "message too large" {
		err = ErrMessageTooLarge
	}
	return err
}

// Read 读取 part 的主体，在其头部之后、下一个 part（如果有）开始之前。
func (p *Part) Read(d []byte) (n int, err error) {
	return p.r.Read(d)
}

// partReader 通过直接从包装的 *Part 读取原始字节来实现 io.Reader，
// 不进行任何 Transfer-Encoding 解码。
type partReader struct {
	p *Part
}

func (pr partReader) Read(d []byte) (int, error) {
	p := pr.p
	br := p.mr.bufReader

	// 读取到缓冲区，直到识别出要返回的数据，
	// 或者找到停止的理由（boundary 或读取错误）。
	for p.n == 0 && p.err == nil {
		peek, _ := br.Peek(br.Buffered())
		p.n, p.err = scanUntilBoundary(peek, p.mr.dashBoundary, p.mr.nlDashBoundary, p.total, p.readErr)
		if p.n == 0 && p.err == nil {
			// 强制缓冲 I/O 读取更多数据到缓冲区。
			_, p.readErr = br.Peek(len(peek) + 1)
			if p.readErr == io.EOF {
				p.readErr = io.ErrUnexpectedEOF
			}
		}
	}

	// 从缓冲区的"要返回的数据"部分读出。
	if p.n == 0 {
		return 0, p.err
	}
	n := len(d)
	if n > p.n {
		n = p.n
	}
	n, _ = br.Read(d[:n])
	p.total += int64(n)
	p.n -= n
	if p.n == 0 {
		return n, p.err
	}
	return n, nil
}

// scanUntilBoundary 扫描 buf 以确定其中有多少可以安全地作为 Part 主体的一部分返回。
// dashBoundary 是 "--boundary"。
// nlDashBoundary 是 "\r\n--boundary" 或 "\n--boundary"，取决于我们所处的模式。
// 下面的注释（和名称）假设是 "\n--boundary"，但两者都可接受。
// total 是到目前为止读出的字节数。如果 total == 0，则识别前导的 "--boundary"。
// readErr 是读取 buf 中字节后的读取错误（如果有）。
// scanUntilBoundary 返回可以作为 Part 主体一部分返回的 buf 数据字节数，
// 以及这些数据字节完成后要返回的错误（如果有）。
func scanUntilBoundary(buf, dashBoundary, nlDashBoundary []byte, total int64, readErr error) (int, error) {
	if total == 0 {
		// 在主体开头，允许 dashBoundary。
		if bytes.HasPrefix(buf, dashBoundary) {
			switch matchAfterPrefix(buf, dashBoundary, readErr) {
			case -1:
				return len(dashBoundary), nil
			case 0:
				return 0, nil
			case +1:
				return 0, io.EOF
			}
		}
		if bytes.HasPrefix(dashBoundary, buf) {
			return 0, readErr
		}
	}

	// 搜索 "\n--boundary"。
	if i := bytes.Index(buf, nlDashBoundary); i >= 0 {
		switch matchAfterPrefix(buf[i:], nlDashBoundary, readErr) {
		case -1:
			return i + len(nlDashBoundary), nil
		case 0:
			return i, nil
		case +1:
			return i, io.EOF
		}
	}
	if bytes.HasPrefix(nlDashBoundary, buf) {
		return 0, readErr
	}

	// 否则，直到最后一个 \n 之前的任何内容都不是 boundary 的一部分，
	// 因此必须是主体的一部分。
	// 另外，如果从最后一个 \n 开始的部分不是 boundary 的前缀，
	// 它也必须是主体的一部分。
	i := bytes.LastIndexByte(buf, nlDashBoundary[0])
	if i >= 0 && bytes.HasPrefix(nlDashBoundary, buf[i:]) {
		return i, nil
	}
	return len(buf), readErr
}

// matchAfterPrefix 检查 buf 是否应被视为匹配 boundary。
// prefix 是 "--boundary" 或 "\r\n--boundary" 或 "\n--boundary"，
// 调用者已验证 bytes.HasPrefix(buf, prefix) 为 true。
//
// 如果缓冲区确实匹配 boundary，matchAfterPrefix 返回 +1，
// 意味着 prefix 后面跟着双破折号、空格、制表符、回车、换行或输入结束。
// 如果缓冲区明确不匹配 boundary，返回 -1，
// 意味着 prefix 后面跟着其他字符。
// 例如，"--foobar" 不匹配 "--foo"。
// 如果需要读取更多输入才能做出决定，返回 0，
// 意味着 len(buf) == len(prefix) 且 readErr == nil。
func matchAfterPrefix(buf, prefix []byte, readErr error) int {
	if len(buf) == len(prefix) {
		if readErr != nil {
			return +1
		}
		return 0
	}
	c := buf[len(prefix)]

	if c == ' ' || c == '\t' || c == '\r' || c == '\n' {
		return +1
	}

	// 尝试检测 boundaryDash
	if c == '-' {
		if len(buf) == len(prefix)+1 {
			if readErr != nil {
				// Prefix + "-" 不匹配
				return -1
			}
			return 0
		}
		if buf[len(prefix)+1] == '-' {
			return +1
		}
	}

	return -1
}

func (p *Part) Close() error {
	io.Copy(io.Discard, p)
	return nil
}

// Reader 是 MIME multipart 主体中各部分的迭代器。
// Reader 的底层解析器根据需要消费其输入。不支持 Seeking。
type Reader struct {
	bufReader *bufio.Reader
	tempDir   string // 用于测试

	currentPart *Part
	partsRead   int

	nl               []byte // "\r\n" 或 "\n"（在看到第一个 boundary 行后设置）
	nlDashBoundary   []byte // nl + "--boundary"
	dashBoundaryDash []byte // "--boundary--"
	dashBoundary     []byte // "--boundary"
}

// maxMIMEHeaderSize 是我们将解析的 MIME 头的最大大小，
// 包括头键、值和 map 开销。
const maxMIMEHeaderSize = 10 << 20

// multipartmaxheaders 是 NextPart 将返回的最大头条目数，
// 也是 Reader.ReadForm 在 FileHeaders 中返回的头条目的最大组合总数。
var multipartmaxheaders = godebug.New("multipartmaxheaders")

func maxMIMEHeaders() int64 {
	if s := multipartmaxheaders.Value(); s != "" {
		if v, err := strconv.ParseInt(s, 10, 64); err == nil && v >= 0 {
			multipartmaxheaders.IncNonDefault()
			return v
		}
	}
	return 10000
}

// NextPart 返回 multipart 中的下一个 part 或错误。
// 当没有更多 part 时，返回错误 [io.EOF]。
//
// 作为特例，如果 "Content-Transfer-Encoding" 头的值为 "quoted-printable"，
// 则隐藏该头，并在 Read 调用期间透明地解码主体。
func (r *Reader) NextPart() (*Part, error) {
	return r.nextPart(false, maxMIMEHeaderSize, maxMIMEHeaders())
}

// NextRawPart 返回 multipart 中的下一个 part 或错误。
// 当没有更多 part 时，返回错误 [io.EOF]。
//
// 与 [Reader.NextPart] 不同，它不对 "Content-Transfer-Encoding: quoted-printable" 进行特殊处理。
func (r *Reader) NextRawPart() (*Part, error) {
	return r.nextPart(true, maxMIMEHeaderSize, maxMIMEHeaders())
}

func (r *Reader) nextPart(rawPart bool, maxMIMEHeaderSize, maxMIMEHeaders int64) (*Part, error) {
	if r.currentPart != nil {
		r.currentPart.Close()
	}
	if string(r.dashBoundary) == "--" {
		return nil, fmt.Errorf("multipart: boundary is empty")
	}
	expectNewPart := false
	for {
		line, err := r.bufReader.ReadSlice('\n')

		if err == io.EOF && r.isFinalBoundary(line) {
			// 如果缓冲区以 "--boundary--" 结束但没有尾随的 "\r\n"，
			// ReadSlice 将返回错误（因为缺少 '\n'），但这是有效的
			// multipart EOF，所以我们需要返回 io.EOF 而不是 fmt 包装的错误。
			return nil, io.EOF
		}
		if err != nil {
			return nil, fmt.Errorf("multipart: NextPart: %w", err)
		}

		if r.isBoundaryDelimiterLine(line) {
			r.partsRead++
			bp, err := newPart(r, rawPart, maxMIMEHeaderSize, maxMIMEHeaders)
			if err != nil {
				return nil, err
			}
			r.currentPart = bp
			return bp, nil
		}

		if r.isFinalBoundary(line) {
			// 预期的 EOF
			return nil, io.EOF
		}

		if expectNewPart {
			return nil, fmt.Errorf("multipart: expecting a new Part; got line %q", string(line))
		}

		if r.partsRead == 0 {
			// 跳过这一行
			continue
		}

		// 消费前一个 part 的主体和我们现在期望跟随的 boundary 行
		// 之间的 "\n" 或 "\r\n" 分隔符。（新 part 或结束 boundary）
		if bytes.Equal(line, r.nl) {
			expectNewPart = true
			continue
		}

		return nil, fmt.Errorf("multipart: unexpected line in Next(): %q", line)
	}
}

// isFinalBoundary 报告 line 是否是表示所有 part 都已结束的最终 boundary 行。
// 它匹配 `^--boundary--[ \t]*(\r\n)?$`
func (r *Reader) isFinalBoundary(line []byte) bool {
	if !bytes.HasPrefix(line, r.dashBoundaryDash) {
		return false
	}
	rest := line[len(r.dashBoundaryDash):]
	rest = skipLWSPChar(rest)
	return len(rest) == 0 || bytes.Equal(rest, r.nl)
}

func (r *Reader) isBoundaryDelimiterLine(line []byte) (ret bool) {
	// https://tools.ietf.org/html/rfc2046#section-5.1
	//   boundary 分隔行定义为完全由两个连字符（"-"，十进制值 45）组成的行，
	//   后跟 Content-Type 头字段中的 boundary 参数值、
	//   可选的线性空白和终止的 CRLF。
	if !bytes.HasPrefix(line, r.dashBoundary) {
		return false
	}
	rest := line[len(r.dashBoundary):]
	rest = skipLWSPChar(rest)

	// 在第一个 part，查看我们的行是否以 \n 而不是 \r\n 结束，
	// 如果是则切换到该模式。这违反了规范，但在实践中会发生。
	if r.partsRead == 0 && len(rest) == 1 && rest[0] == '\n' {
		r.nl = r.nl[1:]
		r.nlDashBoundary = r.nlDashBoundary[1:]
	}
	return bytes.Equal(rest, r.nl)
}

// skipLWSPChar 返回移除前导空格和制表符后的 b。
// RFC 822 定义：
//
//	LWSP-char = SPACE / HTAB
func skipLWSPChar(b []byte) []byte {
	for len(b) > 0 && (b[0] == ' ' || b[0] == '\t') {
		b = b[1:]
	}
	return b
}
