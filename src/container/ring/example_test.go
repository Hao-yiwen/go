// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ring_test

import (
	"container/ring"
	"fmt"
)

func ExampleRing_Len() {
	// 创建一个大小为 4 的新环
	r := ring.New(4)

	// 打印出其长度
	fmt.Println(r.Len())

	// Output:
	// 4
}

func ExampleRing_Next() {
	// 创建一个大小为 5 的新环
	r := ring.New(5)

	// 获取环的长度
	n := r.Len()

	// 用一些整数值初始化环
	for i := 0; i < n; i++ {
		r.Value = i
		r = r.Next()
	}

	// 遍历环并打印其内容
	for j := 0; j < n; j++ {
		fmt.Println(r.Value)
		r = r.Next()
	}

	// Output:
	// 0
	// 1
	// 2
	// 3
	// 4
}

func ExampleRing_Prev() {
	// Create a new ring of size 5
	r := ring.New(5)

	// Get the length of the ring
	n := r.Len()

	// Initialize the ring with some integer values
	for i := 0; i < n; i++ {
		r.Value = i
		r = r.Next()
	}

	// Iterate through the ring backwards and print its contents
	for j := 0; j < n; j++ {
		r = r.Prev()
		fmt.Println(r.Value)
	}

	// Output:
	// 4
	// 3
	// 2
	// 1
	// 0
}

func ExampleRing_Do() {
	// 创建一个大小为 5 的新环
	r := ring.New(5)

	// 获取环的长度
	n := r.Len()

	// 用一些整数值初始化环
	for i := 0; i < n; i++ {
		r.Value = i
		r = r.Next()
	}

	// 遍历环并打印其内容
	r.Do(func(p any) {
		fmt.Println(p.(int))
	})

	// Output:
	// 0
	// 1
	// 2
	// 3
	// 4
}

func ExampleRing_Move() {
	// 创建一个大小为 5 的新环
	r := ring.New(5)

	// 获取环的长度
	n := r.Len()

	// 用一些整数值初始化环
	for i := 0; i < n; i++ {
		r.Value = i
		r = r.Next()
	}

	// 将指针向前移动三步
	r = r.Move(3)

	// 遍历环并打印其内容
	r.Do(func(p any) {
		fmt.Println(p.(int))
	})

	// Output:
	// 3
	// 4
	// 0
	// 1
	// 2
}

func ExampleRing_Link() {
	// 创建两个环 r 和 s，大小为 2
	r := ring.New(2)
	s := ring.New(2)

	// 获取环的长度
	lr := r.Len()
	ls := s.Len()

	// 用 0 初始化 r
	for i := 0; i < lr; i++ {
		r.Value = 0
		r = r.Next()
	}

	// 用 1 初始化 s
	for j := 0; j < ls; j++ {
		s.Value = 1
		s = s.Next()
	}

	// 链接环 r 和环 s
	rs := r.Link(s)

	// 遍历组合的环并打印其内容
	rs.Do(func(p any) {
		fmt.Println(p.(int))
	})

	// Output:
	// 0
	// 0
	// 1
	// 1
}

func ExampleRing_Unlink() {
	// 创建一个大小为 6 的新环
	r := ring.New(6)

	// 获取环的长度
	n := r.Len()

	// 用一些整数值初始化环
	for i := 0; i < n; i++ {
		r.Value = i
		r = r.Next()
	}

	// 从 r.Next() 开始从 r 取消链接三个元素
	r.Unlink(3)

	// 遍历剩余的环并打印其内容
	r.Do(func(p any) {
		fmt.Println(p.(int))
	})

	// Output:
	// 0
	// 4
	// 5
}
