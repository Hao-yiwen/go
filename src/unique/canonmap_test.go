// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package unique

import (
	"internal/abi"
	"runtime"
	"strconv"
	"sync"
	"testing"
	"unsafe"
)

func TestCanonMap(t *testing.T) {
	testCanonMap(t, func() *canonMap[string] {
		return newCanonMap[string]()
	})
}

func TestCanonMapBadHash(t *testing.T) {
	testCanonMap(t, func() *canonMap[string] {
		return newBadCanonMap[string]()
	})
}

func TestCanonMapTruncHash(t *testing.T) {
	testCanonMap(t, func() *canonMap[string] {
		// 用一个不同的糟糕哈希函数替换良好的哈希函数
		// （截断的哈希）。一切应该仍然按预期工作。
		// 这对于独立测试很有用以捕获
		// 近似冲突的问题，其中只有哈希的最后几位不同。
		return newTruncCanonMap[string]()
	})
}

func testCanonMap(t *testing.T, newMap func() *canonMap[string]) {
	t.Run("LoadEmpty", func(t *testing.T) {
		m := newMap()

		for _, s := range testData {
			expectMissing(t, s)(m.Load(s))
		}
	})
	t.Run("LoadOrStore", func(t *testing.T) {
		t.Run("Sequential", func(t *testing.T) {
			m := newMap()

			var refs []*string
			for _, s := range testData {
				expectMissing(t, s)(m.Load(s))
				refs = append(refs, expectPresent(t, s)(m.LoadOrStore(s)))
				expectPresent(t, s)(m.Load(s))
				expectPresent(t, s)(m.LoadOrStore(s))
			}
			drainCleanupQueue(t)
			for _, s := range testData {
				expectPresent(t, s)(m.Load(s))
				expectPresent(t, s)(m.LoadOrStore(s))
			}
			runtime.KeepAlive(refs)
			refs = nil
			drainCleanupQueue(t)
			for _, s := range testData {
				expectMissing(t, s)(m.Load(s))
				expectPresent(t, s)(m.LoadOrStore(s))
			}
		})
		t.Run("ConcurrentUnsharedKeys", func(t *testing.T) {
			makeKey := func(s string, id int) string {
				return s + "-" + strconv.Itoa(id)
			}

			// 多次扩展和收缩映射以尝试让
			// 插入和清理重叠。
			m := newMap()
			gmp := runtime.GOMAXPROCS(-1)
			for try := range 3 {
				var wg sync.WaitGroup
				for i := range gmp {
					wg.Add(1)
					go func(id int) {
						defer wg.Done()

						var refs []*string
						for _, s := range testData {
							key := makeKey(s, id)
							if try == 0 {
								expectMissing(t, key)(m.Load(key))
							}
							refs = append(refs, expectPresent(t, key)(m.LoadOrStore(key)))
							expectPresent(t, key)(m.Load(key))
							expectPresent(t, key)(m.LoadOrStore(key))
						}
						for i, s := range testData {
							key := makeKey(s, id)
							expectPresent(t, key)(m.Load(key))
							if got, want := expectPresent(t, key)(m.LoadOrStore(key)), refs[i]; got != want {
								t.Errorf("canonical entry %p did not match ref %p", got, want)
							}
						}
						// 注：我们避免在这里测试条目清理
						// 因为它会非常不稳定，特别是
						// 在坏哈希的情况下。
					}(i)
				}
				wg.Wait()
			}

			// 运行额外的 GC 周期来去除不稳定。有时候清理
			// 会失败，尽管 drainCleanupQueue。
			//
			// TODO(mknyszek)：找出为什么额外的 GC 是必要的，
			// 以及什么在瞬间保持清理活跃。
			// * 我已确认它们没有完全卡住，并且
			//   它们最终总是会运行。
			// * 我也已确认这不是异步抢占
			//   保持它们（尽管这是一种可能性）。
			// * 我已确认它们不是简单地坐在
			//   队列上，而是 drainCleanupQueue 只是失败了
			//   来实际清空队列。
			// * 我已确认这不是保持它活跃的写屏障，
			//   也不是弱指针解引用
			//   （它在 GC 期间对对象进行着色）。
			// 相应的对象确实似乎瞬间是真正
			// 可达的，但我不知道是什么路径。
			runtime.GC()

			// 清空清理以删除所有内容。
			drainCleanupQueue(t)

			// 再次检查一下它是否都消失了。
			for id := range gmp {
				makeKey := func(s string) string {
					return s + "-" + strconv.Itoa(id)
				}
				for _, s := range testData {
					key := makeKey(s)
					expectMissing(t, key)(m.Load(key))
				}
			}
		})
	})
}

func expectMissing[T comparable](t *testing.T, key T) func(got *T) {
	t.Helper()
	return func(got *T) {
		t.Helper()

		if got != nil {
			t.Errorf("expected key %v to be missing from map, got %p", key, got)
		}
	}
}

func expectPresent[T comparable](t *testing.T, key T) func(got *T) *T {
	t.Helper()
	return func(got *T) *T {
		t.Helper()

		if got == nil {
			t.Errorf("expected key %v to be present in map, got %p", key, got)
		}
		if got != nil && *got != key {
			t.Errorf("key %v is present in map, but canonical version has the wrong value: got %v, want %v", key, *got, key)
		}
		return got
	}
}

// newBadCanonMap 为提供的键类型创建一个新的 canonMap
// 但使用一个故意的坏哈希函数。
func newBadCanonMap[T comparable]() *canonMap[T] {
	// 用一个糟糕的哈希函数替换良好的哈希函数。
	// 一切应该仍然按预期工作。
	m := newCanonMap[T]()
	m.hash = func(_ unsafe.Pointer, _ uintptr) uintptr {
		return 0
	}
	return m
}

// newTruncCanonMap 为提供的键类型创建一个新的 canonMap
// 但使用一个故意的坏哈希函数。
func newTruncCanonMap[T comparable]() *canonMap[T] {
	// 用一个糟糕的哈希函数替换良好的哈希函数。
	// 一切应该仍然按预期工作。
	m := newCanonMap[T]()
	var mx map[string]int
	mapType := abi.TypeOf(mx).MapType()
	hasher := mapType.Hasher
	m.hash = func(p unsafe.Pointer, n uintptr) uintptr {
		return hasher(p, n) & ((uintptr(1) << 4) - 1)
	}
	return m
}
