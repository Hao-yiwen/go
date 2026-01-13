// 版权所有 2019 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package issue34182

type T1 struct {
	f *T2
}

type T2 struct {
	f T3
}

type T3 struct {
	*T2
}
