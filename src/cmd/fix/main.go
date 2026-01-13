// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

/*
Fix 是由 "go fix" 执行的工具，用于更新使用旧语言和库特性的 Go 程序，
并将它们重写为使用较新的特性。更新到新的 Go 版本后，fix 可以帮助进行
必要的更改。

有关如何运行此命令的文档，请参见 "go fix"。
您可以使用 "go fix -fixtool=..." 提供替代工具。

运行 "go tool fix help" 以查看此程序支持的分析器列表。

有关如何编写可以建议修复的分析器的信息，请参见 [golang.org/x/tools/go/analysis]。
*/
package main

import (
	"cmd/internal/objabi"
	"cmd/internal/telemetry/counter"
	"slices"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildtag"
	"golang.org/x/tools/go/analysis/passes/hostport"
	"golang.org/x/tools/go/analysis/passes/inline"
	"golang.org/x/tools/go/analysis/passes/modernize"
	"golang.org/x/tools/go/analysis/unitchecker"
)

func main() {
	// 与 cmd/vet/main.go 保持一致！
	counter.Open()
	objabi.AddVersionFlag()
	counter.Inc("fix/invocations")

	unitchecker.Main(suite...) // （永远不会返回）
}

// fix 套件分析器产生的修复可以明确安全地应用，
// 即使诊断可能不描述实际问题。
var suite = slices.Concat(
	[]*analysis.Analyzer{
		buildtag.Analyzer,
		hostport.Analyzer,
		inline.Analyzer,
	},
	modernize.Suite,
	// TODO(adonovan): 添加任何其他 vet 分析器，其修复始终是安全的。
	// 审计候选：sigchanyzer、printf、assign、unreachable。
	// staticcheck 的许多分析器将是很好的候选
	//   （例如将 WriteString(fmt.Sprintf()) 重写为 Fprintf。）
	// 被拒绝的：
	// - composites: 某些类型（例如 PointXY{1,2}）不需要字段名。
	// - timeformat: 翻转 MM/DD 是行为改变，但代码
	//    可能是另一个错误的解决方法。
	// - stringintconv: 提供两个修复，需要用户输入选择。
	// - fieldalignment: 信号/噪声很差; 修复可能是回归。
)
