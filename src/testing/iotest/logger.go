// 版权所有 2009 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package iotest

import (
	"io"
	"log"
)

type writeLogger struct {
	prefix string
	w      io.Writer
}

func (l *writeLogger) Write(p []byte) (n int, err error) {
	n, err = l.w.Write(p)
	if err != nil {
		log.Printf("%s %x: %v", l.prefix, p[0:n], err)
	} else {
		log.Printf("%s %x", l.prefix, p[0:n])
	}
	return
}

// NewWriteLogger 返回一个表现得像 w 的 writer，除了
// 它记录（使用 [log.Printf]）每次写入到标准错误，
// 打印前缀和写入的十六进制数据。
func NewWriteLogger(prefix string, w io.Writer) io.Writer {
	return &writeLogger{prefix, w}
}

type readLogger struct {
	prefix string
	r      io.Reader
}

func (l *readLogger) Read(p []byte) (n int, err error) {
	n, err = l.r.Read(p)
	if err != nil {
		log.Printf("%s %x: %v", l.prefix, p[0:n], err)
	} else {
		log.Printf("%s %x", l.prefix, p[0:n])
	}
	return
}

// NewReadLogger 返回一个表现得像 r 的 reader，除了
// 它记录（使用 [log.Printf]）每次读取到标准错误，
// 打印前缀和读取的十六进制数据。
func NewReadLogger(prefix string, r io.Reader) io.Reader {
	return &readLogger{prefix, r}
}
