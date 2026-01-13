// 版权所有 2015 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package mime_test

import (
	"bytes"
	"fmt"
	"io"
	"mime"
)

func ExampleWordEncoder_Encode() {
	fmt.Println(mime.QEncoding.Encode("utf-8", "¡Hola, señor!"))
	fmt.Println(mime.QEncoding.Encode("utf-8", "Hello!"))
	fmt.Println(mime.BEncoding.Encode("UTF-8", "¡Hola, señor!"))
	fmt.Println(mime.QEncoding.Encode("ISO-8859-1", "Caf\xE9"))
	// Output:
	// =?utf-8?q?=C2=A1Hola,_se=C3=B1or!?=
	// Hello!
	// =?UTF-8?b?wqFIb2xhLCBzZcOxb3Ih?=
	// =?ISO-8859-1?q?Caf=E9?=
}

func ExampleWordDecoder_Decode() {
	dec := new(mime.WordDecoder)
	header, err := dec.Decode("=?utf-8?q?=C2=A1Hola,_se=C3=B1or!?=")
	if err != nil {
		panic(err)
	}
	fmt.Println(header)

	dec.CharsetReader = func(charset string, input io.Reader) (io.Reader, error) {
		switch charset {
		case "x-case":
			// 示例用的虚假字符集。
			// 实际使用时应集成如 code.google.com/p/go-charset 这样的包
			content, err := io.ReadAll(input)
			if err != nil {
				return nil, err
			}
			return bytes.NewReader(bytes.ToUpper(content)), nil
		default:
			return nil, fmt.Errorf("unhandled charset %q", charset)
		}
	}
	header, err = dec.Decode("=?x-case?q?hello!?=")
	if err != nil {
		panic(err)
	}
	fmt.Println(header)
	// Output:
	// ¡Hola, señor!
	// HELLO!
}

func ExampleWordDecoder_DecodeHeader() {
	dec := new(mime.WordDecoder)
	header, err := dec.DecodeHeader("=?utf-8?q?=C3=89ric?= <eric@example.org>, =?utf-8?q?Ana=C3=AFs?= <anais@example.org>")
	if err != nil {
		panic(err)
	}
	fmt.Println(header)

	header, err = dec.DecodeHeader("=?utf-8?q?=C2=A1Hola,?= =?utf-8?q?_se=C3=B1or!?=")
	if err != nil {
		panic(err)
	}
	fmt.Println(header)

	dec.CharsetReader = func(charset string, input io.Reader) (io.Reader, error) {
		switch charset {
		case "x-case":
			// 示例用的虚假字符集。
			// 实际使用时应集成如 code.google.com/p/go-charset 这样的包
			content, err := io.ReadAll(input)
			if err != nil {
				return nil, err
			}
			return bytes.NewReader(bytes.ToUpper(content)), nil
		default:
			return nil, fmt.Errorf("unhandled charset %q", charset)
		}
	}
	header, err = dec.DecodeHeader("=?x-case?q?hello_?= =?x-case?q?world!?=")
	if err != nil {
		panic(err)
	}
	fmt.Println(header)
	// Output:
	// Éric <eric@example.org>, Anaïs <anais@example.org>
	// ¡Hola, señor!
	// HELLO WORLD!
}

func ExampleFormatMediaType() {
	mediatype := "text/html"
	params := map[string]string{
		"charset": "utf-8",
	}

	result := mime.FormatMediaType(mediatype, params)

	fmt.Println("result:", result)
	// Output:
	// result: text/html; charset=utf-8
}

func ExampleParseMediaType() {
	mediatype, params, err := mime.ParseMediaType("text/html; charset=utf-8")
	if err != nil {
		panic(err)
	}

	fmt.Println("type:", mediatype)
	fmt.Println("charset:", params["charset"])
	// Output:
	// type: text/html
	// charset: utf-8
}
