// 版权所有 2025 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package issue62640

type E struct{}

// F 应该是 hidden within S because of the S.F field.
func (E) F() {}

type S struct {
	E
	F int
}
