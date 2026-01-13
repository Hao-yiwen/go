// 版权所有 2019 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

//go:build !race

package maphash

import (
	"fmt"
	"internal/runtime/maps"
	"internal/testenv"
	"math"
	"math/rand"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// Smhasher 是哈希函数的压力测试。
// https://code.google.com/p/smhasher/
// 此代码是一些 Smhasher 测试移植到 Go 的版本。

// 注意：由于这些测试运行时间较长，
// 目前在 -race 模式下被禁用。

var fixedSeed = MakeSeed()

// 健全性检查。
// 哈希不应依赖于键之外的值。
// 哈希不应依赖于对齐方式。
func TestSmhasherSanity(t *testing.T) {
	t.Parallel()
	r := rand.New(rand.NewSource(1234))
	const REP = 10
	const KEYMAX = 128
	const PAD = 16
	const OFFMAX = 16
	for k := 0; k < REP; k++ {
		for n := 0; n < KEYMAX; n++ {
			for i := 0; i < OFFMAX; i++ {
				var b [KEYMAX + OFFMAX + 2*PAD]byte
				var c [KEYMAX + OFFMAX + 2*PAD]byte
				randBytes(r, b[:])
				randBytes(r, c[:])
				copy(c[PAD+i:PAD+i+n], b[PAD:PAD+n])
				if bytesHash(b[PAD:PAD+n]) != bytesHash(c[PAD+i:PAD+i+n]) {
					t.Errorf("hash depends on bytes outside key")
				}
			}
		}
	}
}

func bytesHash(b []byte) uint64 {
	var h Hash
	h.SetSeed(fixedSeed)
	h.Write(b)
	return h.Sum64()
}
func stringHash(s string) uint64 {
	var h Hash
	h.SetSeed(fixedSeed)
	h.WriteString(s)
	return h.Sum64()
}

const hashSize = 64

func randBytes(r *rand.Rand, b []byte) {
	r.Read(b) // can't fail
}

// hashSet 用于测量哈希冲突的频率。
type hashSet struct {
	list []uint64 // 已添加的哈希列表
}

func newHashSet() *hashSet {
	return &hashSet{list: make([]uint64, 0, 1024)}
}
func (s *hashSet) add(h uint64) {
	s.list = append(s.list, h)
}
func (s *hashSet) addS(x string) {
	s.add(stringHash(x))
}
func (s *hashSet) addB(x []byte) {
	s.add(bytesHash(x))
}
func (s *hashSet) addS_seed(x string, seed Seed) {
	var h Hash
	h.SetSeed(seed)
	h.WriteString(x)
	s.add(h.Sum64())
}
func (s *hashSet) check(t *testing.T) {
	t.Helper()
	list := s.list
	slices.Sort(list)

	collisions := 0
	for i := 1; i < len(list); i++ {
		if list[i] == list[i-1] {
			collisions++
		}
	}
	n := len(list)

	const SLOP = 10.0
	pairs := int64(n) * int64(n-1) / 2
	expected := float64(pairs) / math.Pow(2.0, float64(hashSize))
	stddev := math.Sqrt(expected)
	if float64(collisions) > expected+SLOP*(3*stddev+1) {
		t.Errorf("unexpected number of collisions: got=%d mean=%f stddev=%f", collisions, expected, stddev)
	}
	// 重置以便重用
	s.list = s.list[:0]
}

// 字符串加上添加零必须产生不同的哈希
func TestSmhasherAppendedZeros(t *testing.T) {
	t.Parallel()
	s := "hello" + strings.Repeat("\x00", 256)
	h := newHashSet()
	for i := 0; i <= len(s); i++ {
		h.addS(s[:i])
	}
	h.check(t)
}

// 所有 0-3 字节的字符串都有不同的哈希。
func TestSmhasherSmallKeys(t *testing.T) {
	testenv.ParallelOn64Bit(t)
	h := newHashSet()
	var b [3]byte
	for i := 0; i < 256; i++ {
		b[0] = byte(i)
		h.addB(b[:1])
		for j := 0; j < 256; j++ {
			b[1] = byte(j)
			h.addB(b[:2])
			if !testing.Short() {
				for k := 0; k < 256; k++ {
					b[2] = byte(k)
					h.addB(b[:3])
				}
			}
		}
	}
	h.check(t)
}

// 不同长度的全零字符串具有不同的哈希。
func TestSmhasherZeros(t *testing.T) {
	t.Parallel()
	N := 256 * 1024
	if testing.Short() {
		N = 1024
	}
	h := newHashSet()
	b := make([]byte, N)
	for i := 0; i <= N; i++ {
		h.addB(b[:i])
	}
	h.check(t)
}

// 最多有两个非零字节的字符串都有不同的哈希。
func TestSmhasherTwoNonzero(t *testing.T) {
	if runtime.GOARCH == "wasm" {
		t.Skip("Too slow on wasm")
	}
	if testing.Short() {
		t.Skip("Skipping in short mode")
	}
	testenv.ParallelOn64Bit(t)
	h := newHashSet()
	for n := 2; n <= 16; n++ {
		twoNonZero(h, n)
	}
	h.check(t)
}
func twoNonZero(h *hashSet, n int) {
	b := make([]byte, n)

	// 全零
	h.addB(b)

	// 一个非零字节
	for i := 0; i < n; i++ {
		for x := 1; x < 256; x++ {
			b[i] = byte(x)
			h.addB(b)
			b[i] = 0
		}
	}

	// 两个非零字节
	for i := 0; i < n; i++ {
		for x := 1; x < 256; x++ {
			b[i] = byte(x)
			for j := i + 1; j < n; j++ {
				for y := 1; y < 256; y++ {
					b[j] = byte(y)
					h.addB(b)
					b[j] = 0
				}
			}
			b[i] = 0
		}
	}
}

// 测试具有重复的字符串，如 "abcdabcdabcdabcd..."
func TestSmhasherCyclic(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping in short mode")
	}
	t.Parallel()
	r := rand.New(rand.NewSource(1234))
	const REPEAT = 8
	const N = 1000000
	h := newHashSet()
	for n := 4; n <= 12; n++ {
		b := make([]byte, REPEAT*n)
		for i := 0; i < N; i++ {
			b[0] = byte(i * 79 % 97)
			b[1] = byte(i * 43 % 137)
			b[2] = byte(i * 151 % 197)
			b[3] = byte(i * 199 % 251)
			randBytes(r, b[4:n])
			for j := n; j < n*REPEAT; j++ {
				b[j] = b[j-n]
			}
			h.addB(b)
		}
		h.check(t)
	}
}

