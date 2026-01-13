// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// 这些示例演示了 flag 包的更复杂用法。
package flag_test

import (
	"errors"
	"flag"
	"fmt"
	"strings"
	"time"
)

// 示例 1：一个名为 "species" 的单个字符串标志，默认值为 "gopher"。
var species = flag.String("species", "gopher", "the species we are studying")

// 示例 2：两个标志共享一个变量，因此我们可以有一个简写。
// 初始化的顺序是未定义的，因此确保两者都使用
// 相同的默认值。它们必须在 init 函数中设置。
var gopherType string

func init() {
	const (
		defaultGopher = "pocket"
		usage         = "the variety of gopher"
	)
	flag.StringVar(&gopherType, "gopher_type", defaultGopher, usage)
	flag.StringVar(&gopherType, "g", defaultGopher, usage+" (shorthand)")
}

// 示例 3：一个用户定义的标志类型，时间段的切片。
type interval []time.Duration

// String 是格式化标志值的方法，是 flag.Value 接口的一部分。
// String 方法的输出将用于诊断。
func (i *interval) String() string {
	return fmt.Sprint(*i)
}

// Set 是设置标志值的方法，是 flag.Value 接口的一部分。
// Set 的参数是一个要解析以设置标志的字符串。
// 它是一个逗号分隔的列表，所以我们将其分割。
func (i *interval) Set(value string) error {
	// 如果我们想允许多次设置标志，
	// 累积值，我们会删除这个 if 语句。
	// 这将允许如下用法
	//	-deltaT 10s -deltaT 15s
	// 以及其他组合。
	if len(*i) > 0 {
		return errors.New("interval flag already set")
	}
	for _, dt := range strings.Split(value, ",") {
		duration, err := time.ParseDuration(dt)
		if err != nil {
			return err
		}
		*i = append(*i, duration)
	}
	return nil
}

// 定义一个标志以累积时间段。因为它有特殊的类型，
// 我们需要使用 Var 函数，因此在
// init 中创建标志。

var intervalFlag interval

func init() {
	// 将命令行标志与 intervalFlag 变量绑定并
	// 设置用法消息。
	flag.Var(&intervalFlag, "deltaT", "comma-separated list of intervals to use between events")
}

func Example() {
	// 所有有趣的部分都与上面声明的变量有关，但
	// 要启用 flag 包以查看在那里定义的标志，必须
	// 执行，通常在 main 开始时（不是 init！）：
	//	flag.Parse()
	// 我们在这里不调用它，因为这个代码是一个名为 "Example" 的函数
	// 这是包测试套件的一部分，它已经
	// 解析了标志。但是，当在 pkg.go.dev 上查看时，该函数
	// 被重命名为 "main"，可以作为独立示例运行。
}
