// 版权所有 2021 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// slices 包定义了对任何类型的切片有用的各种函数。
package slices

import (
	"cmp"
	"math/bits"
	"unsafe"
)

// Equal 报告两个切片是否相等：长度相同且所有元素相等。
// 如果长度不同，Equal 返回 false。
// 否则，按索引递增顺序比较元素，比较在第一个不相等的对处停止。
// 空切片和 nil 切片被认为是相等的。
// 浮点数 NaN 不被认为是相等的。
func Equal[S ~[]E, E comparable](s1, s2 S) bool {
	if len(s1) != len(s2) {
		return false
	}
	for i := range s1 {
		if s1[i] != s2[i] {
			return false
		}
	}
	return true
}

// EqualFunc 使用每对元素上的相等函数报告两个切片是否相等。
// 如果长度不同，EqualFunc 返回 false。
// 否则，按索引递增顺序比较元素，比较在 eq 返回 false 的第一个索引处停止。
func EqualFunc[S1 ~[]E1, S2 ~[]E2, E1, E2 any](s1 S1, s2 S2, eq func(E1, E2) bool) bool {
	if len(s1) != len(s2) {
		return false
	}
	for i, v1 := range s1 {
		v2 := s2[i]
		if !eq(v1, v2) {
			return false
		}
	}
	return true
}

// Compare 使用 [cmp.Compare] 比较 s1 和 s2 的每对元素。
// 元素按顺序比较，从索引 0 开始，直到一个元素不等于另一个。
// 返回比较第一个不匹配元素的结果。
// 如果两个切片在其中一个结束之前都相等，则较短的切片被认为小于较长的切片。
// 如果 s1 == s2 结果为 0，如果 s1 < s2 结果为 -1，如果 s1 > s2 结果为 +1。
func Compare[S ~[]E, E cmp.Ordered](s1, s2 S) int {
	for i, v1 := range s1 {
		if i >= len(s2) {
			return +1
		}
		v2 := s2[i]
		if c := cmp.Compare(v1, v2); c != 0 {
			return c
		}
	}
	if len(s1) < len(s2) {
		return -1
	}
	return 0
}

// CompareFunc 类似于 [Compare]，但在每对元素上使用自定义比较函数。
// 结果是 cmp 的第一个非零结果；如果 cmp 始终返回 0，
// 则当 len(s1) == len(s2) 时结果为 0，当 len(s1) < len(s2) 时结果为 -1，
// 当 len(s1) > len(s2) 时结果为 +1。
func CompareFunc[S1 ~[]E1, S2 ~[]E2, E1, E2 any](s1 S1, s2 S2, cmp func(E1, E2) int) int {
	for i, v1 := range s1 {
		if i >= len(s2) {
			return +1
		}
		v2 := s2[i]
		if c := cmp(v1, v2); c != 0 {
			return c
		}
	}
	if len(s1) < len(s2) {
		return -1
	}
	return 0
}

// Index 返回 s 中 v 第一次出现的索引，如果不存在则返回 -1。
func Index[S ~[]E, E comparable](s S, v E) int {
	for i := range s {
		if v == s[i] {
			return i
		}
	}
	return -1
}

// IndexFunc 返回满足 f(s[i]) 的第一个索引 i，如果没有则返回 -1。
func IndexFunc[S ~[]E, E any](s S, f func(E) bool) int {
	for i := range s {
		if f(s[i]) {
			return i
		}
	}
	return -1
}

// Contains 报告 v 是否存在于 s 中。
func Contains[S ~[]E, E comparable](s S, v E) bool {
	return Index(s, v) >= 0
}

// ContainsFunc 报告 s 中是否至少有一个元素 e 满足 f(e)。
func ContainsFunc[S ~[]E, E any](s S, f func(E) bool) bool {
	return IndexFunc(s, f) >= 0
}

