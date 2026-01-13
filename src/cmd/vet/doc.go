// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

/*
Vet 检查 Go 源代码并报告可疑的代码构造，例如 Printf
调用的参数与格式字符串不匹配的情况。Vet 使用启发式方法
来检查，但不能保证所有报告都是真正的问题，但它可以找到
编译器未捕获的错误。

Vet 通常通过 go 命令调用。
此命令检查当前目录中的包：

	go vet

而这个命令检查其路径提供的包：

	go vet my/project/...

使用 "go help packages" 可以看到指定要检查的包的其他方式。

Vet 的退出代码在工具调用出错或报告了问题时非零，
否则为 0。注意该工具不会检查所有可能的问题，
它依赖于不可靠的启发式方法，因此应仅用作指导，
不应作为程序正确性的确凿指示。

要列出可用的检查，运行 "go tool vet help"：

	appends          检查 append 后缺失的值
	asmdecl          报告汇编文件和 Go 声明之间的不匹配
	assign           检查无用的赋值
	atomic           检查使用 sync/atomic 包的常见错误
	bools            检查涉及布尔运算符的常见错误
	buildtag         检查 //go:build 和 // +build 指令
	cgocall          检测 cgo 指针传递规则的一些违反
	composites       检查未加键的复合字面量
	copylocks        检查错误地按值传递的锁
	defers           报告 defer 语句中的常见错误
	directive        检查 Go 工具链指令，例如 //go:debug
	errorsas         报告向 errors.As 传递非指针或非错误值
	framepointer     报告在保存前破坏帧指针的汇编
	hostport         检查传递给 net.Dial 的地址格式
	httpresponse     检查使用 HTTP 响应的错误
	ifaceassert      检测不可能的接口到接口类型断言
	loopclosure      检查嵌套函数内对循环变量的引用
	lostcancel       检查由 context.WithCancel 返回的 cancel 函数是否被调用
	nilfunc          检查函数与 nil 之间的无用比较
	printf           检查 Printf 格式字符串和参数的一致性
	shift            检查移位是否等于或超过整数的宽度
	sigchanyzer      检查 os.Signal 的无缓冲通道
	slog             检查无效的结构化日志调用
	stdmethods       检查众所周知的接口方法的签名
	stdversion       报告使用过新的标准库符号
	stringintconv    检查 string(int) 转换
	structtag        检查结构体字段标签是否符合 reflect.StructTag.Get
	testinggoroutine 报告从测试启动的 goroutine 中对 (*testing.T).Fatal 的调用
	tests            检查常见的测试和示例使用错误
	timeformat       检查 (time.Time).Format 或 time.Parse 的调用（格式为 2006-02-01）
	unmarshal        报告向 unmarshal 传递非指针或非接口值
	unreachable      检查不可达代码
	unsafeptr        检查从 uintptr 到 unsafe.Pointer 的无效转换
	unusedresult     检查对某些函数调用结果的未使用

有关特定检查（如 printf）的详细信息和标志，运行 "go tool vet help printf"。

默认情况下，执行所有检查。
如果明确设置任何标志为真，只运行这些测试。
相反，如果明确设置任何标志为假，只禁用这些测试。
因此 -printf=true 运行 printf 检查，
而 -printf=false 运行除 printf 检查之外的所有检查。

有关编写新检查的信息，请参阅 golang.org/x/tools/go/analysis。

核心标志：

	-c=N
	  	显示违规行加上 N 行周围上下文
	-json
	  	以 JSON 格式发出分析诊断（和错误）
*/
package main
