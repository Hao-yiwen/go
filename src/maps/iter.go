// 版权所有 2024 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package maps

import "iter"

// All 返回 m 中键值对的迭代器。
// 迭代顺序未指定，并且不保证每次调用都相同。
func All[Map ~map[K]V, K comparable, V any](m Map) iter.Seq2[K, V] {
	return func(yield func(K, V) bool) {
		for k, v := range m {
			if !yield(k, v) {
				return
			}
		}
	}
}

// Keys 返回 m 中键的迭代器。
// 迭代顺序未指定，并且不保证每次调用都相同。
func Keys[Map ~map[K]V, K comparable, V any](m Map) iter.Seq[K] {
	return func(yield func(K) bool) {
		for k := range m {
			if !yield(k) {
				return
			}
		}
	}
}

// Values 返回 m 中值的迭代器。
// 迭代顺序未指定，并且不保证每次调用都相同。
func Values[Map ~map[K]V, K comparable, V any](m Map) iter.Seq[V] {
	return func(yield func(V) bool) {
		for _, v := range m {
			if !yield(v) {
				return
			}
		}
	}
}

// Insert 将 seq 中的键值对添加到 m。
// 如果 seq 中的键已存在于 m 中，其值将被覆盖。
func Insert[Map ~map[K]V, K comparable, V any](m Map, seq iter.Seq2[K, V]) {
	for k, v := range seq {
		m[k] = v
	}
}

// Collect 将 seq 中的键值对收集到新映射中并返回。
func Collect[K comparable, V any](seq iter.Seq2[K, V]) map[K]V {
	m := make(map[K]V)
	Insert(m, seq)
	return m
}
