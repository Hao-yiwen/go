// 版权所有 2024 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package abi

type RF_State int

// 这些常量在编译器和运行时之间共享，编译器将它们用于状态函数
// 和 panic 指示器，运行时将它们转换为更有意义的字符串。
// 为了最佳代码生成，RF_DONE 和 RF_READY 应该是 0 和 1。
const (
	RF_DONE          = RF_State(iota) // 循环体以非 panic 方式退出
	RF_READY                          // 循环体尚未退出，未在运行 -- 这不是 panic 索引
	RF_PANIC                          // 循环体当前正在运行，或已 panic
	RF_EXHAUSTED                      // 迭代器函数返回，即序列已"耗尽"
	RF_MISSING_PANIC = 4              // 循环体 panic 但迭代器函数的 defer-recover 将其处理掉了
)
