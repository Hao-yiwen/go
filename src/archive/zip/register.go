// 版权所有 2010 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package zip

import (
	"compress/flate"
	"errors"
	"io"
	"sync"
)

// Compressor 返回一个新的压缩写入器，写入到 w。
// WriteCloser 的 Close 方法必须用于将待处理数据刷新到 w。
// Compressor 本身必须可以安全地从多个 goroutine 同时调用，
// 但每个返回的写入器一次只会被一个 goroutine 使用。
type Compressor func(w io.Writer) (io.WriteCloser, error)

// Decompressor 返回一个新的解压缩读取器，从 r 读取。
// [io.ReadCloser] 的 Close 方法必须用于释放关联的资源。
// Decompressor 本身必须可以安全地从多个 goroutine 同时调用，
// 但每个返回的读取器一次只会被一个 goroutine 使用。
type Decompressor func(r io.Reader) io.ReadCloser

var flateWriterPool sync.Pool

func newFlateWriter(w io.Writer) io.WriteCloser {
	fw, ok := flateWriterPool.Get().(*flate.Writer)
	if ok {
		fw.Reset(w)
	} else {
		fw, _ = flate.NewWriter(w, 5)
	}
	return &pooledFlateWriter{fw: fw}
}

type pooledFlateWriter struct {
	mu sync.Mutex // 保护 Close 和 Write
	fw *flate.Writer
}

func (w *pooledFlateWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.fw == nil {
		return 0, errors.New("Write after Close")
	}
	return w.fw.Write(p)
}

func (w *pooledFlateWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	var err error
	if w.fw != nil {
		err = w.fw.Close()
		flateWriterPool.Put(w.fw)
		w.fw = nil
	}
	return err
}

var flateReaderPool sync.Pool

func newFlateReader(r io.Reader) io.ReadCloser {
	fr, ok := flateReaderPool.Get().(io.ReadCloser)
	if ok {
		fr.(flate.Resetter).Reset(r, nil)
	} else {
		fr = flate.NewReader(r)
	}
	return &pooledFlateReader{fr: fr}
}

type pooledFlateReader struct {
	mu sync.Mutex // 保护 Close 和 Read
	fr io.ReadCloser
}

func (r *pooledFlateReader) Read(p []byte) (n int, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.fr == nil {
		return 0, errors.New("Read after Close")
	}
	return r.fr.Read(p)
}

func (r *pooledFlateReader) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	var err error
	if r.fr != nil {
		err = r.fr.Close()
		flateReaderPool.Put(r.fr)
		r.fr = nil
	}
	return err
}

var (
	compressors   sync.Map // map[uint16]Compressor
	decompressors sync.Map // map[uint16]Decompressor
)

func init() {
	compressors.Store(Store, Compressor(func(w io.Writer) (io.WriteCloser, error) { return &nopCloser{w}, nil }))
	compressors.Store(Deflate, Compressor(func(w io.Writer) (io.WriteCloser, error) { return newFlateWriter(w), nil }))

	decompressors.Store(Store, Decompressor(io.NopCloser))
	decompressors.Store(Deflate, Decompressor(newFlateReader))
}

// RegisterDecompressor 允许为指定的方法 ID 注册自定义解压器。
// 通用方法 [Store] 和 [Deflate] 是内置的。
func RegisterDecompressor(method uint16, dcomp Decompressor) {
	if _, dup := decompressors.LoadOrStore(method, dcomp); dup {
		panic("decompressor already registered")
	}
}

// RegisterCompressor 为指定的方法 ID 注册自定义压缩器。
// 通用方法 [Store] 和 [Deflate] 是内置的。
func RegisterCompressor(method uint16, comp Compressor) {
	if _, dup := compressors.LoadOrStore(method, comp); dup {
		panic("compressor already registered")
	}
}

func compressor(method uint16) Compressor {
	ci, ok := compressors.Load(method)
	if !ok {
		return nil
	}
	return ci.(Compressor)
}

func decompressor(method uint16) Decompressor {
	di, ok := decompressors.Load(method)
	if !ok {
		return nil
	}
	return di.(Decompressor)
}
