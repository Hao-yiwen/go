// 版权所有 2009 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// 此文件实现有符号多精度整数。

package big

import (
	"fmt"
	"io"
	"math/rand"
	"strings"
)

// Int 表示一个有符号的多精度整数。
// Int 的零值表示值 0。
//
// 操作总是采用指针参数 (*Int) 而不是
// Int 值，每个唯一的 Int 值都需要
// 它自己独有的 *Int 指针。要"复制" Int 值，
// 现有的（或新分配的）Int 必须使用
// [Int.Set] 方法设置为新值；不支持浅复制
// Int，这可能导致错误。
//
// 注意方法可能会通过时间侧通道泄漏 Int 的值。
// 由于这个原因以及实现的范围和复杂性，
// Int 不太适合实现密码学操作。
// 标准库避免向
// 攻击者控制的输入公开非平凡的 Int 方法，
// 确定 math/big 中的错误是否被视为
// 安全漏洞可能取决于对标准库的影响。
type Int struct {
	neg bool // 符号
	abs nat  // 整数的绝对值
}

var intOne = &Int{false, natOne}

// Sign 返回：
//   - 如果 x < 0，返回 -1;
//   - 如果 x == 0，返回 0;
//   - 如果 x > 0，返回 +1。
func (x *Int) Sign() int {
	// 此函数用于密码学操作。它不能通过
	// 侧通道泄漏 Int 的符号和位大小之外的任何信息。任何
	// 更改都必须由安全专家审查。
	if len(x.abs) == 0 {
		return 0
	}
	if x.neg {
		return -1
	}
	return 1
}

// SetInt64 将 z 设置为 x 并返回 z。
func (z *Int) SetInt64(x int64) *Int {
	neg := false
	if x < 0 {
		neg = true
		x = -x
	}
	z.abs = z.abs.setUint64(uint64(x))
	z.neg = neg
	return z
}

// SetUint64 将 z 设置为 x 并返回 z。
func (z *Int) SetUint64(x uint64) *Int {
	z.abs = z.abs.setUint64(x)
	z.neg = false
	return z
}

// NewInt 分配并返回一个新的 [Int]，设置为 x。
func NewInt(x int64) *Int {
	// 此代码的排列方式使其内联后可内联化
	// 并在内联时产生零分配。参见 issue 29951。
	u := uint64(x)
	if x < 0 {
		u = -u
	}
	var abs []Word
	if x == 0 {
	} else if _W == 32 && u>>32 != 0 {
		abs = []Word{Word(u), Word(u >> 32)}
	} else {
		abs = []Word{Word(u)}
	}
	return &Int{neg: x < 0, abs: abs}
}

// Set 将 z 设置为 x 并返回 z。
func (z *Int) Set(x *Int) *Int {
	if z != x {
		z.abs = z.abs.set(x.abs)
		z.neg = x.neg
	}
	return z
}

// Bits 通过以小端 [Word] 切片的形式返回其
// 绝对值来提供对 x 的原始（未检查但快速）访问。结果和 x 共享
// 相同的底层数组。
// Bits 旨在支持在此包之外实现缺失的低级 [Int]
// 功能；否则应避免使用。
func (x *Int) Bits() []Word {
	// 此函数用于密码学操作。它不能通过
	// 侧通道泄漏 Int 的符号和位大小之外的任何信息。任何
	// 更改都必须由安全专家审查。
	return x.abs
}

// SetBits 通过将其值设置为 abs（解释为小端 [Word] 切片）
// 并返回 z 来提供对 z 的原始（未检查但快速）访问。结果和 abs 共享
// 相同的底层数组。
// SetBits 旨在支持在此包之外实现缺失的低级 [Int]
// 功能；否则应避免使用。
func (z *Int) SetBits(abs []Word) *Int {
	z.abs = nat(abs).norm()
	z.neg = false
	return z
}

// Abs 将 z 设置为 |x|（x 的绝对值）并返回 z。
func (z *Int) Abs(x *Int) *Int {
	z.Set(x)
	z.neg = false
	return z
}

// Neg 将 z 设置为 -x 并返回 z。
func (z *Int) Neg(x *Int) *Int {
	z.Set(x)
	z.neg = len(z.abs) > 0 && !z.neg // 0 has no sign
	return z
}

// Add 将 z 设置为和 x+y 并返回 z。
func (z *Int) Add(x, y *Int) *Int {
	neg := x.neg
	if x.neg == y.neg {
		// x + y == x + y
		// (-x) + (-y) == -(x + y)
		z.abs = z.abs.add(x.abs, y.abs)
	} else {
		// x + (-y) == x - y == -(y - x)
		// (-x) + y == y - x == -(x - y)
		if x.abs.cmp(y.abs) >= 0 {
			z.abs = z.abs.sub(x.abs, y.abs)
		} else {
			neg = !neg
			z.abs = z.abs.sub(y.abs, x.abs)
		}
	}
	z.neg = len(z.abs) > 0 && neg // 0 has no sign
	return z
}

// Sub 将 z 设置为差 x-y 并返回 z。
func (z *Int) Sub(x, y *Int) *Int {
	neg := x.neg
	if x.neg != y.neg {
		// x - (-y) == x + y
		// (-x) - y == -(x + y)
		z.abs = z.abs.add(x.abs, y.abs)
	} else {
		// x - y == x - y == -(y - x)
		// (-x) - (-y) == y - x == -(x - y)
		if x.abs.cmp(y.abs) >= 0 {
			z.abs = z.abs.sub(x.abs, y.abs)
		} else {
			neg = !neg
			z.abs = z.abs.sub(y.abs, x.abs)
		}
	}
	z.neg = len(z.abs) > 0 && neg // 0 has no sign
	return z
}

// Mul 将 z 设置为乘积 x*y 并返回 z。
func (z *Int) Mul(x, y *Int) *Int {
	z.mul(nil, x, y)
	return z
}

