// 版权所有 2016 The Go 作者。保留所有权利。
// 本源代码的使用受 BSD 风格许可证管制，
// 该许可证可在 LICENSE 文件中找到。

//go:build (!linux && !freebsd && !darwin) || !cgo

package plugin

import "errors"

func lookup(p *Plugin, symName string) (Symbol, error) {
	return nil, errors.New("plugin: not implemented")
}

func open(name string) (*Plugin, error) {
	return nil, errors.New("plugin: not implemented")
}
