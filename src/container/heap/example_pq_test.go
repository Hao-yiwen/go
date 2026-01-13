// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// 此示例演示使用堆接口构建的优先队列。
package heap_test

import (
	"container/heap"
	"fmt"
)

// Item 是我们在优先队列中管理的东西。
type Item struct {
	value    string // 项目的值；任意的。
	priority int    // 队列中项目的优先级。
	// update 需要索引，由 heap.Interface 方法维护。
	index int // 项目在堆中的索引。
}

// PriorityQueue 实现了 heap.Interface 并持有 Items。
type PriorityQueue []*Item

func (pq PriorityQueue) Len() int { return len(pq) }

func (pq PriorityQueue) Less(i, j int) bool {
	// 我们希望 Pop 给我们最高的优先级，而不是最低的，所以我们在这里使用大于。
	return pq[i].priority > pq[j].priority
}

func (pq PriorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].index = i
	pq[j].index = j
}

func (pq *PriorityQueue) Push(x any) {
	n := len(*pq)
	item := x.(*Item)
	item.index = n
	*pq = append(*pq, item)
}

func (pq *PriorityQueue) Pop() any {
	old := *pq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil  // 不要阻止 GC 最终回收该项目
	item.index = -1 // 为了安全
	*pq = old[0 : n-1]
	return item
}

// update 修改队列中 Item 的优先级和值。
func (pq *PriorityQueue) update(item *Item, value string, priority int) {
	item.value = value
	item.priority = priority
	heap.Fix(pq, item.index)
}

// 此示例创建带有一些项目的 PriorityQueue，添加和操作项目，
// 然后按优先级顺序删除项目。
func Example_priorityQueue() {
	// 一些项目及其优先级。
	items := map[string]int{
		"banana": 3, "apple": 2, "pear": 4,
	}

	// 创建优先队列，将项目放入其中，并
	// 建立优先队列（堆）不变量。
	pq := make(PriorityQueue, len(items))
	i := 0
	for value, priority := range items {
		pq[i] = &Item{
			value:    value,
			priority: priority,
			index:    i,
		}
		i++
	}
	heap.Init(&pq)

	// 插入新项目，然后修改其优先级。
	item := &Item{
		value:    "orange",
		priority: 1,
	}
	heap.Push(&pq, item)
	pq.update(item, item.value, 5)

	// 取出项目；它们按优先级降序排列。
	for pq.Len() > 0 {
		item := heap.Pop(&pq).(*Item)
		fmt.Printf("%.2d:%s ", item.priority, item.value)
	}
	// Output:
	// 05:orange 04:pear 03:banana 02:apple
}
