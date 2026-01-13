// 版权所有 2025 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package unique

import (
	"internal/abi"
	"internal/goarch"
	"runtime"
	"sync"
	"sync/atomic"
	"unsafe"
	"weak"
)

// canonMap 是 T -> *T 的映射。该映射控制规范 *T 的创建，
// 当规范 *T 不再被引用时，映射中的元素会自动删除。
type canonMap[T comparable] struct {
	root atomic.Pointer[indirect[T]]
	hash func(unsafe.Pointer, uintptr) uintptr
	seed uintptr
}

func newCanonMap[T comparable]() *canonMap[T] {
	cm := new(canonMap[T])
	cm.root.Store(newIndirectNode[T](nil))

	var m map[T]struct{}
	mapType := abi.TypeOf(m).MapType()
	cm.hash = mapType.Hasher
	cm.seed = uintptr(runtime_rand())
	return cm
}

func (m *canonMap[T]) Load(key T) *T {
	hash := m.hash(abi.NoEscape(unsafe.Pointer(&key)), m.seed)

	i := m.root.Load()
	hashShift := 8 * goarch.PtrSize
	for hashShift != 0 {
		hashShift -= nChildrenLog2

		n := i.children[(hash>>hashShift)&nChildrenMask].Load()
		if n == nil {
			return nil
		}
		if n.isEntry {
			v, _ := n.entry().lookup(key)
			return v
		}
		i = n.indirect()
	}
	panic("unique.canonMap: 迭代时哈希位用尽")
}

func (m *canonMap[T]) LoadOrStore(key T) *T {
	hash := m.hash(abi.NoEscape(unsafe.Pointer(&key)), m.seed)

	var i *indirect[T]
	var hashShift uint
	var slot *atomic.Pointer[node[T]]
	var n *node[T]
	for {
		// 查找键或候选插入位置。
		i = m.root.Load()
		hashShift = 8 * goarch.PtrSize
		haveInsertPoint := false
		for hashShift != 0 {
			hashShift -= nChildrenLog2

			slot = &i.children[(hash>>hashShift)&nChildrenMask]
			n = slot.Load()
			if n == nil {
				// 我们找到了一个 nil 槽位，它是插入的候选位置。
				haveInsertPoint = true
				break
			}
			if n.isEntry {
				// 我们找到了一个现有条目，这是我们能走的最远处。
				// 如果保持这样，我们将不得不用间接节点替换它。
				if v, _ := n.entry().lookup(key); v != nil {
					return v
				}
				haveInsertPoint = true
				break
			}
			i = n.indirect()
		}
		if !haveInsertPoint {
			panic("unique.canonMap: 迭代时哈希位用尽")
		}

		// 获取锁并再次检查我们看到的内容。
		i.mu.Lock()
		n = slot.Load()
		if (n == nil || n.isEntry) && !i.dead.Load() {
			// 我们看到的仍然是真的，所以我们可以继续插入。
			break
		}
		// 我们必须重新开始。
		i.mu.Unlock()
	}
	// 注意：此锁是从我们跳出上面外层循环时持有的。
	// 我们特意将此分离出来，以便可以在这里安全地使用 defer。
	// 一个选择是将其分离到一个新函数中，
	// 但下面使用了太多局部迭代状态，所以这样更简洁。
	defer i.mu.Unlock()

	var oldEntry *entry[T]
	if n != nil {
		oldEntry = n.entry()
		if v, _ := oldEntry.lookup(key); v != nil {
			// 简单情况：通过再次加载，结果正好是我们想要的！
			return v
		}
	}
	newEntry, canon, wp := newEntryNode(key, hash)
	// 修剪死指针。这是为了避免当我们在集合中存储完全相同的值
	// 但由于某种原因清理尚未运行而导致的 O(n) 查找。
	oldEntry = oldEntry.prune()
	if oldEntry == nil {
		// 简单情况：创建一个新条目并存储它。
		slot.Store(&newEntry.node)
	} else {
		// 我们可能需要将已有的条目扩展为一个或多个新节点。
		//
		// 最后发布节点，这将使 oldEntry 和 newEntry 都可见。
		// 我们不希望读者能够观察到 oldEntry 不在树中。
		slot.Store(m.expand(oldEntry, newEntry, hash, hashShift, i))
	}
	runtime.AddCleanup(canon, func(_ struct{}) {
		m.cleanup(hash, wp)
	}, struct{}{})
	return canon
}

