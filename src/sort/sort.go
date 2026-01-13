// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:generate go run gen_sort_variants.go

// Package sort 为排序切片和用户定义的集合提供原始操作。
package sort

import (
	"math/bits"
	"slices"
)

// 实现 Interface 的类型可以通过此包中的例程进行排序。
// 这些方法通过整数索引引用底层集合的元素。
type Interface interface {
	// Len 是集合中元素的数量。
	Len() int

	// Less 报告索引为 i 的元素是否必须排在索引为 j 的元素之前。
	//
	// 如果 Less(i, j) 和 Less(j, i) 都为 false，
	// 那么索引 i 和 j 处的元素被认为相等。
	// Sort 可能会在最终结果中以任何顺序放置相等的元素，
	// 而 Stable 会保留相等元素的原始输入顺序。
	//
	// Less 必须描述一个[严格弱序]。例如：
	//  - 如果 Less(i, j) 和 Less(j, k) 都为 true，那么 Less(i, k) 也必须为 true。
	//  - 如果 Less(i, j) 和 Less(j, k) 都为 false，那么 Less(i, k) 也必须为 false。
	//
	// 注意，浮点比较（float32 或 float64 值上的 < 运算符）
	// 当涉及非数字 (NaN) 值时不是严格弱序。
	// 有关浮点值的正确实现，请参见 Float64Slice.Less。
	//
	// [严格弱序]: https://en.wikipedia.org/wiki/Weak_ordering#Strict_weak_orderings
	Less(i, j int) bool

	// Swap 交换索引为 i 和 j 的元素。
	Swap(i, j int)
}

// Sort 根据 Less 方法确定的顺序按升序排序数据。
// 它对 data.Len 进行一次调用以确定 n，对 data.Less 和 data.Swap 进行 O(n*log(n)) 次调用。
// 该排序不保证稳定。
//
// 注意：在许多情况下，较新的 [slices.SortFunc] 函数更人性化且运行速度更快。
func Sort(data Interface) {
	n := data.Len()
	if n <= 1 {
		return
	}
	limit := bits.Len(uint(n))
	pdqsort(data, 0, n, limit)
}

type sortedHint int // 当选择枢轴时给 pdqsort 的提示

const (
	unknownHint sortedHint = iota
	increasingHint
	decreasingHint
)

// xorshift 论文: https://www.jstatsoft.org/article/view/v008i14/xorshift.pdf
type xorshift uint64

func (r *xorshift) Next() uint64 {
	*r ^= *r << 13
	*r ^= *r >> 7
	*r ^= *r << 17
	return uint64(*r)
}

func nextPowerOfTwo(length int) uint {
	shift := uint(bits.Len(uint(length)))
	return uint(1 << shift)
}

// lessSwap 是一对 Less 和 Swap 函数，用于 zfuncversion.go 中
// 自动生成的 sort.go 的函数优化变体。
type lessSwap struct {
	Less func(i, j int) bool
	Swap func(i, j int)
}

type reverse struct {
	// 这个嵌入的 Interface 允许 Reverse 使用另一个 Interface 实现的方法。
	Interface
}

// Less 返回嵌入式实现的 Less 方法的相反结果。
func (r reverse) Less(i, j int) bool {
	return r.Interface.Less(j, i)
}

// Reverse 返回数据的反向顺序。
func Reverse(data Interface) Interface {
	return &reverse{data}
}

// IsSorted 报告数据是否已排序。
//
// 注意：在许多情况下，较新的 [slices.IsSortedFunc] 函数更人性化且运行速度更快。
func IsSorted(data Interface) bool {
	n := data.Len()
	for i := n - 1; i > 0; i-- {
		if data.Less(i, i-1) {
			return false
		}
	}
	return true
}

// 常见情况的便利类型

// IntSlice 将 Interface 的方法附加到 []int，按升序排序。
type IntSlice []int

