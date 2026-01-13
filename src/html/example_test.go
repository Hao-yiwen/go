// 版权所有 2015 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package html_test

import (
	"fmt"
	"html"
)

func ExampleEscapeString() {
	const s = `"Fran & Freddie's Diner" <tasty@example.com>`
	fmt.Println(html.EscapeString(s))
	// Output: &#34;Fran &amp; Freddie&#39;s Diner&#34; &lt;tasty@example.com&gt;
}

func ExampleUnescapeString() {
	const s = `&quot;Fran &amp; Freddie&#39;s Diner&quot; &lt;tasty@example.com&gt;`
	fmt.Println(html.UnescapeString(s))
	// Output: "Fran & Freddie's Diner" <tasty@example.com>
}
