// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package heap 提供了对任何实现了 heap.Interface 的类型的堆操作。
// 堆是一种树，其中每个节点都是其子树中的最小值节点。
//
// 树中的最小元素位于根部，索引为 0。
//
// 堆是实现优先队列的常见方法。要构建优先队列，请使用（负）优先级
// 作为 Less 方法的排序来实现 Heap 接口，以便 Push 添加项目，
// 而 Pop 从队列中删除优先级最高的项目。示例包括这样的实现；
// 文件 example_pq_test.go 包含完整的源代码。
package heap

import "sort"

// Interface 类型描述了使用此包中的例程的类型的要求。
// 任何实现它的类型都可以用作最小堆，具有以下不变量
//（在调用 [Init] 之后建立，或如果数据为空或已排序）：
//
//	!h.Less(j, i) for 0 <= i < h.Len() and 2*i+1 <= j <= 2*i+2 and j < h.Len()
//
// 注意此接口中的 [Push] 和 [Pop] 是供 heap 包的实现调用的。
// 要在堆中添加和删除内容，请使用 [heap.Push] 和 [heap.Pop]。
type Interface interface {
	sort.Interface
	Push(x any) // add x as element Len()
	Pop() any   // remove and return element Len() - 1.
}

// Init 建立此包中其他例程所需的堆不变量。
// Init 对于堆不变量是幂等的，可以在任何时候调用
// 以防堆不变量可能已被破坏。
// 复杂性为 O(n)，其中 n = h.Len()。
func Init(h Interface) {
	// 堆化
	n := h.Len()
	for i := n/2 - 1; i >= 0; i-- {
		down(h, i, n)
	}
}

// Push 将元素 x 推送到堆上。
// 复杂性为 O(log n)，其中 n = h.Len()。
func Push(h Interface, x any) {
	h.Push(x)
	up(h, h.Len()-1)
}

// Pop 从堆中删除并返回最小元素（根据 Less）。
// 复杂性为 O(log n)，其中 n = h.Len()。
// Pop 等价于 [Remove](h, 0)。
func Pop(h Interface) any {
	n := h.Len() - 1
	h.Swap(0, n)
	down(h, 0, n)
	return h.Pop()
}

// Remove 从堆中删除并返回索引 i 处的元素。
// 复杂性为 O(log n)，其中 n = h.Len()。
func Remove(h Interface, i int) any {
	n := h.Len() - 1
	if n != i {
		h.Swap(i, n)
		if !down(h, i, n) {
			up(h, i)
		}
	}
	return h.Pop()
}

// Fix 在索引 i 处的元素改变其值后重新建立堆的排序。
// 改变索引 i 处元素的值然后调用 Fix 等价于，
// 但比调用 [Remove](h, i) 然后 Push 新值的成本更低。
// 复杂性为 O(log n)，其中 n = h.Len()。
func Fix(h Interface, i int) {
	if !down(h, i, h.Len()) {
		up(h, i)
	}
}

func up(h Interface, j int) {
	for {
		i := (j - 1) / 2 // 父节点
		if i == j || !h.Less(j, i) {
			break
		}
		h.Swap(i, j)
		j = i
	}
}

func down(h Interface, i0, n int) bool {
	i := i0
	for {
		j1 := 2*i + 1
		if j1 >= n || j1 < 0 { // j1 < 0 整数溢出后
			break
		}
		j := j1 // 左孩子
		if j2 := j1 + 1; j2 < n && h.Less(j2, j1) {
			j = j2 // = 2*i + 2  // 右孩子
		}
		if !h.Less(j, i) {
			break
		}
		h.Swap(i, j)
		i = j
	}
	return i > i0
}
