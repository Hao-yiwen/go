// 版权所有 2009 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package strings_test

import (
	"bytes"
	"fmt"
	. "strings"
	"testing"
)

var htmlEscaper = NewReplacer(
	"&", "&amp;",
	"<", "&lt;",
	">", "&gt;",
	`"`, "&quot;",
	"'", "&apos;",
)

var htmlUnescaper = NewReplacer(
	"&amp;", "&",
	"&lt;", "<",
	"&gt;", ">",
	"&quot;", `"`,
	"&apos;", "'",
)

// http 包的旧 HTML 转义函数。
func oldHTMLEscape(s string) string {
	s = Replace(s, "&", "&amp;", -1)
	s = Replace(s, "<", "&lt;", -1)
	s = Replace(s, ">", "&gt;", -1)
	s = Replace(s, `"`, "&quot;", -1)
	s = Replace(s, "'", "&apos;", -1)
	return s
}

var capitalLetters = NewReplacer("a", "A", "b", "B")

// TestReplacer 测试 replacer 实现。
func TestReplacer(t *testing.T) {
	type testCase struct {
		r       *Replacer
		in, out string
	}
	var testCases []testCase

	// str 将 0xff 转换为 "\xff"。这不仅仅是 string(b)，因为那会转换为 UTF-8。
	str := func(b byte) string {
		return string([]byte{b})
	}
	var s []string

	// inc 将 "\x00"->"\x01", ..., "a"->"b", "b"->"c", ..., "\xff"->"\x00" 映射。
	s = nil
	for i := 0; i < 256; i++ {
		s = append(s, str(byte(i)), str(byte(i+1)))
	}
	inc := NewReplacer(s...)

	// 1 字节旧字符串、1 字节新字符串的测试用例。
	testCases = append(testCases,
		testCase{capitalLetters, "brad", "BrAd"},
		testCase{capitalLetters, Repeat("a", (32<<10)+123), Repeat("A", (32<<10)+123)},
		testCase{capitalLetters, "", ""},

		testCase{inc, "brad", "csbe"},
		testCase{inc, "\x00\xff", "\x01\x00"},
		testCase{inc, "", ""},

		testCase{NewReplacer("a", "1", "a", "2"), "brad", "br1d"},
	)

	// repeat 将 "a"->"a", "b"->"bb", "c"->"ccc", ... 映射。
	s = nil
	for i := 0; i < 256; i++ {
		n := i + 1 - 'a'
		if n < 1 {
			n = 1
		}
		s = append(s, str(byte(i)), Repeat(str(byte(i)), n))
	}
	repeat := NewReplacer(s...)

	// 1 字节旧字符串、可变长度新字符串的测试用例。
	testCases = append(testCases,
		testCase{htmlEscaper, "No changes", "No changes"},
		testCase{htmlEscaper, "I <3 escaping & stuff", "I &lt;3 escaping &amp; stuff"},
		testCase{htmlEscaper, "&&&", "&amp;&amp;&amp;"},
		testCase{htmlEscaper, "", ""},

		testCase{repeat, "brad", "bbrrrrrrrrrrrrrrrrrradddd"},
		testCase{repeat, "abba", "abbbba"},
		testCase{repeat, "", ""},

		testCase{NewReplacer("a", "11", "a", "22"), "brad", "br11d"},
	)

	// 其余测试用例具有可变长度的旧字符串。

	testCases = append(testCases,
		testCase{htmlUnescaper, "&amp;amp;", "&amp;"},
		testCase{htmlUnescaper, "&lt;b&gt;HTML&apos;s neat&lt;/b&gt;", "<b>HTML's neat</b>"},
		testCase{htmlUnescaper, "", ""},

		testCase{NewReplacer("a", "1", "a", "2", "xxx", "xxx"), "brad", "br1d"},

		testCase{NewReplacer("a", "1", "aa", "2", "aaa", "3"), "aaaa", "1111"},

		testCase{NewReplacer("aaa", "3", "aa", "2", "a", "1"), "aaaa", "31"},
	)

	// gen1 有多个可变长度的旧字符串。没有
	// 整体非空公共前缀，但有一些成对的公共前缀。
	gen1 := NewReplacer(
		"aaa", "3[aaa]",
		"aa", "2[aa]",
		"a", "1[a]",
		"i", "i",
		"longerst", "most long",
		"longer", "medium",
		"long", "short",
		"xx", "xx",
		"x", "X",
		"X", "Y",
		"Y", "Z",
	)
	testCases = append(testCases,
		testCase{gen1, "fooaaabar", "foo3[aaa]b1[a]r"},
		testCase{gen1, "long, longerst, longer", "short, most long, medium"},
		testCase{gen1, "xxxxx", "xxxxX"},
		testCase{gen1, "XiX", "YiY"},
		testCase{gen1, "", ""},
	)

	// gen2 有多个没有成对公共前缀的旧字符串。
	gen2 := NewReplacer(
		"roses", "red",
		"violets", "blue",
		"sugar", "sweet",
	)
	testCases = append(testCases,
		testCase{gen2, "roses are red, violets are blue...", "red are red, blue are blue..."},
		testCase{gen2, "", ""},
	)

	// gen3 有多个具有整体公共前缀的旧字符串。
	gen3 := NewReplacer(
		"abracadabra", "poof",
		"abracadabrakazam", "splat",
		"abraham", "lincoln",
		"abrasion", "scrape",
		"abraham", "isaac",
	)
	testCases = append(testCases,
		testCase{gen3, "abracadabrakazam abraham", "poofkazam lincoln"},
		testCase{gen3, "abrasion abracad", "scrape abracad"},
		testCase{gen3, "abba abram abrasive", "abba abram abrasive"},
		testCase{gen3, "", ""},
	)

	// foo{1,2,3,4} 有多个具有整体公共前缀的旧字符串
	// 以及从公共前缀扩展 1 或 2 字节。
	foo1 := NewReplacer(
		"foo1", "A",
		"foo2", "B",
		"foo3", "C",
	)
	foo2 := NewReplacer(
		"foo1", "A",
		"foo2", "B",
		"foo31", "C",
		"foo32", "D",
	)
	foo3 := NewReplacer(
		"foo11", "A",
		"foo12", "B",
		"foo31", "C",
		"foo32", "D",
	)
	foo4 := NewReplacer(
		"foo12", "B",
		"foo32", "D",
	)
	testCases = append(testCases,
		testCase{foo1, "fofoofoo12foo32oo", "fofooA2C2oo"},
		testCase{foo1, "", ""},

		testCase{foo2, "fofoofoo12foo32oo", "fofooA2Doo"},
		testCase{foo2, "", ""},

		testCase{foo3, "fofoofoo12foo32oo", "fofooBDoo"},
		testCase{foo3, "", ""},

		testCase{foo4, "fofoofoo12foo32oo", "fofooBDoo"},
		testCase{foo4, "", ""},
	)

	// genAll 将 "\x00\x01\x02...\xfe\xff" 映射到 "[all]"，以及其他内容。
	allBytes := make([]byte, 256)
	for i := range allBytes {
		allBytes[i] = byte(i)
	}
	allString := string(allBytes)
	genAll := NewReplacer(
		allString, "[all]",
		"\xff", "[ff]",
		"\x00", "[00]",
	)
	testCases = append(testCases,
		testCase{genAll, allString, "[all]"},
		testCase{genAll, "a\xff" + allString + "\x00", "a[ff][all][00]"},
		testCase{genAll, "", ""},
	)

	// 空旧字符串的测试用例。

	blankToX1 := NewReplacer("", "X")
	blankToX2 := NewReplacer("", "X", "", "")
	blankHighPriority := NewReplacer("", "X", "o", "O")
	blankLowPriority := NewReplacer("o", "O", "", "X")
	blankNoOp1 := NewReplacer("", "")
	blankNoOp2 := NewReplacer("", "", "", "A")
	blankFoo := NewReplacer("", "X", "foobar", "R", "foobaz", "Z")
	testCases = append(testCases,
		testCase{blankToX1, "foo", "XfXoXoX"},
		testCase{blankToX1, "", "X"},

		testCase{blankToX2, "foo", "XfXoXoX"},
		testCase{blankToX2, "", "X"},

		testCase{blankHighPriority, "oo", "XOXOX"},
		testCase{blankHighPriority, "ii", "XiXiX"},
		testCase{blankHighPriority, "oiio", "XOXiXiXOX"},
		testCase{blankHighPriority, "iooi", "XiXOXOXiX"},
		testCase{blankHighPriority, "", "X"},

		testCase{blankLowPriority, "oo", "OOX"},
		testCase{blankLowPriority, "ii", "XiXiX"},
		testCase{blankLowPriority, "oiio", "OXiXiOX"},
		testCase{blankLowPriority, "iooi", "XiOOXiX"},
		testCase{blankLowPriority, "", "X"},

		testCase{blankNoOp1, "foo", "foo"},
		testCase{blankNoOp1, "", ""},

		testCase{blankNoOp2, "foo", "foo"},
		testCase{blankNoOp2, "", ""},

		testCase{blankFoo, "foobarfoobaz", "XRXZX"},
		testCase{blankFoo, "foobar-foobaz", "XRX-XZX"},
		testCase{blankFoo, "", "X"},
	)

	// 单字符串替换器

	abcMatcher := NewReplacer("abc", "[match]")

	testCases = append(testCases,
		testCase{abcMatcher, "", ""},
		testCase{abcMatcher, "ab", "ab"},
		testCase{abcMatcher, "abc", "[match]"},
		testCase{abcMatcher, "abcd", "[match]d"},
		testCase{abcMatcher, "cabcabcdabca", "c[match][match]d[match]a"},
	)

	// Issue 6659 用例（更多单字符串替换器）

	noHello := NewReplacer("Hello", "")
	testCases = append(testCases,
		testCase{noHello, "Hello", ""},
		testCase{noHello, "Hellox", "x"},
		testCase{noHello, "xHello", "x"},
		testCase{noHello, "xHellox", "xx"},
	)

	// 无参数测试用例。

	nop := NewReplacer()
	testCases = append(testCases,
		testCase{nop, "abc", "abc"},
		testCase{nop, "", ""},
	)

	// 运行测试用例。

	for i, tc := range testCases {
		if s := tc.r.Replace(tc.in); s != tc.out {
			t.Errorf("%d. Replace(%q) = %q, want %q", i, tc.in, s, tc.out)
		}
		var buf bytes.Buffer
		n, err := tc.r.WriteString(&buf, tc.in)
		if err != nil {
			t.Errorf("%d. WriteString: %v", i, err)
			continue
		}
		got := buf.String()
		if got != tc.out {
			t.Errorf("%d. WriteString(%q) wrote %q, want %q", i, tc.in, got, tc.out)
			continue
		}
		if n != len(tc.out) {
			t.Errorf("%d. WriteString(%q) wrote correct string but reported %d bytes; want %d (%q)",
				i, tc.in, n, len(tc.out), tc.out)
		}
	}
}

