// 版权所有 2009 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package math

/*
	浮点数反正弦和反余弦。

	它们通过在适当的范围归约后
	计算反正切来实现。
*/

// Asin 返回 x 的反正弦值，以弧度表示。
//
// 特殊情况：
//
//	Asin(±0) = ±0
//	Asin(x) = NaN 如果 x < -1 或 x > 1
func Asin(x float64) float64 {
	if haveArchAsin {
		return archAsin(x)
	}
	return asin(x)
}

func asin(x float64) float64 {
	if x == 0 {
		return x // 特殊情况
	}
	sign := false
	if x < 0 {
		x = -x
		sign = true
	}
	if x > 1 {
		return NaN() // 特殊情况
	}

	temp := Sqrt(1 - x*x)
	if x > 0.7 {
		temp = Pi/2 - satan(temp/x)
	} else {
		temp = satan(x / temp)
	}

	if sign {
		temp = -temp
	}
	return temp
}

// Acos 返回 x 的反余弦值，以弧度表示。
//
// 特殊情况：
//
//	Acos(x) = NaN 如果 x < -1 或 x > 1
func Acos(x float64) float64 {
	if haveArchAcos {
		return archAcos(x)
	}
	return acos(x)
}

func acos(x float64) float64 {
	return Pi/2 - Asin(x)
}