func (x IntSlice) Len() int           { return len(x) }
func (x IntSlice) Less(i, j int) bool { return x[i] < x[j] }
func (x IntSlice) Swap(i, j int)      { x[i], x[j] = x[j], x[i] }

// Sort 是一个便利方法：x.Sort() 调用 Sort(x)。
func (x IntSlice) Sort() { Sort(x) }

// Float64Slice 为 []float64 实现 Interface，按升序排序，
// 其中非数字 (NaN) 值排在其他值之前。
type Float64Slice []float64

func (x Float64Slice) Len() int { return len(x) }

// Less 报告 x[i] 是否应该排在 x[j] 之前，如 sort Interface 所需。
// 注意，浮点比较本身不是一个传递关系：它不会为非数字 (NaN) 值报告一致的顺序。
// Less 的此实现使用以下方式将 NaN 值放在任何其他值之前：
//
//	x[i] < x[j] || (math.IsNaN(x[i]) && !math.IsNaN(x[j]))
func (x Float64Slice) Less(i, j int) bool { return x[i] < x[j] || (isNaN(x[i]) && !isNaN(x[j])) }
func (x Float64Slice) Swap(i, j int)      { x[i], x[j] = x[j], x[i] }

// isNaN 是 math.IsNaN 的副本，以避免对 math 包的依赖。
func isNaN(f float64) bool {
	return f != f
}

// Sort 是一个便利方法：x.Sort() 调用 Sort(x)。
func (x Float64Slice) Sort() { Sort(x) }

// StringSlice 将 Interface 的方法附加到 []string，按升序排序。
type StringSlice []string

func (x StringSlice) Len() int           { return len(x) }
func (x StringSlice) Less(i, j int) bool { return x[i] < x[j] }
func (x StringSlice) Swap(i, j int)      { x[i], x[j] = x[j], x[i] }

// Sort 是一个便利方法：x.Sort() 调用 Sort(x)。
func (x StringSlice) Sort() { Sort(x) }

// 常见情况的便利包装函数

// Ints 按升序对整数切片进行排序。
//
// 注意：从 Go 1.22 开始，此函数只是调用 [slices.Sort]。
func Ints(x []int) { slices.Sort(x) }

// Float64s 按升序对 float64 切片进行排序。
// 非数字 (NaN) 值排在其他值之前。
//
// 注意：从 Go 1.22 开始，此函数只是调用 [slices.Sort]。
func Float64s(x []float64) { slices.Sort(x) }

// Strings 按升序对字符串切片进行排序。
//
// 注意：从 Go 1.22 开始，此函数只是调用 [slices.Sort]。
func Strings(x []string) { slices.Sort(x) }

// IntsAreSorted 报告切片 x 是否按升序排序。
//
// 注意：从 Go 1.22 开始，此函数只是调用 [slices.IsSorted]。
func IntsAreSorted(x []int) bool { return slices.IsSorted(x) }

// Float64sAreSorted 报告切片 x 是否按升序排序，
// 其中非数字 (NaN) 值排在任何其他值之前。
//
// 注意：从 Go 1.22 开始，此函数只是调用 [slices.IsSorted]。
func Float64sAreSorted(x []float64) bool { return slices.IsSorted(x) }

// StringsAreSorted 报告切片 x 是否按升序排序。
//
// 注意：从 Go 1.22 开始，此函数只是调用 [slices.IsSorted]。
func StringsAreSorted(x []string) bool { return slices.IsSorted(x) }

