// 版权所有 2018 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package issue25301

type (
	A = interface {
		M()
	}
	T interface {
		A
	}
	S struct{}
)

func (S) M() { println("m") }
