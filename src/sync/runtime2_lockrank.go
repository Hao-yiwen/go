// 版权所有 2020 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

//go:build goexperiment.staticlockranking

package sync

import "unsafe"

// runtime/sema.go 中 notifyList 的近似表示。大小和对齐必须一致。
type notifyList struct {
	wait   uint32
	notify uint32
	rank   int     // 互斥锁的 rank 字段
	pad    int     // 互斥锁的 pad 字段
	lock   uintptr // 互斥锁的键字段

	head unsafe.Pointer
	tail unsafe.Pointer
}
