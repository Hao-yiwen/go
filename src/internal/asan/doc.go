// 版权所有 2024 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// asan 包包含用于手动为地址消毒器（address sanitizer）
// 插桩代码的辅助函数。
// runtime 包有意只在 asan 构建中导出这些函数；
// 此包无条件导出它们，但如果没有 "asan" 构建标签，它们是空操作。
package asan
