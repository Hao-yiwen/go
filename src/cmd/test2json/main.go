// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Test2json 将 go test 输出转换为机器可读的 JSON 流。
//
// 用法：
//
//	go tool test2json [-p pkg] [-t] [./pkg.test -test.v=test2json]
//
// Test2json 运行给定的测试命令并将其输出转换为 JSON；
// 如果未指定命令，test2json 期望从标准输入读取测试输出。
// 它将相应的 JSON 事件流写入标准输出。
// 没有不必要的输入或输出缓冲，以便
// JSON 流可以被读取以获得测试状态的"实时更新"。
//
// -p 标志设置在每个测试事件中报告的包。
//
// -t 标志请求将时间戳添加到每个测试事件中。
//
// 应使用 -test.v=test2json 调用测试。仅使用 -test.v
// （或 -test.v=true）是可接受的，但产生较低保真度结果。
//
// 注意 "go test -json" 会正确调用 test2json，
// 所以 "go tool test2json" 仅在测试二进制文件被运行
// 时才需要 与 "go test" 分开。尽可能使用 "go test -json"。
//
// 另请注意，test2json 仅用于转换单个测试
// 二进制文件的输出。要转换 "go test" 命令的输出
// 运行多个包，请再次使用 "go test -json"。
//
// # 输出格式
//
// JSON 流是 TestEvent 对象的换行符分隔序列，
// 对应于 Go 结构：
//
//	type TestEvent struct {
//		Time        time.Time // 编码为 RFC3339 格式字符串
//		Action      string
//		Package     string
//		Test        string
//		Elapsed     float64 // 秒数
//		Output      string
//		FailedBuild string
//	}
//
// Time 字段保持事件发生的时间。
// 对于缓存的测试结果，通常会省略。
//
// Action 字段是一个固定的动作描述集合之一：
//
//	start  - 测试二进制文件即将执行
//	run    - 测试已开始运行
//	pause  - 测试已暂停
//	cont   - 测试已继续运行
//	pass   - 测试通过
//	bench  - 基准打印日志输出但没有失败
//	fail   - 测试或基准失败
//	output - 测试打印输出
//	skip   - 测试被跳过或包不包含测试
//
// 每个 JSON 流都以 "start" 事件开始。
//
// Package 字段（如果存在）指定被测试的包。
// 当 go 命令以 -json 模式运行并行测试时，来自
// 不同测试的事件是交错的；Package 字段允许读取器
// 分离它们。
//
// Test 字段（如果存在）指定导致事件的测试、示例或基准
// 函数。整体包测试的事件
// 不设置 Test。
//
// Elapsed 字段为 "pass" 和 "fail" 事件设置。它给出
// 特定测试或整体包测试通过或失败的已用时间。
//
// Output 字段为 Action == "output" 时设置，是测试输出的一部分
// （标准输出和标准错误合并在一起）。输出是
// 未修改的，除了来自测试的无效 UTF-8 输出被强制
// 成为有效的 UTF-8，通过使用替换字符。除了那个例外，
// 所有输出事件的 Output 字段的连接是准确的
// 测试执行的输出。
//
// FailedBuild 字段在 Action == "fail" 时设置，如果测试失败是
// 由构建失败引起的。它包含包的包 ID
// 未能构建。这与 "go list" 输出的 ImportPath 字段匹配，
// 以及由 "go build -json" 发出的 BuildEvent.ImportPath 字段。
//
// 当基准运行时，通常会产生单行输出
// 给出计时结果。该行在 Action == "output" 的事件中报告
// 且没有 Test 字段。如果基准记录输出或报告失败
// （例如，通过使用 b.Log 或 b.Error），该额外输出被报告
// 为具有设置为基准名称的 Test 的事件序列，以
// 最终事件（Action == "bench" 或 "fail"）终止。
// 基准没有 Action == "pause" 的事件。
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"

	"cmd/internal/telemetry/counter"
	"cmd/internal/test2json"
)

var (
	flagP = flag.String("p", "", "report `pkg` as the package being tested in each event")
	flagT = flag.Bool("t", false, "include timestamps in events")
)

func usage() {
	fmt.Fprintf(os.Stderr, "usage: go tool test2json [-p pkg] [-t] [./pkg.test -test.v]\n")
	os.Exit(2)
}

// ignoreSignals 忽略中断信号。
func ignoreSignals() {
	signal.Ignore(signalsToIgnore...)
}

func main() {
	counter.Open()

	flag.Usage = usage
	flag.Parse()
	counter.Inc("test2json/invocations")
	counter.CountFlags("test2json/flag:", *flag.CommandLine)

	var mode test2json.Mode
	if *flagT {
		mode |= test2json.Timestamp
	}
	c := test2json.NewConverter(os.Stdout, *flagP, mode)
	defer c.Close()

	if flag.NArg() == 0 {
		io.Copy(c, os.Stdin)
	} else {
		args := flag.Args()
		cmd := exec.Command(args[0], args[1:]...)
		w := &countWriter{0, c}
		cmd.Stdout = w
		cmd.Stderr = w
		ignoreSignals()
		err := cmd.Run()
		if err != nil {
			if w.n > 0 {
				// 假设命令打印了失败的原因。
			} else {
				fmt.Fprintf(c, "test2json: %v\n", err)
			}
		}
		c.Exited(err)
		if err != nil {
			c.Close()
			os.Exit(1)
		}
	}
}

type countWriter struct {
	n int64
	w io.Writer
}

func (w *countWriter) Write(b []byte) (int, error) {
	w.n += int64(len(b))
	return w.w.Write(b)
}