var algorithmTestCases = []struct {
	r    *Replacer
	want string
}{
	{capitalLetters, "*strings.byteReplacer"},
	{htmlEscaper, "*strings.byteStringReplacer"},
	{NewReplacer("12", "123"), "*strings.singleStringReplacer"},
	{NewReplacer("1", "12"), "*strings.byteStringReplacer"},
	{NewReplacer("", "X"), "*strings.genericReplacer"},
	{NewReplacer("a", "1", "b", "12", "cde", "123"), "*strings.genericReplacer"},
}

// TestPickAlgorithm 测试 NewReplacer 是否选择了正确的算法。
func TestPickAlgorithm(t *testing.T) {
	for i, tc := range algorithmTestCases {
		got := fmt.Sprintf("%T", tc.r.Replacer())
		if got != tc.want {
			t.Errorf("%d. algorithm = %s, want %s", i, got, tc.want)
		}
	}
}

type errWriter struct{}

func (errWriter) Write(p []byte) (n int, err error) {
	return 0, fmt.Errorf("unwritable")
}

// TestWriteStringError 测试 WriteString 是否返回从底层 io.Writer 收到的错误。
func TestWriteStringError(t *testing.T) {
	for i, tc := range algorithmTestCases {
		n, err := tc.r.WriteString(errWriter{}, "abc")
		if n != 0 || err == nil || err.Error() != "unwritable" {
			t.Errorf("%d. WriteStringError = %d, %v, want 0, unwritable", i, n, err)
		}
	}
}

