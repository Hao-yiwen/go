// 版权所有 2011 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package main

import (
	"flag"
	"fmt"
	"go/build"
	"internal/testenv"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
)

var flagCheck = flag.Bool("check", false, "run API checks")

func TestMain(m *testing.M) {
	flag.Parse()
	for _, c := range contexts {
		c.Compiler = build.Default.Compiler
	}
	build.Default.GOROOT = testenv.GOROOT(nil)

	os.Exit(m.Run())
}

var (
	updateGolden = flag.Bool("updategolden", false, "update golden files")
)

func TestGolden(t *testing.T) {
	if *flagCheck {
		// 速度慢，不值得在 -check 模式下重复运行
		t.Skip("skipping with -check set")
	}

	testenv.MustHaveGoBuild(t)

	td, err := os.Open("testdata/src/pkg")
	if err != nil {
		t.Fatal(err)
	}
	fis, err := td.Readdir(0)
	if err != nil {
		t.Fatal(err)
	}
	for _, fi := range fis {
		if !fi.IsDir() {
			continue
		}

		// TODO(gri) 最终移除额外的 pkg 目录
		goldenFile := filepath.Join("testdata", "src", "pkg", fi.Name(), "golden.txt")
		w := NewWalker(nil, "testdata/src/pkg")
		pkg, err := w.import_(fi.Name())
		if err != nil {
			t.Fatalf("import %s: %v", fi.Name(), err)
		}
		w.export(pkg)

		if *updateGolden {
			os.Remove(goldenFile)
			f, err := os.Create(goldenFile)
			if err != nil {
				t.Fatal(err)
			}
			for _, feat := range w.Features() {
				fmt.Fprintf(f, "%s\n", feat)
			}
			f.Close()
		}

		bs, err := os.ReadFile(goldenFile)
		if err != nil {
			t.Fatalf("opening golden.txt for package %q: %v", fi.Name(), err)
		}
		wanted := strings.Split(string(bs), "\n")
		slices.Sort(wanted)
		for _, feature := range wanted {
			if feature == "" {
				continue
			}
			_, ok := w.features[feature]
			if !ok {
				t.Errorf("package %s: missing feature %q", fi.Name(), feature)
			}
			delete(w.features, feature)
		}

		for _, feature := range w.Features() {
			t.Errorf("package %s: extra feature not in golden file: %q", fi.Name(), feature)
		}
	}
}

func TestCompareAPI(t *testing.T) {
	if *flagCheck {
		// 不值得在 -check 模式下重复运行
		t.Skip("skipping with -check set")
	}

	tests := []struct {
		name                          string
		features, required, exception []string
		ok                            bool   // want
		out                           string // want
	}{
		{
			name:     "equal",
			features: []string{"A", "B", "C"},
			required: []string{"A", "B", "C"},
			ok:       true,
			out:      "",
		},
		{
			name:     "feature added",
			features: []string{"A", "B", "C", "D", "E", "F"},
			required: []string{"B", "D"},
			ok:       false,
			out:      "+A\n+C\n+E\n+F\n",
		},
		{
			name:     "feature removed",
			features: []string{"C", "A"},
			required: []string{"A", "B", "C"},
			ok:       false,
			out:      "-B\n",
		},
		{
			name:      "exception removal",
			features:  []string{"A", "C"},
			required:  []string{"A", "B", "C"},
			exception: []string{"B"},
			ok:        true,
			out:       "",
		},

		// 测试在部分平台上要求的功能可以被在所有平台上实现的相同功能隐式满足。
		// 也就是说，不应该报告 "pkg syscall (darwin-amd64), type RawSockaddrInet6 struct" 缺失。
		// 参见 https://go.dev/issue/4303。
		{
			name: "contexts reconverging after api/next/* update",
			features: []string{
				"A",
				"pkg syscall, type RawSockaddrInet6 struct",
			},
			required: []string{
				"A",
				"pkg syscall (darwin-amd64), type RawSockaddrInet6 struct", // api/go1.n.txt 文件
				"pkg syscall, type RawSockaddrInet6 struct",                // api/next/n.txt 文件
			},
			ok:  true,
			out: "",
		},
		{
			name: "contexts reconverging before api/next/* update",
			features: []string{
				"A",
				"pkg syscall, type RawSockaddrInet6 struct",
			},
			required: []string{
				"A",
				"pkg syscall (darwin-amd64), type RawSockaddrInet6 struct",
			},
			ok:  false,
			out: "+pkg syscall, type RawSockaddrInet6 struct\n",
		},
	}
	for _, tt := range tests {
		buf := new(strings.Builder)
		gotOK := compareAPI(buf, tt.features, tt.required, tt.exception)
		if gotOK != tt.ok {
			t.Errorf("%s: ok = %v; want %v", tt.name, gotOK, tt.ok)
		}
		if got := buf.String(); got != tt.out {
			t.Errorf("%s: output differs\nGOT:\n%s\nWANT:\n%s", tt.name, got, tt.out)
		}
	}
}