// mul 类似于 Mul 但需要一个明确的栈来使用，供内部使用。
// 它不返回 *Int，因为这样做会导致在 natmul.go 中使用的
// 栈分配的 Int 逃逸到堆（即使结果未被使用）。
func (z *Int) mul(stk *stack, x, y *Int) {
	// x * y == x * y
	// x * (-y) == -(x * y)
	// (-x) * y == -(x * y)
	// (-x) * (-y) == x * y
	if x == y {
		z.abs = z.abs.sqr(stk, x.abs)
		z.neg = false
		return
	}
	z.abs = z.abs.mul(stk, x.abs, y.abs)
	z.neg = len(z.abs) > 0 && x.neg != y.neg // 0 has no sign
}

// MulRange 将 z 设置为范围 [a, b] 中所有整数的乘积
// （包括两端）并返回 z。
// 如果 a > b（空范围），结果是 1。
func (z *Int) MulRange(a, b int64) *Int {
	switch {
	case a > b:
		return z.SetInt64(1) // 空范围
	case a <= 0 && b >= 0:
		return z.SetInt64(0) // 范围包括 0
	}
	// a <= b && (b < 0 || a > 0)

	neg := false
	if a < 0 {
		neg = (b-a)&1 == 0
		a, b = -b, -a
	}

	z.abs = z.abs.mulRange(nil, uint64(a), uint64(b))
	z.neg = neg
	return z
}

// Binomial 将 z 设置为二项式系数 C(n, k) 并返回 z。
func (z *Int) Binomial(n, k int64) *Int {
	if k > n {
		return z.SetInt64(0)
	}
	// 通过减少 k 来减少乘法次数
	if k > n-k {
		k = n - k // C(n, k) == C(n, n-k)
	}
	// C(n, k) == n * (n-1) * ... * (n-k+1) / k * (k-1) * ... * 1
	//         == n * (n-1) * ... * (n-k+1) / 1 * (1+1) * ... * k
	//
	// 使用乘法公式在每一步产生更小的值，
	// 需要更少的分配和计算：
	//
	// z = 1
	// for i := 0; i < k; i = i+1 {
	//     z *= n-i
	//     z /= i+1
	// }
	//
	// 最后为了避免每个循环中计算 i+1 两次：
	//
	// z = 1
	// i := 0
	// for i < k {
	//     z *= n-i
	//     i++
	//     z /= i
	// }
	var N, K, i, t Int
	N.SetInt64(n)
	K.SetInt64(k)
	z.Set(intOne)
	for i.Cmp(&K) < 0 {
		z.Mul(z, t.Sub(&N, &i))
		i.Add(&i, intOne)
		z.Quo(z, &i)
	}
	return z
}

// Quo 将 z 设置为商 x/y（y != 0）并返回 z。
// 如果 y == 0，将发生除以零的运行时错误。
// Quo 实现截断除法（如 Go）；有关更多详细信息，请参见 [Int.QuoRem]。
func (z *Int) Quo(x, y *Int) *Int {
	z.abs, _ = z.abs.div(nil, nil, x.abs, y.abs)
	z.neg = len(z.abs) > 0 && x.neg != y.neg // 0 has no sign
	return z
}

// Rem 将 z 设置为余数 x%y（y != 0）并返回 z。
// 如果 y == 0，将发生除以零的运行时错误。
// Rem 实现截断模（如 Go）；有关更多详细信息，请参见 [Int.QuoRem]。
func (z *Int) Rem(x, y *Int) *Int {
	_, z.abs = nat(nil).div(nil, z.abs, x.abs, y.abs)
	z.neg = len(z.abs) > 0 && x.neg // 0 has no sign
	return z
}

// QuoRem 将 z 设置为商 x/y，将 r 设置为余数 x%y
// 并为 y != 0 返回对 (z, r)。
// 如果 y == 0，将发生除以零的运行时错误。
//
// QuoRem 实现 T-除法和模（如 Go）：
//
//	q = x/y      其中结果被截断为零
//	r = x - y*q
//
// （参见 Daan Leijen 的"计算机科学家的除法和模"。）
// 有关欧几里得除法和模（不同于 Go），请参见 [Int.DivMod]。
func (z *Int) QuoRem(x, y, r *Int) (*Int, *Int) {
	z.abs, r.abs = z.abs.div(nil, r.abs, x.abs, y.abs)
	z.neg, r.neg = len(z.abs) > 0 && x.neg != y.neg, len(r.abs) > 0 && x.neg // 0 has no sign
	return z, r
}

// Div 将 z 设置为商 x/y（y != 0）并返回 z。
// 如果 y == 0，将发生除以零的运行时错误。
// Div 实现欧几里得除法（不同于 Go）；有关更多详细信息，请参见 [Int.DivMod]。
func (z *Int) Div(x, y *Int) *Int {
	y_neg := y.neg // z may be an alias for y
	var r Int
	z.QuoRem(x, y, &r)
	if r.neg {
		if y_neg {
			z.Add(z, intOne)
		} else {
			z.Sub(z, intOne)
		}
	}
	return z
}

// Mod 将 z 设置为模 x%y（y != 0）并返回 z。
// 如果 y == 0，将发生除以零的运行时错误。
// Mod 实现欧几里得模（不同于 Go）；有关更多详细信息，请参见 [Int.DivMod]。
func (z *Int) Mod(x, y *Int) *Int {
	y0 := y // 保存 y
	if z == y || alias(z.abs, y.abs) {
		y0 = new(Int).Set(y)
	}
	var q Int
	q.QuoRem(x, y, z)
	if z.neg {
		if y0.neg {
			z.Sub(z, y0)
		} else {
			z.Add(z, y0)
		}
	}
	return z
}

