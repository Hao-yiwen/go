// Copyright 2010 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package flag

import (
	"io"
	"os"
)

// 仅在测试期间编译到包中的其他例程。

var DefaultUsage = Usage

// ResetForTesting 清除所有标志状态并按指示设置用法函数。
// 在调用 ResetForTesting 后，标志处理中的解析错误将不会
// 退出程序。
func ResetForTesting(usage func()) {
	CommandLine = NewFlagSet(os.Args[0], ContinueOnError)
	CommandLine.SetOutput(io.Discard)
	CommandLine.Usage = commandLineUsage
	Usage = usage
}
