// 版权所有 2010 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package textproto

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"sync"
	_ "unsafe" // 用于 linkname
)

// TODO: 这应该是一个可区分的错误（ErrMessageTooLarge）
// 以允许 mime/multipart 检测它。
var errMessageTooLarge = errors.New("message too large")

// Reader 实现了从文本协议网络连接读取请求
// 或响应的便捷方法。
type Reader struct {
	R   *bufio.Reader
	dot *dotReader
	buf []byte // readContinuedLineSlice 的可重用缓冲区
}

// NewReader 返回一个新的 [Reader]，从 r 读取。
//
// 为了避免拒绝服务攻击，提供的 [bufio.Reader]
// 应该从 [io.LimitReader] 或类似的 Reader 读取，以限制
// 响应的大小。
func NewReader(r *bufio.Reader) *Reader {
	return &Reader{R: r}
}

// ReadLine 从 r 读取单行，
// 从返回的字符串中省略最后的 \n 或 \r\n。
func (r *Reader) ReadLine() (string, error) {
	line, err := r.readLineSlice(-1)
	return string(line), err
}

// ReadLineBytes 类似于 [Reader.ReadLine]，但返回 []byte 而不是字符串。
func (r *Reader) ReadLineBytes() ([]byte, error) {
	line, err := r.readLineSlice(-1)
	if line != nil {
		line = bytes.Clone(line)
	}
	return line, err
}

// readLineSlice 从 r 读取单行，
// 最多 lim 字节长（如果 lim 小于 0 则无限制），
// 从返回的字符串中省略最后的 \r 或 \r\n。
func (r *Reader) readLineSlice(lim int64) ([]byte, error) {
	r.closeDot()
	var line []byte
	for {
		l, more, err := r.R.ReadLine()
		if err != nil {
			return nil, err
		}
		if lim >= 0 && int64(len(line))+int64(len(l)) > lim {
			return nil, errMessageTooLarge
		}
		// 如果第一次调用产生了完整行，则避免复制。
		if line == nil && !more {
			return l, nil
		}
		line = append(line, l...)
		if !more {
			break
		}
	}
	return line, nil
}

// ReadContinuedLine 从 r 读取可能的续行，
// 省略最后的尾随 ASCII 空格。
// 第一行之后的行如果以空格或制表符开头，则被视为续行。
// 在返回的数据中，续行与前一行仅由单个空格分隔：
// 换行符和前导空格被删除。
//
// 例如，考虑以下输入：
//
//	Line 1
//	  continued...
//	Line 2
//
// 第一次调用 ReadContinuedLine 将返回 "Line 1 continued..."
// 第二次调用将返回 "Line 2"。
//
// 空行永远不会被续接。
func (r *Reader) ReadContinuedLine() (string, error) {
	line, err := r.readContinuedLineSlice(-1, noValidation)
	return string(line), err
}

// trim 返回删除了前导和尾随空格及制表符的 s。
// 它不假设 Unicode 或 UTF-8。
func trim(s []byte) []byte {
	i := 0
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	s = s[i:]
	n := len(s) - 1
	for n >= 0 && (s[n] == ' ' || s[n] == '\t') {
		n--
	}
	return s[:n+1]
}

// ReadContinuedLineBytes 类似于 [Reader.ReadContinuedLine]，但
// 返回 []byte 而不是字符串。
func (r *Reader) ReadContinuedLineBytes() ([]byte, error) {
	line, err := r.readContinuedLineSlice(-1, noValidation)
	if line != nil {
		line = bytes.Clone(line)
	}
	return line, err
}

