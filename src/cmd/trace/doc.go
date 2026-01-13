// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

/*
Trace 是用于查看跟踪文件的工具。

跟踪文件可以通过以下方式生成：
  - runtime/trace.Start
  - net/http/pprof 包
  - go test -trace

示例用法：
使用 'go test' 生成跟踪文件：

	go test -trace trace.out pkg

在网络浏览器中查看跟踪：

	go tool trace trace.out

从跟踪生成类似 pprof 的配置文件：

	go tool trace -pprof=TYPE trace.out > TYPE.pprof

支持的配置文件类型为：
  - net: 网络阻塞配置文件
  - sync: 同步阻塞配置文件
  - syscall: 系统调用阻塞配置文件
  - sched: 调度程序延迟配置文件

然后，你可以使用 pprof 工具来分析配置文件：

	go tool pprof TYPE.pprof

注意，虽然启动 'go tool trace' 时提供的各种配置文件
在每个浏览器上都能工作，但跟踪查看器本身
（'view trace' 页面）来自 Chrome/Chromium 项目
并且仅在该浏览器上进行积极测试。
*/
package main
