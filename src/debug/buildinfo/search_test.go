// 版权所有 2024 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package buildinfo

import (
	"bytes"
	"fmt"
	"io"
	"testing"
)

type byteExe struct {
	b []byte
}

func (x *byteExe) DataReader(addr uint64) (io.ReaderAt, error) {
	if addr >= uint64(len(x.b)) {
		return nil, fmt.Errorf("ReadData(%d) out of bounds of %d-byte slice", addr, len(x.b))
	}
	return bytes.NewReader(x.b[addr:]), nil
}

func (x *byteExe) DataStart() (uint64, uint64) {
	return 0, uint64(len(x.b))
}

func TestSearchMagic(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		want    uint64
		wantErr error
	}{
		{
			name: "beginning",
			data: func() []byte {
				b := make([]byte, buildInfoHeaderSize)
				copy(b, buildInfoMagic)
				return b
			}(),
			want: 0,
		},
		{
			name: "offset",
			data: func() []byte {
				b := make([]byte, 512)
				copy(b[4*buildInfoAlign:], buildInfoMagic)
				return b
			}(),
			want: 4 * buildInfoAlign,
		},
		{
			name: "second_chunk",
			data: func() []byte {
				b := make([]byte, 4*searchChunkSize)
				copy(b[searchChunkSize+4*buildInfoAlign:], buildInfoMagic)
				return b
			}(),
			want: searchChunkSize + 4*buildInfoAlign,
		},
		{
			name: "second_chunk_short",
			data: func() []byte {
				// 魔数位于第二块的 64 字节处，
				// 该块较短；刚好足够容纳头部。
				b := make([]byte, searchChunkSize+4*buildInfoAlign+buildInfoHeaderSize)
				copy(b[searchChunkSize+4*buildInfoAlign:], buildInfoMagic)
				return b
			}(),
			want: searchChunkSize + 4*buildInfoAlign,
		},
		{
			name: "missing",
			data: func() []byte {
				b := make([]byte, buildInfoHeaderSize)
				return b
			}(),
			wantErr: errNotGoExe,
		},
		{
			name: "too_short",
			data: func() []byte {
				// 需要有足够的空间容纳整个头部，
				// 而不仅仅是魔数。
				b := make([]byte, len(buildInfoMagic))
				copy(b, buildInfoMagic)
				return b
			}(),
			wantErr: errNotGoExe,
		},
		{
			name: "misaligned",
			data: func() []byte {
				b := make([]byte, 512)
				copy(b[7:], buildInfoMagic)
				return b
			}(),
			wantErr: errNotGoExe,
		},
		{
			name: "misaligned_across_chunk",
			data: func() []byte {
				// 魔数跨越块边界。根据定义，它必须是未对齐的。
				b := make([]byte, 2*searchChunkSize)
				copy(b[searchChunkSize-8:], buildInfoMagic)
				return b
			}(),
			wantErr: errNotGoExe,
		},
		{
			name: "header_across_chunk",
			data: func() []byte {
				// 魔数在第一块内是对齐的，
				// 但 32 字节头部的其余部分跨越了块边界。
				b := make([]byte, 2*searchChunkSize)
				copy(b[searchChunkSize-buildInfoAlign:], buildInfoMagic)
				return b
			}(),
			want: searchChunkSize - buildInfoAlign,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			x := &byteExe{tc.data}
			dataAddr, dataSize := x.DataStart()
			addr, err := searchMagic(x, dataAddr, dataSize)
			if tc.wantErr == nil {
				if err != nil {
					t.Errorf("searchMagic got err %v want nil", err)
				}
				if addr != tc.want {
					t.Errorf("searchMagic got addr %d want %d", addr, tc.want)
				}
			} else {
				if err != tc.wantErr {
					t.Errorf("searchMagic got err %v want %v", err, tc.wantErr)
				}
			}
		})
	}
}