// DivMod 将 z 设置为商 x div y，将 m 设置为模 x mod y
// 并为 y != 0 返回对 (z, m)。
// 如果 y == 0，将发生除以零的运行时错误。
//
// DivMod 实现欧几里得除法和模（不同于 Go）：
//
//	q = x div y  使得
//	m = x - y*q  其中 0 <= m < |y|
//
// （参见 Raymond T. Boute 的"函数 div 和 mod 的欧几里得定义"。
// ACM Transactions on Programming Languages and
// Systems (TOPLAS), 14(2):127-144, New York, NY, USA, 4/1992.
// ACM press.）
// 有关 T-除法和模（如 Go），请参见 [Int.QuoRem]。
func (z *Int) DivMod(x, y, m *Int) (*Int, *Int) {
	y0 := y // 保存 y
	if z == y || alias(z.abs, y.abs) {
		y0 = new(Int).Set(y)
	}
	z.QuoRem(x, y, m)
	if m.neg {
		if y0.neg {
			z.Add(z, intOne)
			m.Sub(m, y0)
		} else {
			z.Sub(z, intOne)
			m.Add(m, y0)
		}
	}
	return z, m
}

// Cmp 比较 x 和 y 并返回：
//   - 如果 x < y，返回 -1；
//   - 如果 x == y，返回 0；
//   - 如果 x > y，返回 +1。
func (x *Int) Cmp(y *Int) (r int) {
	// x cmp y == x cmp y
	// x cmp (-y) == x
	// (-x) cmp y == y
	// (-x) cmp (-y) == -(x cmp y)
	switch {
	case x == y:
		// 无需做任何事情
	case x.neg == y.neg:
		r = x.abs.cmp(y.abs)
		if x.neg {
			r = -r
		}
	case x.neg:
		r = -1
	default:
		r = 1
	}
	return
}

// CmpAbs 比较 x 和 y 的绝对值并返回：
//   - 如果 |x| < |y|，返回 -1；
//   - 如果 |x| == |y|，返回 0；
//   - 如果 |x| > |y|，返回 +1。
func (x *Int) CmpAbs(y *Int) int {
	return x.abs.cmp(y.abs)
}

// low32 返回 x 的最低有效 32 位。
func low32(x nat) uint32 {
	if len(x) == 0 {
		return 0
	}
	return uint32(x[0])
}

// low64 返回 x 的最低有效 64 位。
func low64(x nat) uint64 {
	if len(x) == 0 {
		return 0
	}
	v := uint64(x[0])
	if _W == 32 && len(x) > 1 {
		return uint64(x[1])<<32 | v
	}
	return v
}

// Int64 返回 x 的 int64 表示。
// 如果 x 无法在 int64 中表示，结果未定义。
func (x *Int) Int64() int64 {
	v := int64(low64(x.abs))
	if x.neg {
		v = -v
	}
	return v
}

// Uint64 返回 x 的 uint64 表示。
// 如果 x 无法在 uint64 中表示，结果未定义。
func (x *Int) Uint64() uint64 {
	return low64(x.abs)
}

// IsInt64 报告 x 是否可以表示为 int64。
func (x *Int) IsInt64() bool {
	if len(x.abs) <= 64/_W {
		w := int64(low64(x.abs))
		return w >= 0 || x.neg && w == -w
	}
	return false
}

// IsUint64 报告 x 是否可以表示为 uint64。
func (x *Int) IsUint64() bool {
	return !x.neg && len(x.abs) <= 64/_W
}

// Float64 返回最接近 x 的 float64 值，
// 以及发生的任何舍入的指示。
func (x *Int) Float64() (float64, Accuracy) {
	n := x.abs.bitLen() // NB: still uses slow crypto impl!
	if n == 0 {
		return 0.0, Exact
	}

	// 快速路径：不超过 53 个有效位。
	if n <= 53 || n < 64 && n-int(x.abs.trailingZeroBits()) <= 53 {
		f := float64(low64(x.abs))
		if x.neg {
			f = -f
		}
		return f, Exact
	}

	return new(Float).SetInt(x).Float64()
}

// SetString 将 z 设置为 s 的值，在给定的基数中解释，
// 并返回 z 和一个布尔值，指示成功。整个字符串
// （不仅仅是前缀）必须有效才能成功。如果 SetString 失败，
// z 的值未定义，但返回的值为 nil。
//
// 基数参数必须是 0 或介于 2 和 [MaxBase] 之间的值。
// 对于基数 0，数字前缀确定实际基数：前缀为
// "0b" 或 "0B" 选择基数 2，"0"、"0o" 或 "0O" 选择基数 8，
// "0x" 或 "0X" 选择基数 16。否则，选定的基数为 10
// 且不接受前缀。
//
// 对于基数 <= 36，大小写字母被视为相同：
// 字母 'a' 到 'z' 和 'A' 到 'Z' 代表数字值 10 到 35。
// 对于基数 > 36，大写字母 'A' 到 'Z' 代表数字
// 值 36 到 61。
//
// 对于基数 0，下划线字符 "_" 可能出现在基数
// 前缀和相邻数字之间，以及连续数字之间；这样的
// 下划线不会改变数字的值。
// 如果没有其他错误，下划线的不正确放置会被报告为错误。如果 base != 0，下划线无法识别
// 并表现为任何其他无效数字的字符。
func (z *Int) SetString(s string, base int) (*Int, bool) {
	return z.setFromScanner(strings.NewReader(s), base)
}

// setFromScanner 实现给定 io.ByteScanner 的 SetString。
// 有关文档，请参见 SetString 的注释。
func (z *Int) setFromScanner(r io.ByteScanner, base int) (*Int, bool) {
	if _, _, err := z.scan(r, base); err != nil {
		return nil, false
	}
	// 整个内容必须已被消耗
	if _, err := r.ReadByte(); err != io.EOF {
		return nil, false
	}
	return z, true // err == io.EOF => 扫描消耗了 r 的所有内容
}

