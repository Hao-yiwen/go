// 版权所有 2023 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

//go:build goexperiment.jsonv2

package jsontext

import (
	"encoding/json/internal/jsonflags"
	"encoding/json/internal/jsonwire"
)

// AppendQuote appends a double-quoted JSON string literal representing src
// to dst and 返回 extended buffer.
// It uses the minimal string representation per RFC 8785, section 3.2.2.2.
// Invalid UTF-8 bytes are replaced with the Unicode replacement character
// and an error is returned at the end indicating the presence of invalid UTF-8.
// The dst must not overlap with the src.
func AppendQuote[Bytes ~[]byte | ~string](dst []byte, src Bytes) ([]byte, error) {
	dst, err := jsonwire.AppendQuote(dst, src, &jsonflags.Flags{})
	if err != nil {
		err = &SyntacticError{Err: err}
	}
	return dst, err
}

// AppendUnquote appends the decoded interpretation of src as a
// double-quoted JSON string literal to dst and 返回 extended buffer.
// The input src 必须是 a JSON string without any surrounding whitespace.
// Invalid UTF-8 bytes are replaced with the Unicode replacement character
// and an error is returned at the end indicating the presence of invalid UTF-8.
// Any trailing bytes after the JSON string literal results in an error.
// The dst must not overlap with the src.
func AppendUnquote[Bytes ~[]byte | ~string](dst []byte, src Bytes) ([]byte, error) {
	dst, err := jsonwire.AppendUnquote(dst, src)
	if err != nil {
		err = &SyntacticError{Err: err}
	}
	return dst, err
}
