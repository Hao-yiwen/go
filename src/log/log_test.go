// 版权所有 2009 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package log

// 这些测试太简单了。

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"internal/testenv"
	"io"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	Rdate         = `[0-9][0-9][0-9][0-9]/[0-9][0-9]/[0-9][0-9]`
	Rtime         = `[0-9][0-9]:[0-9][0-9]:[0-9][0-9]`
	Rmicroseconds = `\.[0-9][0-9][0-9][0-9][0-9][0-9]`
	Rline         = `(67|69):` // 如果下面对 l.Printf / l.Print 的调用移动了位置，必须更新此值
	Rlongfile     = `.*/[A-Za-z0-9_\-]+\.go:` + Rline
	Rshortfile    = `[A-Za-z0-9_\-]+\.go:` + Rline
)

type tester struct {
	flag    int
	prefix  string
	pattern string // 日志输出必须匹配的正则表达式；我们总是添加 ^ 和 expected_text$
}

var tests = []tester{
	// 单独的部分：
	{0, "", ""},
	{0, "XXX", "XXX"},
	{Ldate, "", Rdate + " "},
	{Ltime, "", Rtime + " "},
	{Ltime | Lmsgprefix, "XXX", Rtime + " XXX"},
	{Ltime | Lmicroseconds, "", Rtime + Rmicroseconds + " "},
	{Lmicroseconds, "", Rtime + Rmicroseconds + " "}, // 微秒隐含时间
	{Llongfile, "", Rlongfile + " "},
	{Lshortfile, "", Rshortfile + " "},
	{Llongfile | Lshortfile, "", Rshortfile + " "}, // shortfile 覆盖 longfile
	// 全部一起：
	{Ldate | Ltime | Lmicroseconds | Llongfile, "XXX", "XXX" + Rdate + " " + Rtime + Rmicroseconds + " " + Rlongfile + " "},
	{Ldate | Ltime | Lmicroseconds | Lshortfile, "XXX", "XXX" + Rdate + " " + Rtime + Rmicroseconds + " " + Rshortfile + " "},
	{Ldate | Ltime | Lmicroseconds | Llongfile | Lmsgprefix, "XXX", Rdate + " " + Rtime + Rmicroseconds + " " + Rlongfile + " XXX"},
	{Ldate | Ltime | Lmicroseconds | Lshortfile | Lmsgprefix, "XXX", Rdate + " " + Rtime + Rmicroseconds + " " + Rshortfile + " XXX"},
}

// 使用 Println("hello", 23, "world") 或使用 Printf("hello %d world", 23) 进行测试
func testPrint(t *testing.T, flag int, prefix string, pattern string, useFormat bool) {
	buf := new(strings.Builder)
	SetOutput(buf)
	SetFlags(flag)
	SetPrefix(prefix)
	if useFormat {
		Printf("hello %d world", 23)
	} else {
		Println("hello", 23, "world")
	}
	line := buf.String()
	line = line[0 : len(line)-1]
	pattern = "^" + pattern + "hello 23 world$"
	matched, err := regexp.MatchString(pattern, line)
	if err != nil {
		t.Fatal("pattern did not compile:", err)
	}
	if !matched {
		t.Errorf("log output should match %q is %q", pattern, line)
	}
	SetOutput(os.Stderr)
}

func TestDefault(t *testing.T) {
	if got := Default(); got != std {
		t.Errorf("Default [%p] should be std [%p]", got, std)
	}
}

func TestAll(t *testing.T) {
	for _, testcase := range tests {
		testPrint(t, testcase.flag, testcase.prefix, testcase.pattern, false)
		testPrint(t, testcase.flag, testcase.prefix, testcase.pattern, true)
	}
}

func TestOutput(t *testing.T) {
	const testString = "test"
	var b strings.Builder
	l := New(&b, "", 0)
	l.Println(testString)
	if expect := testString + "\n"; b.String() != expect {
		t.Errorf("log output should match %q is %q", expect, b.String())
	}
}

func TestNonNewLogger(t *testing.T) {
	var l Logger
	l.SetOutput(new(bytes.Buffer)) // 初始化 Logger 所需的最小工作量
	l.Print("hello")
}

