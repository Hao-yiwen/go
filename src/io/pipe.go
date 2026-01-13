// 版权所有 2009 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// 管道适配器，用于连接期望 io.Reader 的代码
// 和期望 io.Writer 的代码。

package io

import (
	"errors"
	"sync"
)

// onceError 是一个只会存储一次错误的对象。
type onceError struct {
	sync.Mutex // 保护以下字段
	err        error
}

func (a *onceError) Store(err error) {
	a.Lock()
	defer a.Unlock()
	if a.err != nil {
		return
	}
	a.err = err
}
func (a *onceError) Load() error {
	a.Lock()
	defer a.Unlock()
	return a.err
}

// ErrClosedPipe 是对已关闭管道进行读取或写入操作时使用的错误。
var ErrClosedPipe = errors.New("io: read/write on closed pipe")

// pipe 是 PipeReader 和 PipeWriter 底层的共享管道结构。
type pipe struct {
	wrMu sync.Mutex // 串行化 Write 操作
	wrCh chan []byte
	rdCh chan int

	once sync.Once // 保护关闭 done
	done chan struct{}
	rerr onceError
	werr onceError
}

func (p *pipe) read(b []byte) (n int, err error) {
	select {
	case <-p.done:
		return 0, p.readCloseError()
	default:
	}

	select {
	case bw := <-p.wrCh:
		nr := copy(b, bw)
		p.rdCh <- nr
		return nr, nil
	case <-p.done:
		return 0, p.readCloseError()
	}
}

func (p *pipe) closeRead(err error) error {
	if err == nil {
		err = ErrClosedPipe
	}
	p.rerr.Store(err)
	p.once.Do(func() { close(p.done) })
	return nil
}

func (p *pipe) write(b []byte) (n int, err error) {
	select {
	case <-p.done:
		return 0, p.writeCloseError()
	default:
		p.wrMu.Lock()
		defer p.wrMu.Unlock()
	}

	for once := true; once || len(b) > 0; once = false {
		select {
		case p.wrCh <- b:
			nw := <-p.rdCh
			b = b[nw:]
			n += nw
		case <-p.done:
			return n, p.writeCloseError()
		}
	}
	return n, nil
}

func (p *pipe) closeWrite(err error) error {
	if err == nil {
		err = EOF
	}
	p.werr.Store(err)
	p.once.Do(func() { close(p.done) })
	return nil
}

// readCloseError 被认为是 pipe 类型的内部方法。
func (p *pipe) readCloseError() error {
	rerr := p.rerr.Load()
	if werr := p.werr.Load(); rerr == nil && werr != nil {
		return werr
	}
	return ErrClosedPipe
}

// writeCloseError 被认为是 pipe 类型的内部方法。
func (p *pipe) writeCloseError() error {
	werr := p.werr.Load()
	if rerr := p.rerr.Load(); werr == nil && rerr != nil {
		return rerr
	}
	return ErrClosedPipe
}

// PipeReader 是管道的读取端。
type PipeReader struct{ pipe }

// Read 实现了标准的 Read 接口：
// 它从管道读取数据，阻塞直到有写入者到来
// 或写入端被关闭。
// 如果写入端因错误而关闭，该错误将作为 err 返回；
// 否则 err 是 EOF。
func (r *PipeReader) Read(data []byte) (n int, err error) {
	return r.pipe.read(data)
}

// Close 关闭读取器；后续对管道写入端的
// 写入将返回错误 [ErrClosedPipe]。
func (r *PipeReader) Close() error {
	return r.CloseWithError(nil)
}

// CloseWithError 关闭读取器；后续对管道写入端的
// 写入将返回错误 err。
//
// 如果先前的错误已存在，CloseWithError 不会覆盖它，
// 并且总是返回 nil。
func (r *PipeReader) CloseWithError(err error) error {
	return r.pipe.closeRead(err)
}

// PipeWriter 是管道的写入端。
type PipeWriter struct{ r PipeReader }

// Write 实现了标准的 Write 接口：
// 它向管道写入数据，阻塞直到一个或多个读取器
// 消耗了所有数据或读取端被关闭。
// 如果读取端因错误而关闭，该错误将作为 err 返回；
// 否则 err 是 [ErrClosedPipe]。
func (w *PipeWriter) Write(data []byte) (n int, err error) {
	return w.r.pipe.write(data)
}

// Close 关闭写入器；后续从管道读取端的
// 读取将返回零字节和 EOF。
func (w *PipeWriter) Close() error {
	return w.CloseWithError(nil)
}

// CloseWithError 关闭写入器；后续从管道读取端的
// 读取将返回零字节和错误 err，
// 如果 err 为 nil 则返回 EOF。
//
// 如果先前的错误已存在，CloseWithError 不会覆盖它，
// 并且总是返回 nil。
func (w *PipeWriter) CloseWithError(err error) error {
	return w.r.pipe.closeWrite(err)
}

// Pipe 创建一个同步的内存管道。
// 它可以用来连接期望 [io.Reader] 的代码
// 和期望 [io.Writer] 的代码。
//
// 管道上的读取和写入是一对一匹配的，
// 除非需要多次读取来消耗单次写入。
// 也就是说，对 [PipeWriter] 的每次写入都会阻塞，
// 直到它满足了来自 [PipeReader] 的一次或多次读取，
// 完全消耗了写入的数据。
// 数据直接从 Write 复制到对应的 Read（或多次 Read）；
// 没有内部缓冲。
//
// 并行调用 Read 和 Write，或者与 Close 并行调用是安全的。
// 并行调用 Read 和并行调用 Write 也是安全的：
// 各个调用将按顺序进行控制。
func Pipe() (*PipeReader, *PipeWriter) {
	pw := &PipeWriter{r: PipeReader{pipe: pipe{
		wrCh: make(chan []byte),
		rdCh: make(chan int),
		done: make(chan struct{}),
	}}}
	return &pw.r, pw
}
