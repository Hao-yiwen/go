// 版权所有 2012 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// Addr2line 是 GNU addr2line 工具的最小模拟实现，
// 仅提供足够支持 pprof 的功能。
//
// 用法：
//
//	go tool addr2line binary
//
// Addr2line 从标准输入读取十六进制地址，每行一个，可带可选的 0x 前缀。
// 对于每个输入地址，addr2line 输出两行，
// 第一行是包含该地址的函数名，第二行是对应源代码的 文件:行号。
//
// 本工具仅供 pprof 使用；其接口可能会更改，
// 或在未来版本中被完全删除。
package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"cmd/internal/objfile"
	"cmd/internal/telemetry/counter"
)

func printUsage(w *os.File) {
	fmt.Fprintf(w, "usage: addr2line binary\n")
	fmt.Fprintf(w, "reads addresses from standard input and writes two lines for each:\n")
	fmt.Fprintf(w, "\tfunction name\n")
	fmt.Fprintf(w, "\tfile:line\n")
}

func usage() {
	printUsage(os.Stderr)
	os.Exit(2)
}

func main() {
	log.SetFlags(0)
	log.SetPrefix("addr2line: ")
	counter.Open()

	// pprof 在检查 addr2line 时期望这种行为
	if len(os.Args) > 1 && os.Args[1] == "--help" {
		printUsage(os.Stdout)
		os.Exit(0)
	}

	flag.Usage = usage
	flag.Parse()
	counter.Inc("addr2line/invocations")
	counter.CountFlags("addr2line/flag:", *flag.CommandLine)
	if flag.NArg() != 1 {
		usage()
	}

	f, err := objfile.Open(flag.Arg(0))
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	tab, err := f.PCLineTable()
	if err != nil {
		log.Fatalf("reading %s: %v", flag.Arg(0), err)
	}

	stdin := bufio.NewScanner(os.Stdin)
	stdout := bufio.NewWriter(os.Stdout)

	for stdin.Scan() {
		p := stdin.Text()
		if strings.Contains(p, ":") {
			// 将 文件:行号 反向转换为程序计数器(pc)。
			// 这是旧版 C 语言实现的 'go tool addr2line' 中的扩展功能，
			// 可能没有人使用，但我们仍然识别这种语法。
			// 我们没有实现此功能。
			fmt.Fprintf(stdout, "!reverse translation not implemented\n")
			continue
		}
		pc, _ := strconv.ParseUint(strings.TrimPrefix(p, "0x"), 16, 64)
		file, line, fn := tab.PCToLine(pc)
		name := "?"
		if fn != nil {
			name = fn.Name
		} else {
			file = "?"
			line = 0
		}
		fmt.Fprintf(stdout, "%s\n%s:%d\n", name, file, line)
	}
	stdout.Flush()
}
