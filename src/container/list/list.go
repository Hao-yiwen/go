// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package list 实现了双向链表。
//
// 要遍历列表（其中 l 是 *List）：
//
//	for e := l.Front(); e != nil; e = e.Next() {
//		// 对 e.Value 做一些事情
//	}
package list

// Element 是链表的一个元素。
type Element struct {
	// 双向链表中的下一个和前一个指针。
	// 为了简化实现，内部一个列表 l 被实现为
	// 一个环，使得 &l.root 既是最后一个
	// 列表元素 (l.Back()) 的下一个元素，也是第一个列表
	// 元素 (l.Front()) 的前一个元素。
	next, prev *Element

	// 此元素所属的列表。
	list *List

	// 与此元素一起存储的值。
	Value any
}

// Next 返回下一个列表元素或 nil。
func (e *Element) Next() *Element {
	if p := e.next; e.list != nil && p != &e.list.root {
		return p
	}
	return nil
}

// Prev 返回前一个列表元素或 nil。
func (e *Element) Prev() *Element {
	if p := e.prev; e.list != nil && p != &e.list.root {
		return p
	}
	return nil
}

// List 表示双向链表。
// List 的零值是一个空列表，可以使用。
type List struct {
	root Element // 哨兵列表元素，仅使用 &root、root.prev 和 root.next
	len  int     // 当前列表长度，不包括（这个）哨兵元素
}

// Init 初始化或清除列表 l。
func (l *List) Init() *List {
	l.root.next = &l.root
	l.root.prev = &l.root
	l.len = 0
	return l
}

// New 返回一个初始化的列表。
func New() *List { return new(List).Init() }

// Len 返回列表 l 的元素数量。
// 复杂性为 O(1)。
func (l *List) Len() int { return l.len }

// Front 返回列表 l 的第一个元素，如果列表为空，则返回 nil。
func (l *List) Front() *Element {
	if l.len == 0 {
		return nil
	}
	return l.root.next
}

// Back 返回列表 l 的最后一个元素，如果列表为空，则返回 nil。
func (l *List) Back() *Element {
	if l.len == 0 {
		return nil
	}
	return l.root.prev
}

// lazyInit 懒惰地初始化零 List 值。
func (l *List) lazyInit() {
	if l.root.next == nil {
		l.Init()
	}
}

// insert 在 at 之后插入 e，增加 l.len，并返回 e。
func (l *List) insert(e, at *Element) *Element {
	e.prev = at
	e.next = at.next
	e.prev.next = e
	e.next.prev = e
	e.list = l
	l.len++
	return e
}

// insertValue 是 insert(&Element{Value: v}, at) 的便利包装器。
func (l *List) insertValue(v any, at *Element) *Element {
	return l.insert(&Element{Value: v}, at)
}

// remove 从其列表中删除 e，减少 l.len
func (l *List) remove(e *Element) {
	e.prev.next = e.next
	e.next.prev = e.prev
	e.next = nil // 避免内存泄漏
	e.prev = nil // 避免内存泄漏
	e.list = nil
	l.len--
}

// move 将 e 移动到 at 的下一个位置。
func (l *List) move(e, at *Element) {
	if e == at {
		return
	}
	e.prev.next = e.next
	e.next.prev = e.prev

	e.prev = at
	e.next = at.next
	e.prev.next = e
	e.next.prev = e
}

// Remove 从 l 中删除 e，如果 e 是列表 l 的元素。
// 它返回元素值 e.Value。
// 元素不能为 nil。
func (l *List) Remove(e *Element) any {
	if e.list == l {
		// 如果 e.list == l，当 e 被插入到 l 时，l 必须已被初始化
		// 或 l == nil（e 是零元素）并且 l.remove 将崩溃
		l.remove(e)
	}
	return e.Value
}

// PushFront 在列表 l 的前面插入一个值为 v 的新元素 e，并返回 e。
func (l *List) PushFront(v any) *Element {
	l.lazyInit()
	return l.insertValue(v, &l.root)
}

// PushBack 在列表 l 的后面插入一个值为 v 的新元素 e，并返回 e。
func (l *List) PushBack(v any) *Element {
	l.lazyInit()
	return l.insertValue(v, l.root.prev)
}

// InsertBefore 在 mark 之前立即插入一个值为 v 的新元素 e，并返回 e。
// 如果 mark 不是 l 的元素，列表不被修改。
// mark 不能为 nil。
func (l *List) InsertBefore(v any, mark *Element) *Element {
	if mark.list != l {
		return nil
	}
	// 参见 List.Remove 中关于 l 初始化的注释
	return l.insertValue(v, mark.prev)
}

// InsertAfter 在 mark 之后立即插入一个值为 v 的新元素 e，并返回 e。
// 如果 mark 不是 l 的元素，列表不被修改。
// mark 不能为 nil。
func (l *List) InsertAfter(v any, mark *Element) *Element {
	if mark.list != l {
		return nil
	}
	// 参见 List.Remove 中关于 l 初始化的注释
	return l.insertValue(v, mark)
}

// MoveToFront 将元素 e 移动到列表 l 的前面。
// 如果 e 不是 l 的元素，列表不被修改。
// 元素不能为 nil。
func (l *List) MoveToFront(e *Element) {
	if e.list != l || l.root.next == e {
		return
	}
	// 参见 List.Remove 中关于 l 初始化的注释
	l.move(e, &l.root)
}

// MoveToBack 将元素 e 移动到列表 l 的后面。
// 如果 e 不是 l 的元素，列表不被修改。
// 元素不能为 nil。
func (l *List) MoveToBack(e *Element) {
	if e.list != l || l.root.prev == e {
		return
	}
	// 参见 List.Remove 中关于 l 初始化的注释
	l.move(e, l.root.prev)
}

// MoveBefore 将元素 e 移动到其在 mark 之前的新位置。
// 如果 e 或 mark 不是 l 的元素，或 e == mark，列表不被修改。
// 元素和 mark 不能为 nil。
func (l *List) MoveBefore(e, mark *Element) {
	if e.list != l || e == mark || mark.list != l {
		return
	}
	l.move(e, mark.prev)
}

// MoveAfter 将元素 e 移动到其在 mark 之后的新位置。
// 如果 e 或 mark 不是 l 的元素，或 e == mark，列表不被修改。
// 元素和 mark 不能为 nil。
func (l *List) MoveAfter(e, mark *Element) {
	if e.list != l || e == mark || mark.list != l {
		return
	}
	l.move(e, mark)
}

// PushBackList 在列表 l 的后面插入另一个列表的副本。
// 列表 l 和 other 可以是相同的。它们不能为 nil。
func (l *List) PushBackList(other *List) {
	l.lazyInit()
	for i, e := other.Len(), other.Front(); i > 0; i, e = i-1, e.Next() {
		l.insertValue(e.Value, l.root.prev)
	}
}

// PushFrontList 在列表 l 的前面插入另一个列表的副本。
// 列表 l 和 other 可以是相同的。它们不能为 nil。
func (l *List) PushFrontList(other *List) {
	l.lazyInit()
	for i, e := other.Len(), other.Back(); i > 0; i, e = i-1, e.Prev() {
		l.insertValue(e.Value, &l.root)
	}
}