// Insert 在索引 i 处将值 v... 插入 s，返回修改后的切片。
// s[i:] 处的元素向上移动以腾出空间。
// 在返回的切片 r 中，r[i] == v[0]，
// 如果 i < len(s)，r[i+len(v)] == 原来在 s[i] 的值。
// 如果 i > len(s)，Insert 会 panic。
// 此函数的时间复杂度为 O(len(s) + len(v))。
// 如果结果为空，它与 s 具有相同的空性。
func Insert[S ~[]E, E any](s S, i int, v ...E) S {
	_ = s[i:] // 边界检查

	m := len(v)
	if m == 0 {
		return s
	}
	n := len(s)
	if i == n {
		return append(s, v...)
	}
	if n+m > cap(s) {
		// 使用 append 而不是 make，这样我们可以将切片的大小
		// 提升到下一个存储类。
		// 这就是 Grow 所做的，但我们不调用 Grow，
		// 因为那可能会复制值两次。
		s2 := append(s[:i], make(S, n+m-i)...)
		copy(s2[i:], v)
		copy(s2[i+m:], s[i:])
		return s2
	}
	s = s[:n+m]

	// 之前：
	// s: aaaaaaaabbbbccccccccdddd
	//            ^   ^       ^   ^
	//            i  i+m      n  n+m
	// 之后：
	// s: aaaaaaaavvvvbbbbcccccccc
	//            ^   ^       ^   ^
	//            i  i+m      n  n+m
	//
	// a 是 s 中不移动的值。
	// v 是从 v 复制进来的值。
	// b 和 c 是 s 中索引向上移动的值。
	// d 是被覆盖的值，再也看不到了。

	if !overlaps(v, s[i+m:]) {
		// 简单情况 - v 不与 c 或 d 区域重叠。
		// （它可能在 a 或 b 的某些部分，或完全在别处。）
		// 我们复制的数据根本不写入 v，所以直接做就行。

		copy(s[i+m:], s[i:])

		// 现在我们有
		// s: aaaaaaaabbbbbbbbcccccccc
		//            ^   ^       ^   ^
		//            i  i+m      n  n+m
		// 注意 b 值被复制了。

		copy(s[i:], v)

		// 现在我们有
		// s: aaaaaaaavvvvbbbbcccccccc
		//            ^   ^       ^   ^
		//            i  i+m      n  n+m
		// 这就是我们想要的结果。
		return s
	}

	// 困难情况 - v 与 c 或 d 重叠。我们不能只是向上移动数据，
	// 因为我们会移动或破坏我们试图插入的值。
	// 所以相反，先把 v 写在 d 上，然后旋转。
	copy(s[n:], v)

	// 现在我们有
	// s: aaaaaaaabbbbccccccccvvvv
	//            ^   ^       ^   ^
	//            i  i+m      n  n+m

	rotateRight(s[i:], m)

	// 现在我们有
	// s: aaaaaaaavvvvbbbbcccccccc
	//            ^   ^       ^   ^
	//            i  i+m      n  n+m
	// 这就是我们想要的结果。
	return s
}

// Delete 从 s 中删除元素 s[i:j]，返回修改后的切片。
// 如果 j > len(s) 或 s[i:j] 不是 s 的有效切片，Delete 会 panic。
// Delete 的时间复杂度为 O(len(s)-i)，所以如果必须删除多项，
// 最好一次性删除所有项，而不是逐个删除。
// Delete 将 s[len(s)-(j-i):len(s)] 的元素置零。
// 如果结果为空，它与 s 具有相同的空性。
func Delete[S ~[]E, E any](s S, i, j int) S {
	_ = s[i:j:len(s)] // 边界检查

	if i == j {
		return s
	}

	oldlen := len(s)
	s = append(s[:i], s[j:]...)
	clear(s[len(s):oldlen]) // 将过时元素置零/nil，用于 GC
	return s
}

// DeleteFunc 从 s 中删除 del 返回 true 的任何元素，返回修改后的切片。
// DeleteFunc 将新长度和原始长度之间的元素置零。
// 如果结果为空，它与 s 具有相同的空性。
func DeleteFunc[S ~[]E, E any](s S, del func(E) bool) S {
	i := IndexFunc(s, del)
	if i == -1 {
		return s
	}
	// 在找到要删除的元素之前不要开始复制元素。
	for j := i + 1; j < len(s); j++ {
		if v := s[j]; !del(v) {
			s[i] = v
			i++
		}
	}
	clear(s[i:]) // 将过时元素置零/nil，用于 GC
	return s[:i]
}

