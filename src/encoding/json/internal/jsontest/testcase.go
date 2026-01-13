// 版权所有 2023 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

//go:build goexperiment.jsonv2

package jsontest

import (
	"fmt"
	"path"
	"runtime"
)

// TODO(https://go.dev/issue/52751): Replace with native testing support.

// CaseName 是一个 case name annotated with a file and line.
type CaseName struct {
	Name  string
	Where CasePos
}

// Name annotates a case name with the file and line of the caller.
func Name(s string) (c CaseName) {
	c.Name = s
	runtime.Callers(2, c.Where.pc[:])
	return c
}

// CasePos 表示a file and line number.
type CasePos struct{ pc [1]uintptr }

func (pos CasePos) String() string {
	frames := runtime.CallersFrames(pos.pc[:])
	frame, _ := frames.Next()
	return fmt.Sprintf("%s:%d", path.Base(frame.File), frame.Line)
}
