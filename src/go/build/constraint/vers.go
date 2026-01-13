// 版权所有 2023 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package constraint

import (
	"strconv"
	"strings"
)

// GoVersion 返回the minimum Go version implied by a given build expression.
// If the expression can be satisfied without any Go version tags, GoVersion 返回一个 empty string.
//
// For example:
//
//	GoVersion(linux && go1.22) = "go1.22"
//	GoVersion((linux && go1.22) || (windows && go1.20)) = "go1.20" => go1.20
//	GoVersion(linux) = ""
//	GoVersion(linux || (windows && go1.22)) = ""
//	GoVersion(!go1.22) = ""
//
// GoVersion assumes that any tag or negated tag may independently be true,
// so that its analysis can be purely structural, without SAT solving.
// “Impossible” subexpressions may therefore affect the result.
//
// For example:
//
//	GoVersion((linux && !linux && go1.20) || go1.21) = "go1.20"
func GoVersion(x Expr) string {
	v := minVersion(x, +1)
	if v < 0 {
		return ""
	}
	if v == 0 {
		return "go1"
	}
	return "go1." + strconv.Itoa(v)
}

// minVersion 返回the minimum Go major version (9 for go1.9)
// implied by expression z, or if sign < 0, by expression !z.
func minVersion(z Expr, sign int) int {
	switch z := z.(type) {
	default:
		return -1
	case *AndExpr:
		op := andVersion
		if sign < 0 {
			op = orVersion
		}
		return op(minVersion(z.X, sign), minVersion(z.Y, sign))
	case *OrExpr:
		op := orVersion
		if sign < 0 {
			op = andVersion
		}
		return op(minVersion(z.X, sign), minVersion(z.Y, sign))
	case *NotExpr:
		return minVersion(z.X, -sign)
	case *TagExpr:
		if sign < 0 {
			// !foo implies nothing
			return -1
		}
		if z.Tag == "go1" {
			return 0
		}
		_, v, _ := strings.Cut(z.Tag, "go1.")
		n, err := strconv.Atoi(v)
		if err != nil {
			// not a go1.N tag
			return -1
		}
		return n
	}
}

// andVersion 返回the minimum Go version
// implied by the AND of two minimum Go versions,
// which 是 max of the versions.
func andVersion(x, y int) int {
	if x > y {
		return x
	}
	return y
}

// orVersion 返回the minimum Go version
// implied by the OR of two minimum Go versions,
// which 是 min of the versions.
func orVersion(x, y int) int {
	if x < y {
		return x
	}
	return y
}
