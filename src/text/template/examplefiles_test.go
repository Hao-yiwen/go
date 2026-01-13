// 版权所有 2012 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package template_test

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"text/template"
)

// templateFile 定义要存储在文件中的模板内容，用于测试。
type templateFile struct {
	name     string
	contents string
}

func createTestDir(files []templateFile) string {
	dir, err := os.MkdirTemp("", "template")
	if err != nil {
		log.Fatal(err)
	}
	for _, file := range files {
		f, err := os.Create(filepath.Join(dir, file.name))
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()
		_, err = io.WriteString(f, file.contents)
		if err != nil {
			log.Fatal(err)
		}
	}
	return dir
}

// 这里我们演示从目录加载一组模板。
func ExampleTemplate_glob() {
	// 这里我们创建一个临时目录并用我们的示例模板定义文件填充它；
	// 通常模板文件已经存在于程序已知的某个位置。
	dir := createTestDir([]templateFile{
		// T0.tmpl 是一个普通的模板文件，只是调用 T1。
		{"T0.tmpl", `T0 invokes T1: ({{template "T1"}})`},
		// T1.tmpl 定义了一个模板 T1，它调用 T2。
		{"T1.tmpl", `{{define "T1"}}T1 invokes T2: ({{template "T2"}}){{end}}`},
		// T2.tmpl 定义了模板 T2。
		{"T2.tmpl", `{{define "T2"}}This is T2{{end}}`},
	})
	// 测试后清理；作为示例运行的另一个怪癖。
	defer os.RemoveAll(dir)

	// pattern 是用于查找所有模板文件的 glob 模式。
	pattern := filepath.Join(dir, "*.tmpl")

	// 这里开始实际的示例。
	// T0.tmpl 是第一个匹配的名称，所以它成为起始模板，
	// 即 ParseGlob 返回的值。
	tmpl := template.Must(template.ParseGlob(pattern))

	err := tmpl.Execute(os.Stdout, nil)
	if err != nil {
		log.Fatalf("template execution: %s", err)
	}
	// 输出：
	// T0 invokes T1: (T1 invokes T2: (This is T2))
}

// 此示例演示了共享一些模板并在不同上下文中使用它们的一种方法。
// 在这个变体中，我们手动将多个驱动模板添加到现有的模板集合中。
func ExampleTemplate_helpers() {
	// 这里我们创建一个临时目录并用我们的示例模板定义文件填充它；
	// 通常模板文件已经存在于程序已知的某个位置。
	dir := createTestDir([]templateFile{
		// T1.tmpl 定义了一个模板 T1，它调用 T2。
		{"T1.tmpl", `{{define "T1"}}T1 invokes T2: ({{template "T2"}}){{end}}`},
		// T2.tmpl 定义了模板 T2。
		{"T2.tmpl", `{{define "T2"}}This is T2{{end}}`},
	})
	// 测试后清理；作为示例运行的另一个怪癖。
	defer os.RemoveAll(dir)

	// pattern 是用于查找所有模板文件的 glob 模式。
	pattern := filepath.Join(dir, "*.tmpl")

	// 这里开始实际的示例。
	// 加载辅助模板。
	templates := template.Must(template.ParseGlob(pattern))
	// 向集合中添加一个驱动模板；我们通过显式模板定义来完成。
	_, err := templates.Parse("{{define `driver1`}}Driver 1 calls T1: ({{template `T1`}})\n{{end}}")
	if err != nil {
		log.Fatal("parsing driver1: ", err)
	}
	// 添加另一个驱动模板。
	_, err = templates.Parse("{{define `driver2`}}Driver 2 calls T2: ({{template `T2`}})\n{{end}}")
	if err != nil {
		log.Fatal("parsing driver2: ", err)
	}
	// 我们在执行之前加载所有模板。本包不要求这种行为，
	// 但 html/template 的转义需要，所以这是一个好习惯。
	err = templates.ExecuteTemplate(os.Stdout, "driver1", nil)
	if err != nil {
		log.Fatalf("driver1 execution: %s", err)
	}
	err = templates.ExecuteTemplate(os.Stdout, "driver2", nil)
	if err != nil {
		log.Fatalf("driver2 execution: %s", err)
	}
	// 输出：
	// Driver 1 calls T1: (T1 invokes T2: (This is T2))
	// Driver 2 calls T2: (This is T2)
}

// This example demonstrates how to use one group of driver
// templates with distinct sets of helper templates.
func ExampleTemplate_share() {
	// Here we create a temporary directory and populate it with our sample
	// template definition files; usually the template files would already
	// exist in some location known to the program.
	dir := createTestDir([]templateFile{
		// T0.tmpl is a plain template file that just invokes T1.
		{"T0.tmpl", "T0 ({{.}} version) invokes T1: ({{template `T1`}})\n"},
		// T1.tmpl defines a template, T1 that invokes T2. Note T2 is not defined
		{"T1.tmpl", `{{define "T1"}}T1 invokes T2: ({{template "T2"}}){{end}}`},
	})
	// Clean up after the test; another quirk of running as an example.
	defer os.RemoveAll(dir)

	// pattern is the glob pattern used to find all the template files.
	pattern := filepath.Join(dir, "*.tmpl")

	// Here starts the example proper.
	// Load the drivers.
	drivers := template.Must(template.ParseGlob(pattern))

	// We must define an implementation of the T2 template. First we clone
	// the drivers, then add a definition of T2 to the template name space.

	// 1. Clone the helper set to create a new name space from which to run them.
	first, err := drivers.Clone()
	if err != nil {
		log.Fatal("cloning helpers: ", err)
	}
	// 2. Define T2, version A, and parse it.
	_, err = first.Parse("{{define `T2`}}T2, version A{{end}}")
	if err != nil {
		log.Fatal("parsing T2: ", err)
	}

	// Now repeat the whole thing, using a different version of T2.
	// 1. Clone the drivers.
	second, err := drivers.Clone()
	if err != nil {
		log.Fatal("cloning drivers: ", err)
	}
	// 2. Define T2, version B, and parse it.
	_, err = second.Parse("{{define `T2`}}T2, version B{{end}}")
	if err != nil {
		log.Fatal("parsing T2: ", err)
	}

	// Execute the templates in the reverse order to verify the
	// first is unaffected by the second.
	err = second.ExecuteTemplate(os.Stdout, "T0.tmpl", "second")
	if err != nil {
		log.Fatalf("second execution: %s", err)
	}
	err = first.ExecuteTemplate(os.Stdout, "T0.tmpl", "first")
	if err != nil {
		log.Fatalf("first: execution: %s", err)
	}

	// Output:
	// T0 (second version) invokes T1: (T1 invokes T2: (T2, version B))
	// T0 (first version) invokes T1: (T1 invokes T2: (T2, version A))
}
