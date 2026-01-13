// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package singleflight 提供重复函数调用抑制
// 机制。
package singleflight

import "sync"

// call 是运行中或已完成的 singleflight.Do 调用
type call struct {
	wg sync.WaitGroup

	// 这些字段在 WaitGroup 完成前被写入一次
	// 且仅在 WaitGroup 完成后读取。
	val any
	err error

	// 这些字段在 WaitGroup 完成前
	// 由互斥量持有时被读写，之后
	// WaitGroup 完成后被读但不被写。
	dups  int
	chans []chan<- Result
}

// Group 表示一类工作并形成一个命名空间
// 在其中可以执行工作单位并进行重复抑制。
type Group struct {
	mu sync.Mutex       // 保护 m
	m  map[string]*call // 懒惰初始化
}

// Result 保存 Do 的结果，以便它们可以
// 在通道上传递。
type Result struct {
	Val    any
	Err    error
	Shared bool
}

// Do 执行并返回给定函数的结果，确保
// 对于给定的键，在任何时候只有一个执行在进行中。
// 如果重复的请求进来，重复的调用者等待
// 原始的完成并接收相同的结果。
// 返回值 shared 表示 v 是否被给予了多个调用者。
func (g *Group) Do(key string, fn func() (any, error)) (v any, err error, shared bool) {
	g.mu.Lock()
	if g.m == nil {
		g.m = make(map[string]*call)
	}
	if c, ok := g.m[key]; ok {
		c.dups++
		g.mu.Unlock()
		c.wg.Wait()
		return c.val, c.err, true
	}
	c := new(call)
	c.wg.Add(1)
	g.m[key] = c
	g.mu.Unlock()

	g.doCall(c, key, fn)
	return c.val, c.err, c.dups > 0
}

// DoChan 像 Do 一样，但返回一个通道，该通道将在
// 结果准备好时接收结果。
func (g *Group) DoChan(key string, fn func() (any, error)) <-chan Result {
	ch := make(chan Result, 1)
	g.mu.Lock()
	if g.m == nil {
		g.m = make(map[string]*call)
	}
	if c, ok := g.m[key]; ok {
		c.dups++
		c.chans = append(c.chans, ch)
		g.mu.Unlock()
		return ch
	}
	c := &call{chans: []chan<- Result{ch}}
	c.wg.Add(1)
	g.m[key] = c
	g.mu.Unlock()

	go g.doCall(c, key, fn)

	return ch
}

// doCall 处理键的单个调用。
func (g *Group) doCall(c *call, key string, fn func() (any, error)) {
	c.val, c.err = fn()

	g.mu.Lock()
	c.wg.Done()
	if g.m[key] == c {
		delete(g.m, key)
	}
	for _, ch := range c.chans {
		ch <- Result{c.val, c.err, c.dups > 0}
	}
	g.mu.Unlock()
}

// ForgetUnshared 告诉 singleflight 如果键不与
// 任何其他 goroutine 共享，则忘记该键。对于被遗忘的键
// 的未来 Do 调用将调用该函数而不是等待
// 较早的调用完成。
// 返回键是否被遗忘或未知，即是否
// 没有其他 goroutine 等待结果。
func (g *Group) ForgetUnshared(key string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	c, ok := g.m[key]
	if !ok {
		return true
	}
	if c.dups == 0 {
		delete(g.m, key)
		return true
	}
	return false
}