// 测试只有少数位被设置的字符串
func TestSmhasherSparse(t *testing.T) {
	if runtime.GOARCH == "wasm" {
		t.Skip("Too slow on wasm")
	}
	if testing.Short() {
		t.Skip("Skipping in short mode")
	}
	t.Parallel()
	h := newHashSet()
	sparse(t, h, 32, 6)
	sparse(t, h, 40, 6)
	sparse(t, h, 48, 5)
	sparse(t, h, 56, 5)
	sparse(t, h, 64, 5)
	sparse(t, h, 96, 4)
	sparse(t, h, 256, 3)
	sparse(t, h, 2048, 2)
}
func sparse(t *testing.T, h *hashSet, n int, k int) {
	t.Helper()
	b := make([]byte, n/8)
	setbits(h, b, 0, k)
	h.check(t)
}

// 在索引 i 及更大位置设置最多 k 位
func setbits(h *hashSet, b []byte, i int, k int) {
	h.addB(b)
	if k == 0 {
		return
	}
	for j := i; j < len(b)*8; j++ {
		b[j/8] |= byte(1 << uint(j&7))
		setbits(h, b, j+1, k-1)
		b[j/8] &= byte(^(1 << uint(j&7)))
	}
}

// 测试集合 s 中 n 个块的所有可能组合。
// "permutation" 在这里是一个不恰当的名称，但这是 Smhasher 使用的名称。
func TestSmhasherPermutation(t *testing.T) {
	if runtime.GOARCH == "wasm" {
		t.Skip("Too slow on wasm")
	}
	if testing.Short() {
		t.Skip("Skipping in short mode")
	}
	testenv.ParallelOn64Bit(t)
	h := newHashSet()
	permutation(t, h, []uint32{0, 1, 2, 3, 4, 5, 6, 7}, 8)
	permutation(t, h, []uint32{0, 1 << 29, 2 << 29, 3 << 29, 4 << 29, 5 << 29, 6 << 29, 7 << 29}, 8)
	permutation(t, h, []uint32{0, 1}, 20)
	permutation(t, h, []uint32{0, 1 << 31}, 20)
	permutation(t, h, []uint32{0, 1, 2, 3, 4, 5, 6, 7, 1 << 29, 2 << 29, 3 << 29, 4 << 29, 5 << 29, 6 << 29, 7 << 29}, 6)
}
func permutation(t *testing.T, h *hashSet, s []uint32, n int) {
	t.Helper()
	b := make([]byte, n*4)
	genPerm(h, b, s, 0)
	h.check(t)
}
func genPerm(h *hashSet, b []byte, s []uint32, n int) {
	h.addB(b[:n])
	if n == len(b) {
		return
	}
	for _, v := range s {
		b[n] = byte(v)
		b[n+1] = byte(v >> 8)
		b[n+2] = byte(v >> 16)
		b[n+3] = byte(v >> 24)
		genPerm(h, b, s, n+4)
	}
}

