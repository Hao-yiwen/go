// 版权所有 2021 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package abi_test

import (
	"internal/abi"
	"internal/testenv"
	"path/filepath"
	"strings"
	"testing"
)

func TestFuncPC(t *testing.T) {
	// 测试 FuncPC* 能否获取正确的函数 PC。
	pcFromAsm := abi.FuncPCTestFnAddr

	// 测试本地定义函数的 FuncPC
	pcFromGo := abi.FuncPCTest()
	if pcFromGo != pcFromAsm {
		t.Errorf("FuncPC returns wrong PC, want %x, got %x", pcFromAsm, pcFromGo)
	}

	// 测试导入函数的 FuncPC
	pcFromGo = abi.FuncPCABI0(abi.FuncPCTestFn)
	if pcFromGo != pcFromAsm {
		t.Errorf("FuncPC returns wrong PC, want %x, got %x", pcFromAsm, pcFromGo)
	}
}

func TestFuncPCCompileError(t *testing.T) {
	// 测试对 ABI 不匹配的函数调用 FuncPC* 会被拒绝。
	testenv.MustHaveGoBuild(t)

	// 我们想测试 internal 包，但通常无法导入它。
	// 手动运行汇编器和编译器。
	tmpdir := t.TempDir()
	asmSrc := filepath.Join("testdata", "x.s")
	goSrc := filepath.Join("testdata", "x.go")
	symabi := filepath.Join(tmpdir, "symabi")
	obj := filepath.Join(tmpdir, "x.o")

	// 为包的依赖项编写 importcfg 文件。
	importcfgfile := filepath.Join(tmpdir, "hello.importcfg")
	testenv.WriteImportcfg(t, importcfgfile, nil, "internal/abi")

	// 解析汇编代码以获取 symabi。
	cmd := testenv.Command(t, testenv.GoToolPath(t), "tool", "asm", "-p=p", "-gensymabis", "-o", symabi, asmSrc)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go tool asm -gensymabis failed: %v\n%s", err, out)
	}

	// 编译 go 代码。
	cmd = testenv.Command(t, testenv.GoToolPath(t), "tool", "compile", "-importcfg="+importcfgfile, "-p=p", "-symabis", symabi, "-o", obj, goSrc)
	out, err = cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("go tool compile did not fail")
	}

	// 期望在第 17、18、20 行出错，其他行不应有错误。
	want := []string{"x.go:17", "x.go:18", "x.go:20"}
	got := strings.Split(string(out), "\n")
	if got[len(got)-1] == "" {
		got = got[:len(got)-1] // 移除最后的空行
	}
	for i, s := range got {
		if !strings.Contains(s, want[i]) {
			t.Errorf("did not error on line %s", want[i])
		}
	}
	if len(got) != len(want) {
		t.Errorf("unexpected number of errors, want %d, got %d", len(want), len(got))
	}
	if t.Failed() {
		t.Logf("output:\n%s", string(out))
	}
}
