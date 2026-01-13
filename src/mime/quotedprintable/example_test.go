// 版权所有 2016 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package quotedprintable_test

import (
	"fmt"
	"io"
	"mime/quotedprintable"
	"os"
	"strings"
)

func ExampleNewReader() {
	for _, s := range []string{
		`=48=65=6C=6C=6F=2C=20=47=6F=70=68=65=72=73=21`,
		`invalid escape: <b style="font-size: 200%">hello</b>`,
		"Hello, Gophers! This symbol will be unescaped: =3D and this will be written in =\r\none line.",
	} {
		b, err := io.ReadAll(quotedprintable.NewReader(strings.NewReader(s)))
		fmt.Printf("%s %v\n", b, err)
	}
	// Output:
	// Hello, Gophers! <nil>
	// invalid escape: <b style="font-size: 200%">hello</b> <nil>
	// Hello, Gophers! This symbol will be unescaped: = and this will be written in one line. <nil>
}

func ExampleNewWriter() {
	w := quotedprintable.NewWriter(os.Stdout)
	w.Write([]byte("These symbols will be escaped: = \t"))
	w.Close()

	// Output:
	// These symbols will be escaped: =3D =09
}