type key interface {
	clear()              // 将所有位设置为 0
	random(r *rand.Rand) // 将键设置为随机值
	bits() int           // 键有多少位
	flipBit(i int)       // 翻转键的第 i 位
	hash() uint64        // 对键进行哈希
	name() string        // 用于错误报告
}

type bytesKey struct {
	b []byte
}

func (k *bytesKey) clear() {
	clear(k.b)
}
func (k *bytesKey) random(r *rand.Rand) {
	randBytes(r, k.b)
}
func (k *bytesKey) bits() int {
	return len(k.b) * 8
}
func (k *bytesKey) flipBit(i int) {
	k.b[i>>3] ^= byte(1 << uint(i&7))
}
func (k *bytesKey) hash() uint64 {
	return bytesHash(k.b)
}
func (k *bytesKey) name() string {
	return fmt.Sprintf("bytes%d", len(k.b))
}

// 翻转键的单个位应该以 50% 的概率翻转每个输出位。
func TestSmhasherAvalanche(t *testing.T) {
	if runtime.GOARCH == "wasm" {
		t.Skip("Too slow on wasm")
	}
	if testing.Short() {
		t.Skip("Skipping in short mode")
	}
	t.Parallel()
	avalancheTest1(t, &bytesKey{make([]byte, 2)})
	avalancheTest1(t, &bytesKey{make([]byte, 4)})
	avalancheTest1(t, &bytesKey{make([]byte, 8)})
	avalancheTest1(t, &bytesKey{make([]byte, 16)})
	avalancheTest1(t, &bytesKey{make([]byte, 32)})
	avalancheTest1(t, &bytesKey{make([]byte, 200)})
}
func avalancheTest1(t *testing.T, k key) {
	t.Helper()
	const REP = 100000
	r := rand.New(rand.NewSource(1234))
	n := k.bits()

	// grid[i][j] 是翻转输入位 i
	// 是否影响输出位 j 的计数。
	grid := make([][hashSize]int, n)

	for z := 0; z < REP; z++ {
		// 选择一个随机键，对其进行哈希
		k.random(r)
		h := k.hash()

		// 翻转每个位，哈希并比较结果
		for i := 0; i < n; i++ {
			k.flipBit(i)
			d := h ^ k.hash()
			k.flipBit(i)

			// 记录该位翻转的效果
			g := &grid[i]
			for j := 0; j < hashSize; j++ {
				g[j] += int(d & 1)
				d >>= 1
			}
		}
	}

	// 网格中的每个条目应该大约是 REP/2。
	// 更精确地说，我们进行了 N = k.bits() * hashSize 次实验，
	// 每次是 REP 次抛硬币的总和。我们想要找到抛硬币总和的边界，
	// 使得真正随机的实验有 99% 的概率所有总和都在这些边界内。
	N := n * hashSize
	var c float64
	// 找到 c 使得 Prob(mean-c*stddev < x < mean+c*stddev)^N > .9999
	for c = 0.0; math.Pow(math.Erf(c/math.Sqrt(2)), float64(N)) < .9999; c += .1 {
	}
	c *= 11.0 // 允许的偏差：40% 到 60% - 我们不需要完全随机
	mean := .5 * REP
	stddev := .5 * math.Sqrt(REP)
	low := int(mean - c*stddev)
	high := int(mean + c*stddev)
	for i := 0; i < n; i++ {
		for j := 0; j < hashSize; j++ {
			x := grid[i][j]
			if x < low || x > high {
				t.Errorf("bad bias for %s bit %d -> bit %d: %d/%d\n", k.name(), i, j, x, REP)
			}
		}
	}
}

