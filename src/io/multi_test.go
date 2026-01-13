// 版权所有 2010 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package io_test

import (
	"bytes"
	"crypto/sha1"
	"errors"
	"fmt"
	. "io"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestMultiReader(t *testing.T) {
	var mr Reader
	var buf []byte
	nread := 0
	withFooBar := func(tests func()) {
		r1 := strings.NewReader("foo ")
		r2 := strings.NewReader("")
		r3 := strings.NewReader("bar")
		mr = MultiReader(r1, r2, r3)
		buf = make([]byte, 20)
		tests()
	}
	expectRead := func(size int, expected string, eerr error) {
		nread++
		n, gerr := mr.Read(buf[0:size])
		if n != len(expected) {
			t.Errorf("#%d, expected %d bytes; got %d",
				nread, len(expected), n)
		}
		got := string(buf[0:n])
		if got != expected {
			t.Errorf("#%d, expected %q; got %q",
				nread, expected, got)
		}
		if gerr != eerr {
			t.Errorf("#%d, expected error %v; got %v",
				nread, eerr, gerr)
		}
		buf = buf[n:]
	}
	withFooBar(func() {
		expectRead(2, "fo", nil)
		expectRead(5, "o ", nil)
		expectRead(5, "bar", nil)
		expectRead(5, "", EOF)
	})
	withFooBar(func() {
		expectRead(4, "foo ", nil)
		expectRead(1, "b", nil)
		expectRead(3, "ar", nil)
		expectRead(1, "", EOF)
	})
	withFooBar(func() {
		expectRead(5, "foo ", nil)
	})
}

func TestMultiReaderAsWriterTo(t *testing.T) {
	mr := MultiReader(
		strings.NewReader("foo "),
		MultiReader( // 触发缓冲区复用代码路径
			strings.NewReader(""),
			strings.NewReader("bar"),
		),
	)
	mrAsWriterTo, ok := mr.(WriterTo)
	if !ok {
		t.Fatalf("expected cast to WriterTo to succeed")
	}
	sink := &strings.Builder{}
	n, err := mrAsWriterTo.WriteTo(sink)
	if err != nil {
		t.Fatalf("expected no error; got %v", err)
	}
	if n != 7 {
		t.Errorf("expected read 7 bytes; got %d", n)
	}
	if result := sink.String(); result != "foo bar" {
		t.Errorf(`expected "foo bar"; got %q`, result)
	}
}

func TestMultiWriter(t *testing.T) {
	sink := new(bytes.Buffer)
	// 隐藏 bytes.Buffer 的 WriteString 方法：
	testMultiWriter(t, struct {
		Writer
		fmt.Stringer
	}{sink, sink})
}

func TestMultiWriter_String(t *testing.T) {
	testMultiWriter(t, new(bytes.Buffer))
}

// 测试 multiWriter.WriteString 调用最多产生 1 次分配，
// 即使多个目标不支持 WriteString。
func TestMultiWriter_WriteStringSingleAlloc(t *testing.T) {
	var sink1, sink2 bytes.Buffer
	type simpleWriter struct { // 隐藏 bytes.Buffer 的 WriteString
		Writer
	}
	mw := MultiWriter(simpleWriter{&sink1}, simpleWriter{&sink2})
	allocs := int(testing.AllocsPerRun(1000, func() {
		WriteString(mw, "foo")
	}))
	if allocs != 1 {
		t.Errorf("num allocations = %d; want 1", allocs)
	}
}

type writeStringChecker struct{ called bool }

func (c *writeStringChecker) WriteString(s string) (n int, err error) {
	c.called = true
	return len(s), nil
}

func (c *writeStringChecker) Write(p []byte) (n int, err error) {
	return len(p), nil
}

func TestMultiWriter_StringCheckCall(t *testing.T) {
	var c writeStringChecker
	mw := MultiWriter(&c)
	WriteString(mw, "foo")
	if !c.called {
		t.Error("did not see WriteString call to writeStringChecker")
	}
}

func testMultiWriter(t *testing.T, sink interface {
	Writer
	fmt.Stringer
}) {
	sha1 := sha1.New()
	mw := MultiWriter(sha1, sink)

	sourceString := "My input text."
	source := strings.NewReader(sourceString)
	written, err := Copy(mw, source)

	if written != int64(len(sourceString)) {
		t.Errorf("short write of %d, not %d", written, len(sourceString))
	}

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	sha1hex := fmt.Sprintf("%x", sha1.Sum(nil))
	if sha1hex != "01cb303fa8c30a64123067c5aa6284ba7ec2d31b" {
		t.Error("incorrect sha1 value")
	}

	if sink.String() != sourceString {
		t.Errorf("expected %q; got %q", sourceString, sink.String())
	}
}

// writerFunc 是由底层函数实现的 Writer。
type writerFunc func(p []byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) {
	return f(p)
}

// 测试 MultiWriter 正确展平链式 multiWriters。
func TestMultiWriterSingleChainFlatten(t *testing.T) {
	pc := make([]uintptr, 1000) // 1000 应该能容纳完整的调用栈
	n := runtime.Callers(0, pc)
	var myDepth = callDepth(pc[:n])
	var writeDepth int // 将包含调用 writerFunc.Writer 时的深度
	var w Writer = MultiWriter(writerFunc(func(p []byte) (int, error) {
		n := runtime.Callers(1, pc)
		writeDepth += callDepth(pc[:n])
		return 0, nil
	}))

	mw := w
	// 链接一堆 multiWriters
	for i := 0; i < 100; i++ {
		mw = MultiWriter(w)
	}

	mw = MultiWriter(w, mw, w, mw)
	mw.Write(nil) // 不关心错误，只想检查 Write 的调用深度

	if writeDepth != 4*(myDepth+2) { // 2 应该是 multiWriter.Write 和 writerFunc.Write
		t.Errorf("multiWriter did not flatten chained multiWriters: expected writeDepth %d, got %d",
			4*(myDepth+2), writeDepth)
	}
}

func TestMultiWriterError(t *testing.T) {
	f1 := writerFunc(func(p []byte) (int, error) {
		return len(p) / 2, ErrShortWrite
	})
	f2 := writerFunc(func(p []byte) (int, error) {
		t.Errorf("MultiWriter called f2.Write")
		return len(p), nil
	})
	w := MultiWriter(f1, f2)
	n, err := w.Write(make([]byte, 100))
	if n != 50 || err != ErrShortWrite {
		t.Errorf("Write = %d, %v, want 50, ErrShortWrite", n, err)
	}
}

// 测试 MultiReader 复制输入切片并与未来的修改隔离。
func TestMultiReaderCopy(t *testing.T) {
	slice := []Reader{strings.NewReader("hello world")}
	r := MultiReader(slice...)
	slice[0] = nil
	data, err := ReadAll(r)
	if err != nil || string(data) != "hello world" {
		t.Errorf("ReadAll() = %q, %v, want %q, nil", data, err, "hello world")
	}
}

// 测试 MultiWriter 复制输入切片并与未来的修改隔离。
func TestMultiWriterCopy(t *testing.T) {
	var buf strings.Builder
	slice := []Writer{&buf}
	w := MultiWriter(slice...)
	slice[0] = nil
	n, err := w.Write([]byte("hello world"))
	if err != nil || n != 11 {
		t.Errorf("Write(`hello world`) = %d, %v, want 11, nil", n, err)
	}
	if buf.String() != "hello world" {
		t.Errorf("buf.String() = %q, want %q", buf.String(), "hello world")
	}
}

// readerFunc 是由底层函数实现的 Reader。
type readerFunc func(p []byte) (int, error)

func (f readerFunc) Read(p []byte) (int, error) {
	return f(p)
}

// callDepth 返回给定 PC 的逻辑调用深度。
func callDepth(callers []uintptr) (depth int) {
	frames := runtime.CallersFrames(callers)
	more := true
	for more {
		_, more = frames.Next()
		depth++
	}
	return
}

// 测试当调用 Read 时 MultiReader 正确展平链式 multiReaders
func TestMultiReaderFlatten(t *testing.T) {
	pc := make([]uintptr, 1000) // 1000 应该能容纳完整的调用栈
	n := runtime.Callers(0, pc)
	var myDepth = callDepth(pc[:n])
	var readDepth int // 将包含调用 fakeReader.Read 时的深度
	var r Reader = MultiReader(readerFunc(func(p []byte) (int, error) {
		n := runtime.Callers(1, pc)
		readDepth = callDepth(pc[:n])
		return 0, errors.New("irrelevant")
	}))

	// 链接一堆 multiReaders
	for i := 0; i < 100; i++ {
		r = MultiReader(r)
	}

	r.Read(nil) // 不关心错误，只想检查 Read 的调用深度

	if readDepth != myDepth+2 { // 2 应该是 multiReader.Read 和 fakeReader.Read
		t.Errorf("multiReader did not flatten chained multiReaders: expected readDepth %d, got %d",
			myDepth+2, readDepth)
	}
}

// byteAndEOFReader 是一个 Reader，在其 Read 调用中同时读取一个字节
// （底层字节）和 EOF。
type byteAndEOFReader byte

func (b byteAndEOFReader) Read(p []byte) (n int, err error) {
	if len(p) == 0 {
		// Read(0 bytes) 是无用的。我们预期在此测试中
		// 不会有这样无用的调用。
		panic("unexpected call")
	}
	p[0] = byte(b)
	return 1, EOF
}

// 这曾经会永远产生字节；issue 16795。
func TestMultiReaderSingleByteWithEOF(t *testing.T) {
	got, err := ReadAll(LimitReader(MultiReader(byteAndEOFReader('a'), byteAndEOFReader('b')), 10))
	if err != nil {
		t.Fatal(err)
	}
	const want = "ab"
	if string(got) != want {
		t.Errorf("got %q; want %q", got, want)
	}
}

// 测试在 MultiReader 链末尾返回 (n, EOF) 的 reader
// 在其最后一次读取时继续返回 EOF，而不是
// 产生 (0, EOF)。
func TestMultiReaderFinalEOF(t *testing.T) {
	r := MultiReader(bytes.NewReader(nil), byteAndEOFReader('a'))
	buf := make([]byte, 2)
	n, err := r.Read(buf)
	if n != 1 || err != EOF {
		t.Errorf("got %v, %v; want 1, EOF", n, err)
	}
}

func TestMultiReaderFreesExhaustedReaders(t *testing.T) {
	var mr Reader
	closed := make(chan struct{})
	// 闭包确保在 MultiReader 被内联后我们在栈上没有对 buf1 的
	// 活引用（Issue 18819）。这是对活性分析限制的一个变通方法。
	func() {
		buf1 := bytes.NewReader([]byte("foo"))
		buf2 := bytes.NewReader([]byte("bar"))
		mr = MultiReader(buf1, buf2)
		runtime.AddCleanup(buf1, func(ch chan struct{}) { close(ch) }, closed)
	}()

	buf := make([]byte, 4)
	if n, err := ReadFull(mr, buf); err != nil || string(buf) != "foob" {
		t.Fatalf(`ReadFull = %d (%q), %v; want 3, "foo", nil`, n, buf[:n], err)
	}

	runtime.GC()
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for collection of buf1")
	}

	if n, err := ReadFull(mr, buf[:2]); err != nil || string(buf[:2]) != "ar" {
		t.Fatalf(`ReadFull = %d (%q), %v; want 2, "ar", nil`, n, buf[:n], err)
	}
}

func TestInterleavedMultiReader(t *testing.T) {
	r1 := strings.NewReader("123")
	r2 := strings.NewReader("45678")

	mr1 := MultiReader(r1, r2)
	mr2 := MultiReader(mr1)

	buf := make([]byte, 4)

	// 让 mr2 使用 mr1 的 []Readers。
	// 消耗 r1（并清除它让 GC 处理）并消耗 r2 的一部分。
	n, err := ReadFull(mr2, buf)
	if got := string(buf[:n]); got != "1234" || err != nil {
		t.Errorf(`ReadFull(mr2) = (%q, %v), want ("1234", nil)`, got, err)
	}

	// 通过 mr1 消耗 r2 的剩余部分。
	// 即使 mr2 清除了 r1，这也不应该 panic。
	n, err = ReadFull(mr1, buf)
	if got := string(buf[:n]); got != "5678" || err != nil {
		t.Errorf(`ReadFull(mr1) = (%q, %v), want ("5678", nil)`, got, err)
	}
}