// expand 接收 oldEntry 和 newEntry，它们的哈希从第 64 位到 hashShift 位冲突，
// 并生成一个间接节点子树来保存两个新条目。newHash 是新条目中值的哈希。
func (m *canonMap[T]) expand(oldEntry, newEntry *entry[T], newHash uintptr, hashShift uint, parent *indirect[T]) *node[T] {
	// 检查哈希碰撞。
	oldHash := oldEntry.hash
	if oldHash == newHash {
		// 将旧条目存储在新条目的溢出列表中，然后存储新条目。
		newEntry.overflow.Store(oldEntry)
		return &newEntry.node
	}
	// 我们必须添加一个间接节点。更糟的是，我们可能需要添加多个。
	newIndirect := newIndirectNode(parent)
	top := newIndirect
	for {
		if hashShift == 0 {
			panic("unique.canonMap: 插入时哈希位用尽")
		}
		hashShift -= nChildrenLog2 // hashShift 是 parent 所在层级的。我们需要更深入。
		oi := (oldHash >> hashShift) & nChildrenMask
		ni := (newHash >> hashShift) & nChildrenMask
		if oi != ni {
			newIndirect.children[oi].Store(&oldEntry.node)
			newIndirect.children[ni].Store(&newEntry.node)
			break
		}
		nextIndirect := newIndirectNode(newIndirect)
		newIndirect.children[oi].Store(&nextIndirect.node)
		newIndirect = nextIndirect
	}
	return &top.node
}

// cleanup 删除 canon map 中与 wp 对应的条目（如果它仍在映射中）。
// 在调用此函数时，wp 的 Value 方法必须返回 nil。
// hash 必须是 wp 曾经指向的值的哈希（即 *wp.Value() 的哈希）。
func (m *canonMap[T]) cleanup(hash uintptr, wp weak.Pointer[T]) {
	var i *indirect[T]
	var hashShift uint
	var slot *atomic.Pointer[node[T]]
	var n *node[T]
	for {
		// 通过跟随 hash 在映射中查找 wp。
		i = m.root.Load()
		hashShift = 8 * goarch.PtrSize
		haveEntry := false
		for hashShift != 0 {
			hashShift -= nChildrenLog2

			slot = &i.children[(hash>>hashShift)&nChildrenMask]
			n = slot.Load()
			if n == nil {
				// 我们找到了一个 nil 槽位，已被删除。
				return
			}
			if n.isEntry {
				if !n.entry().hasWeakPointer(wp) {
					// 弱指针已被修剪。
					return
				}
				haveEntry = true
				break
			}
			i = n.indirect()
		}
		if !haveEntry {
			panic("unique.canonMap: 迭代时哈希位用尽")
		}

		// 获取锁并再次检查我们看到的内容。
		i.mu.Lock()
		n = slot.Load()
		if n != nil && n.isEntry {
			// 不假思索地修剪条目节点。如果我们做了别人的工作，
			// 比如有人试图插入具有相同哈希的条目（可能是相同的值），
			// 那很好，他们会在不获取锁的情况下退出。
			newEntry := n.entry().prune()
			if newEntry == nil {
				slot.Store(nil)
			} else {
				slot.Store(&newEntry.node)
			}

			// 删除空的内部节点，沿树向上。
			//
			// 我们将在执行此操作时沿树向上进行交接锁定，
			// 因为我们需要删除父节点中每个空节点的链接，
			// 这需要父节点的锁。
			for i.parent != nil && i.empty() {
				if hashShift == 8*goarch.PtrSize {
					panic("unique.canonMap: 迭代时哈希位用尽")
				}
				hashShift += nChildrenLog2

				// 删除父节点中的当前节点。
				parent := i.parent
				parent.mu.Lock()
				i.dead.Store(true) // 可以在父节点的锁之外完成。
				parent.children[(hash>>hashShift)&nChildrenMask].Store(nil)
				i.mu.Unlock()
				i = parent
			}
			i.mu.Unlock()
			return
		}
		// 我们必须重新开始。
		i.mu.Unlock()
	}
}

