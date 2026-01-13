// 版权所有 2019 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package issue31540

type Y struct {
	q int
}

type Z map[int]int

type X = map[Y]Z

type A1 = X

type A2 = A1

type S struct {
	b int
	A2
}

func Hallo() S {
	return S{}
}
