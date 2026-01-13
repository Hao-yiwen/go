// 版权所有 2012 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package template_test

import (
	"log"
	"os"
	"strings"
	"text/template"
)

// 此示例演示了一个用于处理模板文本的自定义函数。
// 它安装 strings.Title 函数并使用它来
// 使标题文本在我们的模板输出中看起来更好。
func ExampleTemplate_func() {
	// 首先我们创建一个 FuncMap 来注册函数。
	funcMap := template.FuncMap{
		// 名称 "title" 是函数在模板文本中被调用的名称。
		"title": strings.Title,
	}

	// 一个简单的模板定义来测试我们的函数。
	// 我们以多种方式打印输入文本：
	// - 原始文本
	// - 标题大小写
	// - 标题大小写然后用 %q 打印
	// - 用 %q 打印然后标题大小写。
	const templateText = `
Input: {{printf "%q" .}}
Output 0: {{title .}}
Output 1: {{title . | printf "%q"}}
Output 2: {{printf "%q" . | title}}
`

	// 创建模板，添加函数映射，并解析文本。
	tmpl, err := template.New("titleTest").Funcs(funcMap).Parse(templateText)
	if err != nil {
		log.Fatalf("parsing: %s", err)
	}

	// 运行模板以验证输出。
	err = tmpl.Execute(os.Stdout, "the go programming language")
	if err != nil {
		log.Fatalf("execution: %s", err)
	}

	// 输出：
	// Input: "the go programming language"
	// Output 0: The Go Programming Language
	// Output 1: "The Go Programming Language"
	// Output 2: "The Go Programming Language"
}

// This example demonstrates registering two custom template functions
// and how to overwite one of the functions after the template has been
// parsed. Overwriting can be used, for example, to alter the operation
// of cloned templates.
func ExampleTemplate_funcs() {

	// Define a simple template to test the functions.
	const tmpl = `{{ . | lower | repeat }}`

	// Define the template funcMap with two functions.
	var funcMap = template.FuncMap{
		"lower":  strings.ToLower,
		"repeat": func(s string) string { return strings.Repeat(s, 2) },
	}

	// Define a New template, add the funcMap using Funcs and then Parse
	// the template.
	parsedTmpl, err := template.New("t").Funcs(funcMap).Parse(tmpl)
	if err != nil {
		log.Fatal(err)
	}
	if err := parsedTmpl.Execute(os.Stdout, "ABC\n"); err != nil {
		log.Fatal(err)
	}

	// [Funcs] must be called before a template is parsed to add
	// functions to the template. [Funcs] can also be used after a
	// template is parsed to overwrite template functions.
	//
	// Here the function identified by "repeat" is overwritten.
	parsedTmpl.Funcs(template.FuncMap{
		"repeat": func(s string) string { return strings.Repeat(s, 3) },
	})
	if err := parsedTmpl.Execute(os.Stdout, "DEF\n"); err != nil {
		log.Fatal(err)
	}
	// Output:
	// abc
	// abc
	// def
	// def
	// def
}

// This example demonstrates how to use "if".
func ExampleTemplate_if() {
	type book struct {
		Stars float32
		Name  string
	}

	tpl, err := template.New("book").Parse(`{{ if (gt .Stars 4.0) }}"{{.Name }}" is a great book.{{ else }}"{{.Name}}" is not a great book.{{ end }}`)
	if err != nil {
		log.Fatalf("failed to parse template: %s", err)
	}

	b := &book{
		Stars: 4.9,
		Name:  "Good Night, Gopher",
	}
	err = tpl.Execute(os.Stdout, b)
	if err != nil {
		log.Fatalf("failed to execute template: %s", err)
	}

	// Output:
	// "Good Night, Gopher" is a great book.
}
