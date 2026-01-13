// 版权所有 2023 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

/*
iter 包提供与序列迭代器相关的基本定义和操作。

# 迭代器

迭代器是一个函数，它将序列的连续元素传递给回调函数，
通常命名为 yield。该函数在序列结束或 yield 返回 false 时停止，
后者表示提前停止迭代。
本包定义了 [Seq] 和 [Seq2]（发音类似 seek——sequence 的第一个音节）
作为每个序列元素向 yield 传递 1 或 2 个值的迭代器的简写：

	type (
		Seq[V any]     func(yield func(V) bool)
		Seq2[K, V any] func(yield func(K, V) bool)
	)

Seq2 表示成对值的序列，通常是键值对或索引值对。

如果迭代器应继续处理序列中的下一个元素，Yield 返回 true，
如果应停止，则返回 false。

如果在 Yield 返回 false 后调用它，Yield 会 panic。

例如，[maps.Keys] 返回一个迭代器，生成 map m 的键序列，
实现如下：

	func Keys[Map ~map[K]V, K comparable, V any](m Map) iter.Seq[K] {
		return func(yield func(K) bool) {
			for k := range m {
				if !yield(k) {
					return
				}
			}
		}
	}

更多示例可在 [The Go Blog: Range Over Function Types] 中找到。

迭代器函数最常由 [range loop] 调用，如：

	func PrintAll[V any](seq iter.Seq[V]) {
		for v := range seq {
			fmt.Println(v)
		}
	}

# 命名约定

迭代器函数和方法以被遍历的序列命名：

	// All 返回 s 中所有元素的迭代器。
	func (s *Set[V]) All() iter.Seq[V]

集合类型上的迭代器方法通常命名为 All，
因为它迭代集合中所有值的序列。

对于包含多个可能序列的类型，迭代器的名称可以指示提供的是哪个序列：

	// Cities 返回该国主要城市的迭代器。
	func (c *Country) Cities() iter.Seq[*City]

	// Languages 返回该国官方语言的迭代器。
	func (c *Country) Languages() iter.Seq[string]

如果迭代器需要额外配置，构造函数可以接受额外的配置参数：

	// Scan 返回满足 min ≤ key ≤ max 的键值对的迭代器。
	func (m *Map[K, V]) Scan(min, max K) iter.Seq2[K, V]

	// Split 返回 s 的（可能为空的）子字符串的迭代器，
	// 这些子字符串由 sep 分隔。
	func Split(s, sep string) iter.Seq[string]

当存在多个可能的迭代顺序时，方法名可以指示该顺序：

	// All 返回从头到尾遍历列表的迭代器。
	func (l *List[V]) All() iter.Seq[V]

	// Backward 返回从尾到头遍历列表的迭代器。
	func (l *List[V]) Backward() iter.Seq[V]

	// Preorder 返回指定根节点下（包括该节点）所有语法树节点的迭代器，
	// 按深度优先前序遍历，先访问父节点再访问子节点。
	func Preorder(root Node) iter.Seq[Node]

# 一次性迭代器

大多数迭代器提供遍历整个序列的能力：调用时，迭代器执行启动序列
所需的任何设置，然后对序列的连续元素调用 yield，
然后在返回前清理。再次调用迭代器会再次遍历序列。

某些迭代器打破了这一惯例，只提供遍历序列一次的能力。
这些"一次性迭代器"通常报告来自无法倒回重新开始的数据流的值。
提前停止后再次调用迭代器可能会继续流，
但在序列结束后再次调用将不产生任何值。
返回一次性迭代器的函数或方法的文档注释应记录此事实：

	// Lines 返回从 r 读取的行的迭代器。
	// 它返回一个一次性迭代器。
	func (r *Reader) Lines() iter.Seq[string]

# 拉取值

接受或返回迭代器的函数和方法应使用标准的 [Seq] 或 [Seq2] 类型，
以确保与 range 循环和其他迭代器适配器的兼容性。
标准迭代器可以被认为是"推送迭代器"，它将值推送到 yield 函数。

有时 range 循环不是消费序列值的最自然方式。
在这种情况下，[Pull] 将标准推送迭代器转换为"拉取迭代器"，
可以调用它从序列中一次拉取一个值。[Pull] 启动一个迭代器并返回
一对函数——next 和 stop——分别返回迭代器的下一个值和停止它。

例如：

	// Pairs 返回 seq 中连续值对的迭代器。
	func Pairs[V any](seq iter.Seq[V]) iter.Seq2[V, V] {
		return func(yield func(V, V) bool) {
			next, stop := iter.Pull(seq)
			defer stop()
			for {
				v1, ok1 := next()
				if !ok1 {
					return
				}
				v2, ok2 := next()
				// 如果 ok2 为 false，v2 应为零值；产生最后一对。
				if !yield(v1, v2) {
					return
				}
				if !ok2 {
					return
				}
			}
		}
	}

如果客户端没有完全消费序列，它们必须调用 stop，
这允许迭代器函数完成并返回。如示例所示，
确保这一点的惯用方法是使用 defer。

# 标准库使用

标准库中的几个包提供基于迭代器的 API，
最值得注意的是 [maps] 和 [slices] 包。
例如，[maps.Keys] 返回 map 键的迭代器，
而 [slices.Sorted] 将迭代器的值收集到切片中，对它们排序，并返回切片，
因此要迭代 map 的排序键：

	for _, key := range slices.Sorted(maps.Keys(m)) {
		...
	}

# 修改

迭代器只提供序列的值，不提供直接修改它的方法。
如果迭代器希望提供在迭代期间修改序列的机制，
通常的方法是定义一个具有额外操作的位置类型，然后提供位置的迭代器。

例如，树实现可能提供：

	// Positions 返回序列中位置的迭代器。
	func (t *Tree[V]) Positions() iter.Seq[*Pos[V]]

	// Pos 表示序列中的位置。
	// 它仅在传递给它的 yield 调用期间有效。
	type Pos[V any] struct { ... }

	// Value 返回游标处的值。
	func (p *Pos[V]) Value() V

	// Delete 删除迭代中此点的值。
	func (p *Pos[V]) Delete()

	// Set 更改游标处的值 v。
	func (p *Pos[V]) Set(v V)

然后客户端可以使用以下方式从树中删除无聊的值：

	for p := range t.Positions() {
		if boring(p.Value()) {
			p.Delete()
		}
	}

[The Go Blog: Range Over Function Types]: https://go.dev/blog/range-functions
[range loop]: https://go.dev/ref/spec#For_range
*/
package iter

