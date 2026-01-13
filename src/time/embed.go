// 版权所有 2020 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// 此文件与 build 标签 timetzdata 一起使用，以将 tzdata 嵌入到
// 二进制文件中。

//go:build timetzdata

package time

import _ "time/tzdata"
