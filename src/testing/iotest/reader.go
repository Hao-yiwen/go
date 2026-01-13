// 版权所有 2009 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// Package iotest 实现了主要用于测试的 Readers 和 Writers。
package iotest

import (
	"bytes"
	"errors"
	"fmt"
	"io"
)

// OneByteReader 返回一个 Reader，每次非空 Read 调用时
// 从 r 中读取一个字节。
func OneByteReader(r io.Reader) io.Reader { return &oneByteReader{r} }

type oneByteReader struct {
	r io.Reader
}

func (r *oneByteReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	return r.r.Read(p[0:1])
}

// HalfReader 返回一个 Reader，它通过从 r 中
// 读取一半的请求字节来实现 Read。
func HalfReader(r io.Reader) io.Reader { return &halfReader{r} }

type halfReader struct {
	r io.Reader
}

func (r *halfReader) Read(p []byte) (int, error) {
	return r.r.Read(p[0 : (len(p)+1)/2])
}

// DataErrReader 改变了 Reader 处理错误的方式。通常，
// Reader 从读取最后一块数据之后的第一次 Read 调用中返回错误（通常是 EOF）。
// DataErrReader 包装一个 Reader 并改变其行为，以便
// 最终错误与最终数据一起返回，而不是在最终数据之后的第一次调用中返回。
func DataErrReader(r io.Reader) io.Reader { return &dataErrReader{r, nil, make([]byte, 1024)} }

type dataErrReader struct {
	r      io.Reader
	unread []byte
	data   []byte
}

func (r *dataErrReader) Read(p []byte) (n int, err error) {
	// 循环，因为第一次调用需要两次读取：
	// 一次获取数据，第二次查找错误。
	for {
		if len(r.unread) == 0 {
			n1, err1 := r.r.Read(r.data)
			r.unread = r.data[0:n1]
			err = err1
		}
		if n > 0 || err != nil {
			break
		}
		n = copy(p, r.unread)
		r.unread = r.unread[n:]
	}
	return
}

// ErrTimeout 是一个假的超时错误。
var ErrTimeout = errors.New("timeout")

// TimeoutReader 在第二次读取时返回 [ErrTimeout]
// 且没有数据。随后的读取调用会成功。
func TimeoutReader(r io.Reader) io.Reader { return &timeoutReader{r, 0} }

type timeoutReader struct {
	r     io.Reader
	count int
}

func (r *timeoutReader) Read(p []byte) (int, error) {
	r.count++
	if r.count == 2 {
		return 0, ErrTimeout
	}
	return r.r.Read(p)
}

// ErrReader 返回一个 [io.Reader]，它从所有 Read 调用中返回 0, err。
func ErrReader(err error) io.Reader {
	return &errReader{err: err}
}

type errReader struct {
	err error
}

func (r *errReader) Read(p []byte) (int, error) {
	return 0, r.err
}

type smallByteReader struct {
	r   io.Reader
	off int
	n   int
}

func (r *smallByteReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	r.n = r.n%3 + 1
	n := r.n
	if n > len(p) {
		n = len(p)
	}
	n, err := r.r.Read(p[0:n])
	if err != nil && err != io.EOF {
		err = fmt.Errorf("Read(%d bytes at offset %d): %v", n, r.off, err)
	}
	r.off += n
	return n, err
}

