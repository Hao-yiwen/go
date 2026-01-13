// Copyright 2024 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package synctest 为测试并发代码提供支持。
//
// 有关函数文档，请参见 testing/synctest 包。
package synctest

import (
	"internal/abi"
	"unsafe"
)

//go:linkname Run
func Run(f func())

//go:linkname Wait
func Wait()

// IsInBubble 报告当前 goroutine 是否在气泡中。
//
//go:linkname IsInBubble
func IsInBubble() bool

// Association 是指针气泡关联的状态。
type Association int

const (
	Unbubbled     = Association(iota) // 未与任何气泡关联
	CurrentBubble                     // 与当前气泡关联
	OtherBubble                       // 与不同的气泡关联
)

// Associate 尝试将 p 与当前气泡关联。
// 它返回 p 的新关联状态。
func Associate[T any](p *T) Association {
	// 确保 p 逃逸以允许我们向其附加特殊内容。
	escapedP := abi.Escape(p)
	return Association(associate(unsafe.Pointer(escapedP)))
}

//go:linkname associate
func associate(p unsafe.Pointer) int

// Disassociate 将 p 与任何气泡解除关联。
func Disassociate[T any](p *T) {
	disassociate(unsafe.Pointer(p))
}

//go:linkname disassociate
func disassociate(b unsafe.Pointer)

// IsAssociated 报告 p 是否与当前气泡关联。
func IsAssociated[T any](p *T) bool {
	return isAssociated(unsafe.Pointer(p))
}

//go:linkname isAssociated
func isAssociated(p unsafe.Pointer) bool

//go:linkname acquire
func acquire() any

//go:linkname release
func release(any)

//go:linkname inBubble
func inBubble(any, func())

// A Bubble 是一个 synctest 气泡。
//
// 不是公开 API。由 syscall/js 用于通过系统调用传播气泡成员资格。
type Bubble struct {
	b any
}

// Acquire 返回对当前 goroutine 的气泡的引用。
// 在调用 Release 之前，气泡不会变为空闲。
func Acquire() *Bubble {
	if b := acquire(); b != nil {
		return &Bubble{b}
	}
	return nil
}

// Release 释放对气泡的引用，
// 允许它再次变为空闲。
func (b *Bubble) Release() {
	if b == nil {
		return
	}
	release(b.b)
	b.b = nil
}

// Run 在气泡中执行 f。
// 当前 goroutine 不得是气泡的一部分。
func (b *Bubble) Run(f func()) {
	if b == nil {
		f()
	} else {
		inBubble(b.b, f)
	}
}