// SetBytes 将 buf 解释为大端无符号
// 整数的字节，将 z 设置为该值，并返回 z。
func (z *Int) SetBytes(buf []byte) *Int {
	z.abs = z.abs.setBytes(buf)
	z.neg = false
	return z
}

// Bytes 将 x 的绝对值返回为大端字节切片。
//
// 要使用固定长度切片或预分配的切片，请使用 [Int.FillBytes]。
func (x *Int) Bytes() []byte {
	// 此函数用于密码学操作。它不能通过
	// 侧通道泄漏 Int 的符号和位大小之外的任何信息。任何
	// 更改都必须由安全专家审查。
	buf := make([]byte, len(x.abs)*_S)
	return buf[x.abs.bytes(buf):]
}

// FillBytes 将 buf 设置为 x 的绝对值，将其存储为零扩展的
// 大端字节切片，并返回 buf。
//
// 如果 x 的绝对值不适合 buf，FillBytes 将发生恐慌。
func (x *Int) FillBytes(buf []byte) []byte {
	// 清除整个缓冲区。
	clear(buf)
	x.abs.bytes(buf)
	return buf
}

// BitLen 返回 x 绝对值以位为单位的长度。
// 0 的位长度是 0。
func (x *Int) BitLen() int {
	// 此函数用于密码学操作。它不能通过
	// 侧通道泄漏 Int 的符号和位大小之外的任何信息。任何
	// 更改都必须由安全专家审查。
	return x.abs.bitLen()
}

// TrailingZeroBits 返回 |x| 的连续最低有效零
// 位的数量。
func (x *Int) TrailingZeroBits() uint {
	return x.abs.trailingZeroBits()
}

// Exp 设置 z = x**y mod |m|（即 m 的符号被忽略），并返回 z。
// 如果 m == nil 或 m == 0，z = x**y，除非 y <= 0 则 z = 1。如果 m != 0、y < 0，
// 且 x 和 m 不互质，z 保持不变并返回 nil。
//
// 特定大小输入的模指数不是
// 密码学恒定时间操作。
func (z *Int) Exp(x, y, m *Int) *Int {
	return z.exp(x, y, m, false)
}

func (z *Int) expSlow(x, y, m *Int) *Int {
	return z.exp(x, y, m, true)
}

func (z *Int) exp(x, y, m *Int, slow bool) *Int {
	// 参见 Knuth，第 2 卷，第 4.6.3 节。
	xWords := x.abs
	if y.neg {
		if m == nil || len(m.abs) == 0 {
			return z.SetInt64(1)
		}
		// 对于 y < 0: x**y mod m == (x**(-1))**|y| mod m
		inverse := new(Int).ModInverse(x, m)
		if inverse == nil {
			return nil
		}
		xWords = inverse.abs
	}
	yWords := y.abs

	var mWords nat
	if m != nil {
		if z == m || alias(z.abs, m.abs) {
			m = new(Int).Set(m)
		}
		mWords = m.abs // 对于 m == 0，m.abs 可能为 nil
	}

	z.abs = z.abs.expNN(nil, xWords, yWords, mWords, slow)
	z.neg = len(z.abs) > 0 && x.neg && len(yWords) > 0 && yWords[0]&1 == 1 // 0 没有符号
	if z.neg && len(mWords) > 0 {
		// 使模的结果为正
		z.abs = z.abs.sub(mWords, z.abs) // z == x**y mod |m| && 0 <= z < |m|
		z.neg = false
	}

	return z
}

// GCD 将 z 设置为 a 和 b 的最大公约数并返回 z。
// 如果 x 或 y 不为 nil，GCD 设置它们的值使得 z = a*x + b*y。
//
// a 和 b 可以是正数、零或负数。（在 Go 1.14 之前，两者都必须
// 大于 0。）无论 a 和 b 的符号如何，z 总是 >= 0。
//
// 如果 a == b == 0，GCD 设置 z = x = y = 0。
//
// 如果 a == 0 且 b != 0，GCD 设置 z = |b|、x = 0、y = sign(b) * 1。
//
// 如果 a != 0 且 b == 0，GCD 设置 z = |a|、x = sign(a) * 1、y = 0。
func (z *Int) GCD(x, y, a, b *Int) *Int {
	if len(a.abs) == 0 || len(b.abs) == 0 {
		lenA, lenB, negA, negB := len(a.abs), len(b.abs), a.neg, b.neg
		if lenA == 0 {
			z.Set(b)
		} else {
			z.Set(a)
		}
		z.neg = false
		if x != nil {
			if lenA == 0 {
				x.SetUint64(0)
			} else {
				x.SetUint64(1)
				x.neg = negA
			}
		}
		if y != nil {
			if lenB == 0 {
				y.SetUint64(0)
			} else {
				y.SetUint64(1)
				y.neg = negB
			}
		}
		return z
	}

	return z.lehmerGCD(x, y, a, b)
}

