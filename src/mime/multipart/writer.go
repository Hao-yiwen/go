// 版权所有 2011 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package multipart

import (
	"bytes"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/textproto"
	"slices"
	"strings"
)

// Writer 生成 multipart 消息。
type Writer struct {
	w        io.Writer
	boundary string
	lastpart *part
}

// NewWriter 返回一个带有随机 boundary 的新 multipart [Writer]，写入 w。
func NewWriter(w io.Writer) *Writer {
	return &Writer{
		w:        w,
		boundary: randomBoundary(),
	}
}

// Boundary 返回 [Writer] 的 boundary。
func (w *Writer) Boundary() string {
	return w.boundary
}

// SetBoundary 用显式值覆盖 [Writer] 的默认随机生成的 boundary 分隔符。
//
// SetBoundary 必须在创建任何 part 之前调用，只能包含某些 ASCII 字符，
// 并且必须非空且最多 70 字节长。
func (w *Writer) SetBoundary(boundary string) error {
	if w.lastpart != nil {
		return errors.New("mime: SetBoundary called after write")
	}
	// rfc2046#section-5.1.1
	if len(boundary) < 1 || len(boundary) > 70 {
		return errors.New("mime: invalid boundary length")
	}
	end := len(boundary) - 1
	for i, b := range boundary {
		if 'A' <= b && b <= 'Z' || 'a' <= b && b <= 'z' || '0' <= b && b <= '9' {
			continue
		}
		switch b {
		case '\'', '(', ')', '+', '_', ',', '-', '.', '/', ':', '=', '?':
			continue
		case ' ':
			if i != end {
				continue
			}
		}
		return errors.New("mime: invalid boundary character")
	}
	w.boundary = boundary
	return nil
}

// FormDataContentType 返回 HTTP multipart/form-data 的 Content-Type，
// 使用这个 [Writer] 的 Boundary。
func (w *Writer) FormDataContentType() string {
	b := w.boundary
	// 如果 boundary 包含 RFC 2045 定义的 tspecials 字符或空格，
	// 我们必须对 boundary 加上引号。
	if strings.ContainsAny(b, `()<>@,;:\"/[]?= `) {
		b = `"` + b + `"`
	}
	return "multipart/form-data; boundary=" + b
}

func randomBoundary() string {
	var buf [30]byte
	_, err := io.ReadFull(rand.Reader, buf[:])
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf("%x", buf[:])
}

// CreatePart 使用提供的头创建新的 multipart 部分。
// part 的主体应该写入返回的 [Writer]。
// 调用 CreatePart 后，任何先前的 part 将无法再进行写入。
func (w *Writer) CreatePart(header textproto.MIMEHeader) (io.Writer, error) {
	if w.lastpart != nil {
		if err := w.lastpart.close(); err != nil {
			return nil, err
		}
	}
	var b bytes.Buffer
	if w.lastpart != nil {
		fmt.Fprintf(&b, "\r\n--%s\r\n", w.boundary)
	} else {
		fmt.Fprintf(&b, "--%s\r\n", w.boundary)
	}

	for _, k := range slices.Sorted(maps.Keys(header)) {
		for _, v := range header[k] {
			fmt.Fprintf(&b, "%s: %s\r\n", k, v)
		}
	}
	fmt.Fprintf(&b, "\r\n")
	_, err := io.Copy(w.w, &b)
	if err != nil {
		return nil, err
	}
	p := &part{
		mw: w,
	}
	w.lastpart = p
	return p, nil
}

var quoteEscaper = strings.NewReplacer("\\", "\\\\", `"`, "\\\"", "\r", "%0D", "\n", "%0A")

// escapeQuotes 转义字段参数值中的特殊字符。
//
// 由于历史原因，这对 " 和 \ 字符使用 \ 转义，
// 对 CR 和 LF 使用百分号编码。
//
// WhatWG 表单数据编码规范建议我们应该
// 对 "（%22）使用百分号编码，而不应转义 \。
// https://html.spec.whatwg.org/multipage/form-control-infrastructure.html#multipart/form-data-encoding-algorithm
//
// 根据经验，在写这个注释的时候，有必要
// 转义 \ 字符，否则 Chrome（以及可能的其他浏览器）会
// 将未转义的 \ 解释为转义符。
func escapeQuotes(s string) string {
	return quoteEscaper.Replace(s)
}

// CreateFormFile 是 [Writer.CreatePart] 的便利包装器。它使用提供的字段名和文件名
// 创建一个新的 form-data 头。
func (w *Writer) CreateFormFile(fieldname, filename string) (io.Writer, error) {
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", FileContentDisposition(fieldname, filename))
	h.Set("Content-Type", "application/octet-stream")
	return w.CreatePart(h)
}

// CreateFormField 使用给定的字段名调用 [Writer.CreatePart] 和一个头。
func (w *Writer) CreateFormField(fieldname string) (io.Writer, error) {
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition",
		fmt.Sprintf(`form-data; name="%s"`, escapeQuotes(fieldname)))
	return w.CreatePart(h)
}

// FileContentDisposition 返回带有提供的字段名和文件名的 Content-Disposition 头的值。
func FileContentDisposition(fieldname, filename string) string {
	return fmt.Sprintf(`form-data; name="%s"; filename="%s"`,
		escapeQuotes(fieldname), escapeQuotes(filename))
}

// WriteField 调用 [Writer.CreateFormField]，然后写入给定的值。
func (w *Writer) WriteField(fieldname, value string) error {
	p, err := w.CreateFormField(fieldname)
	if err != nil {
		return err
	}
	_, err = p.Write([]byte(value))
	return err
}

// Close 完成 multipart 消息并将尾部 boundary 结束行写入输出。
func (w *Writer) Close() error {
	if w.lastpart != nil {
		if err := w.lastpart.close(); err != nil {
			return err
		}
		w.lastpart = nil
	}
	_, err := fmt.Fprintf(w.w, "\r\n--%s--\r\n", w.boundary)
	return err
}

type part struct {
	mw     *Writer
	closed bool
	we     error // last error that occurred writing
}

func (p *part) close() error {
	p.closed = true
	return p.we
}

func (p *part) Write(d []byte) (n int, err error) {
	if p.closed {
		return 0, errors.New("multipart: can't write to finished part")
	}
	n, err = p.mw.w.Write(d)
	if err != nil {
		p.we = err
	}
	return
}
