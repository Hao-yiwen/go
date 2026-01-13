// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Dist 帮助引导、构建和测试 Go 发行版。
//
// 用法：
//
//	go tool dist [command]
//
// 命令是：
//
//	banner         打印安装横幅
//	bootstrap      重建一切
//	clean          删除所有已构建的文件
//	env [-p]       打印环境（-p：包括 $PATH）
//	install [dir]  安装单个目录
//	list [-json]   列出所有支持的平台
//	test [-h]      运行 Go 测试
//	version        打印 Go 版本
package main
