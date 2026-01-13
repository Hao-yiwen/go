// 版权所有 2024 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package slices

import (
	"cmp"
	"iter"
)

// All 返回切片中索引-值对的迭代器，按通常顺序。
func All[Slice ~[]E, E any](s Slice) iter.Seq2[int, E] {
	return func(yield func(int, E) bool) {
		for i, v := range s {
			if !yield(i, v) {
				return
			}
		}
	}
}

// Backward 返回切片中索引-值对的迭代器，
// 以降序索引向后遍历。
func Backward[Slice ~[]E, E any](s Slice) iter.Seq2[int, E] {
	return func(yield func(int, E) bool) {
		for i := len(s) - 1; i >= 0; i-- {
			if !yield(i, s[i]) {
				return
			}
		}
	}
}

// Values 返回按顺序产生切片元素的迭代器。
func Values[Slice ~[]E, E any](s Slice) iter.Seq[E] {
	return func(yield func(E) bool) {
		for _, v := range s {
			if !yield(v) {
				return
			}
		}
	}
}

// AppendSeq 将 seq 中的值追加到切片并返回扩展后的切片。
// 如果 seq 为空，结果保留 s 的空性。
func AppendSeq[Slice ~[]E, E any](s Slice, seq iter.Seq[E]) Slice {
	for v := range seq {
		s = append(s, v)
	}
	return s
}

// Collect 将 seq 中的值收集到新切片中并返回。
// 如果 seq 为空，结果是 nil。
func Collect[E any](seq iter.Seq[E]) []E {
	return AppendSeq([]E(nil), seq)
}

// Sorted 将 seq 中的值收集到新切片中，对切片排序，然后返回。
// 如果 seq 为空，结果是 nil。
func Sorted[E cmp.Ordered](seq iter.Seq[E]) []E {
	s := Collect(seq)
	Sort(s)
	return s
}

// SortedFunc 将 seq 中的值收集到新切片中，
// 使用比较函数对切片排序，然后返回。
// 如果 seq 为空，结果是 nil。
func SortedFunc[E any](seq iter.Seq[E], cmp func(E, E) int) []E {
	s := Collect(seq)
	SortFunc(s, cmp)
	return s
}

// SortedStableFunc 将 seq 中的值收集到新切片中。
// 然后在保持相等元素原始顺序的同时对切片排序，
// 使用比较函数比较元素。
// 它返回新切片。
// 如果 seq 为空，结果是 nil。
func SortedStableFunc[E any](seq iter.Seq[E], cmp func(E, E) int) []E {
	s := Collect(seq)
	SortStableFunc(s, cmp)
	return s
}

// Chunk 返回 s 的连续子切片的迭代器，每个子切片最多 n 个元素。
// 除最后一个外，所有子切片的大小都是 n。
// 所有子切片都被裁剪为没有超出长度的容量。
// 如果 s 为空，序列为空：序列中没有空切片。
// 如果 n 小于 1，Chunk 会 panic。
func Chunk[Slice ~[]E, E any](s Slice, n int) iter.Seq[Slice] {
	if n < 1 {
		panic("cannot be less than 1")
	}

	return func(yield func(Slice) bool) {
		for i := 0; i < len(s); i += n {
			// 根据需要将最后一个块限制在切片边界内。
			end := min(n, len(s[i:]))

			// 设置每个块的容量，使追加到块不会修改原始切片。
			if !yield(s[i : i+end : i+end]) {
				return
			}
		}
	}
}
