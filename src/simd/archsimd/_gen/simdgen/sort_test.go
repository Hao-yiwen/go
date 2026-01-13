// 版权所有 2025 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package main

import "testing"

func TestSort(t *testing.T) {
	testCases := []struct {
		s1, s2 string
		want   int
	}{
		{"a1", "a2", -1},
		{"a11a", "a11b", -1},
		{"a01a1", "a1a01", -1},
		{"a2", "a1", 1},
		{"a10", "a2", 1},
		{"a1", "a10", -1},
		{"z11", "z2", 1},
		{"z2", "z11", -1},
		{"abc", "abd", -1},
		{"123", "45", 1},
		{"file1", "file1", 0},
		{"file", "file1", -1},
		{"file1", "file", 1},
		{"a01", "a1", -1},
		{"a1a", "a1b", -1},
	}

	for _, tc := range testCases {
		got := compareNatural(tc.s1, tc.s2)
		result := "✅"
		if got != tc.want {
			result = "❌"
			t.Errorf("%s CompareNatural(\"%s\", \"%s\") -> got %2d, want %2d\n", result, tc.s1, tc.s2, got, tc.want)
		} else {
			t.Logf("%s CompareNatural(\"%s\", \"%s\") -> got %2d, want %2d\n", result, tc.s1, tc.s2, got, tc.want)
		}
	}
}
