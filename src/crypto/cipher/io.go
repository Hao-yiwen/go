// 版权所有 2010 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package cipher

import "io"

// Stream* 对象非常简单，所有成员都是公开的。用户可以自己创建它们。

// StreamReader 将 [Stream] 包装成 [io.Reader]。
// 它调用 XORKeyStream 来处理通过的每个数据切片。
type StreamReader struct {
	S Stream
	R io.Reader
}

func (r StreamReader) Read(dst []byte) (n int, err error) {
	n, err = r.R.Read(dst)
	r.S.XORKeyStream(dst[:n], dst[:n])
	return
}

// StreamWriter 将 [Stream] 包装成 io.Writer。它调用 XORKeyStream
// 来处理通过的每个数据切片。如果任何 [StreamWriter.Write] 调用返回短写，
// 则 StreamWriter 已失去同步，必须丢弃。
// StreamWriter 没有内部缓冲；不需要调用 [StreamWriter.Close] 来刷新写入数据。
type StreamWriter struct {
	S   Stream
	W   io.Writer
	Err error // 未使用
}

func (w StreamWriter) Write(src []byte) (n int, err error) {
	c := make([]byte, len(src))
	w.S.XORKeyStream(c, src)
	n, err = w.W.Write(c)
	if n != len(src) && err == nil { // 不应该发生
		err = io.ErrShortWrite
	}
	return
}

// Close 关闭底层 Writer 并返回其 Close 返回值（如果 Writer
// 也是 io.Closer）。否则返回 nil。
func (w StreamWriter) Close() error {
	if c, ok := w.W.(io.Closer); ok {
		return c.Close()
	}
	return nil
}
