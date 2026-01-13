// Copyright 2022 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package saferio 提供避免不必要分配大量
// 内存的 I/O 函数。这是为从 [io.Reader] 读取数据的包设计的，
// 其中大小是输入数据的一部分，但输入可能已损坏或
// 可能由不可信的攻击者提供。
package saferio

import (
	"io"
	"unsafe"
)

// chunk 是我们愿意无需关心分配多少内存的任意限制。
const chunk = 10 << 20 // 10M

// ReadData 从输入流读取 n 个字节，但避免在 n 很大时分配
// 所有 n 个字节。这避免了在 n 不正确的情况下
// 分配所有 n 个字节导致程序崩溃。
//
// 只有在没有读取字节时，错误才是 io.EOF。
// 如果 io.EOF 发生在读取一些但不是所有字节之后，
// ReadData 返回 io.ErrUnexpectedEOF。
func ReadData(r io.Reader, n uint64) ([]byte, error) {
	if int64(n) < 0 || n != uint64(int(n)) {
		// n 太大了，无法放入 int，所以我们无法分配
		// 足够大的缓冲区。将此视为读失败。
		return nil, io.ErrUnexpectedEOF
	}

	if n < chunk {
		buf := make([]byte, n)
		_, err := io.ReadFull(r, buf)
		if err != nil {
			return nil, err
		}
		return buf, nil
	}

	var buf []byte
	buf1 := make([]byte, chunk)
	for n > 0 {
		next := n
		if next > chunk {
			next = chunk
		}
		_, err := io.ReadFull(r, buf1[:next])
		if err != nil {
			if len(buf) > 0 && err == io.EOF {
				err = io.ErrUnexpectedEOF
			}
			return nil, err
		}
		buf = append(buf, buf1[:next]...)
		n -= next
	}
	return buf, nil
}

// ReadDataAt 在偏移量 off 处从输入流读取 n 个字节，但避免在 n 很大时分配
// 所有 n 个字节。这避免了在 n 不正确的情况下
// 分配所有 n 个字节导致程序崩溃。
func ReadDataAt(r io.ReaderAt, n uint64, off int64) ([]byte, error) {
	if int64(n) < 0 || n != uint64(int(n)) {
		// n 太大了，无法放入 int，所以我们无法分配
		// 足够大的缓冲区。将此视为读失败。
		return nil, io.ErrUnexpectedEOF
	}

	if n < chunk {
		buf := make([]byte, n)
		_, err := r.ReadAt(buf, off)
		if err != nil {
			// io.SectionReader 可以为 n == 0 返回 EOF，
			// 但对我们的目的而言这是成功的。
			if err != io.EOF || n > 0 {
				return nil, err
			}
		}
		return buf, nil
	}

	var buf []byte
	buf1 := make([]byte, chunk)
	for n > 0 {
		next := n
		if next > chunk {
			next = chunk
		}
		_, err := r.ReadAt(buf1[:next], off)
		if err != nil {
			return nil, err
		}
		buf = append(buf, buf1[:next]...)
		n -= next
		off += int64(next)
	}
	return buf, nil
}

// SliceCapWithSize 返回分配切片时要使用的容量。
// 使用该容量分配切片后，应该
// 使用 append 构建。如果容量很大且不正确，
// 这将避免分配过多内存。
//
// 负结果意味着该值总是太大。
func SliceCapWithSize(size, c uint64) int {
	if int64(c) < 0 || c != uint64(int(c)) {
		return -1
	}
	if size > 0 && c > (1<<64-1)/size {
		return -1
	}
	if c*size > chunk {
		c = chunk / size
		if c == 0 {
			c = 1
		}
	}
	return int(c)
}

// SliceCap 像 SliceCapWithSize 但使用泛型。
func SliceCap[E any](c uint64) int {
	var v E
	size := uint64(unsafe.Sizeof(v))
	return SliceCapWithSize(size, c)
}
