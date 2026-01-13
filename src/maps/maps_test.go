// 版权所有 2021 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package maps

import (
	"math"
	"strconv"
	"testing"
)

var m1 = map[int]int{1: 2, 2: 4, 4: 8, 8: 16}
var m2 = map[int]string{1: "2", 2: "4", 4: "8", 8: "16"}

func TestEqual(t *testing.T) {
	if !Equal(m1, m1) {
		t.Errorf("Equal(%v, %v) = false, want true", m1, m1)
	}
	if Equal(m1, (map[int]int)(nil)) {
		t.Errorf("Equal(%v, nil) = true, want false", m1)
	}
	if Equal((map[int]int)(nil), m1) {
		t.Errorf("Equal(nil, %v) = true, want false", m1)
	}
	if !Equal[map[int]int, map[int]int](nil, nil) {
		t.Error("Equal(nil, nil) = false, want true")
	}
	if ms := map[int]int{1: 2}; Equal(m1, ms) {
		t.Errorf("Equal(%v, %v) = true, want false", m1, ms)
	}

	// 期望比较 NaN 的相等性会失败。
	mf := map[int]float64{1: 0, 2: math.NaN()}
	if Equal(mf, mf) {
		t.Errorf("Equal(%v, %v) = true, want false", mf, mf)
	}
}

// equal 就是简单的 ==。
func equal[T comparable](v1, v2 T) bool {
	return v1 == v2
}

// equalNaN 像 == 一样，但所有 NaN 都相等。
func equalNaN[T comparable](v1, v2 T) bool {
	isNaN := func(f T) bool { return f != f }
	return v1 == v2 || (isNaN(v1) && isNaN(v2))
}

// equalIntStr 比较整数和字符串。
func equalIntStr(v1 int, v2 string) bool {
	return strconv.Itoa(v1) == v2
}

func TestEqualFunc(t *testing.T) {
	if !EqualFunc(m1, m1, equal[int]) {
		t.Errorf("EqualFunc(%v, %v, equal) = false, want true", m1, m1)
	}
	if EqualFunc(m1, (map[int]int)(nil), equal[int]) {
		t.Errorf("EqualFunc(%v, nil, equal) = true, want false", m1)
	}
	if EqualFunc((map[int]int)(nil), m1, equal[int]) {
		t.Errorf("EqualFunc(nil, %v, equal) = true, want false", m1)
	}
	if !EqualFunc[map[int]int, map[int]int](nil, nil, equal[int]) {
		t.Error("EqualFunc(nil, nil, equal) = false, want true")
	}
	if ms := map[int]int{1: 2}; EqualFunc(m1, ms, equal[int]) {
		t.Errorf("EqualFunc(%v, %v, equal) = true, want false", m1, ms)
	}

	// 期望比较 NaN 的相等性会失败。
	mf := map[int]float64{1: 0, 2: math.NaN()}
	if EqualFunc(mf, mf, equal[float64]) {
		t.Errorf("EqualFunc(%v, %v, equal) = true, want false", mf, mf)
	}
	// 但使用 equalNaN 时应该成功。
	if !EqualFunc(mf, mf, equalNaN[float64]) {
		t.Errorf("EqualFunc(%v, %v, equalNaN) = false, want true", mf, mf)
	}

	if !EqualFunc(m1, m2, equalIntStr) {
		t.Errorf("EqualFunc(%v, %v, equalIntStr) = false, want true", m1, m2)
	}
}

func TestClone(t *testing.T) {
	mc := Clone(m1)
	if !Equal(mc, m1) {
		t.Errorf("Clone(%v) = %v, want %v", m1, mc, m1)
	}
	mc[16] = 32
	if Equal(mc, m1) {
		t.Errorf("Equal(%v, %v) = true, want false", mc, m1)
	}
}

func TestCloneNil(t *testing.T) {
	var m1 map[string]int
	mc := Clone(m1)
	if mc != nil {
		t.Errorf("Clone(%v) = %v, want %v", m1, mc, m1)
	}
}

