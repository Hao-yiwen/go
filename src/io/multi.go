// 版权所有 2010 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package io

type eofReader struct{}

func (eofReader) Read([]byte) (int, error) {
	return 0, EOF
}

type multiReader struct {
	readers []Reader
}

func (mr *multiReader) Read(p []byte) (n int, err error) {
	for len(mr.readers) > 0 {
		// 优化：展平嵌套的 multiReaders（Issue 13558）。
		if len(mr.readers) == 1 {
			if r, ok := mr.readers[0].(*multiReader); ok {
				mr.readers = r.readers
				continue
			}
		}
		n, err = mr.readers[0].Read(p)
		if err == EOF {
			// 使用 eofReader 而不是 nil 以避免在执行展平后
			// 发生 nil panic（Issue 18232）。
			mr.readers[0] = eofReader{} // 允许更早的 GC
			mr.readers = mr.readers[1:]
		}
		if n > 0 || err != EOF {
			if err == EOF && len(mr.readers) > 0 {
				// 还不返回 EOF。还有更多 readers。
				err = nil
			}
			return
		}
	}
	return 0, EOF
}

func (mr *multiReader) WriteTo(w Writer) (sum int64, err error) {
	return mr.writeToWithBuffer(w, make([]byte, 1024*32))
}

func (mr *multiReader) writeToWithBuffer(w Writer, buf []byte) (sum int64, err error) {
	for i, r := range mr.readers {
		var n int64
		if subMr, ok := r.(*multiReader); ok { // 与嵌套的 multiReaders 复用缓冲区
			n, err = subMr.writeToWithBuffer(w, buf)
		} else {
			n, err = copyBuffer(w, r, buf)
		}
		sum += n
		if err != nil {
			mr.readers = mr.readers[i:] // 允许在错误后恢复/重试
			return sum, err
		}
		mr.readers[i] = nil // 允许提前 GC
	}
	mr.readers = nil
	return sum, nil
}

var _ WriterTo = (*multiReader)(nil)

// MultiReader 返回一个 Reader，它是所提供输入 readers 的逻辑连接。
// 它们按顺序读取。一旦所有输入都返回 EOF，Read 将返回 EOF。
// 如果任何 readers 返回非 nil、非 EOF 的错误，Read 将返回该错误。
func MultiReader(readers ...Reader) Reader {
	r := make([]Reader, len(readers))
	copy(r, readers)
	return &multiReader{r}
}

type multiWriter struct {
	writers []Writer
}

func (t *multiWriter) Write(p []byte) (n int, err error) {
	for _, w := range t.writers {
		n, err = w.Write(p)
		if err != nil {
			return
		}
		if n != len(p) {
			err = ErrShortWrite
			return
		}
	}
	return len(p), nil
}

var _ StringWriter = (*multiWriter)(nil)

func (t *multiWriter) WriteString(s string) (n int, err error) {
	var p []byte // 在需要时延迟初始化
	for _, w := range t.writers {
		if sw, ok := w.(StringWriter); ok {
			n, err = sw.WriteString(s)
		} else {
			if p == nil {
				p = []byte(s)
			}
			n, err = w.Write(p)
		}
		if err != nil {
			return
		}
		if n != len(s) {
			err = ErrShortWrite
			return
		}
	}
	return len(s), nil
}

// MultiWriter 创建一个 writer，它将其写入复制到所有
// 提供的 writers，类似于 Unix 的 tee(1) 命令。
//
// 每次写入都会依次写入每个列出的 writer。
// 如果某个列出的 writer 返回错误，整个写入操作
// 将停止并返回该错误；它不会继续处理列表中的其余部分。
func MultiWriter(writers ...Writer) Writer {
	allWriters := make([]Writer, 0, len(writers))
	for _, w := range writers {
		if mw, ok := w.(*multiWriter); ok {
			allWriters = append(allWriters, mw.writers...)
		} else {
			allWriters = append(allWriters, w)
		}
	}
	return &multiWriter{allWriters}
}