func TestOutputRace(t *testing.T) {
	var b bytes.Buffer
	l := New(&b, "", 0)
	var wg sync.WaitGroup
	wg.Add(100)
	for i := 0; i < 100; i++ {
		go func() {
			defer wg.Done()
			l.SetFlags(0)
			l.Output(0, "")
		}()
	}
	wg.Wait()
}

func TestFlagAndPrefixSetting(t *testing.T) {
	var b bytes.Buffer
	l := New(&b, "Test:", LstdFlags)
	f := l.Flags()
	if f != LstdFlags {
		t.Errorf("Flags 1: expected %x got %x", LstdFlags, f)
	}
	l.SetFlags(f | Lmicroseconds)
	f = l.Flags()
	if f != LstdFlags|Lmicroseconds {
		t.Errorf("Flags 2: expected %x got %x", LstdFlags|Lmicroseconds, f)
	}
	p := l.Prefix()
	if p != "Test:" {
		t.Errorf(`Prefix: expected "Test:" got %q`, p)
	}
	l.SetPrefix("Reality:")
	p = l.Prefix()
	if p != "Reality:" {
		t.Errorf(`Prefix: expected "Reality:" got %q`, p)
	}
	// 验证日志消息看起来正确，包含我们的前缀和微秒。
	l.Print("hello")
	pattern := "^Reality:" + Rdate + " " + Rtime + Rmicroseconds + " hello\n"
	matched, err := regexp.Match(pattern, b.Bytes())
	if err != nil {
		t.Fatalf("pattern %q did not compile: %s", pattern, err)
	}
	if !matched {
		t.Error("message did not match pattern")
	}

	// 确保只有当缓冲区缺少换行符后缀时才添加换行符。
	b.Reset()
	l.SetFlags(0)
	l.SetPrefix("\n")
	l.Output(0, "")
	if got := b.String(); got != "\n" {
		t.Errorf("message mismatch:\ngot  %q\nwant %q", got, "\n")
	}
}

func TestUTCFlag(t *testing.T) {
	var b strings.Builder
	l := New(&b, "Test:", LstdFlags)
	l.SetFlags(Ldate | Ltime | LUTC)
	// 验证日志消息在正确的时区中看起来正确。仅精确到秒。
	now := time.Now().UTC()
	l.Print("hello")
	want := fmt.Sprintf("Test:%d/%.2d/%.2d %.2d:%.2d:%.2d hello\n",
		now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute(), now.Second())
	got := b.String()
	if got == want {
		return
	}
	// 在获取 now 和记录日志之间，我们可能跨越了一秒的边界，
	// 所以加一秒然后重试。这几乎总是能成功。
	now = now.Add(time.Second)
	want = fmt.Sprintf("Test:%d/%.2d/%.2d %.2d:%.2d:%.2d hello\n",
		now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute(), now.Second())
	if got == want {
		return
	}
	t.Errorf("got %q; want %q", got, want)
}

func TestEmptyPrintCreatesLine(t *testing.T) {
	var b strings.Builder
	l := New(&b, "Header:", LstdFlags)
	l.Print()
	l.Println("non-empty")
	output := b.String()
	if n := strings.Count(output, "Header"); n != 2 {
		t.Errorf("expected 2 headers, got %d", n)
	}
	if n := strings.Count(output, "\n"); n != 2 {
		t.Errorf("expected 2 lines, got %d", n)
	}
}

func TestDiscard(t *testing.T) {
	l := New(io.Discard, "", 0)
	s := strings.Repeat("a", 102400)
	c := testing.AllocsPerRun(100, func() { l.Printf("%s", s) })
	// 传递给 Printf 的切片有一次分配，
	// 但格式化长字符串没有分配。
	if c > 1 {
		t.Errorf("got %v allocs, want at most 1", c)
	}
}