// readContinuedLineSlice 从读取器缓冲区读取续行，
// 返回包含所有行的字节切片。validateFirstLine 函数
// 在第一个读取行上运行，如果它返回错误，则该
// 错误从 readContinuedLineSlice 返回。
// 它最多读取 lim 字节的数据（如果 lim 小于 0 则无限制）。
func (r *Reader) readContinuedLineSlice(lim int64, validateFirstLine func([]byte) error) ([]byte, error) {
	if validateFirstLine == nil {
		return nil, fmt.Errorf("missing validateFirstLine func")
	}

	// 读取第一行。
	line, err := r.readLineSlice(lim)
	if err != nil {
		return nil, err
	}
	if len(line) == 0 { // 空行 - 无续行
		return line, nil
	}

	if err := validateFirstLine(line); err != nil {
		return nil, err
	}

	// 乐观地假设我们已经开始缓冲下一行
	// 并且它以 ASCII 字母开头（下一个标题键），或者是空行，
	// 这样我们可以避免在内存中复制缓冲数据
	// 并跳过不存在的空格。
	if r.R.Buffered() > 1 {
		peek, _ := r.R.Peek(2)
		if len(peek) > 0 && (isASCIILetter(peek[0]) || peek[0] == '\n') ||
			len(peek) == 2 && peek[0] == '\r' && peek[1] == '\n' {
			return trim(line), nil
		}
	}

	// ReadByte 或下一个 readLineSlice 将刷新读取缓冲区；
	// 将切片复制到 buf 中。
	r.buf = append(r.buf[:0], trim(line)...)

	if lim < 0 {
		lim = math.MaxInt64
	}
	lim -= int64(len(r.buf))

	// 读取续行。
	for r.skipSpace() > 0 {
		r.buf = append(r.buf, ' ')
		if int64(len(r.buf)) >= lim {
			return nil, errMessageTooLarge
		}
		line, err := r.readLineSlice(lim - int64(len(r.buf)))
		if err != nil {
			break
		}
		r.buf = append(r.buf, trim(line)...)
	}
	return r.buf, nil
}

// skipSpace 跳过 R 上的所有空格并返回跳过的字节数。
func (r *Reader) skipSpace() int {
	n := 0
	for {
		c, err := r.R.ReadByte()
		if err != nil {
			// Bufio 将在下一次读取之前保持错误。
			break
		}
		if c != ' ' && c != '\t' {
			r.R.UnreadByte()
			break
		}
		n++
	}
	return n
}

func (r *Reader) readCodeLine(expectCode int) (code int, continued bool, message string, err error) {
	line, err := r.ReadLine()
	if err != nil {
		return
	}
	return parseCodeLine(line, expectCode)
}

func parseCodeLine(line string, expectCode int) (code int, continued bool, message string, err error) {
	if len(line) < 4 || line[3] != ' ' && line[3] != '-' {
		err = ProtocolError("short response: " + line)
		return
	}
	continued = line[3] == '-'
	code, err = strconv.Atoi(line[0:3])
	if err != nil || code < 100 {
		err = ProtocolError("invalid response code: " + line)
		return
	}
	message = line[4:]
	if 1 <= expectCode && expectCode < 10 && code/100 != expectCode ||
		10 <= expectCode && expectCode < 100 && code/10 != expectCode ||
		100 <= expectCode && expectCode < 1000 && code != expectCode {
		err = &Error{code, message}
	}
	return
}

// ReadCodeLine 读取形式为以下的响应代码行
//
//	code message
//
// 其中 code 是三位数状态代码，message
// 扩展到行的其余部分。这样一行的示例是：
//
//	220 plan9.bell-labs.com ESMTP
//
// 如果状态的前缀与 expectCode 中的数字不匹配，
// ReadCodeLine 返回错误设置为 &Error{code, message}。
// 例如，如果 expectCode 是 31，如果
// 状态不在范围 [310,319] 内，将返回错误。
//
// 如果响应是多行的，ReadCodeLine 返回错误。
//
// expectCode <= 0 禁用状态代码检查。
func (r *Reader) ReadCodeLine(expectCode int) (code int, message string, err error) {
	code, continued, message, err := r.readCodeLine(expectCode)
	if err == nil && continued {
		err = ProtocolError("unexpected multi-line response: " + message)
	}
	return
}

