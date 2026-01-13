// 版权所有 2010 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package image

import (
	"image/color"
	"math/bits"
	"strconv"
)

// Point 是一个 X、Y 坐标对。坐标轴向右和向下增加。
type Point struct {
	X, Y int
}

// String 返回 p 的字符串表示，如 "(3,4)"。
func (p Point) String() string {
	return "(" + strconv.Itoa(p.X) + "," + strconv.Itoa(p.Y) + ")"
}

// Add 返回向量 p+q。
func (p Point) Add(q Point) Point {
	return Point{p.X + q.X, p.Y + q.Y}
}

// Sub 返回向量 p-q。
func (p Point) Sub(q Point) Point {
	return Point{p.X - q.X, p.Y - q.Y}
}

// Mul 返回向量 p*k。
func (p Point) Mul(k int) Point {
	return Point{p.X * k, p.Y * k}
}

// Div 返回向量 p/k。
func (p Point) Div(k int) Point {
	return Point{p.X / k, p.Y / k}
}

// In 报告 p 是否在 r 中。
func (p Point) In(r Rectangle) bool {
	return r.Min.X <= p.X && p.X < r.Max.X &&
		r.Min.Y <= p.Y && p.Y < r.Max.Y
}

// Mod 返回 r 中的点 q，使得 p.X-q.X 是 r 宽度的倍数，
// 且 p.Y-q.Y 是 r 高度的倍数。
func (p Point) Mod(r Rectangle) Point {
	w, h := r.Dx(), r.Dy()
	p = p.Sub(r.Min)
	p.X = p.X % w
	if p.X < 0 {
		p.X += w
	}
	p.Y = p.Y % h
	if p.Y < 0 {
		p.Y += h
	}
	return p.Add(r.Min)
}

// Eq 报告 p 和 q 是否相等。
func (p Point) Eq(q Point) bool {
	return p == q
}

// ZP 是零值 [Point]。
//
// 已弃用：请改用 [image.Point] 字面量。
var ZP Point

// Pt 是 [Point]{X, Y} 的简写。
func Pt(X, Y int) Point {
	return Point{X, Y}
}

// Rectangle 包含满足 Min.X <= X < Max.X, Min.Y <= Y < Max.Y 的点。
// 如果 Min.X <= Max.X 且 Y 也如此，则它是规范的。点总是规范的。
// 对于规范的输入，矩形的方法总是返回规范的输出。
//
// Rectangle 也是一个 [Image]，其边界就是矩形本身。At 方法
// 对矩形内的点返回 color.Opaque，否则返回 color.Transparent。
type Rectangle struct {
	Min, Max Point
}

// String 返回 r 的字符串表示，如 "(3,4)-(6,5)"。
func (r Rectangle) String() string {
	return r.Min.String() + "-" + r.Max.String()
}

// Dx 返回 r 的宽度。
func (r Rectangle) Dx() int {
	return r.Max.X - r.Min.X
}

// Dy 返回 r 的高度。
func (r Rectangle) Dy() int {
	return r.Max.Y - r.Min.Y
}

// Size 返回 r 的宽度和高度。
func (r Rectangle) Size() Point {
	return Point{
		r.Max.X - r.Min.X,
		r.Max.Y - r.Min.Y,
	}
}

// Add 返回按 p 平移后的矩形 r。
func (r Rectangle) Add(p Point) Rectangle {
	return Rectangle{
		Point{r.Min.X + p.X, r.Min.Y + p.Y},
		Point{r.Max.X + p.X, r.Max.Y + p.Y},
	}
}

// Sub 返回按 -p 平移后的矩形 r。
func (r Rectangle) Sub(p Point) Rectangle {
	return Rectangle{
		Point{r.Min.X - p.X, r.Min.Y - p.Y},
		Point{r.Max.X - p.X, r.Max.Y - p.Y},
	}
}

// Inset 返回向内缩进 n 的矩形 r，n 可以为负数。如果 r 的任一维度
// 小于 2*n，则将返回靠近 r 中心的空矩形。
func (r Rectangle) Inset(n int) Rectangle {
	if r.Dx() < 2*n {
		r.Min.X = (r.Min.X + r.Max.X) / 2
		r.Max.X = r.Min.X
	} else {
		r.Min.X += n
		r.Max.X -= n
	}
	if r.Dy() < 2*n {
		r.Min.Y = (r.Min.Y + r.Max.Y) / 2
		r.Max.Y = r.Min.Y
	} else {
		r.Min.Y += n
		r.Max.Y -= n
	}
	return r
}

// Intersect 返回同时被 r 和 s 包含的最大矩形。如果两个矩形
// 不重叠，则返回零矩形。
func (r Rectangle) Intersect(s Rectangle) Rectangle {
	if r.Min.X < s.Min.X {
		r.Min.X = s.Min.X
	}
	if r.Min.Y < s.Min.Y {
		r.Min.Y = s.Min.Y
	}
	if r.Max.X > s.Max.X {
		r.Max.X = s.Max.X
	}
	if r.Max.Y > s.Max.Y {
		r.Max.Y = s.Max.Y
	}
	// 设 r0 和 s0 为调用此方法时 r 和 s 的值，
	// 下一行等同于：
	//
	// if max(r0.Min.X, s0.Min.X) >= min(r0.Max.X, s0.Max.X) || likewiseForY { etc }
	if r.Empty() {
		return Rectangle{}
	}
	return r
}

