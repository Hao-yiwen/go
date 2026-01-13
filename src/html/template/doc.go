// 版权所有 2011 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

/*
Package template (html/template) 实现了用于生成 HTML 输出的数据驱动模板，
能防止代码注入。它提供与 [text/template] 相同的接口，在输出是 HTML 时应该
用于替代 [text/template]。

本文档重点介绍该包的安全功能。
有关如何编程模板本身的信息，请参阅 [text/template] 的文档。

# 介绍

该包封装了 [text/template]，以便您可以共享其模板 API
来安全地解析和执行 HTML 模板。

	tmpl, err := template.New("name").Parse(...)
	// 错误检查已省略
	err = tmpl.Execute(out, data)

如果成功，tmpl 现在将是注入安全的。否则，err 是在
ErrorCode 文档中定义的错误。

HTML 模板将数据值视为纯文本，应该对其进行编码，以便
它们可以安全地嵌入到 HTML 文档中。转义是上下文相关的，因此
操作可以出现在 JavaScript、CSS 和 URI 上下文中。

注释从输出中剥离，除了通过
[HTML]、[CSS] 和 [JS] 类型传入的那些分别用于各自的上下文。

该包使用的安全模型假设模板作者是可信的，而 Execute 的
data 参数则不是。下面提供了更多详细信息。

示例

	import "text/template"
	...
	t, err := template.New("foo").Parse(`{{define "T"}}Hello, {{.}}!{{end}}`)
	err = t.ExecuteTemplate(out, "T", "<script>alert('you have been pwned')</script>")

产生

	Hello, <script>alert('you have been pwned')</script>!

但 html/template 中的上下文自动转义

	import "html/template"
	...
	t, err := template.New("foo").Parse(`{{define "T"}}Hello, {{.}}!{{end}}`)
	err = t.ExecuteTemplate(out, "T", "<script>alert('you have been pwned')</script>")

产生安全的转义 HTML 输出

	Hello, &lt;script&gt;alert(&#39;you have been pwned&#39;)&lt;/script&gt;!

# 上下文

该包理解 HTML、CSS、JavaScript 和 URI。它在每个简单的
操作管道中添加清理函数，给定摘录

	<a href="/search?q={{.}}">{{.}}</a>

在解析时，每个 {{.}} 都被覆盖以根据需要添加转义函数。
在这种情况下，它变成

	<a href="/search?q={{. | urlescaper | attrescaper}}">{{. | htmlescaper}}</a>

其中 urlescaper、attrescaper 和 htmlescaper 是内部转义
函数的别名。

对于这些内部转义函数，如果一个操作管道计算为
nil 接口值，它被视为空字符串。

# 命名空间和 data- 属性

具有命名空间的属性被视为没有命名空间的属性。
给定摘录

	<a my:href="{{.}}"></a>

在解析时，属性将被视为就像 "href"。
所以在解析时模板变成：

	<a my:href="{{. | urlescaper | attrescaper}}"></a>

类似地，具有 "data-" 前缀的属性被视为没有 "data-" 前缀。给定

	<a data-href="{{.}}"></a>

在解析时变成

	<a data-href="{{. | urlescaper | attrescaper}}"></a>

如果属性同时具有命名空间和 "data-" 前缀，仅命名空间
在确定上下文时将被删除。例如

	<a my:data-href="{{.}}"></a>

这被处理为 "my:data-href" 就像 "data-href"，而不是 "href"
如果 "data-" 前缀也被忽略的话。因此在解析
时这只是变成

	<a my:data-href="{{. | attrescaper}}"></a>

作为特殊情况，具有命名空间 "xmlns" 的属性始终被视为
包含 URL。给定摘录

	<a xmlns:title="{{.}}"></a>
	<a xmlns:href="{{.}}"></a>
	<a xmlns:onclick="{{.}}"></a>

在解析时它们变成：

	<a xmlns:title="{{. | urlescaper | attrescaper}}"></a>
	<a xmlns:href="{{. | urlescaper | attrescaper}}"></a>
	<a xmlns:onclick="{{. | urlescaper | attrescaper}}"></a>

# 错误

有关详细信息，请参阅 ErrorCode 的文档。

# 更完整的图景

该包注释的其余部分可在首次阅读时跳过；它包括
理解转义上下文和错误消息所需的详细信息。大多数用户
将不需要理解这些细节。

# 上下文

假设 {{.}} 是 `O'Reilly: How are <i>you</i>?`，下表显示
当在左侧上下文中使用时 {{.}} 是如何出现的。

	Context                          {{.}} After
	{{.}}                            O'Reilly: How are &lt;i&gt;you&lt;/i&gt;?
	<a title='{{.}}'>                O&#39;Reilly: How are you?
	<a href="/{{.}}">                O&#39;Reilly: How are %3ci%3eyou%3c/i%3e?
	<a href="?q={{.}}">              O&#39;Reilly%3a%20How%20are%3ci%3e...%3f
	<a onx='f("{{.}}")'>             O\x27Reilly: How are \x3ci\x3eyou...?
	<a onx='f({{.}})'>               "O\x27Reilly: How are \x3ci\x3eyou...?"
	<a onx='pattern = /{{.}}/;'>     O\x27Reilly: How are \x3ci\x3eyou...\x3f

如果在不安全的上下文中使用，则值可能被过滤掉：

	Context                          {{.}} After
	<a href="{{.}}">                 #ZgotmplZ

因为 "O'Reilly:" 不是允许的协议（如 "http:"）。

如果 {{.}} 是无害的单词 `left`，那么它可以更广泛地出现，

	Context                              {{.}} After
	{{.}}                                left
	<a title='{{.}}'>                    left
	<a href='{{.}}'>                     left
	<a href='/{{.}}'>                    left
	<a href='?dir={{.}}'>                left
	<a style="border-{{.}}: 4px">        left
	<a style="align: {{.}}">             left
	<a style="background: '{{.}}'>       left
	<a style="background: url('{{.}}')>  left
	<style>p.{{.}} {color:red}</style>   left

非字符串值可以在 JavaScript 上下文中使用。
如果 {{.}} 是

	struct{A,B string}{ "foo", "bar" }

在转义的模板中

	<script>var pair = {{.}};</script>

那么模板输出是

	<script>var pair = {"A": "foo", "B": "bar"};</script>

请参阅 json 包来了解如何编组非字符串内容以供
在 JavaScript 上下文中嵌入。

# 类型化字符串

默认情况下，该包假设所有管道都产生纯文本字符串。
它添加必要的转义管道阶段，以正确安全地嵌入
纯文本字符串到适当的上下文中。

当数据值不是纯文本时，您可以通过
用其类型标记来确保它不被过度转义。

来自 content.go 的类型 HTML、JS、URL 和其他类型可以
包含免于转义的安全内容。

模板

	Hello, {{.}}!

可以通过以下方式调用

	tmpl.Execute(out, template.HTML(`<b>World</b>`))

产生

	Hello, <b>World</b>!

而不是

	Hello, &lt;b&gt;World&lt;b&gt;!

如果 {{.}} 是常规字符串，则会产生的结果。

# 安全模型

https://web.archive.org/web/20160501113828/http://js-quasis-libraries-and-repl.googlecode.com/svn/trunk/safetemplate.html#problem_definition 定义了该包中使用的 "safe"。

该包假设模板作者是可信的，Execute 的 data
参数则不是，并寻求在面对
不受信任的数据时保护以下属性：

结构保留属性：
"...当模板作者在安全模板语言中编写 HTML 标签时，
浏览器将解释输出的相应部分作为标签，
而不管不可信数据的值，
以及其他结构如属性边界和 JS 和 CSS 字符串边界也是如此。"

代码效果属性：
"...仅由模板作者指定的代码应在以下情况下运行
将模板输出注入页面，并且模板作者指定的所有代码应在
相同结果的情况下运行。"

最少惊讶属性：
"熟悉 HTML、CSS 和 JavaScript 的开发人员（或代码审查者），
知道发生上下文自动转义应该能够查看 {{.}}
并正确推断发生什么清理。"

之前，ECMAScript 6 模板文字默认被禁用，并且可以
通过 GODEBUG=jstmpllitinterp=1 环境变量启用。模板
文字现在默认受支持，设置 jstmpllitinterp 没有
效果。
*/
package template
