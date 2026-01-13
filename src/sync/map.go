// 版权所有 2024 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package sync

import (
	isync "internal/sync"
)

// Map 类似于 Go 的 map[any]any，但可以安全地被多个 goroutine 并发使用，
// 无需额外的锁定或协调。加载、存储和删除操作的均摊时间复杂度为常数。
//
// Map 类型是专用的。大多数代码应该使用普通的 Go map，配合单独的锁定或协调，
// 以获得更好的类型安全性，并使维护 map 内容之外的其他不变量更容易。
//
// Map 类型针对两种常见用例进行了优化：（1）当给定键的条目只写入一次但读取多次时，
// 如只增长的缓存；（2）当多个 goroutine 读取、写入和覆盖不相交键集的条目时。
// 在这两种情况下，与配合单独 [Mutex] 或 [RWMutex] 使用的 Go map 相比，
// 使用 Map 可以显著减少锁竞争。
//
// 零值 Map 是空的且可以直接使用。Map 在首次使用后不得被复制。
//
// 按照 [Go 内存模型] 的术语，Map 保证写操作 "同步先于" 任何观察到该写操作效果的读操作，
// 其中读操作和写操作定义如下。
// [Map.Load]、[Map.LoadAndDelete]、[Map.LoadOrStore]、[Map.Swap]、[Map.CompareAndSwap]
// 和 [Map.CompareAndDelete] 是读操作；
// [Map.Delete]、[Map.LoadAndDelete]、[Map.Store] 和 [Map.Swap] 是写操作；
// 当 [Map.LoadOrStore] 返回的 loaded 为 false 时，它是写操作；
// 当 [Map.CompareAndSwap] 返回的 swapped 为 true 时，它是写操作；
// 当 [Map.CompareAndDelete] 返回的 deleted 为 true 时，它是写操作。
//
// [Go 内存模型]: https://go.dev/ref/mem
type Map struct {
	_ noCopy

	m isync.HashTrieMap[any, any]
}

// Load 返回 map 中键对应存储的值，如果不存在则返回 nil。
// ok 结果表示是否在 map 中找到了该值。
func (m *Map) Load(key any) (value any, ok bool) {
	return m.m.Load(key)
}

// Store 设置键对应的值。
func (m *Map) Store(key, value any) {
	m.m.Store(key, value)
}

// Clear 删除所有条目，使 Map 变为空。
func (m *Map) Clear() {
	m.m.Clear()
}

// LoadOrStore 如果键存在则返回现有值。
// 否则，它存储并返回给定的值。
// 如果值是加载的，loaded 结果为 true；如果是存储的，则为 false。
func (m *Map) LoadOrStore(key, value any) (actual any, loaded bool) {
	return m.m.LoadOrStore(key, value)
}

// LoadAndDelete 删除键对应的值，并返回之前的值（如果有）。
// loaded 结果报告键是否存在。
func (m *Map) LoadAndDelete(key any) (value any, loaded bool) {
	return m.m.LoadAndDelete(key)
}

// Delete 删除键对应的值。
// 如果键不在 map 中，Delete 不执行任何操作。
func (m *Map) Delete(key any) {
	m.m.Delete(key)
}

// Swap 交换键对应的值并返回之前的值（如果有）。
// loaded 结果报告键是否存在。
func (m *Map) Swap(key, value any) (previous any, loaded bool) {
	return m.m.Swap(key, value)
}

// CompareAndSwap 如果 map 中存储的值等于 old，则交换键的旧值和新值。
// old 值必须是可比较类型。
func (m *Map) CompareAndSwap(key, old, new any) (swapped bool) {
	return m.m.CompareAndSwap(key, old, new)
}

// CompareAndDelete 如果键的值等于 old，则删除该键的条目。
// old 值必须是可比较类型。
//
// 如果 map 中不存在该键的当前值，CompareAndDelete 返回 false
// （即使 old 值是 nil 接口值）。
func (m *Map) CompareAndDelete(key, old any) (deleted bool) {
	return m.m.CompareAndDelete(key, old)
}

// Range 对 map 中存在的每个键值对依次调用 f。
// 如果 f 返回 false，range 停止迭代。
//
// Range 不一定对应 Map 内容的任何一致快照：每个键最多只会被访问一次，
// 但如果任何键的值被并发存储或删除（包括被 f 删除），Range 可能反映
// 该键在 Range 调用期间任意时刻的映射。Range 不会阻塞接收者上的其他方法；
// 甚至 f 本身也可以调用 m 上的任何方法。
//
// 即使 f 在常数次调用后返回 false，Range 的时间复杂度也可能是 O(N)，
// 其中 N 是 map 中元素的数量。
func (m *Map) Range(f func(key, value any) bool) {
	m.m.Range(f)
}
