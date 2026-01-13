// 版权所有 2017 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"cmd/internal/buildid"
	"cmd/internal/telemetry/counter"
)

func usage() {
	fmt.Fprintf(os.Stderr, "usage: go tool buildid [-w] file\n")
	flag.PrintDefaults()
	os.Exit(2)
}

var wflag = flag.Bool("w", false, "write build ID")

func main() {
	log.SetPrefix("buildid: ")
	log.SetFlags(0)
	counter.Open()
	flag.Usage = usage
	flag.Parse()
	counter.Inc("buildid/invocations")
	counter.CountFlags("buildid/flag:", *flag.CommandLine)
	if flag.NArg() != 1 {
		usage()
	}

	file := flag.Arg(0)
	id, err := buildid.ReadFile(file)
	if err != nil {
		log.Fatal(err)
	}
	if !*wflag {
		fmt.Printf("%s\n", id)
		return
	}

	// 与 src/cmd/go/internal/work/buildid.go:updateBuildID 保持同步

	f, err := os.Open(file)
	if err != nil {
		log.Fatal(err)
	}
	matches, hash, err := buildid.FindAndHash(f, id, 0)
	f.Close()
	if err != nil {
		log.Fatal(err)
	}

	// <= go 1.7 不嵌入 contentID 或 actionID，因此没有斜杠
	if !strings.Contains(id, "/") {
		log.Fatalf("%s: build ID is a legacy format...binary too old for this tool", file)
	}

	newID := id[:strings.LastIndex(id, "/")] + "/" + buildid.HashToString(hash)
	if len(newID) != len(id) {
		log.Fatalf("%s: build ID length mismatch %q vs %q", file, id, newID)
	}

	if len(matches) == 0 {
		return
	}

	f, err = os.OpenFile(file, os.O_RDWR, 0)
	if err != nil {
		log.Fatal(err)
	}
	if err := buildid.Rewrite(f, matches, newID); err != nil {
		log.Fatal(err)
	}
	if err := f.Close(); err != nil {
		log.Fatal(err)
	}
}
