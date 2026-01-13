// 版权所有 2018 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

//go:build js && wasm

package js

import (
	"internal/synctest"
	"sync"
)

var (
	funcsMu    sync.Mutex
	funcs             = make(map[uint32]func(Value, []Value) any)
	nextFuncID uint32 = 1
)

// Func 是一个被包装的 Go 函数，将由 JavaScript 调用。
type Func struct {
	Value  // 调用 Go 函数的 JavaScript 函数
	bubble *synctest.Bubble
	id     uint32
}

// FuncOf 返回一个要被 JavaScript 使用的函数。
//
// Go 函数 fn 被调用时，使用 JavaScript 的 "this" 关键字的值和
// 调用的参数。调用的返回值是
// Go 函数的结果根据 ValueOf 映射回 JavaScript。
//
// 从 JavaScript 调用被包装的 Go 函数将
// 暂停事件循环并生成一个新的 goroutine。
// 在从 Go 到 JavaScript 的调用期间触发的其他被包装的函数
// 在同一个 goroutine 上执行。
//
// 因此，如果一个被包装的函数阻塞，JavaScript 的事件循环
// 将被阻塞，直到该函数返回。因此，调用任何需要
// 事件循环的异步 JavaScript API，如 fetch (http.Client)，将导致
// 立即死锁。因此，阻塞函数应该显式启动一个
// 新的 goroutine。
//
// 当函数不再被调用时，必须调用 Func.Release 以释放资源。
func FuncOf(fn func(this Value, args []Value) any) Func {
	funcsMu.Lock()
	id := nextFuncID
	nextFuncID++
	bubble := synctest.Acquire()
	if bubble != nil {
		origFn := fn
		fn = func(this Value, args []Value) any {
			var r any
			bubble.Run(func() {
				r = origFn(this, args)
			})
			return r
		}
	}
	funcs[id] = fn
	funcsMu.Unlock()
	return Func{
		id:     id,
		bubble: bubble,
		Value:  jsGo.Call("_makeFuncWrapper", id),
	}
}

// Release 释放为该函数分配的资源。
// 调用 Release 后，该函数不得被调用。
// 允许在函数仍在运行时调用 Release。
func (c Func) Release() {
	c.bubble.Release()
	funcsMu.Lock()
	delete(funcs, c.id)
	funcsMu.Unlock()
}

// setEventHandler 在 runtime 包中定义。
func setEventHandler(fn func() bool)

func init() {
	setEventHandler(handleEvent)
}

// handleEvent 检索挂起的事件（window._pendingEvent）并在其上调用 js.Func。
// 如果处理了事件，返回 true。
func handleEvent() bool {
	// 从 js 中检索事件
	cb := jsGo.Get("_pendingEvent")
	if cb.IsNull() {
		return false
	}
	jsGo.Set("_pendingEvent", Null())

	id := uint32(cb.Get("id").Int())
	if id == 0 { // 零表示死锁
		select {}
	}

	// 检索关联的 js.Func
	funcsMu.Lock()
	f, ok := funcs[id]
	funcsMu.Unlock()
	if !ok {
		Global().Get("console").Call("error", "call to released function")
		return true
	}

	// 使用参数调用 js.Func
	this := cb.Get("this")
	argsObj := cb.Get("args")
	args := make([]Value, argsObj.Length())
	for i := range args {
		args[i] = argsObj.Index(i)
	}
	result := f(this, args)

	// 将结果返回给 js
	cb.Set("result", result)
	return true
}