func TestCallDepth(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	testenv.MustHaveExec(t)
	ep, err := os.Executable()
	if err != nil {
		t.Fatalf("Executable failed: %v", err)
	}

	tests := []struct {
		name string
		log  func()
	}{
		{"Fatal", func() { Fatal("Fatal") }},
		{"Fatalf", func() { Fatalf("Fatalf") }},
		{"Fatalln", func() { Fatalln("Fatalln") }},
		{"Output", func() { Output(1, "Output") }},
		{"Panic", func() { Panic("Panic") }},
		{"Panicf", func() { Panicf("Panicf") }},
		{"Panicln", func() { Panicf("Panicln") }},
		{"Default.Fatal", func() { Default().Fatal("Default.Fatal") }},
		{"Default.Fatalf", func() { Default().Fatalf("Default.Fatalf") }},
		{"Default.Fatalln", func() { Default().Fatalln("Default.Fatalln") }},
		{"Default.Output", func() { Default().Output(1, "Default.Output") }},
		{"Default.Panic", func() { Default().Panic("Default.Panic") }},
		{"Default.Panicf", func() { Default().Panicf("Default.Panicf") }},
		{"Default.Panicln", func() { Default().Panicf("Default.Panicln") }},
	}

	// 计算到第一个测试用例的行偏移量
	_, _, line, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller failed")
	}
	line -= len(tests) + 3

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 其中一些调用使用 os.Exit() 来生成命令并捕获输出
			const envVar = "LOGTEST_CALL_DEPTH"
			if os.Getenv(envVar) == "1" {
				SetFlags(Lshortfile)
				tt.log()
				os.Exit(1)
			}

			// 生成测试可执行文件
			cmd := testenv.Command(t, ep,
				"-test.run=^"+regexp.QuoteMeta(t.Name())+"$",
				"-test.count=1",
			)
			cmd.Env = append(cmd.Environ(), envVar+"=1")

			out, err := cmd.CombinedOutput()
			if _, ok := errors.AsType[*exec.ExitError](err); !ok {
				t.Fatalf("expected exec.ExitError: %v", err)
			}

			_, firstLine, err := bufio.ScanLines(out, true)
			if err != nil {
				t.Fatalf("failed to split line: %v", err)
			}
			got := string(firstLine)

			want := fmt.Sprintf(
				"log_test.go:%d: %s",
				line+i, tt.name,
			)
			if got != want {
				t.Errorf(
					"output from %s() mismatch:\n\t got: %s\n\twant: %s",
					tt.name, got, want,
				)
			}
		})
	}
}

func BenchmarkItoa(b *testing.B) {
	dst := make([]byte, 0, 64)
	for i := 0; i < b.N; i++ {
		dst = dst[0:0]
		itoa(&dst, 2015, 4)   // year
		itoa(&dst, 1, 2)      // month
		itoa(&dst, 30, 2)     // day
		itoa(&dst, 12, 2)     // hour
		itoa(&dst, 56, 2)     // minute
		itoa(&dst, 0, 2)      // second
		itoa(&dst, 987654, 6) // microsecond
	}
}

func BenchmarkPrintln(b *testing.B) {
	const testString = "test"
	var buf bytes.Buffer
	l := New(&buf, "", LstdFlags)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		l.Println(testString)
	}
}

func BenchmarkPrintlnNoFlags(b *testing.B) {
	const testString = "test"
	var buf bytes.Buffer
	l := New(&buf, "", 0)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		l.Println(testString)
	}
}

// discard 与 io.Discard 相同，
// 但复制到这里是为了避免 Logger 中的 io.Discard 优化。
type discard struct{}

func (discard) Write(p []byte) (int, error) {
	return len(p), nil
}

func BenchmarkConcurrent(b *testing.B) {
	l := New(discard{}, "prefix: ", Ldate|Ltime|Lmicroseconds|Llongfile|Lmsgprefix)
	var group sync.WaitGroup
	for i := runtime.NumCPU(); i > 0; i-- {
		group.Add(1)
		go func() {
			for i := 0; i < b.N; i++ {
				l.Output(0, "hello, world!")
			}
			defer group.Done()
		}()
	}
	group.Wait()
}

func BenchmarkDiscard(b *testing.B) {
	l := New(io.Discard, "", LstdFlags|Lshortfile)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		l.Printf("processing %d objects from bucket %q", 1234, "fizzbuzz")
	}
}