import (
	"internal/race"
	"runtime"
	"unsafe"
)

// Seq 是单个值序列的迭代器。
// 当作为 seq(yield) 调用时，seq 对序列中的每个值 v 调用 yield(v)，
// 如果 yield 返回 false 则提前停止。
// 有关更多详细信息，请参阅 [iter] 包文档。
type Seq[V any] func(yield func(V) bool)

// Seq2 是成对值序列的迭代器，最常见的是键值对。
// 当作为 seq(yield) 调用时，seq 对序列中的每对 (k, v) 调用 yield(k, v)，
// 如果 yield 返回 false 则提前停止。
// 有关更多详细信息，请参阅 [iter] 包文档。
type Seq2[K, V any] func(yield func(K, V) bool)

type coro struct{}

//go:linkname newcoro runtime.newcoro
func newcoro(func(*coro)) *coro

//go:linkname coroswitch runtime.coroswitch
func coroswitch(*coro)

// Pull 将"推送风格"迭代器序列 seq 转换为通过两个函数
// next 和 stop 访问的"拉取风格"迭代器。
//
// Next 返回序列中的下一个值和一个布尔值，指示该值是否有效。
// 当序列结束时，next 返回零值 V 和 false。
// 在到达序列末尾或调用 stop 之后调用 next 是有效的。
// 这些调用将继续返回零值 V 和 false。
//
// Stop 结束迭代。当调用者不再对下一个值感兴趣且 next 尚未
// 发出序列结束信号（通过 false 布尔返回值）时，必须调用它。
// 多次调用 stop 以及在 next 已经返回 false 时调用都是有效的。
// 通常，调用者应该 "defer stop()"。
//
// 从多个 goroutine 同时调用 next 或 stop 是错误的。
//
// 如果迭代器在调用 next（或 stop）期间 panic，
// 则 next（或 stop）本身会以相同的值 panic。
func Pull[V any](seq Seq[V]) (next func() (V, bool), stop func()) {
	var pull struct {
		v          V
		ok         bool
		done       bool
		yieldNext  bool
		seqDone    bool // 用于检测 Goexit
		racer      int
		panicValue any
	}
	c := newcoro(func(c *coro) {
		race.Acquire(unsafe.Pointer(&pull.racer))
		if pull.done {
			race.Release(unsafe.Pointer(&pull.racer))
			return
		}
		yield := func(v1 V) bool {
			if pull.done {
				return false
			}
			if !pull.yieldNext {
				panic("iter.Pull: yield called again before next")
			}
			pull.yieldNext = false
			pull.v, pull.ok = v1, true
			race.Release(unsafe.Pointer(&pull.racer))
			coroswitch(c)
			race.Acquire(unsafe.Pointer(&pull.racer))
			return !pull.done
		}
		// 从 seq 恢复并传播 panic。
		defer func() {
			if p := recover(); p != nil {
				pull.panicValue = p
			} else if !pull.seqDone {
				pull.panicValue = goexitPanicValue
			}
			pull.done = true // 使迭代器失效
			race.Release(unsafe.Pointer(&pull.racer))
		}()
		seq(yield)
		var v0 V
		pull.v, pull.ok = v0, false
		pull.seqDone = true
	})
	next = func() (v1 V, ok1 bool) {
		race.Write(unsafe.Pointer(&pull.racer)) // 检测竞争

		if pull.done {
			return
		}
		if pull.yieldNext {
			panic("iter.Pull: next called again before yield")
		}
		pull.yieldNext = true
		race.Release(unsafe.Pointer(&pull.racer))
		coroswitch(c)
		race.Acquire(unsafe.Pointer(&pull.racer))

		// 从 seq 传播 panic 和 goexit。
		if pull.panicValue != nil {
			if pull.panicValue == goexitPanicValue {
				// 从 seq 传播 runtime.Goexit。
				runtime.Goexit()
			} else {
				panic(pull.panicValue)
			}
		}
		return pull.v, pull.ok
	}
	stop = func() {
		race.Write(unsafe.Pointer(&pull.racer)) // 检测竞争

		if !pull.done {
			pull.done = true
			race.Release(unsafe.Pointer(&pull.racer))
			coroswitch(c)
			race.Acquire(unsafe.Pointer(&pull.racer))

			// 从 seq 传播 panic 和 goexit。
			if pull.panicValue != nil {
				if pull.panicValue == goexitPanicValue {
					// 从 seq 传播 runtime.Goexit。
					runtime.Goexit()
				} else {
					panic(pull.panicValue)
				}
			}
		}
	}
	return next, stop
}