// Replace 用给定的 v 替换元素 s[i:j]，并返回修改后的切片。
// 如果 j > len(s) 或 s[i:j] 不是 s 的有效切片，Replace 会 panic。
// 当 len(v) < (j-i) 时，Replace 将新长度和原始长度之间的元素置零。
// 如果结果为空，它与 s 具有相同的空性。
func Replace[S ~[]E, E any](s S, i, j int, v ...E) S {
	_ = s[i:j] // 边界检查

	if i == j {
		return Insert(s, i, v...)
	}
	if j == len(s) {
		s2 := append(s[:i], v...)
		if len(s2) < len(s) {
			clear(s[len(s2):]) // 将过时元素置零/nil，用于 GC
		}
		return s2
	}

	tot := len(s[:i]) + len(v) + len(s[j:])
	if tot > cap(s) {
		// 太大放不下，分配并复制。
		s2 := append(s[:i], make(S, tot-i)...) // 参见 Insert
		copy(s2[i:], v)
		copy(s2[i+len(v):], s[j:])
		return s2
	}

	r := s[:tot]

	if i+len(v) <= j {
		// 简单，因为 v 适合删除的部分。
		copy(r[i:], v)
		copy(r[i+len(v):], s[j:])
		clear(s[tot:]) // 将过时元素置零/nil，用于 GC
		return r
	}

	// 我们正在扩展（v 比 j-i 大）。
	// 情况类似这样：
	// （示例 i=4,j=8,len(s)=16,len(v)=6）
	// s: aaaaxxxxbbbbbbbbyy
	//        ^   ^       ^ ^
	//        i   j  len(s) tot
	// a: s 的前缀
	// x: 删除的范围
	// b: s 的更多部分
	// y: 要扩展到的区域

	if !overlaps(r[i+len(v):], v) {
		// 简单，因为 v 不会被第一次复制破坏。
		copy(r[i+len(v):], s[j:])
		copy(r[i:], v)
		return r
	}

	// 这是我们没有单一位置可以复制 v 的情况。
	// 它的部分需要去两个不同的地方。
	// 我们想将 v 的前缀复制到 y，后缀复制到 x，
	// 然后向右旋转 |y| 个位置。
	//
	//        v[2:]      v[:2]
	//         |           |
	// s: aaaavvvvbbbbbbbbvv
	//        ^   ^       ^ ^
	//        i   j  len(s) tot
	//
	// 如果这两个目的地中的任何一个不与 v 别名，那就没问题。
	y := len(v) - (j - i) // y 部分的长度

	if !overlaps(r[i:j], v) {
		copy(r[i:j], v[y:])
		copy(r[len(s):], v[:y])
		rotateRight(r[i:], y)
		return r
	}
	if !overlaps(r[len(s):], v) {
		copy(r[len(s):], v[:y])
		copy(r[i:j], v[y:])
		rotateRight(r[i:], y)
		return r
	}

	// 现在我们知道 v 与 x 和 y 都重叠。
	// 这意味着 b 的全部都在 v *内部*。
	// 所以我们根本不需要保留 b；相反我们可以先复制 v，
	// 然后将 v 的 b 部分从 v 复制到正确的目的地。
	k := startIdx(v, s[j:])
	copy(r[i:], v)
	copy(r[i+len(v):], r[i+k:])
	return r
}

// Clone 返回切片的副本。
// 元素使用赋值复制，所以这是浅克隆。
// 结果可能有额外的未使用容量。
// 结果保留 s 的空性。
func Clone[S ~[]E, E any](s S) S {
	// 保留空性以防它很重要。
	if s == nil {
		return nil
	}
	// 避免 s[:0:0]，因为在克隆大数组的零长度切片时会导致不想要的活性；
	// 参见 https://go.dev/issue/68488。
	return append(S{}, s...)
}

// Compact 用单个副本替换连续的相等元素运行。
// 这类似于 Unix 上的 uniq 命令。
// Compact 修改切片 s 的内容并返回修改后的切片，其长度可能更短。
// Compact 将新长度和原始长度之间的元素置零。
// 结果保留 s 的空性。
func Compact[S ~[]E, E comparable](s S) S {
	if len(s) < 2 {
		return s
	}
	for k := 1; k < len(s); k++ {
		if s[k] == s[k-1] {
			s2 := s[k:]
			for k2 := 1; k2 < len(s2); k2++ {
				if s2[k2] != s2[k2-1] {
					s[k] = s2[k2]
					k++
				}
			}

			clear(s[k:]) // 将过时元素置零/nil，用于 GC
			return s[:k]
		}
	}
	return s
}