// TestGenericTrieBuilding 验证生成的 trie 的结构。每行一个节点，
// 如果以 "+" 结尾，则以当前行结尾的键在 trie 中。
func TestGenericTrieBuilding(t *testing.T) {
	testCases := []struct{ in, out string }{
		{"abc;abdef;abdefgh;xx;xy;z", `-
			a-
			.b-
			..c+
			..d-
			...ef+
			.....gh+
			x-
			.x+
			.y+
			z+
			`},
		{"abracadabra;abracadabrakazam;abraham;abrasion", `-
			a-
			.bra-
			....c-
			.....adabra+
			...........kazam+
			....h-
			.....am+
			....s-
			.....ion+
			`},
		{"aaa;aa;a;i;longerst;longer;long;xx;x;X;Y", `-
			X+
			Y+
			a+
			.a+
			..a+
			i+
			l-
			.ong+
			....er+
			......st+
			x+
			.x+
			`},
		{"foo;;foo;foo1", `+
			f-
			.oo+
			...1+
			`},
	}

	for _, tc := range testCases {
		keys := Split(tc.in, ";")
		args := make([]string, len(keys)*2)
		for i, key := range keys {
			args[i*2] = key
		}

		got := NewReplacer(args...).PrintTrie()
		// 从 tc.out 中移除制表符
		wantbuf := make([]byte, 0, len(tc.out))
		for i := 0; i < len(tc.out); i++ {
			if tc.out[i] != '\t' {
				wantbuf = append(wantbuf, tc.out[i])
			}
		}
		want := string(wantbuf)

		if got != want {
			t.Errorf("PrintTrie(%q)\ngot\n%swant\n%s", tc.in, got, want)
		}
	}
}