// 一组不同键的所有位旋转
func TestSmhasherWindowed(t *testing.T) {
	t.Parallel()
	windowed(t, &bytesKey{make([]byte, 128)})
}
func windowed(t *testing.T, k key) {
	if runtime.GOARCH == "wasm" {
		t.Skip("Too slow on wasm")
	}
	if testing.Short() {
		t.Skip("Skipping in short mode")
	}
	const BITS = 16

	h := newHashSet()
	for r := 0; r < k.bits(); r++ {
		for i := 0; i < 1<<BITS; i++ {
			k.clear()
			for j := 0; j < BITS; j++ {
				if i>>uint(j)&1 != 0 {
					k.flipBit((j + r) % k.bits())
				}
			}
			h.add(k.hash())
		}
		h.check(t)
	}
}

// 所有形式为 prefix + [A-Za-z0-9]*N + suffix 的键。
func TestSmhasherText(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping in short mode")
	}
	t.Parallel()
	h := newHashSet()
	text(t, h, "Foo", "Bar")
	text(t, h, "FooBar", "")
	text(t, h, "", "FooBar")
}
func text(t *testing.T, h *hashSet, prefix, suffix string) {
	t.Helper()
	const N = 4
	const S = "ABCDEFGHIJKLMNOPQRSTabcdefghijklmnopqrst0123456789"
	const L = len(S)
	b := make([]byte, len(prefix)+N+len(suffix))
	copy(b, prefix)
	copy(b[len(prefix)+N:], suffix)
	c := b[len(prefix):]
	for i := 0; i < L; i++ {
		c[0] = S[i]
		for j := 0; j < L; j++ {
			c[1] = S[j]
			for k := 0; k < L; k++ {
				c[2] = S[k]
				for x := 0; x < L; x++ {
					c[3] = S[x]
					h.addB(b)
				}
			}
		}
	}
	h.check(t)
}

// 确保不同的种子值生成不同的哈希。
func TestSmhasherSeed(t *testing.T) {
	if !maps.Use64BitHash {
		t.Skip("32-bit platforms don't have ideal seed-input distributions (see issue 33988)")
	}
	t.Parallel()
	h := newHashSet()
	const N = 100000
	s := "hello"
	for i := 0; i < N; i++ {
		h.addS_seed(s, Seed{s: uint64(i + 1)})
		h.addS_seed(s, Seed{s: uint64(i+1) << 32}) // 确保使用高位
	}
	h.check(t)
}
