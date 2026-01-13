// 版权所有 2011 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

//go:build !goexperiment.jsonv2

package json

import (
	"strings"
)

// tagOptions 是结构体字段的 "json" 标签中逗号后的字符串，
// 或空字符串。它不包括前导逗号。
type tagOptions string

// parseTag 将结构体字段的 json 标签分割为其名称和
// 以逗号分隔的选项。
func parseTag(tag string) (string, tagOptions) {
	tag, opt, _ := strings.Cut(tag, ",")
	return tag, tagOptions(opt)
}

// Contains 报告以逗号分隔的选项列表是否
// 包含特定的 substr 标志。substr 必须由
// 字符串边界或逗号包围。
func (o tagOptions) Contains(optionName string) bool {
	if len(o) == 0 {
		return false
	}
	s := string(o)
	for s != "" {
		var name string
		name, s, _ = strings.Cut(s, ",")
		if name == optionName {
			return true
		}
	}
	return false
}