func BenchmarkGenericNoMatch(b *testing.B) {
	str := Repeat("A", 100) + Repeat("B", 100)
	generic := NewReplacer("a", "A", "b", "B", "12", "123") // 不同长度强制使用 generic
	for i := 0; i < b.N; i++ {
		generic.Replace(str)
	}
}

func BenchmarkGenericMatch1(b *testing.B) {
	str := Repeat("a", 100) + Repeat("b", 100)
	generic := NewReplacer("a", "A", "b", "B", "12", "123")
	for i := 0; i < b.N; i++ {
		generic.Replace(str)
	}
}

func BenchmarkGenericMatch2(b *testing.B) {
	str := Repeat("It&apos;s &lt;b&gt;HTML&lt;/b&gt;!", 100)
	for i := 0; i < b.N; i++ {
		htmlUnescaper.Replace(str)
	}
}

func benchmarkSingleString(b *testing.B, pattern, text string) {
	r := NewReplacer(pattern, "[match]")
	b.SetBytes(int64(len(text)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.Replace(text)
	}
}

func BenchmarkSingleMaxSkipping(b *testing.B) {
	benchmarkSingleString(b, Repeat("b", 25), Repeat("a", 10000))
}

func BenchmarkSingleLongSuffixFail(b *testing.B) {
	benchmarkSingleString(b, "b"+Repeat("a", 500), Repeat("a", 1002))
}

func BenchmarkSingleMatch(b *testing.B) {
	benchmarkSingleString(b, "abcdef", Repeat("abcdefghijklmno", 1000))
}

func BenchmarkByteByteNoMatch(b *testing.B) {
	str := Repeat("A", 100) + Repeat("B", 100)
	for i := 0; i < b.N; i++ {
		capitalLetters.Replace(str)
	}
}

func BenchmarkByteByteMatch(b *testing.B) {
	str := Repeat("a", 100) + Repeat("b", 100)
	for i := 0; i < b.N; i++ {
		capitalLetters.Replace(str)
	}
}

func BenchmarkByteStringMatch(b *testing.B) {
	str := "<" + Repeat("a", 99) + Repeat("b", 99) + ">"
	for i := 0; i < b.N; i++ {
		htmlEscaper.Replace(str)
	}
}

func BenchmarkHTMLEscapeNew(b *testing.B) {
	str := "I <3 to escape HTML & other text too."
	for i := 0; i < b.N; i++ {
		htmlEscaper.Replace(str)
	}
}

func BenchmarkHTMLEscapeOld(b *testing.B) {
	str := "I <3 to escape HTML & other text too."
	for i := 0; i < b.N; i++ {
		oldHTMLEscape(str)
	}
}

func BenchmarkByteStringReplacerWriteString(b *testing.B) {
	str := Repeat("I <3 to escape HTML & other text too.", 100)
	buf := new(bytes.Buffer)
	for i := 0; i < b.N; i++ {
		htmlEscaper.WriteString(buf, str)
		buf.Reset()
	}
}

func BenchmarkByteReplacerWriteString(b *testing.B) {
	str := Repeat("abcdefghijklmnopqrstuvwxyz", 100)
	buf := new(bytes.Buffer)
	for i := 0; i < b.N; i++ {
		capitalLetters.WriteString(buf, str)
		buf.Reset()
	}
}

// BenchmarkByteByteReplaces 将 byteByteImpl 与多个 Replace 进行比较。
func BenchmarkByteByteReplaces(b *testing.B) {
	str := Repeat("a", 100) + Repeat("b", 100)
	for i := 0; i < b.N; i++ {
		Replace(Replace(str, "a", "A", -1), "b", "B", -1)
	}
}

// BenchmarkByteByteMap 将 byteByteImpl 与 Map 进行比较。
func BenchmarkByteByteMap(b *testing.B) {
	str := Repeat("a", 100) + Repeat("b", 100)
	fn := func(r rune) rune {
		switch r {
		case 'a':
			return 'A'
		case 'b':
			return 'B'
		}
		return r
	}
	for i := 0; i < b.N; i++ {
		Map(fn, str)
	}
}

var mapdata = []struct{ name, data string }{
	{"ASCII", "a b c d e f g h i j k l m n o p q r s t u v w x y z"},
	{"Greek", "α β γ δ ε ζ η θ ι κ λ μ ν ξ ο π ρ ς σ τ υ φ χ ψ ω"},
}

func BenchmarkMap(b *testing.B) {
	mapidentity := func(r rune) rune {
		return r
	}

	b.Run("identity", func(b *testing.B) {
		for _, md := range mapdata {
			b.Run(md.name, func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					Map(mapidentity, md.data)
				}
			})
		}
	})

	mapchange := func(r rune) rune {
		if 'a' <= r && r <= 'z' {
			return r + 'A' - 'a'
		}
		if 'α' <= r && r <= 'ω' {
			return r + 'Α' - 'α'
		}
		return r
	}

	b.Run("change", func(b *testing.B) {
		for _, md := range mapdata {
			b.Run(md.name, func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					Map(mapchange, md.data)
				}
			})
		}
	})
}