// TestReader 测试从 r 读取是否返回预期的文件内容。
// 它执行不同大小的读取，直到 EOF。
// 如果 r 实现了 [io.ReaderAt] 或 [io.Seeker]，TestReader 也会检查
// 这些操作是否表现正确。
//
// 如果 TestReader 发现任何不当行为，它会返回报告它们的错误。
// 错误文本可能跨越多行。
func TestReader(r io.Reader, content []byte) error {
	if len(content) > 0 {
		n, err := r.Read(nil)
		if n != 0 || err != nil {
			return fmt.Errorf("Read(0) = %d, %v, want 0, nil", n, err)
		}
	}

	data, err := io.ReadAll(&smallByteReader{r: r})
	if err != nil {
		return err
	}
	if !bytes.Equal(data, content) {
		return fmt.Errorf("ReadAll(small amounts) = %q\n\twant %q", data, content)
	}
	n, err := r.Read(make([]byte, 10))
	if n != 0 || err != io.EOF {
		return fmt.Errorf("Read(10) at EOF = %v, %v, want 0, EOF", n, err)
	}

	if r, ok := r.(io.ReadSeeker); ok {
		// Seek(0, 1) 应该报告当前文件位置 (EOF)。
		if off, err := r.Seek(0, 1); off != int64(len(content)) || err != nil {
			return fmt.Errorf("Seek(0, 1) from EOF = %d, %v, want %d, nil", off, err, len(content))
		}

		// 分两步向后寻找文件。
		// 如果 middle == 0，len(content) == 0，不能使用 -1 和 +1 寻找。
		middle := len(content) - len(content)/3
		if middle > 0 {
			if off, err := r.Seek(-1, 1); off != int64(len(content)-1) || err != nil {
				return fmt.Errorf("Seek(-1, 1) from EOF = %d, %v, want %d, nil", -off, err, len(content)-1)
			}
			if off, err := r.Seek(int64(-len(content)/3), 1); off != int64(middle-1) || err != nil {
				return fmt.Errorf("Seek(%d, 1) from %d = %d, %v, want %d, nil", -len(content)/3, len(content)-1, off, err, middle-1)
			}
			if off, err := r.Seek(+1, 1); off != int64(middle) || err != nil {
				return fmt.Errorf("Seek(+1, 1) from %d = %d, %v, want %d, nil", middle-1, off, err, middle)
			}
		}

		// Seek(0, 1) 应该报告当前文件位置 (middle)。
		if off, err := r.Seek(0, 1); off != int64(middle) || err != nil {
			return fmt.Errorf("Seek(0, 1) from %d = %d, %v, want %d, nil", middle, off, err, middle)
		}

		// 向前读取应该返回文件的最后一部分。
		data, err := io.ReadAll(&smallByteReader{r: r})
		if err != nil {
			return fmt.Errorf("ReadAll from offset %d: %v", middle, err)
		}
		if !bytes.Equal(data, content[middle:]) {
			return fmt.Errorf("ReadAll from offset %d = %q\n\twant %q", middle, data, content[middle:])
		}

		// 相对于文件末尾寻找，但从其他位置开始。
		if off, err := r.Seek(int64(middle/2), 0); off != int64(middle/2) || err != nil {
			return fmt.Errorf("Seek(%d, 0) from EOF = %d, %v, want %d, nil", middle/2, off, err, middle/2)
		}
		if off, err := r.Seek(int64(-len(content)/3), 2); off != int64(middle) || err != nil {
			return fmt.Errorf("Seek(%d, 2) from %d = %d, %v, want %d, nil", -len(content)/3, middle/2, off, err, middle)
		}

		// 向前读取应该返回文件的最后一部分（再次）。
		data, err = io.ReadAll(&smallByteReader{r: r})
		if err != nil {
			return fmt.Errorf("ReadAll from offset %d: %v", middle, err)
		}
		if !bytes.Equal(data, content[middle:]) {
			return fmt.Errorf("ReadAll from offset %d = %q\n\twant %q", middle, data, content[middle:])
		}

		// 绝对寻找和向前读取。
		if off, err := r.Seek(int64(middle/2), 0); off != int64(middle/2) || err != nil {
			return fmt.Errorf("Seek(%d, 0) from EOF = %d, %v, want %d, nil", middle/2, off, err, middle/2)
		}
		data, err = io.ReadAll(r)
		if err != nil {
			return fmt.Errorf("ReadAll from offset %d: %v", middle/2, err)
		}
		if !bytes.Equal(data, content[middle/2:]) {
			return fmt.Errorf("ReadAll from offset %d = %q\n\twant %q", middle/2, data, content[middle/2:])
		}
	}

	if r, ok := r.(io.ReaderAt); ok {
		data := make([]byte, len(content), len(content)+1)
		for i := range data {
			data[i] = 0xfe
		}
		n, err := r.ReadAt(data, 0)
		if n != len(data) || err != nil && err != io.EOF {
			return fmt.Errorf("ReadAt(%d, 0) = %v, %v, want %d, nil or EOF", len(data), n, err, len(data))
		}
		if !bytes.Equal(data, content) {
			return fmt.Errorf("ReadAt(%d, 0) = %q\n\twant %q", len(data), data, content)
		}

		n, err = r.ReadAt(data[:1], int64(len(data)))
		if n != 0 || err != io.EOF {
			return fmt.Errorf("ReadAt(1, %d) = %v, %v, want 0, EOF", len(data), n, err)
		}

		for i := range data {
			data[i] = 0xfe
		}
		n, err = r.ReadAt(data[:cap(data)], 0)
		if n != len(data) || err != io.EOF {
			return fmt.Errorf("ReadAt(%d, 0) = %v, %v, want %d, EOF", cap(data), n, err, len(data))
		}
		if !bytes.Equal(data, content) {
			return fmt.Errorf("ReadAt(%d, 0) = %q\n\twant %q", len(data), data, content)
		}

		for i := range data {
			data[i] = 0xfe
		}
		for i := range data {
			n, err = r.ReadAt(data[i:i+1], int64(i))
			if n != 1 || err != nil && (i != len(data)-1 || err != io.EOF) {
				want := "nil"
				if i == len(data)-1 {
					want = "nil or EOF"
				}
				return fmt.Errorf("ReadAt(1, %d) = %v, %v, want 1, %s", i, n, err, want)
			}
			if data[i] != content[i] {
				return fmt.Errorf("ReadAt(1, %d) = %q want %q", i, data[i:i+1], content[i:i+1])
			}
		}
	}
	return nil
}