// 关于稳定排序的注释：
// 所使用的算法很简单，可在所有输入上证明正确，并且仅使用对数级额外栈空间。
// 与其他原地稳定排序算法相比，它们的性能良好。
//
// 对其他已评估的算法的评论：
//  - GCC 4.6.3 stable_sort 与来自 libstdc++ 的 merge_without_buffer：
//    速度不快。
//  - GCC 的 __rotate 用于块旋转：速度不快。
//  - Jyrki Katajainen、Tomi A. Pasanen 和 Jukka Teuhola 的
//    "Practical in-place mergesort"；Nordic Journal of Computing 3,1 (1996)，27-40：
//    给定的算法是原地的，Swap 和赋值的数量随 n log n 增长，但算法不稳定。
//  - J.I. Munro 和 V. Raman 在 Algorithmica (1996) 16, 115-160 中的
//    "Fast Stable In-Place Sorting with O(n) Data Moves"：
//    此算法要么需要额外的 2n 比特，要么只在有足够不同元素时工作
//    以对某些必须稍后撤销的排列进行编码（因此在任何输入上都不稳定）。
//  - 我找到的所有最优原地排序/合并算法要么不稳定，要么依赖于
//    每一步中有足够的不同元素来编码执行的块重排。
//    另请参阅"In-Place Merging Algorithms"，
//    Denham Coates-Evely，计算机科学部门，国王学院，
//    2004 年 1 月及其中的参考资料。
//  - "最优"算法通常在赋值数量上是最优的，但 Interface 只有 Swap 作为操作。

// Stable 根据 Less 方法确定的顺序按升序排序数据，
// 同时保持相等元素的原始顺序。
//
// 它对 data.Len 进行一次调用以确定 n，对 data.Less 进行 O(n*log(n)) 次调用，
// 对 data.Swap 进行 O(n*log(n)*log(n)) 次调用。
//
// 注意：在许多情况下，较新的 slices.SortStableFunc 函数更人性化且运行速度更快。
func Stable(data Interface) {
	stable(data, data.Len())
}

/*
稳定排序的复杂性


块交换旋转的复杂性

每个 Swap 将一个新元素放入其正确的最终位置。
到达最终位置的元素不再被移动。
因此块交换旋转需要 |u|+|v| 次 Swap 调用。
这是最优的，因为每个元素可能需要移动。

与其他通常计算赋值数量而不是交换的最优算法进行比较时要小心：
例如，Dudzinski 和 Dydek 用于原地旋转的最优算法使用
O(u + v + gcd(u,v)) 赋值，这比我们的 O(3 * (u+v)) 更好，
因为 gcd(u,v) <= u。


通过 SymMerge 和 BlockSwap 旋转进行稳定排序

SymMerg 对相同大小输入 M = N 的复杂性：
Less 调用：O(M*log(N/M+1)) = O(N*log(2)) = O(N)
Swap 调用：O((M+N)*log(M)) = O(2*N*log(N)) = O(N*log(N))

（以下论证不会对缺少的 -1 或其他不影响最终结果的内容产生歧义）。

设 n = data.Len()。假设 n = 2^k。

普通合并排序执行 log(n) = k 次迭代。
在第 i 次迭代中，算法合并 2^(k-i) 个块，每个大小为 2^i。

因此合并排序的第 i 次迭代执行：
Less 调用  O(2^(k-i) * 2^i) = O(2^k) = O(2^log(n)) = O(n)
Swap 调用  O(2^(k-i) * 2^i * log(2^i)) = O(2^k * i) = O(n*i)

总共执行 k = log(n) 次迭代；所以总共：
Less 调用 O(log(n) * n)
Swap 调用 O(n + 2*n + 3*n + ... + (k-1)*n + k*n)
   = O((k/2) * k * n) = O(n * k^2) = O(n * log^2(n))


上述结果应该推广到任意 n = 2^k + p，
并且不应受到初始插入排序阶段的影响：
在 Swap 和 Less 上，插入排序为 O(n^2)，
因此在 n/bs 个块的大小为 bs 的块上为 O(bs^2)：
插入排序期间 O(bs*n) Swap 和 Less。
合并排序迭代从 i = log(bs) 开始。以 t = log(bs) 常数：
Less 调用 O((log(n)-t) * n + bs*n) = O(log(n)*n + (bs-t)*n)
   = O(n * log(n))
Swap 调用 O(n * log^2(n) - (t^2+t)/2*n) = O(n * log^2(n))

*/
