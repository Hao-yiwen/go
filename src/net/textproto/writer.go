// 版权所有 2010 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package textproto

import (
	"bufio"
	"fmt"
	"io"
)

// Writer 实现了便捷方法，用于向文本协议网络连接
// 写入请求或响应。
type Writer struct {
	W   *bufio.Writer
	dot *dotWriter
}

// NewWriter 返回一个新的 [Writer]，写入 w。
func NewWriter(w *bufio.Writer) *Writer {
	return &Writer{W: w}
}

var crnl = []byte{'\r', '\n'}
var dotcrnl = []byte{'.', '\r', '\n'}

// PrintfLine 写入格式化的输出，后跟 \r\n。
func (w *Writer) PrintfLine(format string, args ...any) error {
	w.closeDot()
	fmt.Fprintf(w.W, format, args...)
	w.W.Write(crnl)
	return w.W.Flush()
}

// DotWriter 返回一个可用于向 w 写入点编码的写入器。
// 它负责在必要时插入前导点，
// 将行结束 \n 转换为 \r\n，并在关闭
// DotWriter 时添加最终的 .\r\n 行。调用者应在下一次
// 调用 w 上的方法之前关闭 DotWriter。
//
// 有关点编码的详细信息，请参阅 [Reader.DotReader] 方法的文档。
func (w *Writer) DotWriter() io.WriteCloser {
	w.closeDot()
	w.dot = &dotWriter{w: w}
	return w.dot
}

func (w *Writer) closeDot() {
	if w.dot != nil {
		w.dot.Close() // 设置 w.dot = nil
	}
}

type dotWriter struct {
	w     *Writer
	state int
}

const (
	wstateBegin     = iota // 初始状态；必须为零
	wstateBeginLine        // 行首
	wstateCR               // 写入 \r（可能在行尾）
	wstateData             // 在行中间写入数据
)

func (d *dotWriter) Write(b []byte) (n int, err error) {
	bw := d.w.W
	for n < len(b) {
		c := b[n]
		switch d.state {
		case wstateBegin, wstateBeginLine:
			d.state = wstateData
			if c == '.' {
				// 转义前导点
				bw.WriteByte('.')
			}
			fallthrough

		case wstateData:
			if c == '\r' {
				d.state = wstateCR
			}
			if c == '\n' {
				bw.WriteByte('\r')
				d.state = wstateBeginLine
			}

		case wstateCR:
			d.state = wstateData
			if c == '\n' {
				d.state = wstateBeginLine
			}
		}
		if err = bw.WriteByte(c); err != nil {
			break
		}
		n++
	}
	return
}

func (d *dotWriter) Close() error {
	if d.w.dot == d {
		d.w.dot = nil
	}
	bw := d.w.W
	switch d.state {
	default:
		bw.WriteByte('\r')
		fallthrough
	case wstateCR:
		bw.WriteByte('\n')
		fallthrough
	case wstateBeginLine:
		bw.Write(dotcrnl)
	}
	return bw.Flush()
}