func TestSkipInternal(t *testing.T) {
	if *flagCheck {
		// 不值得在 -check 模式下重复运行
		t.Skip("skipping with -check set")
	}

	tests := []struct {
		pkg  string
		want bool
	}{
		{"net/http", true},
		{"net/http/internal-foo", true},
		{"net/http/internal", false},
		{"net/http/internal/bar", false},
		{"internal/foo", false},
		{"internal", false},
	}
	for _, tt := range tests {
		got := !internalPkg.MatchString(tt.pkg)
		if got != tt.want {
			t.Errorf("%s is internal = %v; want %v", tt.pkg, got, tt.want)
		}
	}
}

func BenchmarkAll(b *testing.B) {
	for i := 0; i < b.N; i++ {
		for _, context := range contexts {
			w := NewWalker(context, filepath.Join(testenv.GOROOT(b), "src"))
			for _, name := range w.stdPackages {
				pkg, err := w.import_(name)
				if _, nogo := err.(*build.NoGoError); nogo {
					continue
				}
				if err != nil {
					b.Fatalf("import %s (%s-%s): %v", name, context.GOOS, context.GOARCH, err)
				}
				w.export(pkg)
			}
			w.Features()
		}
	}
}

var warmupCache = sync.OnceFunc(func() {
	// 并行预热导入缓存。
	var wg sync.WaitGroup
	for _, context := range contexts {
		context := context
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = NewWalker(context, filepath.Join(testenv.GOROOT(nil), "src"))
		}()
	}
	wg.Wait()
})

func TestIssue21181(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping with -short")
	}
	if *flagCheck {
		// 速度慢，不值得在 -check 模式下重复运行
		t.Skip("skipping with -check set")
	}
	testenv.MustHaveGoBuild(t)

	warmupCache()

	for _, context := range contexts {
		w := NewWalker(context, "testdata/src/issue21181")
		pkg, err := w.import_("p")
		if err != nil {
			t.Fatalf("import %s (%s-%s): %v", "p", context.GOOS, context.GOARCH, err)
		}
		w.export(pkg)
	}
}

func TestIssue29837(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping with -short")
	}
	if *flagCheck {
		// 速度慢，不值得在 -check 模式下重复运行
		t.Skip("skipping with -check set")
	}
	testenv.MustHaveGoBuild(t)

	warmupCache()

	for _, context := range contexts {
		w := NewWalker(context, "testdata/src/issue29837")
		_, err := w.ImportFrom("p", "", 0)
		if _, nogo := err.(*build.NoGoError); !nogo {
			t.Errorf("expected *build.NoGoError, got %T", err)
		}
	}
}

func TestIssue41358(t *testing.T) {
	if *flagCheck {
		// 速度慢，不值得在 -check 模式下重复运行
		t.Skip("skipping with -check set")
	}
	testenv.MustHaveGoBuild(t)
	context := new(build.Context)
	*context = build.Default
	context.Dir = filepath.Join(testenv.GOROOT(t), "src")

	w := NewWalker(context, context.Dir)
	for _, pkg := range w.stdPackages {
		if strings.HasPrefix(pkg, "vendor/") || strings.HasPrefix(pkg, "golang.org/x/") {
			t.Fatalf("stdPackages contains unexpected package %s", pkg)
		}
	}
}

func TestIssue64958(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping with -short")
	}
	if *flagCheck {
		// 速度慢，不值得在 -check 模式下重复运行
		t.Skip("skipping with -check set")
	}
	testenv.MustHaveGoBuild(t)

	defer func() {
		if x := recover(); x != nil {
			t.Errorf("expected no panic; recovered %v", x)
		}
	}()
	for _, context := range contexts {
		w := NewWalker(context, "testdata/src/issue64958")
		pkg, err := w.importFrom("p", "", 0)
		if err != nil {
			t.Errorf("expected no error importing; got %T", err)
		}
		w.export(pkg)
	}
}

func TestCheck(t *testing.T) {
	if !*flagCheck {
		t.Skip("-check not specified")
	}
	testenv.MustHaveGoBuild(t)
	Check(t)
}