// CompactFunc 类似于 [Compact]，但使用相等函数比较元素。
// 对于比较相等的元素运行，CompactFunc 保留第一个。
// CompactFunc 将新长度和原始长度之间的元素置零。
// 结果保留 s 的空性。
func CompactFunc[S ~[]E, E any](s S, eq func(E, E) bool) S {
	if len(s) < 2 {
		return s
	}
	for k := 1; k < len(s); k++ {
		if eq(s[k], s[k-1]) {
			s2 := s[k:]
			for k2 := 1; k2 < len(s2); k2++ {
				if !eq(s2[k2], s2[k2-1]) {
					s[k] = s2[k2]
					k++
				}
			}

			clear(s[k:]) // 将过时元素置零/nil，用于 GC
			return s[:k]
		}
	}
	return s
}

// Grow 如有必要增加切片的容量，以保证另外 n 个元素的空间。
// 在 Grow(n) 之后，至少可以向切片追加 n 个元素而无需另一次分配。
// 如果 n 是负数或太大无法分配内存，Grow 会 panic。
// 结果保留 s 的空性。
func Grow[S ~[]E, E any](s S, n int) S {
	if n < 0 {
		panic("cannot be negative")
	}
	if n -= cap(s) - len(s); n > 0 {
		// 此表达式只分配一次（参见测试）。
		s = append(s[:cap(s)], make([]E, n)...)[:len(s)]
	}
	return s
}

// Clip 从切片中删除未使用的容量，返回 s[:len(s):len(s)]。
// 结果保留 s 的空性。
func Clip[S ~[]E, E any](s S) S {
	return s[:len(s):len(s)]
}

// TODO: 还有其他旋转算法。
// 此算法的理想特性是每个元素最多移动两次。
// 循环跟踪算法可以是 1 次写入，但它对缓存不太友好。

// rotateLeft 将 s 向左旋转 r 个空间。
// s_final[i] = s_orig[i+r]，回绕。
func rotateLeft[E any](s []E, r int) {
	Reverse(s[:r])
	Reverse(s[r:])
	Reverse(s)
}
func rotateRight[E any](s []E, r int) {
	rotateLeft(s, len(s)-r)
}

// overlaps 报告内存范围 a[:len(a)] 和 b[:len(b)] 是否重叠。
func overlaps[E any](a, b []E) bool {
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	elemSize := unsafe.Sizeof(a[0])
	if elemSize == 0 {
		return false
	}
	// TODO: 一旦可用，使用 runtime/unsafe 设施。参见 issue 12445。
	// 另请参见 crypto/internal/fips140/alias/alias.go:AnyOverlap
	return uintptr(unsafe.Pointer(&a[0])) <= uintptr(unsafe.Pointer(&b[len(b)-1]))+(elemSize-1) &&
		uintptr(unsafe.Pointer(&b[0])) <= uintptr(unsafe.Pointer(&a[len(a)-1]))+(elemSize-1)
}

// startIdx 返回 needle 在 haystack 中开始的索引。
// 前提条件：needle 必须完全在 haystack 内部别名。
func startIdx[E any](haystack, needle []E) int {
	p := &needle[0]
	for i := range haystack {
		if p == &haystack[i] {
			return i
		}
	}
	// TODO: 如果重叠是非整数个 E 怎么办？
	panic("needle not found")
}

// Reverse 原地反转切片的元素。
func Reverse[S ~[]E, E any](s S) {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
}

// Concat 返回连接传入切片的新切片。
// 如果连接为空，结果是 nil。
func Concat[S ~[]E, E any](slices ...S) S {
	size := 0
	for _, s := range slices {
		size += len(s)
		if size < 0 {
			panic("len out of range")
		}
	}
	// 使用 Grow，而不是 make，以向上舍入到大小类：
	// 额外的空间在其他情况下未使用，并帮助那些向结果追加几个元素的调用者。
	newslice := Grow[S](nil, size)
	for _, s := range slices {
		newslice = append(newslice, s...)
	}
	return newslice
}

// Repeat 返回将提供的切片重复给定次数的新切片。
// 结果的长度和容量为 (len(x) * count)。
// 结果永远不是 nil。
// 如果 count 为负数或 (len(x) * count) 的结果溢出，Repeat 会 panic。
func Repeat[S ~[]E, E any](x S, count int) S {
	if count < 0 {
		panic("cannot be negative")
	}

	const maxInt = ^uint(0) >> 1
	hi, lo := bits.Mul(uint(len(x)), uint(count))
	if hi > 0 || lo > maxInt {
		panic("the result of (len(x) * count) overflows")
	}

	newslice := make(S, int(lo)) // lo = len(x) * count
	n := copy(newslice, x)
	for n < len(newslice) {
		n += copy(newslice[n:], newslice[:n])
	}
	return newslice
}