func TestCopy(t *testing.T) {
	mc := Clone(m1)
	Copy(mc, mc)
	if !Equal(mc, m1) {
		t.Errorf("Copy(%v, %v) = %v, want %v", m1, m1, mc, m1)
	}
	Copy(mc, map[int]int{16: 32})
	want := map[int]int{1: 2, 2: 4, 4: 8, 8: 16, 16: 32}
	if !Equal(mc, want) {
		t.Errorf("Copy result = %v, want %v", mc, want)
	}

	type M1 map[int]bool
	type M2 map[int]bool
	Copy(make(M1), make(M2))
}

func TestDeleteFunc(t *testing.T) {
	mc := Clone(m1)
	DeleteFunc(mc, func(int, int) bool { return false })
	if !Equal(mc, m1) {
		t.Errorf("DeleteFunc(%v, true) = %v, want %v", m1, mc, m1)
	}
	DeleteFunc(mc, func(k, v int) bool { return k > 3 })
	want := map[int]int{1: 2, 2: 4}
	if !Equal(mc, want) {
		t.Errorf("DeleteFunc result = %v, want %v", mc, want)
	}
}

var n map[int]int

func BenchmarkMapClone(b *testing.B) {
	var m = make(map[int]int)
	for i := 0; i < 1000000; i++ {
		m[i] = i
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		n = Clone(m)
	}
}

func TestCloneWithDelete(t *testing.T) {
	var m = make(map[int]int)
	for i := 0; i < 32; i++ {
		m[i] = i
	}
	for i := 8; i < 32; i++ {
		delete(m, i)
	}
	m2 := Clone(m)
	if len(m2) != 8 {
		t.Errorf("len2(m2) = %d, want %d", len(m2), 8)
	}
	for i := 0; i < 8; i++ {
		if m2[i] != m[i] {
			t.Errorf("m2[%d] = %d, want %d", i, m2[i], m[i])
		}
	}
}

func TestCloneWithMapAssign(t *testing.T) {
	var m = make(map[int]int)
	const N = 25
	for i := 0; i < N; i++ {
		m[i] = i
	}
	m2 := Clone(m)
	if len(m2) != N {
		t.Errorf("len2(m2) = %d, want %d", len(m2), N)
	}
	for i := 0; i < N; i++ {
		if m2[i] != m[i] {
			t.Errorf("m2[%d] = %d, want %d", i, m2[i], m[i])
		}
	}
}

func TestCloneLarge(t *testing.T) {
	// 参见 issue 64474。
	type K [17]float64 // > 128 字节
	type V [17]float64

	var zero float64
	negZero := -zero

	for tst := 0; tst < 3; tst++ {
		// 用一个键值对初始化 m。
		m := map[K]V{}
		var k1 K
		var v1 V
		m[k1] = v1

		switch tst {
		case 0: // 什么都不做，只是一个 1 项的映射
		case 1:
			// 添加更多条目以构成 2 个桶
			// 已有 1 项
			// 再加 7 项填满 1 个桶
			// 再加 1 项以增长到 2 个桶
			for i := 0; i < 7+1; i++ {
				m[K{float64(i) + 1}] = V{}
			}
		case 2:
			// 在映射增长中途捕获
			// 已有 1 项
			// 再加 7 项填满 1 个桶
			// 再加 5 项（共 13 项）填满 2 个桶
			// 再加 13 项（共 26 项）填满 4 个桶
			// 再加 1 项以启动 4->8 桶的增长
			for i := 0; i < 7+5+13+1; i++ {
				m[K{float64(i) + 1}] = V{}
			}
		}

		// 克隆 m，这应该冻结映射的内容。
		c := Clone(m)

		// 用新的键和值更新 m。
		k2, v2 := k1, v1
		k2[0] = negZero
		v2[0] = 1.0
		m[k2] = v2

		// 确保 c 仍然有旧的键和值。
		for k, v := range c {
			if math.Signbit(k[0]) {
				t.Errorf("tst%d: sign bit of key changed; got %v want %v", tst, k, k1)
			}
			if v != v1 {
				t.Errorf("tst%d: value changed; got %v want %v", tst, v, v1)
			}
		}
	}
}
