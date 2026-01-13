// 版权所有 2010 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package html

import (
	"testing"
	"unicode/utf8"
)

func TestEntityLength(t *testing.T) {
	entity, entity2 := entityMaps()

	if len(entity) == 0 || len(entity2) == 0 {
		t.Fatal("maps not loaded")
	}

	// 我们验证每个值的 UTF-8 编码长度 <= 1 + len(key)。
	// +1 来自前导的 "&"。此属性意味着反转义文本的长度 <= 转义文本的长度。
	for k, v := range entity {
		if 1+len(k) < utf8.RuneLen(v) {
			t.Error("escaped entity &" + k + " is shorter than its UTF-8 encoding " + string(v))
		}
		if len(k) > longestEntityWithoutSemicolon && k[len(k)-1] != ';' {
			t.Errorf("entity name %s is %d characters, but longestEntityWithoutSemicolon=%d", k, len(k), longestEntityWithoutSemicolon)
		}
	}
	for k, v := range entity2 {
		if 1+len(k) < utf8.RuneLen(v[0])+utf8.RuneLen(v[1]) {
			t.Error("escaped entity &" + k + " is shorter than its UTF-8 encoding " + string(v[0]) + string(v[1]))
		}
	}
}
