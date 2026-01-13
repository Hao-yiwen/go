// Copyright 2010 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package suffixarray 实现了使用内存中的后缀数组在对数时间内的子串搜索。
//
// 使用示例：
//
//	// 为某些数据创建索引
//	index := suffixarray.New(data)
//
//	// 查找字节切片 s
//	offsets1 := index.Lookup(s, -1) // s 在 data 中出现的所有索引列表
//	offsets2 := index.Lookup(s, 3)  // s 在 data 中出现的最多 3 个索引列表
package suffixarray

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"regexp"
	"slices"
	"sort"
)

// 可以为了测试而改变
var maxData32 int = realMaxData32

const realMaxData32 = math.MaxInt32

// Index 实现了用于快速子串搜索的后缀数组。
type Index struct {
	data []byte
	sa   ints // data 的后缀数组；sa.len() == len(data)
}

// An ints 要么是 []int32，要么是 []int64。
// 即其中一个为空，另一个是真实的数据。
// 当 len(data) > maxData32 时使用 int64 形式
type ints struct {
	int32 []int32
	int64 []int64
}

func (a *ints) len() int {
	return len(a.int32) + len(a.int64)
}

func (a *ints) get(i int) int64 {
	if a.int32 != nil {
		return int64(a.int32[i])
	}
	return a.int64[i]
}

func (a *ints) set(i int, v int64) {
	if a.int32 != nil {
		a.int32[i] = int32(v)
	} else {
		a.int64[i] = v
	}
}

func (a *ints) slice(i, j int) ints {
	if a.int32 != nil {
		return ints{a.int32[i:j], nil}
	}
	return ints{nil, a.int64[i:j]}
}

// New 为数据创建一个新的 [Index]。
// [Index] 创建时间是 O(N)，其中 N = len(data)。
func New(data []byte) *Index {
	ix := &Index{data: data}
	if len(data) <= maxData32 {
		ix.sa.int32 = make([]int32, len(data))
		text_32(data, ix.sa.int32)
	} else {
		ix.sa.int64 = make([]int64, len(data))
		text_64(data, ix.sa.int64)
	}
	return ix
}

// writeInt 使用 buf 缓冲写入将整数 x 写入 w。
func writeInt(w io.Writer, buf []byte, x int) error {
	binary.PutVarint(buf, int64(x))
	_, err := w.Write(buf[0:binary.MaxVarintLen64])
	return err
}

// readInt 使用 buf 缓冲读取从 r 读取整数 x 并返回 x。
func readInt(r io.Reader, buf []byte) (int64, error) {
	_, err := io.ReadFull(r, buf[0:binary.MaxVarintLen64]) // ok to continue with error
	x, _ := binary.Varint(buf)
	return x, err
}

// writeSlice 将 data[:n] 写入 w 并返回 n。
// 它使用 buf 缓冲写入。
func writeSlice(w io.Writer, buf []byte, data ints) (n int, err error) {
	// 编码尽可能多的元素以适应缓冲区
	p := binary.MaxVarintLen64
	m := data.len()
	for ; n < m && p+binary.MaxVarintLen64 <= len(buf); n++ {
		p += binary.PutUvarint(buf[p:], uint64(data.get(n)))
	}

	// 更新缓冲区大小
	binary.PutVarint(buf, int64(p))

	// 写入缓冲区
	_, err = w.Write(buf[0:p])
	return
}

var errTooBig = errors.New("suffixarray: data too large")

// readSlice 从 r 读取 data[:n] 并返回 n。
// 它使用 buf 缓冲读取。
func readSlice(r io.Reader, buf []byte, data ints) (n int, err error) {
	// 读取缓冲区大小
	var size64 int64
	size64, err = readInt(r, buf)
	if err != nil {
		return
	}
	if int64(int(size64)) != size64 || int(size64) < 0 {
		// 我们无论如何都不会写这么大的块。
		return 0, errTooBig
	}
	size := int(size64)

	// 读取缓冲区（不包括大小）
	if _, err = io.ReadFull(r, buf[binary.MaxVarintLen64:size]); err != nil {
		return
	}

	// 解码缓冲区中存在的尽可能多的元素
	for p := binary.MaxVarintLen64; p < size; n++ {
		x, w := binary.Uvarint(buf[p:])
		data.set(n, int64(x))
		p += w
	}

	return
}

const bufSize = 16 << 10 // 对于 BenchmarkSaveRestore 合理

// Read 从 r 读取索引到 x；x 不能为 nil。
func (x *Index) Read(r io.Reader) error {
	// 所有读取的缓冲区
	buf := make([]byte, bufSize)

	// 读取长度
	n64, err := readInt(r, buf)
	if err != nil {
		return err
	}
	if int64(int(n64)) != n64 || int(n64) < 0 {
		return errTooBig
	}
	n := int(n64)

	// 分配空间
	if 2*n < cap(x.data) || cap(x.data) < n || x.sa.int32 != nil && n > maxData32 || x.sa.int64 != nil && n <= maxData32 {
		// 新数据比现有缓冲区明显更小或更大 - 分配新的
		x.data = make([]byte, n)
		x.sa.int32 = nil
		x.sa.int64 = nil
		if n <= maxData32 {
			x.sa.int32 = make([]int32, n)
		} else {
			x.sa.int64 = make([]int64, n)
		}
	} else {
		// 重用现有缓冲区
		x.data = x.data[0:n]
		x.sa = x.sa.slice(0, n)
	}

	// 读取数据
	if _, err := io.ReadFull(r, x.data); err != nil {
		return err
	}

	// 读取索引
	sa := x.sa
	for sa.len() > 0 {
		n, err := readSlice(r, buf, sa)
		if err != nil {
			return err
		}
		sa = sa.slice(n, sa.len())
	}
	return nil
}