// Union 返回同时包含 r 和 s 的最小矩形。
func (r Rectangle) Union(s Rectangle) Rectangle {
	if r.Empty() {
		return s
	}
	if s.Empty() {
		return r
	}
	if r.Min.X > s.Min.X {
		r.Min.X = s.Min.X
	}
	if r.Min.Y > s.Min.Y {
		r.Min.Y = s.Min.Y
	}
	if r.Max.X < s.Max.X {
		r.Max.X = s.Max.X
	}
	if r.Max.Y < s.Max.Y {
		r.Max.Y = s.Max.Y
	}
	return r
}

// Empty 报告矩形是否不包含任何点。
func (r Rectangle) Empty() bool {
	return r.Min.X >= r.Max.X || r.Min.Y >= r.Max.Y
}

// Eq 报告 r 和 s 是否包含相同的点集。所有空矩形都被认为是相等的。
func (r Rectangle) Eq(s Rectangle) bool {
	return r == s || r.Empty() && s.Empty()
}

// Overlaps 报告 r 和 s 是否有非空交集。
func (r Rectangle) Overlaps(s Rectangle) bool {
	return !r.Empty() && !s.Empty() &&
		r.Min.X < s.Max.X && s.Min.X < r.Max.X &&
		r.Min.Y < s.Max.Y && s.Min.Y < r.Max.Y
}

// In 报告 r 中的每个点是否都在 s 中。
func (r Rectangle) In(s Rectangle) bool {
	if r.Empty() {
		return true
	}
	// 注意 r.Max 是 r 的排他边界，所以 r.In(s)
	// 不要求 r.Max.In(s)。
	return s.Min.X <= r.Min.X && r.Max.X <= s.Max.X &&
		s.Min.Y <= r.Min.Y && r.Max.Y <= s.Max.Y
}

// Canon 返回 r 的规范版本。如有必要，返回的矩形会交换最小和最大坐标，
// 以使其规范化。
func (r Rectangle) Canon() Rectangle {
	if r.Max.X < r.Min.X {
		r.Min.X, r.Max.X = r.Max.X, r.Min.X
	}
	if r.Max.Y < r.Min.Y {
		r.Min.Y, r.Max.Y = r.Max.Y, r.Min.Y
	}
	return r
}

// At 实现 [Image] 接口。
func (r Rectangle) At(x, y int) color.Color {
	if (Point{x, y}).In(r) {
		return color.Opaque
	}
	return color.Transparent
}

// RGBA64At 实现 [RGBA64Image] 接口。
func (r Rectangle) RGBA64At(x, y int) color.RGBA64 {
	if (Point{x, y}).In(r) {
		return color.RGBA64{0xffff, 0xffff, 0xffff, 0xffff}
	}
	return color.RGBA64{}
}

// Bounds 实现 [Image] 接口。
func (r Rectangle) Bounds() Rectangle {
	return r
}

// ColorModel 实现 [Image] 接口。
func (r Rectangle) ColorModel() color.Model {
	return color.Alpha16Model
}

// ZR 是零值 [Rectangle]。
//
// 已弃用：请改用 [image.Rectangle] 字面量。
var ZR Rectangle

// Rect 是 [Rectangle]{Pt(x0, y0), [Pt](x1, y1)} 的简写。如有必要，
// 返回的矩形会交换最小和最大坐标，以使其规范化。
func Rect(x0, y0, x1, y1 int) Rectangle {
	if x0 > x1 {
		x0, x1 = x1, x0
	}
	if y0 > y1 {
		y0, y1 = y1, y0
	}
	return Rectangle{Point{x0, y0}, Point{x1, y1}}
}

// mul3NonNeg 返回 (x * y * z)，除非至少有一个参数为负数或
// 计算溢出 int 类型，在这种情况下返回 -1。
func mul3NonNeg(x int, y int, z int) int {
	if (x < 0) || (y < 0) || (z < 0) {
		return -1
	}
	hi, lo := bits.Mul64(uint64(x), uint64(y))
	if hi != 0 {
		return -1
	}
	hi, lo = bits.Mul64(lo, uint64(z))
	if hi != 0 {
		return -1
	}
	a := int(lo)
	if (a < 0) || (uint64(a) != lo) {
		return -1
	}
	return a
}

// add2NonNeg 返回 (x + y)，除非至少有一个参数为负数或
// 计算溢出 int 类型，在这种情况下返回 -1。
func add2NonNeg(x int, y int) int {
	if (x < 0) || (y < 0) {
		return -1
	}
	a := x + y
	if a < 0 {
		return -1
	}
	return a
}