// lehmerSimulate 尝试使用 A 和 B 的前导数字模拟多个欧几里得更新步骤。
// 它返回 u0、u1、v0、v1，使得 A 和 B 可以更新为：
//
//	A = u0*A + v0*B
//	B = u1*A + v1*B
//
// 要求：A >= B 且 len(B.abs) >= 2
// 由于我们使用完整字来计算以避免溢出，
// 我们使用 'even' 来跟踪结果的符号。
// 对于偶数迭代：u0, v1 >= 0 && u1, v0 <= 0
// 对于奇数迭代：u0, v1 <= 0 && u1, v0 >= 0
func lehmerSimulate(A, B *Int) (u0, u1, v0, v1 Word, even bool) {
	// 初始化数字
	var a1, a2, u2, v2 Word

	m := len(B.abs) // m >= 2
	n := len(A.abs) // n >= m >= 2

	// 从 A 和 B 中提取最高位的 Word
	h := nlz(A.abs[n-1])
	a1 = A.abs[n-1]<<h | A.abs[n-2]>>(_W-h)
	// 如果长度不同，B 在高位可能有隐含的零字
	switch {
	case n == m:
		a2 = B.abs[n-1]<<h | B.abs[n-2]>>(_W-h)
	case n == m+1:
		a2 = B.abs[n-2] >> (_W - h)
	default:
		a2 = 0
	}

	// 由于我们使用完整字来计算以避免溢出，
	// 我们使用 'even' 来跟踪结果的符号。
	// 对于偶数迭代：u0, v1 >= 0 && u1, v0 <= 0
	// 对于奇数迭代：u0, v1 <= 0 && u1, v0 >= 0
	// 第一次迭代从 k=1（奇数）开始。
	even = false
	// 跟踪结果的变量
	u0, u1, u2 = 0, 1, 0
	v0, v1, v2 = 0, 0, 1

	// 使用 Collins 停止条件计算商和结果。
	// 请注意，在计算余数时不可能出现 Word 溢出
	// 序列和结果，因为结果大小由输入大小限制。
	// 有关详细信息，请参见 Jebelean 的第 4.2 节。
	for a2 >= v2 && a1-a2 >= v1+v2 {
		q, r := a1/a2, a1%a2
		a1, a2 = a2, r
		u0, u1, u2 = u1, u2, u1+q*u2
		v0, v1, v2 = v1, v2, v1+q*v2
		even = !even
	}
	return
}

// lehmerUpdate 更新输入 A 和 B 使得：
//
//	A = u0*A + v0*B
//	B = u1*A + v1*B
//
// 其中 u0、u1、v0、v1 的符号由 even 给出
// 对于 even == true：u0, v1 >= 0 && u1, v0 <= 0
// 对于 even == false：u0, v1 <= 0 && u1, v0 >= 0
// q、r、s、t 是临时变量，用于避免乘法中的分配。
func lehmerUpdate(A, B, q, r *Int, u0, u1, v0, v1 Word, even bool) {
	mulW(q, B, even, v0)
	mulW(r, A, even, u1)
	mulW(A, A, !even, u0)
	mulW(B, B, !even, v1)
	A.Add(A, q)
	B.Add(B, r)
}

// mulW 设置 z = x * (-?)w
// 其中当 neg 为 true 时存在负号。
func mulW(z, x *Int, neg bool, w Word) {
	z.abs = z.abs.mulAddWW(x.abs, w, 0)
	z.neg = x.neg != neg
}

// euclidUpdate 执行欧几里得 GCD 算法的单个步骤
// 如果 extended 为 true，它还会更新结果 Ua、Ub。
// q 和 r 用作临时变量；初始值被忽略。
func euclidUpdate(A, B, Ua, Ub, q, r *Int, extended bool) (nA, nB, nr, nUa, nUb *Int) {
	q.QuoRem(A, B, r)

	if extended {
		// Ua, Ub = Ub, Ua-q*Ub
		q.Mul(q, Ub)
		Ua, Ub = Ub, Ua
		Ub.Sub(Ub, q)
	}

	return B, r, A, Ua, Ub
}

// lehmerGCD 将 z 设置为 a 和 b 的最大公约数，
// 两者都必须 != 0，并返回 z。
// 如果 x 或 y 不为 nil，设置它们的值使得 z = a*x + b*y。
// 参见 Knuth，《计算机程序设计艺术》，第 2 卷，第 4.5.2 节，算法 L。
// 此实现使用 Collins 的改进条件，只需要一个
// 商并避免单个 Word 溢出的可能性。
// 参见 Jebelean，"改进多精度欧几里得算法"，
// 符号计算系统的设计和实现，第 45-58 页。
// 结果根据来自 Cohen et al 的算法 10.45 更新
// "椭圆和超椭圆曲线密码学手册"第 192 页。
func (z *Int) lehmerGCD(x, y, a, b *Int) *Int {
	var A, B, Ua, Ub *Int

	A = new(Int).Abs(a)
	B = new(Int).Abs(b)

	extended := x != nil || y != nil

	if extended {
		// Ua (Ub) 跟踪输入 a 被累积到 A (B) 中的次数。
		Ua = new(Int).SetInt64(1)
		Ub = new(Int)
	}

	// 多精度更新的临时变量
	q := new(Int)
	r := new(Int)

	// 确保 A >= B
	if A.abs.cmp(B.abs) < 0 {
		A, B = B, A
		Ub, Ua = Ua, Ub
	}

	// 循环不变式 A >= B
	for len(B.abs) > 1 {
		// 尝试使用 A 和 B 的前导字以单精度计算。
		u0, u1, v0, v1, even := lehmerSimulate(A, B)

		// 多精度步骤
		if v0 != 0 {
			// 使用结果模拟单精度步骤的效果。
			// A = u0*A + v0*B
			// B = u1*A + v1*B
			lehmerUpdate(A, B, q, r, u0, u1, v0, v1, even)

			if extended {
				// Ua = u0*Ua + v0*Ub
				// Ub = u1*Ua + v1*Ub
				lehmerUpdate(Ua, Ub, q, r, u0, u1, v0, v1, even)
			}

		} else {
			// 单精度计算未能模拟任何商。
			// 执行标准的欧几里得步骤。
			A, B, r, Ua, Ub = euclidUpdate(A, B, Ua, Ub, q, r, extended)
		}
	}

	if len(B.abs) > 0 {
		// 如果 B 是单个 Word，扩展欧几里得算法的基本情况
		if len(A.abs) > 1 {
			// A 比单个 Word 长，因此需要一次更新。
			A, B, r, Ua, Ub = euclidUpdate(A, B, Ua, Ub, q, r, extended)
		}
		if len(B.abs) > 0 {
			// A 和 B 都是单个 Word。
			aWord, bWord := A.abs[0], B.abs[0]
			if extended {
				var ua, ub, va, vb Word
				ua, ub = 1, 0
				va, vb = 0, 1
				even := true
				for bWord != 0 {
					q, r := aWord/bWord, aWord%bWord
					aWord, bWord = bWord, r
					ua, ub = ub, ua+q*ub
					va, vb = vb, va+q*vb
					even = !even
				}

				mulW(Ua, Ua, !even, ua)
				mulW(Ub, Ub, even, va)
				Ua.Add(Ua, Ub)
			} else {
				for bWord != 0 {
					aWord, bWord = bWord, aWord%bWord
				}
			}
			A.abs[0] = aWord
		}
	}
	negA := a.neg
	if y != nil {
		// 避免下面除法中需要的 b 的别名
		if y == b {
			B.Set(b)
		} else {
			B = b
		}
		// y = (z - a*x)/b
		y.Mul(a, Ua) // y 可以安全地别名 a
		if negA {
			y.neg = !y.neg
		}
		y.Sub(A, y)
		y.Div(y, B)
	}

	if x != nil {
		x.Set(Ua)
		if negA {
			x.neg = !x.neg
		}
	}

	z.Set(A)

	return z
}

