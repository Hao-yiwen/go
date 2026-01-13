// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bufio

// 仅为测试导出。
import (
	"unicode/utf8"
)

var IsSpace = isSpace

const DefaultBufSize = defaultBufSize

func (s *Scanner) MaxTokenSize(n int) {
	if n < utf8.UTFMax || n > 1e9 {
		panic("bad max token size")
	}
	if n < len(s.buf) {
		s.buf = make([]byte, n)
	}
	s.maxTokenSize = n
}

// ErrOrEOF 类似于 Err，但返回 EOF。用于测试边界情况。
func (s *Scanner) ErrOrEOF() error {
	return s.err
}
