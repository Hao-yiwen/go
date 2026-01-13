// 版权所有 2013 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package html

import (
	"strings"
	"testing"
)

type unescapeTest struct {
	// 测试用例的简短描述。
	desc string
	// HTML 文本。
	html string
	// 反转义后的文本。
	unescaped string
}

var unescapeTests = []unescapeTest{
	// 处理无实体的情况。
	{
		"copy",
		"A\ttext\nstring",
		"A\ttext\nstring",
	},
	// 处理简单的命名实体。
	{
		"simple",
		"&amp; &gt; &lt;",
		"& > <",
	},
	// 处理到达字符串末尾的情况。
	{
		"stringEnd",
		"&amp &amp",
		"& &",
	},
	// 处理具有两个码点的实体。
	{
		"multiCodepoint",
		"text &gesl; blah",
		"text \u22db\ufe00 blah",
	},
	// 处理十进制数字实体。
	{
		"decimalEntity",
		"Delta = &#916; ",
		"Delta = Δ ",
	},
	// 处理十六进制数字实体。
	{
		"hexadecimalEntity",
		"Lambda = &#x3bb; = &#X3Bb ",
		"Lambda = λ = λ ",
	},
	// 处理数字提前终止的情况。
	{
		"numericEnds",
		"&# &#x &#128;43 &copy = &#169f = &#xa9",
		"&# &#x €43 © = ©f = ©",
	},
	// 处理数字 ISO-8859-1 实体替换。
	{
		"numericReplacements",
		"Footnote&#x87;",
		"Footnote‡",
	},
	// 处理单个 & 符号。
	{
		"copySingleAmpersand",
		"&",
		"&",
	},
	// 处理 & 符号后跟非实体的情况。
	{
		"copyAmpersandNonEntity",
		"text &test",
		"text &test",
	},
	// 处理 "&#" 的情况。
	{
		"copyAmpersandHash",
		"text &#",
		"text &#",
	},
}

func TestUnescape(t *testing.T) {
	for _, tt := range unescapeTests {
		unescaped := UnescapeString(tt.html)
		if unescaped != tt.unescaped {
			t.Errorf("TestUnescape %s: want %q, got %q", tt.desc, tt.unescaped, unescaped)
		}
	}
}

func TestUnescapeEscape(t *testing.T) {
	ss := []string{
		``,
		`abc def`,
		`a & b`,
		`a&amp;b`,
		`a &amp b`,
		`&quot;`,
		`"`,
		`"<&>"`,
		`&quot;&lt;&amp;&gt;&quot;`,
		`3&5==1 && 0<1, "0&lt;1", a+acute=&aacute;`,
		`The special characters are: <, >, &, ' and "`,
	}
	for _, s := range ss {
		if got := UnescapeString(EscapeString(s)); got != s {
			t.Errorf("got %q want %q", got, s)
		}
	}
}

var (
	benchEscapeData     = strings.Repeat("AAAAA < BBBBB > CCCCC & DDDDD ' EEEEE \" ", 100)
	benchEscapeNone     = strings.Repeat("AAAAA x BBBBB x CCCCC x DDDDD x EEEEE x ", 100)
	benchUnescapeSparse = strings.Repeat(strings.Repeat("AAAAA x BBBBB x CCCCC x DDDDD x EEEEE x ", 10)+"&amp;", 10)
	benchUnescapeDense  = strings.Repeat("&amp;&lt; &amp; &lt;", 100)
)

func BenchmarkEscape(b *testing.B) {
	n := 0
	for i := 0; i < b.N; i++ {
		n += len(EscapeString(benchEscapeData))
	}
}

func BenchmarkEscapeNone(b *testing.B) {
	n := 0
	for i := 0; i < b.N; i++ {
		n += len(EscapeString(benchEscapeNone))
	}
}

func BenchmarkUnescape(b *testing.B) {
	s := EscapeString(benchEscapeData)
	n := 0
	for i := 0; i < b.N; i++ {
		n += len(UnescapeString(s))
	}
}

func BenchmarkUnescapeNone(b *testing.B) {
	s := EscapeString(benchEscapeNone)
	n := 0
	for i := 0; i < b.N; i++ {
		n += len(UnescapeString(s))
	}
}

func BenchmarkUnescapeSparse(b *testing.B) {
	n := 0
	for i := 0; i < b.N; i++ {
		n += len(UnescapeString(benchUnescapeSparse))
	}
}

func BenchmarkUnescapeDense(b *testing.B) {
	n := 0
	for i := 0; i < b.N; i++ {
		n += len(UnescapeString(benchUnescapeDense))
	}
}