// Rand 将 z 设置为 [0, n) 中的伪随机数并返回 z。
//
// 由于此使用 [math/rand] 包，不能用于
// 安全敏感的工作。改用 [crypto/rand.Int]。
func (z *Int) Rand(rnd *rand.Rand, n *Int) *Int {
	// z.neg 在 if 检查之前未被修改，因为 z 和 n 可能会别名。
	if n.neg || len(n.abs) == 0 {
		z.neg = false
		z.abs = nil
		return z
	}
	z.neg = false
	z.abs = z.abs.random(rnd, n.abs, n.abs.bitLen())
	return z
}

// ModInverse 将 z 设置为 g 在环 ℤ/nℤ 中的乘法逆元
// 并返回 z。如果 g 和 n 不互质，g 在环 ℤ/nℤ 中没有乘法逆元。
// 在这种情况下，z 保持不变，返回值
// 为 nil。如果 n == 0，将发生除以零的运行时错误。
func (z *Int) ModInverse(g, n *Int) *Int {
	// GCD 期望参数 a 和 b 大于 0。
	if n.neg {
		var n2 Int
		n = n2.Neg(n)
	}
	if g.neg {
		var g2 Int
		g = g2.Mod(g, n)
	}
	var d, x Int
	d.GCD(&x, nil, g, n)

	// 当且仅当 d==1 时，g 和 n 互质
	if d.Cmp(intOne) != 0 {
		return nil
	}

	// x 和 y 使得 g*x + n*y = 1，因此 x 是逆元素，
	// 但它可能是负数，所以转换到范围 0 <= z < |n|
	if x.neg {
		z.Add(&x, n)
	} else {
		z.Set(&x)
	}
	return z
}

func (z nat) modInverse(g, n nat) nat {
	// TODO(rsc): ModInverse 应该根据此函数实现。
	return (&Int{abs: z}).ModInverse(&Int{abs: g}, &Int{abs: n}).abs
}

// Jacobi 返回 Jacobi 符号 (x/y)，为 +1、-1 或 0。
// y 参数必须是奇整数。
func Jacobi(x, y *Int) int {
	if len(y.abs) == 0 || y.abs[0]&1 == 0 {
		panic(fmt.Sprintf("big: invalid 2nd argument to Int.Jacobi: need odd integer but got %s", y.String()))
	}

	// 我们使用第 2 章第 2.4 节中描述的公式，
	// "Yacas 算法书"：
	// http://yacas.sourceforge.net/Algo.book.pdf

	var a, b, c Int
	a.Set(x)
	b.Set(y)
	j := 1

	if b.neg {
		if a.neg {
			j = -1
		}
		b.neg = false
	}

	for {
		if b.Cmp(intOne) == 0 {
			return j
		}
		if len(a.abs) == 0 {
			return 0
		}
		a.Mod(&a, &b)
		if len(a.abs) == 0 {
			return 0
		}
		// a > 0

		// 处理 'a' 中的 2 的因子
		s := a.abs.trailingZeroBits()
		if s&1 != 0 {
			bmod8 := b.abs[0] & 7
			if bmod8 == 3 || bmod8 == 5 {
				j = -j
			}
		}
		c.Rsh(&a, s) // a = 2^s*c

		// 交换分子和分母
		if b.abs[0]&3 == 3 && c.abs[0]&3 == 3 {
			j = -j
		}
		a.Set(&b)
		b.Set(&c)
	}
}

// modSqrt3Mod4 使用恒等式
//
//	   (a^((p+1)/4))^2  mod p
//	== u^(p+1)          mod p
//	== u^2              mod p
//
// 快速计算任何二次剩余 mod p 的平方根，用于 3
// mod 4 素数。
func (z *Int) modSqrt3Mod4Prime(x, p *Int) *Int {
	e := new(Int).Add(p, intOne) // e = p + 1
	e.Rsh(e, 2)                  // e = (p + 1) / 4
	z.Exp(x, e, p)               // z = x^e mod p
	return z
}

