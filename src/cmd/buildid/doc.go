// 版权所有 2017 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

/*
Buildid 显示或更新存储在 Go 包或二进制文件中的构建 ID。

用法：

	go tool buildid [-w] file

默认情况下，buildid 打印在指定文件中找到的构建 ID。
如果给出 -w 选项，buildid 会重写文件中找到的构建 ID，
以准确记录文件的内容哈希。

此工具仅供 go 命令或其他构建系统使用。
*/
package main
