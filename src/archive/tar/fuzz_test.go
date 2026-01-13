// 版权所有 2021 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package tar

import (
	"bytes"
	"io"
	"testing"
)

func FuzzReader(f *testing.F) {
	b := bytes.NewBuffer(nil)
	w := NewWriter(b)
	inp := []byte("Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat. Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur. Excepteur sint occaecat cupidatat non proident, sunt in culpa qui officia deserunt mollit anim id est laborum.")
	err := w.WriteHeader(&Header{
		Name: "lorem.txt",
		Mode: 0600,
		Size: int64(len(inp)),
	})
	if err != nil {
		f.Fatalf("failed to create writer: %s", err)
	}
	_, err = w.Write(inp)
	if err != nil {
		f.Fatalf("failed to write file to archive: %s", err)
	}
	if err := w.Close(); err != nil {
		f.Fatalf("failed to write archive: %s", err)
	}
	f.Add(b.Bytes())

	f.Fuzz(func(t *testing.T, b []byte) {
		r := NewReader(bytes.NewReader(b))
		type file struct {
			header  *Header
			content []byte
		}
		files := []file{}
		for {
			hdr, err := r.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return
			}
			buf := bytes.NewBuffer(nil)
			if _, err := io.Copy(buf, r); err != nil {
				continue
			}
			files = append(files, file{header: hdr, content: buf.Bytes()})
		}

		// 如果我们无法从归档中读取任何内容，
		// 就不必尝试进行往返测试。
		if len(files) == 0 {
			return
		}

		out := bytes.NewBuffer(nil)
		w := NewWriter(out)
		for _, f := range files {
			if err := w.WriteHeader(f.header); err != nil {
				t.Fatalf("unable to write previously parsed header: %s", err)
			}
			if _, err := w.Write(f.content); err != nil {
				t.Fatalf("unable to write previously parsed content: %s", err)
			}
		}
		if err := w.Close(); err != nil {
			t.Fatalf("Unable to write archive: %s", err)
		}

		// TODO: 我们可能想要检查归档是否能往返。这需要考虑
		// Writer.Close 附加的两个零尾块的添加。
	})
}
