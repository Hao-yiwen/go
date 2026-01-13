// 版权所有 2010 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package image

import (
	"bufio"
	"errors"
	"io"
	"sync"
	"sync/atomic"
)

// ErrFormat 表示解码时遇到了未知格式。
var ErrFormat = errors.New("image: unknown format")

// format 保存图像格式的名称、魔数头和解码方法。
type format struct {
	name, magic  string
	decode       func(io.Reader) (Image, error)
	decodeConfig func(io.Reader) (Config, error)
}

// formats 是已注册格式的列表。
var (
	formatsMu     sync.Mutex
	atomicFormats atomic.Value
)

// RegisterFormat 注册一个图像格式供 [Decode] 使用。
// name 是格式的名称，如 "jpeg" 或 "png"。
// magic 是标识格式编码的魔数前缀。魔数字符串可以包含 "?" 通配符，
// 每个通配符匹配任意一个字节。
// [Decode] 是解码已编码图像的函数。
// [DecodeConfig] 是仅解码其配置的函数。
func RegisterFormat(name, magic string, decode func(io.Reader) (Image, error), decodeConfig func(io.Reader) (Config, error)) {
	formatsMu.Lock()
	formats, _ := atomicFormats.Load().([]format)
	atomicFormats.Store(append(formats, format{name, magic, decode, decodeConfig}))
	formatsMu.Unlock()
}

// reader 是一个可以预览数据的 io.Reader。
type reader interface {
	io.Reader
	Peek(int) ([]byte, error)
}

// asReader 将 io.Reader 转换为 reader。
func asReader(r io.Reader) reader {
	if rr, ok := r.(reader); ok {
		return rr
	}
	return bufio.NewReader(r)
}

// match 报告 magic 是否匹配 b。magic 可以包含 "?" 通配符。
func match(magic string, b []byte) bool {
	if len(magic) != len(b) {
		return false
	}
	for i, c := range b {
		if magic[i] != c && magic[i] != '?' {
			return false
		}
	}
	return true
}

// sniff 确定 r 数据的格式。
func sniff(r reader) format {
	formats, _ := atomicFormats.Load().([]format)
	for _, f := range formats {
		b, err := r.Peek(len(f.magic))
		if err == nil && match(f.magic, b) {
			return f
		}
	}
	return format{}
}

// Decode 解码以已注册格式编码的图像。
// 返回的字符串是格式注册时使用的格式名称。
// 格式注册通常由编解码器特定包中的 init 函数完成。
func Decode(r io.Reader) (Image, string, error) {
	rr := asReader(r)
	f := sniff(rr)
	if f.decode == nil {
		return nil, "", ErrFormat
	}
	m, err := f.decode(rr)
	return m, f.name, err
}

// DecodeConfig 解码以已注册格式编码的图像的颜色模型和尺寸。
// 返回的字符串是格式注册时使用的格式名称。
// 格式注册通常由编解码器特定包中的 init 函数完成。
func DecodeConfig(r io.Reader) (Config, string, error) {
	rr := asReader(r)
	f := sniff(rr)
	if f.decodeConfig == nil {
		return Config{}, "", ErrFormat
	}
	c, err := f.decodeConfig(rr)
	return c, f.name, err
}
