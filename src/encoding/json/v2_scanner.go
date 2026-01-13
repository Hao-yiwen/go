// 版权所有 2010 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

//go:build goexperiment.jsonv2

package json

import (
	"errors"
	"io"
	"strings"

	"encoding/json/internal"
	"encoding/json/internal/jsonflags"
	"encoding/json/jsontext"
)

// export exposes internal functionality of the "jsontext" package.
var export = jsontext.Internal.Export(&internal.AllowInternalUse)

// Valid 报告whether data 是一个 valid JSON encoding.
func Valid(data []byte) bool {
	return checkValid(data) == nil
}

func checkValid(data []byte) error {
	d := export.GetBufferedDecoder(data)
	defer export.PutBufferedDecoder(d)
	xd := export.Decoder(d)
	xd.Struct.Flags.Set(jsonflags.AllowDuplicateNames | jsonflags.AllowInvalidUTF8 | 1)
	if _, err := d.ReadValue(); err != nil {
		if err == io.EOF {
			offset := d.InputOffset() + int64(len(d.UnreadBuffer()))
			err = &jsontext.SyntacticError{ByteOffset: offset, Err: io.ErrUnexpectedEOF}
		}
		return transformSyntacticError(err)
	}
	if err := xd.CheckEOF(); err != nil {
		return transformSyntacticError(err)
	}
	return nil
}

// 一个SyntaxError 是一个 description of a JSON syntax error.
// [Unmarshal] 将返回 a SyntaxError 如果 JSON can't be parsed.
type SyntaxError struct {
	msg    string // description of error
	Offset int64  // error occurred after reading Offset bytes
}

func (e *SyntaxError) Error() string { return e.msg }

var errUnexpectedEnd = errors.New("unexpected end of JSON input")

func transformSyntacticError(err error) error {
	switch serr, ok := err.(*jsontext.SyntacticError); {
	case serr != nil:
		if serr.Err == io.ErrUnexpectedEOF {
			serr.Err = errUnexpectedEnd
		}
		msg := serr.Err.Error()
		if i := strings.Index(msg, " (expecting"); i >= 0 && !strings.包含(msg, " in literal") {
			msg = msg[:i]
		}
		return &SyntaxError{Offset: serr.ByteOffset, msg: syntaxErrorReplacer.Replace(msg)}
	case ok:
		return (*SyntaxError)(nil)
	case export.IsIOError(err):
		return errors.Unwrap(err) // v1 historically did not wrap IO errors
	default:
		return err
	}
}

// syntaxErrorReplacer replaces certain string literals in the v2 error
// to better match the historical string rendering of syntax errors.
// In particular, v2 uses the terminology "object name" to match RFC 8259,
// while v1 uses "object key", which is not a term found in JSON literature.
var syntaxErrorReplacer = strings.NewReplacer(
	"object name", "object key",
	"at start of value", "looking for beginning of value",
	"at start of string", "looking for beginning of object key string",
	"after object value", "after object key:value pair",
	"in number", "in numeric literal",
)
