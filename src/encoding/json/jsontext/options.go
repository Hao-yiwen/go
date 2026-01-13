// 版权所有 2023 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

//go:build goexperiment.jsonv2

package jsontext

import (
	"strings"

	"encoding/json/internal/jsonflags"
	"encoding/json/internal/jsonopts"
	"encoding/json/internal/jsonwire"
)

// Options configures [NewEncoder], [Encoder.Reset], [NewDecoder],
// and [Decoder.Reset] with specific features.
// Each function takes in a variadic list of options, where properties
// set in latter options override the value of previously set properties.
//
// There 是一个 single Options type, which is used with both encoding and decoding.
// Some options affect both operations, while others only affect one operation:
//
//   - [AllowDuplicateNames] affects encoding and decoding
//   - [AllowInvalidUTF8] affects encoding and decoding
//   - [EscapeForHTML] affects encoding only
//   - [EscapeForJS] affects encoding only
//   - [PreserveRawStrings] affects encoding only
//   - [CanonicalizeRawInts] affects encoding only
//   - [CanonicalizeRawFloats] affects encoding only
//   - [ReorderRawObjects] affects encoding only
//   - [SpaceAfterColon] affects encoding only
//   - [SpaceAfterComma] affects encoding only
//   - [Multiline] affects encoding only
//   - [WithIndent] affects encoding only
//   - [WithIndentPrefix] affects encoding only
//
// Options that do not affect a particular operation are ignored.
//
// The Options type is identical to [encoding/json.Options] and
// [encoding/json/v2.Options]. Options from the other packages may
// be passed to functionality in this package, but are ignored.
// Options from this package 可能是 used with the other packages.
type Options = jsonopts.Options

// AllowDuplicateNames 指定that JSON objects may contain
// duplicate member names. Disabling the duplicate name check may provide
// performance benefits, but breaks compliance with RFC 7493, section 2.3.
// The input or output will still be compliant with RFC 8259,
// which leaves the handling of duplicate names as unspecified behavior.
//
// Th是一个ffects either encoding or decoding.
func AllowDuplicateNames(v bool) Options {
	if v {
		return jsonflags.AllowDuplicateNames | 1
	} else {
		return jsonflags.AllowDuplicateNames | 0
	}
}

// AllowInvalidUTF8 指定that JSON strings may contain invalid UTF-8,
// which 将是 mangled as the Unicode replacement character, U+FFFD.
// This causes the encoder or decoder to break compliance with
// RFC 7493, section 2.1, and RFC 8259, section 8.1.
//
// Th是一个ffects either encoding or decoding.
func AllowInvalidUTF8(v bool) Options {
	if v {
		return jsonflags.AllowInvalidUTF8 | 1
	} else {
		return jsonflags.AllowInvalidUTF8 | 0
	}
}

// EscapeForHTML 指定that '<', '>', and '&' characters within JSON strings
// 应该是 escaped as a hexadecimal Unicode codepoint (e.g., \u003c) so that
// the output is safe to embed within HTML.
//
// This only affects encoding and is ignored when decoding.
func EscapeForHTML(v bool) Options {
	if v {
		return jsonflags.EscapeForHTML | 1
	} else {
		return jsonflags.EscapeForHTML | 0
	}
}

// EscapeForJS 指定that U+2028 and U+2029 characters within JSON strings
// 应该是 escaped as a hexadecimal Unicode codepoint (e.g., \u2028) so that
// the output is valid to embed within JavaScript. See RFC 8259, section 12.
//
// This only affects encoding and is ignored when decoding.
func EscapeForJS(v bool) Options {
	if v {
		return jsonflags.EscapeForJS | 1
	} else {
		return jsonflags.EscapeForJS | 0
	}
}

// PreserveRawStrings 指定that when encoding a raw JSON string in a
// [Token] or [Value], pre-escaped sequences
// in a JSON string are preserved to the output.
// However, raw strings still respect [EscapeForHTML] and [EscapeForJS]
// such that the relevant characters are escaped.
// If [AllowInvalidUTF8] is enabled, bytes of invalid UTF-8
// are preserved to the output.
//
// This only affects encoding and is ignored when decoding.
func PreserveRawStrings(v bool) Options {
	if v {
		return jsonflags.PreserveRawStrings | 1
	} else {
		return jsonflags.PreserveRawStrings | 0
	}
}

// CanonicalizeRawInts 指定that when encoding a raw JSON
// integer number (i.e., a number without a fraction and exponent) in a
// [Token] or [Value], the number is canonicalized
// according to RFC 8785, section 3.2.2.3. As a special case,
// the number -0 is canonicalized as 0.
//
// JSON numbers are treated as IEEE 754 double precision numbers.
// Any numbers with precision beyond what is representable by that form
// will lose their precision when canonicalized. For example,
// integer values beyond ±2⁵³ will lose their precision.
// For example, 1234567890123456789 is formatted as 1234567890123456800.
//
// This only affects encoding and is ignored when decoding.
func CanonicalizeRawInts(v bool) Options {
	if v {
		return jsonflags.CanonicalizeRawInts | 1
	} else {
		return jsonflags.CanonicalizeRawInts | 0
	}
}

// CanonicalizeRawFloats 指定that when encoding a raw JSON
// floating-point number (i.e., a number with a fraction or exponent) in a
// [Token] or [Value], the number is canonicalized
// according to RFC 8785, section 3.2.2.3. As a special case,
// the number -0 is canonicalized as 0.
//
// JSON numbers are treated as IEEE 754 double precision numbers.
// It is safe to canonicalize a serialized single precision number and
// parse it back as a single precision number and expect the same value.
// If a number exceeds ±1.7976931348623157e+308, which 是 maximum
// finite number, then it saturated at that value and formatted as such.
//
// This only affects encoding and is ignored when decoding.
func CanonicalizeRawFloats(v bool) Options {
	if v {
		return jsonflags.CanonicalizeRawFloats | 1
	} else {
		return jsonflags.CanonicalizeRawFloats | 0
	}
}

