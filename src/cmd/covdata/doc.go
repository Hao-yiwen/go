// Copyright 2022 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

/*
Covdata 是一个用于操作和生成来自
第二代覆盖率测试输出文件的报告的程序，
这些文件从运行应用程序或集成测试产生。例如：

	$ mkdir ./profiledir
	$ go build -cover -o myapp.exe .
	$ GOCOVERDIR=./profiledir ./myapp.exe <arguments>
	$ ls ./profiledir
	covcounters.cce1b350af34b6d0fb59cc1725f0ee27.821598.1663006712821344241
	covmeta.cce1b350af34b6d0fb59cc1725f0ee27
	$

通过 "go tool covdata <mode>" 运行 covdata，其中 'mode' 是一个子命令
选择特定的报告、合并或数据操作。
关于各种模式的描述（运行 "go tool cover <mode> -help"
以了解给定模式的使用具体信息）：

1. 报告在每个分析包中覆盖的语句百分比

	$ go tool covdata percent -i=profiledir
	cov-example/p	coverage: 41.1% of statements
	main	coverage: 87.5% of statements
	$

2. 报告分析包的导入路径

	$ go tool covdata pkglist -i=profiledir
	cov-example/p
	main
	$

3. 报告按函数覆盖的语句百分比：

	$ go tool covdata func -i=profiledir
	cov-example/p/p.go:12:		emptyFn			0.0%
	cov-example/p/p.go:32:		Small			100.0%
	cov-example/p/p.go:47:		Medium			90.9%
	...
	$

4. 将覆盖率数据转换为传统文本格式：

	$ go tool covdata textfmt -i=profiledir -o=cov.txt
	$ head cov.txt
	mode: set
	cov-example/p/p.go:12.22,13.2 0 0
	cov-example/p/p.go:15.31,16.2 1 0
	cov-example/p/p.go:16.3,18.3 0 0
	cov-example/p/p.go:19.3,21.3 0 0
	...
	$ go tool cover -html=cov.txt
	$

5. 合并配置文件：

	$ go tool covdata merge -i=indir1,indir2 -o=outdir -modpaths=github.com/go-delve/delve
	$

6. 从另一个配置文件中减去一个配置文件

	$ go tool covdata subtract -i=indir1,indir2 -o=outdir
	$

7. 交集配置文件

	$ go tool covdata intersect -i=indir1,indir2 -o=outdir
	$

8. 转储配置文件用于调试目的。

	$ go tool covdata debugdump -i=indir
	<human readable output>
	$
*/
package main
