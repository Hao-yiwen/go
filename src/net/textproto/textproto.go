// 版权所有 2010 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// Package textproto 为 HTTP、NNTP 和 SMTP 风格的基于文本的请求/响应
// 协议提供通用支持。
//
// 此包强制 RFC 9112 为标题键和值定义的 HTTP/1.1 字符集。
//
// 该包提供：
//
// [Error]，表示来自
// 服务器的数字错误响应。
//
// [Pipeline]，管理客户端中的流水线请求和响应。
//
// [Reader]，读取数字响应代码行，
// key: value 标题，前导空格包裹的行
// 在续行上，以及以
// 行本身上的点结尾的完整文本块。
//
// [Writer]，写入点编码的文本块。
//
// [Conn]，[Reader]、[Writer] 和 [Pipeline] 的便捷打包，用于
// 单个网络连接。
package textproto

import (
	"bufio"
	"fmt"
	"io"
	"net"
)

// Error 表示来自服务器的数字错误响应。
type Error struct {
	Code int
	Msg  string
}

func (e *Error) Error() string {
	return fmt.Sprintf("%03d %s", e.Code, e.Msg)
}

// ProtocolError 描述协议违规，例如
// 无效响应或挂起的连接。
type ProtocolError string

func (p ProtocolError) Error() string {
	return string(p)
}

// Conn 表示文本网络协议连接。
// 它由 [Reader] 和 [Writer] 组成，用于管理 I/O
// 以及 [Pipeline] 在连接上排序并发请求。
// 这些嵌入的类型带有方法；
// 有关详细信息，请参阅这些类型的文档。
type Conn struct {
	Reader
	Writer
	Pipeline
	conn io.ReadWriteCloser
}

// NewConn 返回一个新的 [Conn]，使用 conn 进行 I/O。
func NewConn(conn io.ReadWriteCloser) *Conn {
	return &Conn{
		Reader: Reader{R: bufio.NewReader(conn)},
		Writer: Writer{W: bufio.NewWriter(conn)},
		conn:   conn,
	}
}

// Close 关闭连接。
func (c *Conn) Close() error {
	return c.conn.Close()
}

// Dial 使用 [net.Dial] 连接到给定网络上的给定地址，
// 然后返回连接的新 [Conn]。
func Dial(network, addr string) (*Conn, error) {
	c, err := net.Dial(network, addr)
	if err != nil {
		return nil, err
	}
	return NewConn(c), nil
}

// Cmd 是一个便捷方法，在等待其在流水线中的轮流后发送命令。
// 命令文本是用 args 格式化 format 并追加 \r\n 的结果。
// Cmd 返回命令的 id，用于 StartResponse 和 EndResponse。
//
// 例如，客户端可能运行返回点体的 HELP 命令
// 通过使用：
//
//	id, err := c.Cmd("HELP")
//	if err != nil {
//		return nil, err
//	}
//
//	c.StartResponse(id)
//	defer c.EndResponse(id)
//
//	if _, _, err = c.ReadCodeLine(110); err != nil {
//		return nil, err
//	}
//	text, err := c.ReadDotBytes()
//	if err != nil {
//		return nil, err
//	}
//	return c.ReadCodeLine(250)
func (c *Conn) Cmd(format string, args ...any) (id uint, err error) {
	id = c.Next()
	c.StartRequest(id)
	err = c.PrintfLine(format, args...)
	c.EndRequest(id)
	if err != nil {
		return 0, err
	}
	return id, nil
}

// TrimString 返回 s 去掉前导和尾随 ASCII 空格。
func TrimString(s string) string {
	for len(s) > 0 && isASCIISpace(s[0]) {
		s = s[1:]
	}
	for len(s) > 0 && isASCIISpace(s[len(s)-1]) {
		s = s[:len(s)-1]
	}
	return s
}

// TrimBytes 返回 b 去掉前导和尾随 ASCII 空格。
func TrimBytes(b []byte) []byte {
	for len(b) > 0 && isASCIISpace(b[0]) {
		b = b[1:]
	}
	for len(b) > 0 && isASCIISpace(b[len(b)-1]) {
		b = b[:len(b)-1]
	}
	return b
}

func isASCIISpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

func isASCIILetter(b byte) bool {
	b |= 0x20 // 转换为小写
	return 'a' <= b && b <= 'z'
}
