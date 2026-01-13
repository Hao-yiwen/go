// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package flag_test

import (
	"flag"
	"fmt"
	"time"
)

func ExampleFlagSet() {

	start := func(args []string) {
		// 一个真实的程序（不是示例）将使用 flag.ExitOnError。
		fs := flag.NewFlagSet("start", flag.ContinueOnError)
		addr := fs.String("addr", ":8080", "`address` to listen on")
		if err := fs.Parse(args); err != nil {
			fmt.Printf("error: %s", err)
			return
		}
		fmt.Printf("starting server on %s\n", *addr)
	}

	stop := func(args []string) {
		fs := flag.NewFlagSet("stop", flag.ContinueOnError)
		timeout := fs.Duration("timeout", time.Second, "stop timeout duration")
		if err := fs.Parse(args); err != nil {
			fmt.Printf("error: %s", err)
			return
		}
		fmt.Printf("stopping server (timeout=%v)\n", *timeout)
	}

	main := func(args []string) {
		subArgs := args[2:] // 删除程序名称和命令。
		switch args[1] {
		case "start":
			start(subArgs)
		case "stop":
			stop(subArgs)
		default:
			fmt.Printf("error: unknown command - %q\n", args[1])
			// 在一个真实的程序（不是示例）中，打印到 os.Stderr 并以非零值退出程序。
		}
	}

	main([]string{"httpd", "start", "-addr", ":9999"})
	main([]string{"httpd", "stop"})
	main([]string{"http", "start", "-log-level", "verbose"})

	// Output:
	// starting server on :9999
	// stopping server (timeout=1s)
	// error: flag provided but not defined: -log-level
}