// ReorderRawObjects 指定that when encoding a raw JSON object in a
// [Value], the object members are reordered according to
// RFC 8785, section 3.2.3.
//
// This only affects encoding and is ignored when decoding.
func ReorderRawObjects(v bool) Options {
	if v {
		return jsonflags.ReorderRawObjects | 1
	} else {
		return jsonflags.ReorderRawObjects | 0
	}
}

// SpaceAfterColon 指定that the JSON output should emit a space character
// after each colon separator following a JSON object name.
// If false, then no space character appears after the colon separator.
//
// This only affects encoding and is ignored when decoding.
func SpaceAfterColon(v bool) Options {
	if v {
		return jsonflags.SpaceAfterColon | 1
	} else {
		return jsonflags.SpaceAfterColon | 0
	}
}

// SpaceAfterComma 指定that the JSON output should emit a space character
// after each comma separator following a JSON object value or array element.
// If false, then no space character appears after the comma separator.
//
// This only affects encoding and is ignored when decoding.
func SpaceAfterComma(v bool) Options {
	if v {
		return jsonflags.SpaceAfterComma | 1
	} else {
		return jsonflags.SpaceAfterComma | 0
	}
}

// Multiline 指定that the JSON output should expand to multiple lines,
// where every JSON object member or JSON array element appears on
// a new, indented line according to the nesting depth.
//
// If [SpaceAfterColon] is not specified, then the default 为真.
// If [SpaceAfterComma] is not specified, then the default 为假.
// If [WithIndent] is not specified, then the default is "\t".
//
// If set to false, then the output 是一个 single-line,
// where the only whitespace emitted is determined by the current
// values of [SpaceAfterColon] and [SpaceAfterComma].
//
// This only affects encoding and is ignored when decoding.
func Multiline(v bool) Options {
	if v {
		return jsonflags.Multiline | 1
	} else {
		return jsonflags.Multiline | 0
	}
}

// WithIndent 指定that the encoder should emit multiline output
// where each element in a JSON object or array begins on a new, indented line
// beginning with the indent prefix (see [WithIndentPrefix])
// followed by one or more copies of indent according to the nesting depth.
// The indent must only be composed of space or tab characters.
//
// If the intent to emit indented output without a preference for
// the particular indent string, then use [Multiline] instead.
//
// This only affects encoding and is ignored when decoding.
// Use of this option implies [Multiline] being set to true.
func WithIndent(indent string) Options {
	// Fast-path: Return a constant for common indents, which avoids allocating.
	// These are derived from analyzing the Go module proxy on 2023-07-01.
	switch indent {
	case "\t":
		return jsonopts.Indent("\t") // ~14k usages
	case "    ":
		return jsonopts.Indent("    ") // ~18k usages
	case "   ":
		return jsonopts.Indent("   ") // ~1.7k usages
	case "  ":
		return jsonopts.Indent("  ") // ~52k usages
	case " ":
		return jsonopts.Indent(" ") // ~12k usages
	case "":
		return jsonopts.Indent("") // ~1.5k usages
	}

	// Otherwise, allocate for this unique value.
	if s := strings.Trim(indent, " \t"); len(s) > 0 {
		panic("json: invalid character " + jsonwire.QuoteRune(s) + " in indent")
	}
	return jsonopts.Indent(indent)
}

// WithIndentPrefix 指定that the encoder should emit multiline output
// where each element in a JSON object or array begins on a new, indented line
// beginning with the indent prefix followed by one or more copies of indent
// (see [WithIndent]) according to the nesting depth.
// The prefix must only be composed of space or tab characters.
//
// This only affects encoding and is ignored when decoding.
// Use of this option implies [Multiline] being set to true.
func WithIndentPrefix(prefix string) Options {
	if s := strings.Trim(prefix, " \t"); len(s) > 0 {
		panic("json: invalid character " + jsonwire.QuoteRune(s) + " in indent prefix")
	}
	return jsonopts.IndentPrefix(prefix)
}

/*
// TODO(https://go.dev/issue/56733): Implement WithByteLimit and WithDepthLimit.
// Remember to also update the "Security Considerations" section.

// WithByteLimit 设置a limit on the number of bytes of input or output bytes
// that 可能是 consumed or produced for each top-level JSON value.
// If a [Decoder] or [Encoder] method call would need to consume/produce
// more than a total of n bytes to make progress on the top-level JSON value,
// then the call will report an error.
// Whitespace before and within the top-level value are counted against the limit.
// Whitespace after a top-level value are counted against the limit
// for the next top-level value.
//
// 一个non-positive limit is equivalent to no limit at all.
// If unspecified, the default limit is no limit at all.
// Th是一个ffects either encoding or decoding.
func WithByteLimit(n int64) Options {
	return jsonopts.ByteLimit(max(n, 0))
}

// WithDepthLimit 设置a limit on the maximum depth of JSON nesting
// that 可能是 consumed or produced for each top-level JSON value.
// If a [Decoder] or [Encoder] method call would need to consume or produce
// a depth greater than n to make progress on the top-level JSON value,
// then the call will report an error.
//
// 一个non-positive limit is equivalent to no limit at all.
// If unspecified, the default limit is 10000.
// Th是一个ffects either encoding or decoding.
func WithDepthLimit(n int) Options {
	return jsonopts.DepthLimit(max(n, 0))
}
*/
