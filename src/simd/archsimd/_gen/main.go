// 版权所有 2025 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// 运行所有 SIMD 相关的代码生成器。
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const defaultXedPath = "$XEDPATH" + string(filepath.ListSeparator) + "./simdgen/xeddata" + string(filepath.ListSeparator) + "$HOME/xed/obj/dgen"

var (
	flagTmplgen = flag.Bool("tmplgen", true, "run tmplgen generator")
	flagSimdgen = flag.Bool("simdgen", true, "run simdgen generator")

	flagN       = flag.Bool("n", false, "dry run")
	flagXedPath = flag.String("xedPath", defaultXedPath, "load XED datafile from `path`, which must be the XED obj/dgen directory")
)

var goRoot string

func main() {
	flag.Parse()
	if flag.NArg() > 0 {
		flag.Usage()
		os.Exit(1)
	}

	if *flagXedPath == defaultXedPath {
		// 通常我们希望 shell 来进行变量展开，但对于
		// 默认值我们无法获得这个功能，所以自己来处理。
		*flagXedPath = os.ExpandEnv(defaultXedPath)
	}

	var err error
	goRoot, err = resolveGOROOT()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if *flagTmplgen {
		doTmplgen()
	}
	if *flagSimdgen {
		doSimdgen()
	}
}

func doTmplgen() {
	goRun("-C", "tmplgen", ".")
}

func doSimdgen() {
	xedPath, err := resolveXEDPath(*flagXedPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// 重新生成基于 XED 的 SIMD 文件
	goRun("-C", "simdgen", ".", "-o", "godefs", "-goroot", goRoot, "-xedPath", prettyPath("./simdgen", xedPath), "go.yaml", "types.yaml", "categories.yaml")

	// simdgen 会生成 SSA 规则文件，所以需要更新 SSA 文件
	goRun("-C", prettyPath(".", filepath.Join(goRoot, "src", "cmd", "compile", "internal", "ssa", "_gen")), ".")
}

func resolveXEDPath(pathList string) (xedPath string, err error) {
	for _, path := range filepath.SplitList(pathList) {
		if path == "" {
			// 可能是未知的 shell 变量。忽略。
			continue
		}
		if _, err := os.Stat(filepath.Join(path, "all-dec-instructions.txt")); err == nil {
			return filepath.Abs(path)
		}
	}
	return "", fmt.Errorf("set $XEDPATH or -xedPath to the XED obj/dgen directory")
}

func resolveGOROOT() (goRoot string, err error) {
	cmd := exec.Command("go", "env", "GOROOT")
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("%s: %s", cmd, err)
	}
	goRoot = strings.TrimSuffix(string(out), "\n")
	return goRoot, nil
}

func goRun(args ...string) {
	exe := filepath.Join(goRoot, "bin", "go")
	cmd := exec.Command(exe, append([]string{"run"}, args...)...)
	run(cmd)
}

func run(cmd *exec.Cmd) {
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	fmt.Fprintf(os.Stderr, "%s\n", cmdString(cmd))
	if *flagN {
		return
	}
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "%s failed: %s\n", cmd, err)
	}
}

func prettyPath(base, path string) string {
	base, err := filepath.Abs(base)
	if err != nil {
		return path
	}
	p, err := filepath.Rel(base, path)
	if err != nil {
		return path
	}
	return p
}

func cmdString(cmd *exec.Cmd) string {
	// TODO: Shell 引号处理？
	// TODO: 环境变量。

	var buf strings.Builder

	cmdPath, err := exec.LookPath(filepath.Base(cmd.Path))
	if err == nil && cmdPath == cmd.Path {
		cmdPath = filepath.Base(cmdPath)
	} else {
		cmdPath = prettyPath(".", cmd.Path)
	}
	buf.WriteString(cmdPath)

	for _, arg := range cmd.Args[1:] {
		buf.WriteByte(' ')
		buf.WriteString(arg)
	}

	return buf.String()
}
