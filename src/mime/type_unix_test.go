// 版权所有 2021 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

//go:build unix || (js && wasm)

package mime

import (
	"testing"
)

func initMimeUnixTest(t *testing.T) {
	once.Do(initMime)
	err := loadMimeGlobsFile("testdata/test.types.globs2")
	if err != nil {
		t.Fatal(err)
	}

	loadMimeFile("testdata/test.types")
}

func TestTypeByExtensionUNIX(t *testing.T) {
	initMimeUnixTest(t)
	typeTests := map[string]string{
		".T1":       "application/test",
		".t2":       "text/test; charset=utf-8",
		".t3":       "document/test",
		".t4":       "example/test",
		".png":      "image/png",
		",v":        "",
		"~":         "",
		".foo?ar":   "",
		".foo*r":    "",
		".foo[1-3]": "",
	}

	for ext, want := range typeTests {
		val := TypeByExtension(ext)
		if val != want {
			t.Errorf("TypeByExtension(%q) = %q, want %q", ext, val, want)
		}
	}
}