// modSqrt5Mod8Prime 使用 Atkin 的观察，即 2 不是 mod p 的平方
//
//	alpha ==  (2*a)^((p-5)/8)    mod p
//	beta  ==  2*a*alpha^2        mod p  是 -1 的平方根
//	b     ==  a*alpha*(beta-1)   mod p  是 a 的平方根
//
// 快速计算任何二次剩余 mod p 的平方根，用于 5
// mod 8 素数。
func (z *Int) modSqrt5Mod8Prime(x, p *Int) *Int {
	// p == 5 mod 8 意味着 p = e*8 + 5
	// e 是商，5 是除以 8 的余数
	e := new(Int).Rsh(p, 3)  // e = (p - 5) / 8
	tx := new(Int).Lsh(x, 1) // tx = 2*x
	alpha := new(Int).Exp(tx, e, p)
	beta := new(Int).Mul(alpha, alpha)
	beta.Mod(beta, p)
	beta.Mul(beta, tx)
	beta.Mod(beta, p)
	beta.Sub(beta, intOne)
	beta.Mul(beta, x)
	beta.Mod(beta, p)
	beta.Mul(beta, alpha)
	z.Mod(beta, p)
	return z
}

// modSqrtTonelliShanks 使用 Tonelli-Shanks 算法找到平方
// 任何素数模的二次剩余的根。
func (z *Int) modSqrtTonelliShanks(x, p *Int) *Int {
	// 将 p-1 分解为 s*2^e，使得 s 是奇数。
	var s Int
	s.Sub(p, intOne)
	e := s.abs.trailingZeroBits()
	s.Rsh(&s, e)

	// 找到一些非平方 n
	var n Int
	n.SetInt64(2)
	for Jacobi(&n, p) != -1 {
		n.Add(&n, intOne)
	}

	// Tonelli-Shanks 算法的核心。遵循描述 in
	// Ezra Brown 的"从 1; 24, 51, 10 到 Dan Shanks 的平方根"的第 6 节：
	// https://www.maa.org/sites/default/files/pdf/upload_library/22/Polya/07468342.di020786.02p0470a.pdf
	var y, b, g, t Int
	y.Add(&s, intOne)
	y.Rsh(&y, 1)
	y.Exp(x, &y, p)  // y = x^((s+1)/2)
	b.Exp(x, &s, p)  // b = x^s
	g.Exp(&n, &s, p) // g = n^s
	r := e
	for {
		// 找到最小的 m 使得 ord_p(b) = 2^m
		var m uint
		t.Set(&b)
		for t.Cmp(intOne) != 0 {
			t.Mul(&t, &t).Mod(&t, p)
			m++
		}

		if m == 0 {
			return z.Set(&y)
		}

		t.SetInt64(0).SetBit(&t, int(r-m-1), 1).Exp(&g, &t, p)
		// t = g^(2^(r-m-1)) mod p
		g.Mul(&t, &t).Mod(&g, p) // g = g^(2^(r-m)) mod p
		y.Mul(&y, &t).Mod(&y, p)
		b.Mul(&b, &g).Mod(&b, p)
		r = m
	}
}

// ModSqrt 如果平方根存在，将 z 设置为 x mod p 的平方根，
// 并返回 z。模 p 必须是奇素数。如果 x 不是 mod p 的平方，
// ModSqrt 将 z 保持不变并返回 nil。如果 p 不是
// 奇整数，此函数会引发恐慌；如果 p 是奇数但不是素数，其行为未定义。
func (z *Int) ModSqrt(x, p *Int) *Int {
	switch Jacobi(x, p) {
	case -1:
		return nil // x 不是 mod p 的平方
	case 0:
		return z.SetInt64(0) // sqrt(0) mod p = 0
	case 1:
		break
	}
	if x.neg || x.Cmp(p) >= 0 { // 确保 0 <= x < p
		x = new(Int).Mod(x, p)
	}

	switch {
	case p.abs[0]%4 == 3:
		// 检查 p 是否为 3 mod 4，如果是，使用更快的算法。
		return z.modSqrt3Mod4Prime(x, p)
	case p.abs[0]%8 == 5:
		// 检查 p 是否为 5 mod 8，使用 Atkin 的算法。
		return z.modSqrt5Mod8Prime(x, p)
	default:
		// 否则，使用 Tonelli-Shanks。
		return z.modSqrtTonelliShanks(x, p)
	}
}

// Lsh 设置 z = x << n 并返回 z。
func (z *Int) Lsh(x *Int, n uint) *Int {
	z.abs = z.abs.lsh(x.abs, n)
	z.neg = x.neg
	return z
}

// Rsh 设置 z = x >> n 并返回 z。
func (z *Int) Rsh(x *Int, n uint) *Int {
	if x.neg {
		// (-x) >> s == ^(x-1) >> s == ^((x-1) >> s) == -(((x-1) >> s) + 1)
		t := z.abs.sub(x.abs, natOne) // 没有下溢，因为 |x| > 0
		t = t.rsh(t, n)
		z.abs = t.add(t, natOne)
		z.neg = true // 如果 x 是负数，z 不能为零
		return z
	}

	z.abs = z.abs.rsh(x.abs, n)
	z.neg = false
	return z
}

// Bit 返回 x 第 i 位的值。也就是说，它
// 返回 (x>>i)&1。位索引 i 必须 >= 0。
func (x *Int) Bit(i int) uint {
	if i == 0 {
		// 常见情况的优化：x 的奇偶性测试
		if len(x.abs) > 0 {
			return uint(x.abs[0] & 1) // 位 0 对于 -x 相同
		}
		return 0
	}
	if i < 0 {
		panic("negative bit index")
	}
	if x.neg {
		t := nat(nil).sub(x.abs, natOne)
		return t.bit(uint(i)) ^ 1
	}

	return x.abs.bit(uint(i))
}

// SetBit 将 z 设置为 x，将 x 的第 i 位设置为 b（0 或 1）。
// 也就是说，
//   - 如果 b 是 1，SetBit 设置 z = x | (1 << i)；
//   - 如果 b 是 0，SetBit 设置 z = x &^ (1 << i)；
//   - 如果 b 不是 0 或 1，SetBit 将引发恐慌。
func (z *Int) SetBit(x *Int, i int, b uint) *Int {
	if i < 0 {
		panic("negative bit index")
	}
	if x.neg {
		t := z.abs.sub(x.abs, natOne)
		t = t.setBit(t, uint(i), b^1)
		z.abs = t.add(t, natOne)
		z.neg = len(z.abs) > 0
		return z
	}
	z.abs = z.abs.setBit(x.abs, uint(i), b)
	z.neg = false
	return z
}

