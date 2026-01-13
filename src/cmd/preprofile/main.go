// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Preprofile 为编译器中的 PGO 使用创建 pprof 配置文件的中间表示。
// 此转换仅取决于配置文件本身，因此在编译器的每次调用中执行会很浪费。
//
// 用法：
//
//	go tool preprofile [-V] [-o output] -i input
package main

import (
	"bufio"
	"cmd/internal/objabi"
	"cmd/internal/pgo"
	"cmd/internal/telemetry/counter"
	"flag"
	"fmt"
	"log"
	"os"
)

func usage() {
	fmt.Fprintf(os.Stderr, "用法: go tool preprofile [-V] [-o output] -i input\n\n")
	flag.PrintDefaults()
	os.Exit(2)
}

var (
	output = flag.String("o", "", "输出文件路径")
	input  = flag.String("i", "", "输入 pprof 文件路径")
)

func preprocess(profileFile string, outputFile string) error {
	f, err := os.Open(profileFile)
	if err != nil {
		return fmt.Errorf("打开配置文件时出错: %w", err)
	}
	defer f.Close()

	r := bufio.NewReader(f)
	d, err := pgo.FromPProf(r)
	if err != nil {
		return fmt.Errorf("解析配置文件时出错: %w", err)
	}

	var out *os.File
	if outputFile == "" {
		out = os.Stdout
	} else {
		out, err = os.Create(outputFile)
		if err != nil {
			return fmt.Errorf("创建输出文件时出错: %w", err)
		}
		defer out.Close()
	}

	w := bufio.NewWriter(out)
	if _, err := d.WriteTo(w); err != nil {
		return fmt.Errorf("写入输出文件时出错: %w", err)
	}

	return nil
}

func main() {
	objabi.AddVersionFlag()

	log.SetFlags(0)
	log.SetPrefix("preprofile: ")
	counter.Open()

	flag.Usage = usage
	flag.Parse()
	counter.Inc("preprofile/invocations")
	counter.CountFlags("preprofile/flag:", *flag.CommandLine)
	if *input == "" {
		log.Print("需要输入 pprof 路径 (-i)")
		usage()
	}

	if err := preprocess(*input, *output); err != nil {
		log.Fatal(err)
	}
}
