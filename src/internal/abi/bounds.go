// 版权所有 2025 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package abi

// 此类型和常量用于编码不同类型的边界检查失败。
type BoundsErrorCode uint8

const (
	BoundsIndex      BoundsErrorCode = iota // s[x], 0 <= x < len(s) 检查失败
	BoundsSliceAlen                         // s[?:x], 0 <= x <= len(s) 检查失败
	BoundsSliceAcap                         // s[?:x], 0 <= x <= cap(s) 检查失败
	BoundsSliceB                            // s[x:y], 0 <= x <= y 检查失败（但 boundsSliceA 未发生）
	BoundsSlice3Alen                        // s[?:?:x], 0 <= x <= len(s) 检查失败
	BoundsSlice3Acap                        // s[?:?:x], 0 <= x <= cap(s) 检查失败
	BoundsSlice3B                           // s[?:x:y], 0 <= x <= y 检查失败（但 boundsSlice3A 未发生）
	BoundsSlice3C                           // s[x:y:?], 0 <= x <= y 检查失败（但 boundsSlice3A/B 未发生）
	BoundsConvert                           // (*[x]T)(s), 0 <= x <= len(s) 检查失败
	numBoundsCodes
)

const (
	BoundsMaxReg   = 15
	BoundsMaxConst = 31
)

// 以下是我们如何编码 PCDATA_PanicBounds 条目：

// 我们允许 16 个寄存器（0-15）和 32 个常量（0-31）。
// 编码以下常量 c：
//     位      用途
// -----------------------------
//       0     x 在寄存器中
//       1     y 在寄存器中
//
// 如果 x 在寄存器中
//       2     x 是有符号的
//     [3:6]   x 的寄存器编号
// 否则
//     [2:6]   x 的常量值
//
// 如果 y 在寄存器中
//     [7:10]  y 的寄存器编号
// 否则
//     [7:11]  y 的常量值
//
// 最终整数为 c * numBoundsCode + code

// TODO: 32 位

// BoundsEncode 将边界检查失败信息编码为 PCDATA_PanicBounds 的整数。
// 寄存器编号必须在 0-15 之间。常量必须在 0-31 之间。
func BoundsEncode(code BoundsErrorCode, signed, xIsReg, yIsReg bool, xVal, yVal int) int {
	c := int(0)
	if xIsReg {
		c |= 1 << 0
		if signed {
			c |= 1 << 2
		}
		if xVal < 0 || xVal > BoundsMaxReg {
			panic("bad xReg")
		}
		c |= xVal << 3
	} else {
		if xVal < 0 || xVal > BoundsMaxConst {
			panic("bad xConst")
		}
		c |= xVal << 2
	}
	if yIsReg {
		c |= 1 << 1
		if yVal < 0 || yVal > BoundsMaxReg {
			panic("bad yReg")
		}
		c |= yVal << 7
	} else {
		if yVal < 0 || yVal > BoundsMaxConst {
			panic("bad yConst")
		}
		c |= yVal << 7
	}
	return c*int(numBoundsCodes) + int(code)
}
func BoundsDecode(v int) (code BoundsErrorCode, signed, xIsReg, yIsReg bool, xVal, yVal int) {
	code = BoundsErrorCode(v % int(numBoundsCodes))
	c := v / int(numBoundsCodes)
	xIsReg = c&1 != 0
	c >>= 1
	yIsReg = c&1 != 0
	c >>= 1
	if xIsReg {
		signed = c&1 != 0
		c >>= 1
		xVal = c & 0xf
		c >>= 4
	} else {
		xVal = c & 0x1f
		c >>= 5
	}
	if yIsReg {
		yVal = c & 0xf
		c >>= 4
	} else {
		yVal = c & 0x1f
		c >>= 5
	}
	if c != 0 {
		panic("BoundsDecode decoding error")
	}
	return
}
