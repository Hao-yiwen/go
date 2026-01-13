// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package ring 实现了环形列表上的操作。
package ring

// Ring 是环形列表的元素，或环。
// 环没有开始或结束；对任何环元素的指针
// 作为对整个环的引用。空环表示为
// nil Ring 指针。Ring 的零值是一个单元素
// 环，其值为 nil。
type Ring struct {
	next, prev *Ring
	Value      any // 供客户端使用；此库不修改
}

func (r *Ring) init() *Ring {
	r.next = r
	r.prev = r
	return r
}

// Next 返回下一个环元素。r 不能为空。
func (r *Ring) Next() *Ring {
	if r.next == nil {
		return r.init()
	}
	return r.next
}

// Prev 返回前一个环元素。r 不能为空。
func (r *Ring) Prev() *Ring {
	if r.next == nil {
		return r.init()
	}
	return r.prev
}

// Move 在环中向后（n < 0）或向前（n >= 0）移动 n % r.Len() 个元素
// 并返回那个环元素。r 不能为空。
func (r *Ring) Move(n int) *Ring {
	if r.next == nil {
		return r.init()
	}
	switch {
	case n < 0:
		for ; n < 0; n++ {
			r = r.prev
		}
	case n > 0:
		for ; n > 0; n-- {
			r = r.next
		}
	}
	return r
}

// New 创建一个 n 个元素的环。
func New(n int) *Ring {
	if n <= 0 {
		return nil
	}
	r := new(Ring)
	p := r
	for i := 1; i < n; i++ {
		p.next = &Ring{prev: p}
		p = p.next
	}
	p.next = r
	r.prev = p
	return r
}

// Link 连接环 r 和环 s，使得 r.Next()
// 变成 s 并返回 r.Next() 的原始值。
// r 不能为空。
//
// 如果 r 和 s 指向同一个环，链接
// 它们会从环中删除 r 和 s 之间的元素。
// 被删除的元素形成一个子环，结果是
// 对该子环的引用（如果没有删除元素，
// 结果仍然是 r.Next() 的原始值，
// 而不是 nil）。
//
// 如果 r 和 s 指向不同的环，链接
// 它们会创建一个单一环，其中 s 的元素被插入
// r 之后。结果指向 s 的最后一个元素之后的元素
// 插入后。
func (r *Ring) Link(s *Ring) *Ring {
	n := r.Next()
	if s != nil {
		p := s.Prev()
		// 注意：不能使用多重赋值，因为
		// LHS 的求值顺序未指定。
		r.next = s
		s.prev = r
		n.prev = p
		p.next = n
	}
	return n
}

// Unlink 从环 r 中删除 n % r.Len() 个元素，从
// r.Next() 开始。如果 n % r.Len() == 0，r 保持不变。
// 结果是被删除的子环。r 不能为空。
func (r *Ring) Unlink(n int) *Ring {
	if n <= 0 {
		return nil
	}
	return r.Link(r.Move(n + 1))
}

// Len 计算环 r 中的元素数量。
// 执行时间与元素数量成比例。
func (r *Ring) Len() int {
	n := 0
	if r != nil {
		n = 1
		for p := r.Next(); p != r; p = p.next {
			n++
		}
	}
	return n
}

// Do 按前向顺序对环的每个元素调用函数 f。
// 如果 f 改变 *r，Do 的行为是未定义的。
func (r *Ring) Do(f func(any)) {
	if r != nil {
		f(r.Value)
		for p := r.Next(); p != r; p = p.next {
			f(p.Value)
		}
	}
}