// Pull2 将"推送风格"迭代器序列 seq 转换为通过两个函数
// next 和 stop 访问的"拉取风格"迭代器。
//
// Next 返回序列中的下一对值和一个布尔值，指示该对是否有效。
// 当序列结束时，next 返回一对零值和 false。
// 在到达序列末尾或调用 stop 之后调用 next 是有效的。
// 这些调用将继续返回一对零值和 false。
//
// Stop 结束迭代。当调用者不再对下一个值感兴趣且 next 尚未
// 发出序列结束信号（通过 false 布尔返回值）时，必须调用它。
// 多次调用 stop 以及在 next 已经返回 false 时调用都是有效的。
// 通常，调用者应该 "defer stop()"。
//
// 从多个 goroutine 同时调用 next 或 stop 是错误的。
//
// 如果迭代器在调用 next（或 stop）期间 panic，
// 则 next（或 stop）本身会以相同的值 panic。
func Pull2[K, V any](seq Seq2[K, V]) (next func() (K, V, bool), stop func()) {
	var pull struct {
		k          K
		v          V
		ok         bool
		done       bool
		yieldNext  bool
		seqDone    bool
		racer      int
		panicValue any
	}
	c := newcoro(func(c *coro) {
		race.Acquire(unsafe.Pointer(&pull.racer))
		if pull.done {
			race.Release(unsafe.Pointer(&pull.racer))
			return
		}
		yield := func(k1 K, v1 V) bool {
			if pull.done {
				return false
			}
			if !pull.yieldNext {
				panic("iter.Pull2: yield called again before next")
			}
			pull.yieldNext = false
			pull.k, pull.v, pull.ok = k1, v1, true
			race.Release(unsafe.Pointer(&pull.racer))
			coroswitch(c)
			race.Acquire(unsafe.Pointer(&pull.racer))
			return !pull.done
		}
		// 从 seq 恢复并传播 panic。
		defer func() {
			if p := recover(); p != nil {
				pull.panicValue = p
			} else if !pull.seqDone {
				pull.panicValue = goexitPanicValue
			}
			pull.done = true // 使迭代器失效.
			race.Release(unsafe.Pointer(&pull.racer))
		}()
		seq(yield)
		var k0 K
		var v0 V
		pull.k, pull.v, pull.ok = k0, v0, false
		pull.seqDone = true
	})
	next = func() (k1 K, v1 V, ok1 bool) {
		race.Write(unsafe.Pointer(&pull.racer)) // 检测竞争

		if pull.done {
			return
		}
		if pull.yieldNext {
			panic("iter.Pull2: next called again before yield")
		}
		pull.yieldNext = true
		race.Release(unsafe.Pointer(&pull.racer))
		coroswitch(c)
		race.Acquire(unsafe.Pointer(&pull.racer))

		// 从 seq 传播 panic 和 goexit。
		if pull.panicValue != nil {
			if pull.panicValue == goexitPanicValue {
				// 从 seq 传播 runtime.Goexit。
				runtime.Goexit()
			} else {
				panic(pull.panicValue)
			}
		}
		return pull.k, pull.v, pull.ok
	}
	stop = func() {
		race.Write(unsafe.Pointer(&pull.racer)) // 检测竞争

		if !pull.done {
			pull.done = true
			race.Release(unsafe.Pointer(&pull.racer))
			coroswitch(c)
			race.Acquire(unsafe.Pointer(&pull.racer))

			// 从 seq 传播 panic 和 goexit。
			if pull.panicValue != nil {
				if pull.panicValue == goexitPanicValue {
					// 从 seq 传播 runtime.Goexit。
					runtime.Goexit()
				} else {
					panic(pull.panicValue)
				}
			}
		}
	}
	return next, stop
}

// goexitPanicValue 是一个哨兵值，指示迭代器通过 runtime.Goexit 退出。
var goexitPanicValue any = new(int)