// ReadResponse 读取以下形式的多行响应：
//
//	code-message line 1
//	code-message line 2
//	...
//	code message line n
//
// 其中 code 是三位数状态代码。第一行以
// 代码和连字符开头。响应由以
// 相同代码后跟空格的行终止。消息中的每一行
// 由换行符 (\n) 分隔。
//
// 有关接受的响应的另一种形式的详细信息，请参阅
// RFC 959 (https://www.ietf.org/rfc/rfc959.txt) 的第 36 页：
//
//	code-message line 1
//	message line 2
//	...
//	code message line n
//
// 如果状态的前缀与 expectCode 中的数字不匹配，
// ReadResponse 返回错误设置为 &Error{code, message}。
// 例如，如果 expectCode 是 31，如果
// 状态不在范围 [310,319] 内，将返回错误。
//
// expectCode <= 0 禁用状态代码检查。
func (r *Reader) ReadResponse(expectCode int) (code int, message string, err error) {
	code, continued, first, err := r.readCodeLine(expectCode)
	multi := continued
	var messageBuilder strings.Builder
	messageBuilder.WriteString(first)
	for continued {
		line, err := r.ReadLine()
		if err != nil {
			return 0, "", err
		}

		var code2 int
		var moreMessage string
		code2, continued, moreMessage, err = parseCodeLine(line, 0)
		if err != nil || code2 != code {
			messageBuilder.WriteByte('\n')
			messageBuilder.WriteString(strings.TrimRight(line, "\r\n"))
			continued = true
			continue
		}
		messageBuilder.WriteByte('\n')
		messageBuilder.WriteString(moreMessage)
	}
	message = messageBuilder.String()
	if err != nil && multi && message != "" {
		// 用所有行（完整消息）替换一行错误消息
		err = &Error{code, message}
	}
	return
}

// DotReader 返回一个新的 [Reader]，使用
// 从 r 读取的点编码块的解码文本来满足 Reads。
// 返回的 Reader 仅在下一次调用之前有效
// r 上的方法。
//
// 点编码是文本协议（如 SMTP）中用于数据块的常见框架。
// 数据由一系列行组成，每行以 "\r\n" 结尾。
// 序列本身以包含单个点的行结尾：".\r\n"。
// 以点开头的行使用附加点进行转义，以避免
// 看起来像序列的结尾。
//
// Reader 的 Read 方法返回的解码形式
// 将 "\r\n" 行结尾重写为更简单的 "\n"，
// 删除前导点转义（如果存在），并在消费（和丢弃）
// 行末标记行后使用错误 [io.EOF] 停止。
func (r *Reader) DotReader() io.Reader {
	r.closeDot()
	r.dot = &dotReader{r: r}
	return r.dot
}

type dotReader struct {
	r     *Reader
	state int
}

// Read 通过解码从 d.r 读取的点编码数据来满足读取。
func (d *dotReader) Read(b []byte) (n int, err error) {
	// 通过简单状态机运行数据，以
	// 省略前导点，将尾随 \r\n 重写为 \n，
	// 并检测结尾 .\r\n 行。
	const (
		stateBeginLine = iota // 行首；初始状态；必须为零
		stateDot              // 在行首读取 .
		stateDotCR            // 在行首读取 .\r
		stateCR               // 读取 \r（可能在行尾）
		stateData             // 在行中间读取数据
		stateEOF              // 到达 .\r\n 结束标记行
	)
	br := d.r.R
	for n < len(b) && d.state != stateEOF {
		var c byte
		c, err = br.ReadByte()
		if err != nil {
			if err == io.EOF {
				err = io.ErrUnexpectedEOF
			}
			break
		}
		switch d.state {
		case stateBeginLine:
			if c == '.' {
				d.state = stateDot
				continue
			}
			if c == '\r' {
				d.state = stateCR
				continue
			}
			d.state = stateData

		case stateDot:
			if c == '\r' {
				d.state = stateDotCR
				continue
			}
			if c == '\n' {
				d.state = stateEOF
				continue
			}
			d.state = stateData

		case stateDotCR:
			if c == '\n' {
				d.state = stateEOF
				continue
			}
			// 不是 .\r\n 的一部分。
			// 消费前导点并发出保存的 \r。
			br.UnreadByte()
			c = '\r'
			d.state = stateData

		case stateCR:
			if c == '\n' {
				d.state = stateBeginLine
				break
			}
			// 不是 \r\n 的一部分。发出保存的 \r
			br.UnreadByte()
			c = '\r'
			d.state = stateData

		case stateData:
			if c == '\r' {
				d.state = stateCR
				continue
			}
			if c == '\n' {
				d.state = stateBeginLine
			}
		}
		b[n] = c
		n++
	}
	if err == nil && d.state == stateEOF {
		err = io.EOF
	}
	if err != nil && d.r.dot == d {
		d.r.dot = nil
	}
	return
}

// closeDot 如果存在，耗尽当前 DotReader，
// 确保它读取直到结尾点行。
func (r *Reader) closeDot() {
	if r.dot == nil {
		return
	}
	buf := make([]byte, 128)
	for r.dot != nil {
		// 当 Read 到达 EOF 或错误时，
		// 它将设置 r.dot == nil。
		r.dot.Read(buf)
	}
}

