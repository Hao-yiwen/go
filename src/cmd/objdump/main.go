// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Objdump 反汇编可执行文件。
//
// 用法：
//
//	go tool objdump [-s symregexp] binary
//
// Objdump 打印二进制文件中所有文本符号（代码）的反汇编。
// 如果存在 -s 选项，objdump 只反汇编
// 名称匹配正则表达式的符号。
//
// 替代用法：
//
//	go tool objdump binary start end
//
// 在此模式下，objdump 从起始地址开始反汇编二进制文件，
// 在结束地址处停止。起始和结束地址是以十六进制
// 写入的程序计数器，可选带有前导 0x 前缀。
// 在此模式下，objdump 打印以下形式的一系列节：
//
//	file:line
//	 address: assembly
//	 address: assembly
//	 ...
//
// 每个节给出地址连续范围的反汇编，
// 所有地址都映射到同一原始源文件和行号。
// 此模式旨在供 pprof 使用。
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"

	"cmd/internal/disasm"
	"cmd/internal/objfile"
	"cmd/internal/telemetry/counter"
)

var printCode = flag.Bool("S", false, "在汇编旁打印 Go 代码")
var symregexp = flag.String("s", "", "只转储名称匹配此正则表达式的符号")
var gnuAsm = flag.Bool("gnu", false, "在 Go 汇编旁打印 GNU 汇编（如果支持）")
var symRE *regexp.Regexp

func usage() {
	fmt.Fprintf(os.Stderr, "用法: go tool objdump [-S] [-gnu] [-s symregexp] binary [start end]\n\n")
	flag.PrintDefaults()
	os.Exit(2)
}

func main() {
	log.SetFlags(0)
	log.SetPrefix("objdump: ")
	counter.Open()

	flag.Usage = usage
	flag.Parse()
	counter.Inc("objdump/invocations")
	counter.CountFlags("objdump/flag:", *flag.CommandLine)
	if flag.NArg() != 1 && flag.NArg() != 3 {
		usage()
	}

	if *symregexp != "" {
		re, err := regexp.Compile(*symregexp)
		if err != nil {
			log.Fatalf("无效的 -s 正则表达式: %v", err)
		}
		symRE = re
	}

	f, err := objfile.Open(flag.Arg(0))
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	dis, err := disasm.DisasmForFile(f)
	if err != nil {
		log.Fatalf("反汇编 %s: %v", flag.Arg(0), err)
	}

	switch flag.NArg() {
	default:
		usage()
	case 1:
		// 整个对象的反汇编
		dis.Print(os.Stdout, symRE, 0, ^uint64(0), *printCode, *gnuAsm)

	case 3:
		// PC 范围的反汇编
		start, err := strconv.ParseUint(strings.TrimPrefix(flag.Arg(1), "0x"), 16, 64)
		if err != nil {
			log.Fatalf("无效的起始 PC: %v", err)
		}
		end, err := strconv.ParseUint(strings.TrimPrefix(flag.Arg(2), "0x"), 16, 64)
		if err != nil {
			log.Fatalf("无效的结束 PC: %v", err)
		}
		dis.Print(os.Stdout, symRE, start, end, *printCode, *gnuAsm)
	}
}
