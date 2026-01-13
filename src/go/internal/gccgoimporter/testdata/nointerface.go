// 版权所有 2018 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package nointerface

type I int

//go:nointerface
func (p *I) Get() int { return int(*p) }

func (p *I) Set(v int) { *p = I(v) }