// ReadDotBytes 读取点编码并返回解码数据。
//
// 有关点编码的详细信息，请参阅 [Reader.DotReader] 方法的文档。
func (r *Reader) ReadDotBytes() ([]byte, error) {
	return io.ReadAll(r.DotReader())
}

// ReadDotLines 读取点编码并返回包含的切片
// 解码的行，每一行都省略最后的 \r\n 或 \n。
//
// 有关点编码的详细信息，请参阅 [Reader.DotReader] 方法的文档。
func (r *Reader) ReadDotLines() ([]string, error) {
	// 我们可以使用 ReadDotBytes 然后 Split 它，
	// 但一次读取一行避免需要
	// 大的连续内存块，也更简单。
	var v []string
	var err error
	for {
		var line string
		line, err = r.ReadLine()
		if err != nil {
			if err == io.EOF {
				err = io.ErrUnexpectedEOF
			}
			break
		}

		// 点本身标记结尾；否则切掉一个点。
		if len(line) > 0 && line[0] == '.' {
			if len(line) == 1 {
				break
			}
			line = line[1:]
		}
		v = append(v, line)
	}
	return v, err
}

var colon = []byte(":")

// ReadMIMEHeader 从 r 读取 MIME 风格的标题。
// 标题是可能续接的 Key: Value 行的序列，
// 以空行结尾。
// 返回的映射 m 将 [CanonicalMIMEHeaderKey](key) 映射到
// 值序列，按在输入中遇到的相同顺序。
//
// 例如，考虑以下输入：
//
//	My-Key: Value 1
//	Long-Key: Even
//	       Longer Value
//	My-Key: Value 2
//
// 给定该输入，ReadMIMEHeader 返回映射：
//
//	map[string][]string{
//		"My-Key": {"Value 1", "Value 2"},
//		"Long-Key": {"Even Longer Value"},
//	}
func (r *Reader) ReadMIMEHeader() (MIMEHeader, error) {
	return readMIMEHeader(r, math.MaxInt64, math.MaxInt64)
}

// readMIMEHeader 从 mime/multipart 访问。
//go:linkname readMIMEHeader

// readMIMEHeader 是 ReadMIMEHeader 的一个版本，对标题大小进行限制。
// 它由 mime/multipart 包调用。
func readMIMEHeader(r *Reader, maxMemory, maxHeaders int64) (MIMEHeader, error) {
	// 避免稍后进行大量小切片分配，方法是提前分配一个
	// 大的切片，我们将其分割成更小的
	// 切片。如果稍后这还不够大，我们分配小的。
	var strs []string
	hint := r.upcomingHeaderKeys()
	if hint > 0 {
		if hint > 1000 {
			hint = 1000 // 设置上限以避免过度分配
		}
		strs = make([]string, hint)
	}

	m := make(MIMEHeader, hint)

	// 说明 MIMEHeader 的 400 字节开销，加上每个条目 200 字节。
	// go1.20 时的基准测试，一个单条目 MIMEHeader 是 416 字节，大
	// MIMEHeaders 平均每个条目约 200 字节。
	maxMemory -= 400
	const mapEntryOverhead = 200

	// 第一行不能以前导空格开头。
	if buf, err := r.R.Peek(1); err == nil && (buf[0] == ' ' || buf[0] == '\t') {
		const errorLimit = 80 // 我们引用多少行的任意限制
		line, err := r.readLineSlice(errorLimit)
		if err != nil {
			return m, err
		}
		return m, ProtocolError("malformed MIME header initial line: " + string(line))
	}

	for {
		kv, err := r.readContinuedLineSlice(maxMemory, mustHaveFieldNameColon)
		if len(kv) == 0 {
			return m, err
		}

		// 键在第一个冒号处结束。
		k, v, ok := bytes.Cut(kv, colon)
		if !ok {
			return m, ProtocolError("malformed MIME header line: " + string(kv))
		}
		key, ok := canonicalMIMEHeaderKey(k)
		if !ok {
			return m, ProtocolError("malformed MIME header line: " + string(kv))
		}
		for _, c := range v {
			if !validHeaderValueByte(c) {
				return m, ProtocolError("malformed MIME header line: " + string(kv))
			}
		}

		maxHeaders--
		if maxHeaders < 0 {
			return nil, errMessageTooLarge
		}

		// 跳过值中的初始空格。
		value := string(bytes.TrimLeft(v, " \t"))

		vv := m[key]
		if vv == nil {
			maxMemory -= int64(len(key))
			maxMemory -= mapEntryOverhead
		}
		maxMemory -= int64(len(value))
		if maxMemory < 0 {
			return m, errMessageTooLarge
		}
		if vv == nil && len(strs) > 0 {
			// 很可能这将是单元素键。
			// 大多数标题不是多值的。
			// 在 strs[0] 上设置容量为 1，因此任何未来的 append
			// 不会将切片扩展到其他字符串中。
			vv, strs = strs[:1:1], strs[1:]
			vv[0] = value
			m[key] = vv
		} else {
			m[key] = append(vv, value)
		}

		if err != nil {
			return m, err
		}
	}
}

