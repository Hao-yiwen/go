// Copyright 2024 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !js

package runtime

// 在一次性事件上睡眠和唤醒。
// 在调用 notesleep 或 notewakeup 之前，
// 必须调用 noteclear 来初始化 Note。
// 然后，恰好一个线程可以调用 notesleep，
// 恰好一个线程可以调用 notewakeup（一次）。
// 一旦 notewakeup 被调用，notesleep
// 将返回。未来 notesleep 将立即返回。
// 后续 noteclear 必须仅在
// 之前的 notesleep 返回后调用，例如不允许
// 在 notewakeup 之后直接调用 noteclear。
//
// notetsleep 类似于 notesleep 但在给定的纳秒数后唤醒，
// 即使事件尚未发生。如果 goroutine 使用 notetsleep 来
// 提前唤醒，它必须等待调用 noteclear，直到
// 确保没有其他 goroutine 调用
// notewakeup。
//
// notesleep/notetsleep 通常在 g0 上调用，
// notetsleepg 类似于 notetsleep 但在用户 g 上调用。
type note struct {
	// 基于 Futex 的实现将其视为 uint32 键，
	// 而基于 sema 的实现将其视为 M* waitm。
	key uintptr
}