// node 是节点的头部。它是多态的，
// 实际上要么是 entry，要么是 indirect。
type node[T comparable] struct {
	isEntry bool
}

func (n *node[T]) entry() *entry[T] {
	if !n.isEntry {
		panic("在非条目节点上调用了 entry")
	}
	return (*entry[T])(unsafe.Pointer(n))
}

func (n *node[T]) indirect() *indirect[T] {
	if n.isEntry {
		panic("在条目节点上调用了 indirect")
	}
	return (*indirect[T])(unsafe.Pointer(n))
}

const (
	// 16 个子节点。这似乎是加载性能的最佳点：
	// 更小的话我们会在 CPU 性能上损失 50% 或更多。
	// 更大的话收益微乎其微（32 个子节点约 1% 的改进）。
	nChildrenLog2 = 4
	nChildren     = 1 << nChildrenLog2
	nChildrenMask = nChildren - 1
)

// indirect 是哈希字典树中的内部节点。
type indirect[T comparable] struct {
	node[T]
	dead     atomic.Bool
	parent   *indirect[T]
	mu       sync.Mutex // 保护对 children 和任何是条目节点的子节点的修改。
	children [nChildren]atomic.Pointer[node[T]]
}

func newIndirectNode[T comparable](parent *indirect[T]) *indirect[T] {
	return &indirect[T]{node: node[T]{isEntry: false}, parent: parent}
}

func (i *indirect[T]) empty() bool {
	for j := range i.children {
		if i.children[j].Load() != nil {
			return false
		}
	}
	return true
}

// entry 是哈希字典树中的叶子节点。
type entry[T comparable] struct {
	node[T]
	overflow atomic.Pointer[entry[T]] // 哈希碰撞的溢出。
	key      weak.Pointer[T]
	hash     uintptr
}

func newEntryNode[T comparable](key T, hash uintptr) (*entry[T], *T, weak.Pointer[T]) {
	k := new(T)
	*k = key
	wp := weak.Make(k)
	return &entry[T]{
		node: node[T]{isEntry: true},
		key:  wp,
		hash: hash,
	}, k, wp
}

// lookup 在溢出链中查找具有提供的键的条目。
//
// 返回键的规范指针和该规范指针的弱指针。
func (e *entry[T]) lookup(key T) (*T, weak.Pointer[T]) {
	for e != nil {
		s := e.key.Value()
		if s != nil && *s == key {
			return s, e.key
		}
		e = e.overflow.Load()
	}
	return nil, weak.Pointer[T]{}
}

// hasWeakPointer 如果提供的弱指针可以在溢出链中找到则返回 true。
func (e *entry[T]) hasWeakPointer(wp weak.Pointer[T]) bool {
	for e != nil {
		if e.key == wp {
			return true
		}
		e = e.overflow.Load()
	}
	return false
}

// prune 删除溢出链中所有键为 nil 的条目。
//
// 调用者必须持有 e 的父节点的锁。
func (e *entry[T]) prune() *entry[T] {
	// 修剪列表的头部。
	for e != nil {
		if e.key.Value() != nil {
			break
		}
		e = e.overflow.Load()
	}
	if e == nil {
		return nil
	}

	// 修剪列表中的各个节点。
	newHead := e
	i := &e.overflow
	e = i.Load()
	for e != nil {
		if e.key.Value() != nil {
			i = &e.overflow
		} else {
			i.Store(e.overflow.Load())
		}
		e = e.overflow.Load()
	}
	return newHead
}

// 引入 runtime.rand，这样我们就不需要依赖 math/rand/v2。
//
//go:linkname runtime_rand runtime.rand
func runtime_rand() uint64
