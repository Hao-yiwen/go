// 版权所有 2022 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

/*
Package comment implements parsing and reformatting of Go doc comments,
(documentation comments), which are comments that immediately precede
a top-level declaration of a package, const, func, type, or var.

Go doc comment syntax 是一个 simplified subset of Markdown that supports
links, headings, paragraphs, lists (without nesting), and preformatted text blocks.
The details of the syntax are documented at https://go.dev/doc/comment.

To parse the text associated with a doc comment (after removing comment markers),
use a [Parser]:

	var p comment.Parser
	doc := p.Parse(text)

The result 是一个 [*Doc].
To reformat it as a doc comment, HTML, Markdown, or plain text,
use a [Printer]:

	var pr comment.Printer
	os.Stdout.Write(pr.Text(doc))

The [Parser] and [Printer] types are structs whose fields can be
modified to customize the operations.
For details, see the documentation for those types.

Use cases that need additional control over reformatting can
implement their own logic by inspecting the parsed syntax itself.
See the documentation for [Doc], [Block], [Text] for an overview
and links to additional types.
*/
package comment
