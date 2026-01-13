// 版权所有 2009 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package iotest

import "io"

// TruncateWriter 返回一个 Writer，它写入到 w
// 但在 n 字节后静默停止。
func TruncateWriter(w io.Writer, n int64) io.Writer {
	return &truncateWriter{w, n}
}

type truncateWriter struct {
	w io.Writer
	n int64
}

func (t *truncateWriter) Write(p []byte) (n int, err error) {
	if t.n <= 0 {
		return len(p), nil
	}
	// 真实写入
	n = len(p)
	if int64(n) > t.n {
		n = int(t.n)
	}
	n, err = t.w.Write(p[0:n])
	t.n -= int64(n)
	if err == nil {
		n = len(p)
	}
	return
}