// Write 将索引 x 写入 w。
func (x *Index) Write(w io.Writer) error {
	// 所有写入的缓冲区
	buf := make([]byte, bufSize)

	// 写入长度
	if err := writeInt(w, buf, len(x.data)); err != nil {
		return err
	}

	// 写入数据
	if _, err := w.Write(x.data); err != nil {
		return err
	}

	// 写入索引
	sa := x.sa
	for sa.len() > 0 {
		n, err := writeSlice(w, buf, sa)
		if err != nil {
			return err
		}
		sa = sa.slice(n, sa.len())
	}
	return nil
}

// Bytes 返回创建索引的数据。
// 不得修改。
func (x *Index) Bytes() []byte {
	return x.data
}

func (x *Index) at(i int) []byte {
	return x.data[x.sa.get(i):]
}

// lookupAll 返回索引的匹配区域中的一个切片。
// 运行时是 O(log(N)*len(s))。
func (x *Index) lookupAll(s []byte) ints {
	// 查找匹配的后缀索引范围 [i:j]
	// 找到第一个 s 将成为前缀的索引
	i := sort.Search(x.sa.len(), func(i int) bool { return bytes.Compare(x.at(i), s) >= 0 })
	// 从 i 开始，找到第一个 s 不是前缀的索引
	j := i + sort.Search(x.sa.len()-i, func(j int) bool { return !bytes.HasPrefix(x.at(j+i), s) })
	return x.sa.slice(i, j)
}

// Lookup 返回字节串 s 在索引数据中出现的最多 n 个索引的无序列表。
// 如果 n < 0，返回所有出现位置。
// 如果 s 为空、找不到 s 或 n == 0，结果为 nil。
// Lookup 时间是 O(log(N)*len(s) + len(result))，其中 N 是
// 索引数据的大小。
func (x *Index) Lookup(s []byte, n int) (result []int) {
	if len(s) > 0 && n != 0 {
		matches := x.lookupAll(s)
		count := matches.len()
		if n < 0 || count < n {
			n = count
		}
		// 0 <= n <= count
		if n > 0 {
			result = make([]int, n)
			if matches.int32 != nil {
				for i := range result {
					result[i] = int(matches.int32[i])
				}
			} else {
				for i := range result {
					result[i] = int(matches.int64[i])
				}
			}
		}
	}
	return
}

// FindAllIndex 返回正则表达式 r 的非重叠匹配的排序列表，
// 其中匹配是指定 x.Bytes() 的匹配切片的一对索引。
// 如果 n < 0，所有匹配都按连续顺序返回。
// 否则，最多返回 n 个匹配，它们可能不连续。
// 如果没有匹配或 n == 0，结果为 nil。
func (x *Index) FindAllIndex(r *regexp.Regexp, n int) (result [][]int) {
	// 使用非空字面前缀来确定可能的匹配开始索引
	// 使用 Lookup
	prefix, complete := r.LiteralPrefix()
	lit := []byte(prefix)

	// 最坏的情况：没有字面前缀
	if prefix == "" {
		return r.FindAllIndex(x.data, n)
	}

	// 如果 regexp 是字面量，只需使用 Lookup 并将其结果转换为匹配对
	if complete {
		// Lookup 返回的索引可能属于重叠的匹配。
		// 消除它们后，我们最终可能会有少于 n 个匹配。
		// 如果最后没有足够的，重做搜索
		// 增加了值 n1，但仅当 Lookup 首先返回所有请求的
		// 索引（如果返回的少于那个，那么不能有更多）。
		for n1 := n; ; n1 += 2 * (n - len(result)) /* 溢出 ok */ {
			indices := x.Lookup(lit, n1)
			if len(indices) == 0 {
				return
			}
			slices.Sort(indices)
			pairs := make([]int, 2*len(indices))
			result = make([][]int, len(indices))
			count := 0
			prev := 0
			for _, i := range indices {
				if count == n {
					break
				}
				// 忽略导致重叠匹配的索引
				if prev <= i {
					j := 2 * count
					pairs[j+0] = i
					pairs[j+1] = i + len(lit)
					result[count] = pairs[j : j+2]
					count++
					prev = i + len(lit)
				}
			}
			result = result[0:count]
			if len(result) >= n || len(indices) != n1 {
				// 找到所有匹配或没有机会找到更多
				// （n 和 n1 可以是负数）
				break
			}
		}
		if len(result) == 0 {
			result = nil
		}
		return
	}

	// regexp 有非空字面前缀；Lookup(lit) 计算
	// 可能的完整匹配的索引；使用这些作为
	// 锚定搜索的起点
	// （regexp "^" 匹配输入的开始，不是行的开始）
	r = regexp.MustCompile("^" + r.String()) // 编译因为 r 编译了

	// 与上面循环中的 Lookup 相同的注释也适用于这里
	for n1 := n; ; n1 += 2 * (n - len(result)) /* 溢出 ok */ {
		indices := x.Lookup(lit, n1)
		if len(indices) == 0 {
			return
		}
		slices.Sort(indices)
		result = result[0:0]
		prev := 0
		for _, i := range indices {
			if len(result) == n {
				break
			}
			m := r.FindIndex(x.data[i:]) // 锚定搜索 - 不会溢出
			// 忽略导致重叠匹配的索引
			if m != nil && prev <= i {
				m[0] = i // 正确的 m
				m[1] += i
				result = append(result, m)
				prev = m[1]
			}
		}
		if len(result) >= n || len(indices) != n1 {
			// 找到所有匹配或没有机会找到更多
			// （n 和 n1 可以是负数）
			break
		}
	}
	if len(result) == 0 {
		result = nil
	}
	return
}