// noValidation 是 readContinuedLineSlice 的空操作验证函数，
// 允许任何行。
func noValidation(_ []byte) error { return nil }

// mustHaveFieldNameColon 确保根据 RFC 7230，
// field-name 在单一行上，因此第一行必须
// 包含冒号。
func mustHaveFieldNameColon(line []byte) error {
	if bytes.IndexByte(line, ':') < 0 {
		return ProtocolError(fmt.Sprintf("malformed MIME header: missing colon: %q", line))
	}
	return nil
}

var nl = []byte("\n")

// upcomingHeaderKeys 返回这个标题中将包含的键数的近似值。
// 如果它感到困惑，则返回 0。
func (r *Reader) upcomingHeaderKeys() (n int) {
	// 尝试确定"提示"大小。
	r.R.Peek(1) // 如果为空，强制加载缓冲区
	s := r.R.Buffered()
	if s == 0 {
		return
	}
	peek, _ := r.R.Peek(s)
	for len(peek) > 0 && n < 1000 {
		var line []byte
		line, peek, _ = bytes.Cut(peek, nl)
		if len(line) == 0 || (len(line) == 1 && line[0] == '\r') {
			// 分隔标题和正文的空行。
			break
		}
		if line[0] == ' ' || line[0] == '\t' {
			// 前一行的折叠续行。
			continue
		}
		n++
	}
	return n
}

// CanonicalMIMEHeaderKey 返回 MIME 标题键 s 的规范格式。
// 规范化将第一个字母和任何跟在连字符后的字母
// 转换为大写；其余的转换为小写。
// 例如，"accept-encoding" 的规范键是 "Accept-Encoding"。
// MIME 标题键假定仅为 ASCII。
// 如果 s 包含空格或无效的标题字段字节，如
// RFC 9112 所定义的，则返回不经修改的值。
func CanonicalMIMEHeaderKey(s string) string {
	// 快速检查规范编码。
	upper := true
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !validHeaderFieldByte(c) {
			return s
		}
		if upper && 'a' <= c && c <= 'z' {
			s, _ = canonicalMIMEHeaderKey([]byte(s))
			return s
		}
		if !upper && 'A' <= c && c <= 'Z' {
			s, _ = canonicalMIMEHeaderKey([]byte(s))
			return s
		}
		upper = c == '-'
	}
	return s
}

const toLower = 'a' - 'A'

// validHeaderFieldByte 报告 c 是否是标题字段名称中的有效字节。
// RFC 7230 说：
//
//	header-field   = field-name ":" OWS field-value OWS
//	field-name     = token
//	tchar = "!" / "#" / "$" / "%" / "&" / "'" / "*" / "+" / "-" / "." /
//	        "^" / "_" / "`" / "|" / "~" / DIGIT / ALPHA
//	token = 1*tchar
func validHeaderFieldByte(c byte) bool {
	// mask 是一个 128 位位图，其中 1s 表示允许的字节，
	// 以便可以使用移位和与来测试字节 c。
	// 如果 c >= 128，则 1<<c 和 1<<(c-64) 都将为零，
	// 此函数将返回 false。
	const mask = 0 |
		(1<<(10)-1)<<'0' |
		(1<<(26)-1)<<'a' |
		(1<<(26)-1)<<'A' |
		1<<'!' |
		1<<'#' |
		1<<'$' |
		1<<'%' |
		1<<'&' |
		1<<'\'' |
		1<<'*' |
		1<<'+' |
		1<<'-' |
		1<<'.' |
		1<<'^' |
		1<<'_' |
		1<<'`' |
		1<<'|' |
		1<<'~'
	return ((uint64(1)<<c)&(mask&(1<<64-1)) |
		(uint64(1)<<(c-64))&(mask>>64)) != 0
}