// And 设置 z = x & y 并返回 z。
func (z *Int) And(x, y *Int) *Int {
	if x.neg == y.neg {
		if x.neg {
			// (-x) & (-y) == ^(x-1) & ^(y-1) == ^((x-1) | (y-1)) == -(((x-1) | (y-1)) + 1)
			x1 := nat(nil).sub(x.abs, natOne)
			y1 := nat(nil).sub(y.abs, natOne)
			z.abs = z.abs.add(z.abs.or(x1, y1), natOne)
			z.neg = true // 如果 x 和 y 都是负数，z 不能为零
			return z
		}

		// x & y == x & y
		z.abs = z.abs.and(x.abs, y.abs)
		z.neg = false
		return z
	}

	// x.neg != y.neg
	if x.neg {
		x, y = y, x // & 是对称的
	}

	// x & (-y) == x & ^(y-1) == x &^ (y-1)
	y1 := nat(nil).sub(y.abs, natOne)
	z.abs = z.abs.andNot(x.abs, y1)
	z.neg = false
	return z
}

// AndNot 设置 z = x &^ y 并返回 z。
func (z *Int) AndNot(x, y *Int) *Int {
	if x.neg == y.neg {
		if x.neg {
			// (-x) &^ (-y) == ^(x-1) &^ ^(y-1) == ^(x-1) & (y-1) == (y-1) &^ (x-1)
			x1 := nat(nil).sub(x.abs, natOne)
			y1 := nat(nil).sub(y.abs, natOne)
			z.abs = z.abs.andNot(y1, x1)
			z.neg = false
			return z
		}

		// x &^ y == x &^ y
		z.abs = z.abs.andNot(x.abs, y.abs)
		z.neg = false
		return z
	}

	if x.neg {
		// (-x) &^ y == ^(x-1) &^ y == ^(x-1) & ^y == ^((x-1) | y) == -(((x-1) | y) + 1)
		x1 := nat(nil).sub(x.abs, natOne)
		z.abs = z.abs.add(z.abs.or(x1, y.abs), natOne)
		z.neg = true // 如果 x 是负数且 y 是正数，z 不能为零
		return z
	}

	// x &^ (-y) == x &^ ^(y-1) == x & (y-1)
	y1 := nat(nil).sub(y.abs, natOne)
	z.abs = z.abs.and(x.abs, y1)
	z.neg = false
	return z
}

// Or 设置 z = x | y 并返回 z。
func (z *Int) Or(x, y *Int) *Int {
	if x.neg == y.neg {
		if x.neg {
			// (-x) | (-y) == ^(x-1) | ^(y-1) == ^((x-1) & (y-1)) == -(((x-1) & (y-1)) + 1)
			x1 := nat(nil).sub(x.abs, natOne)
			y1 := nat(nil).sub(y.abs, natOne)
			z.abs = z.abs.add(z.abs.and(x1, y1), natOne)
			z.neg = true // 如果 x 和 y 都是负数，z 不能为零
			return z
		}

		// x | y == x | y
		z.abs = z.abs.or(x.abs, y.abs)
		z.neg = false
		return z
	}

	// x.neg != y.neg
	if x.neg {
		x, y = y, x // | 是对称的
	}

	// x | (-y) == x | ^(y-1) == ^((y-1) &^ x) == -(^((y-1) &^ x) + 1)
	y1 := nat(nil).sub(y.abs, natOne)
	z.abs = z.abs.add(z.abs.andNot(y1, x.abs), natOne)
	z.neg = true // 如果 x 或 y 之一是负数，z 不能为零
	return z
}

// Xor 设置 z = x ^ y 并返回 z。
func (z *Int) Xor(x, y *Int) *Int {
	if x.neg == y.neg {
		if x.neg {
			// (-x) ^ (-y) == ^(x-1) ^ ^(y-1) == (x-1) ^ (y-1)
			x1 := nat(nil).sub(x.abs, natOne)
			y1 := nat(nil).sub(y.abs, natOne)
			z.abs = z.abs.xor(x1, y1)
			z.neg = false
			return z
		}

		// x ^ y == x ^ y
		z.abs = z.abs.xor(x.abs, y.abs)
		z.neg = false
		return z
	}

	// x.neg != y.neg
	if x.neg {
		x, y = y, x // ^ 是对称的
	}

	// x ^ (-y) == x ^ ^(y-1) == ^(x ^ (y-1)) == -((x ^ (y-1)) + 1)
	y1 := nat(nil).sub(y.abs, natOne)
	z.abs = z.abs.add(z.abs.xor(x.abs, y1), natOne)
	z.neg = true // 如果仅 x 或 y 之一是负数，z 不能为零
	return z
}

// Not 设置 z = ^x 并返回 z。
func (z *Int) Not(x *Int) *Int {
	if x.neg {
		// ^(-x) == ^(^(x-1)) == x-1
		z.abs = z.abs.sub(x.abs, natOne)
		z.neg = false
		return z
	}

	// ^x == -x-1 == -(x+1)
	z.abs = z.abs.add(x.abs, natOne)
	z.neg = true // 如果 x 是正数，z 不能为零
	return z
}

// Sqrt 将 z 设置为 ⌊√x⌋，最大整数使得 z² ≤ x，并返回 z。
// 如果 x 是负数，它会引发恐慌。
func (z *Int) Sqrt(x *Int) *Int {
	if x.neg {
		panic("square root of negative number")
	}
	z.neg = false
	z.abs = z.abs.sqrt(nil, x.abs)
	return z
}