// validHeaderValueByte 报告 c 是否是标题字段值中的有效字节。
// RFC 7230 说：
//
//	field-content  = field-vchar [ 1*( SP / HTAB ) field-vchar ]
//	field-vchar    = VCHAR / obs-text
//	obs-text       = %x80-FF
//
// RFC 5234 说：
//
//	HTAB           =  %x09
//	SP             =  %x20
//	VCHAR          =  %x21-7E
func validHeaderValueByte(c byte) bool {
	// mask 是一个 128 位位图，其中 1s 表示允许的字节，
	// 以便可以使用移位和与来测试字节 c。
	// 如果 c >= 128，则 1<<c 和 1<<(c-64) 都将为零。
	// 由于这是 obs-text 范围，我们反转掩码以
	// 创建一个 1s 表示不允许字节的位图。
	const mask = 0 |
		(1<<(0x7f-0x21)-1)<<0x21 | // VCHAR: %x21-7E
		1<<0x20 | // SP: %x20
		1<<0x09 // HTAB: %x09
	return ((uint64(1)<<c)&^(mask&(1<<64-1)) |
		(uint64(1)<<(c-64))&^(mask>>64)) == 0
}

// canonicalMIMEHeaderKey 类似于 CanonicalMIMEHeaderKey，但允许
// 在返回字符串之前改变提供的字节切片。
//
// 对于无效输入（如果 a 包含空格或非令牌字节），a
// 保持不变，返回字符串副本。
//
// 如果标题键仅包含有效字符和空格，则 ok 为真。
// ReadMIMEHeader 接受包含空格的标题键，但不会
// 规范化它们。
func canonicalMIMEHeaderKey(a []byte) (_ string, ok bool) {
	if len(a) == 0 {
		return "", false
	}

	// 查看 a 是否看起来像标题键。如果没有，按原样返回。
	noCanon := false
	for _, c := range a {
		if validHeaderFieldByte(c) {
			continue
		}
		// 不要规范化。
		if c == ' ' {
			// 我们接受冒号前带空格的无效标题，
			// 但必须不规范化它们。
			// 参见 https://go.dev/issue/34540。
			noCanon = true
			continue
		}
		return string(a), false
	}
	if noCanon {
		return string(a), true
	}

	upper := true
	for i, c := range a {
		// 规范化：第一个字母大写
		// 以及每个破折号后的大写字母。
		// (Host, User-Agent, If-Modified-Since).
		// MIME 标题仅为 ASCII，因此没有 Unicode 问题。
		if upper && 'a' <= c && c <= 'z' {
			c -= toLower
		} else if !upper && 'A' <= c && c <= 'Z' {
			c += toLower
		}
		a[i] = c
		upper = c == '-' // 用于下一次
	}
	commonHeaderOnce.Do(initCommonHeader)
	// 编译器将 m[string(byteSlice)] 识别为特殊
	// 情况，因此 a 的字节复制到新字符串中不会
	// 在此映射查找中发生：
	if v := commonHeader[string(a)]; v != "" {
		return v, true
	}
	return string(a), true
}

// commonHeader 内部化常见标题字符串。
var commonHeader map[string]string

var commonHeaderOnce sync.Once

func initCommonHeader() {
	commonHeader = make(map[string]string)
	for _, v := range []string{
		"Accept",
		"Accept-Charset",
		"Accept-Encoding",
		"Accept-Language",
		"Accept-Ranges",
		"Cache-Control",
		"Cc",
		"Connection",
		"Content-Id",
		"Content-Language",
		"Content-Length",
		"Content-Transfer-Encoding",
		"Content-Type",
		"Cookie",
		"Date",
		"Dkim-Signature",
		"Etag",
		"Expires",
		"From",
		"Host",
		"If-Modified-Since",
		"If-None-Match",
		"In-Reply-To",
		"Last-Modified",
		"Location",
		"Message-Id",
		"Mime-Version",
		"Pragma",
		"Received",
		"Return-Path",
		"Server",
		"Set-Cookie",
		"Subject",
		"To",
		"User-Agent",
		"Via",
		"X-Forwarded-For",
		"X-Imforwards",
		"X-Powered-By",
	} {
		commonHeader[v] = v
	}
}
