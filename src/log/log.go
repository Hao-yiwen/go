// 版权所有 2009 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// log 包实现了一个简单的日志记录包。它定义了一个 [Logger] 类型，
// 该类型具有格式化输出的方法。它还有一个预定义的"标准"日志记录器，
// 可以通过辅助函数 Print[f|ln]、Fatal[f|ln] 和 Panic[f|ln] 来访问，
// 这比手动创建 Logger 更容易使用。
// 该日志记录器写入标准错误输出，并打印每条日志消息的日期和时间。
// 每条日志消息输出在单独的一行：如果正在打印的消息不以换行符结尾，
// 日志记录器会添加一个换行符。
// Fatal 函数在写入日志消息后调用 [os.Exit](1)。
// Panic 函数在写入日志消息后调用 panic。
package log

import (
	"fmt"
	"io"
	"log/internal"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// 这些标志定义了在 [Logger] 生成的每条日志条目前面添加什么文本。
// 这些位通过或运算组合在一起来控制打印的内容。
// 除了 Lmsgprefix 标志外，无法控制它们出现的顺序（此处列出的顺序）
// 或它们呈现的格式（如注释中所述）。
// 只有在指定 Llongfile 或 Lshortfile 时，前缀后面才会跟冒号。
// 例如，标志 Ldate | Ltime（或 LstdFlags）产生：
//
//	2009/01/23 01:23:23 message
//
// 而标志 Ldate | Ltime | Lmicroseconds | Llongfile 产生：
//
//	2009/01/23 01:23:23.123123 /a/b/c/d.go:23: message
const (
	Ldate         = 1 << iota     // 本地时区的日期：2009/01/23
	Ltime                         // 本地时区的时间：01:23:23
	Lmicroseconds                 // 微秒精度：01:23:23.123123，需要同时设置 Ltime
	Llongfile                     // 完整文件名和行号：/a/b/c/d.go:23
	Lshortfile                    // 最终文件名元素和行号：d.go:23，会覆盖 Llongfile
	LUTC                          // 如果设置了 Ldate 或 Ltime，则使用 UTC 而不是本地时区
	Lmsgprefix                    // 将"前缀"从行首移到消息之前
	LstdFlags     = Ldate | Ltime // 标准日志记录器的初始值
)

// Logger 表示一个活动的日志记录对象，它向 [io.Writer] 生成输出行。
// 每个日志记录操作都会对 Writer 的 Write 方法进行一次调用。
// Logger 可以同时从多个 goroutine 中使用；它保证对 Writer 的访问是串行化的。
type Logger struct {
	outMu sync.Mutex
	out   io.Writer // 输出目标

	prefix    atomic.Pointer[string] // 每行的前缀，用于标识日志记录器（但请参见 Lmsgprefix）
	flag      atomic.Int32           // 属性
	isDiscard atomic.Bool
}

// New 创建一个新的 [Logger]。out 变量设置日志数据将写入的目标。
// 前缀出现在每个生成的日志行的开头，或者如果提供了 [Lmsgprefix] 标志，
// 则出现在日志头之后。
// flag 参数定义日志记录属性。
func New(out io.Writer, prefix string, flag int) *Logger {
	l := new(Logger)
	l.SetOutput(out)
	l.SetPrefix(prefix)
	l.SetFlags(flag)
	return l
}

// SetOutput 设置日志记录器的输出目标。
func (l *Logger) SetOutput(w io.Writer) {
	l.outMu.Lock()
	defer l.outMu.Unlock()
	l.out = w
	l.isDiscard.Store(w == io.Discard)
}

var std = New(os.Stderr, "", LstdFlags)

// Default 返回包级别输出函数使用的标准日志记录器。
func Default() *Logger { return std }

// 将整数快速转换为固定宽度的十进制 ASCII。传入负宽度以避免零填充。
func itoa(buf *[]byte, i int, wid int) {
	// 以逆序组装十进制数。
	var b [20]byte
	bp := len(b) - 1
	for i >= 10 || wid > 1 {
		wid--
		q := i / 10
		b[bp] = byte('0' + i - q*10)
		bp--
		i = q
	}
	// i < 10
	b[bp] = byte('0' + i)
	*buf = append(*buf, b[bp:]...)
}

// formatHeader 按以下顺序将日志头写入 buf：
//   - l.prefix（如果不为空且未设置 Lmsgprefix），
//   - 日期和/或时间（如果提供了相应的标志），
//   - 文件名和行号（如果提供了相应的标志），
//   - l.prefix（如果不为空且设置了 Lmsgprefix）。
func formatHeader(buf *[]byte, t time.Time, prefix string, flag int, file string, line int) {
	if flag&Lmsgprefix == 0 {
		*buf = append(*buf, prefix...)
	}
	if flag&(Ldate|Ltime|Lmicroseconds) != 0 {
		if flag&LUTC != 0 {
			t = t.UTC()
		}
		if flag&Ldate != 0 {
			year, month, day := t.Date()
			itoa(buf, year, 4)
			*buf = append(*buf, '/')
			itoa(buf, int(month), 2)
			*buf = append(*buf, '/')
			itoa(buf, day, 2)
			*buf = append(*buf, ' ')
		}
		if flag&(Ltime|Lmicroseconds) != 0 {
			hour, min, sec := t.Clock()
			itoa(buf, hour, 2)
			*buf = append(*buf, ':')
			itoa(buf, min, 2)
			*buf = append(*buf, ':')
			itoa(buf, sec, 2)
			if flag&Lmicroseconds != 0 {
				*buf = append(*buf, '.')
				itoa(buf, t.Nanosecond()/1e3, 6)
			}
			*buf = append(*buf, ' ')
		}
	}
	if flag&(Lshortfile|Llongfile) != 0 {
		if flag&Lshortfile != 0 {
			short := file
			for i := len(file) - 1; i > 0; i-- {
				if file[i] == '/' {
					short = file[i+1:]
					break
				}
			}
			file = short
		}
		*buf = append(*buf, file...)
		*buf = append(*buf, ':')
		itoa(buf, line, -1)
		*buf = append(*buf, ": "...)
	}
	if flag&Lmsgprefix != 0 {
		*buf = append(*buf, prefix...)
	}
}

var bufferPool = sync.Pool{New: func() any { return new([]byte) }}

func getBuffer() *[]byte {
	p := bufferPool.Get().(*[]byte)
	*p = (*p)[:0]
	return p
}

func putBuffer(p *[]byte) {
	// sync.Pool 的正确用法要求每个条目具有大致相同的内存开销。
	// 当存储类型包含可变大小的缓冲区时，为了获得此属性，
	// 我们对放回池中的最大缓冲区添加了硬限制。
	//
	// 参见 https://go.dev/issue/23199
	if cap(*p) > 64<<10 {
		*p = nil
	}
	bufferPool.Put(p)
}

// Output 为日志事件写入输出。字符串 s 包含要在 Logger 标志指定的
// 前缀之后打印的文本。如果 s 的最后一个字符不是换行符，则会追加一个换行符。
// calldepth 用于恢复 PC，为了通用性而提供，尽管目前在所有预定义的
// 路径上它都是 2。
func (l *Logger) Output(calldepth int, s string) error {
	return l.output(0, calldepth+1, func(b []byte) []byte { // +1 用于此帧。
		return append(b, s...)
	})
}

// output 可以接受 calldepth 或 pc 来获取源代码行信息。
// 如果 pc 非零，则使用 pc。
func (l *Logger) output(pc uintptr, calldepth int, appendOutput func([]byte) []byte) error {
	if l.isDiscard.Load() {
		return nil
	}

	now := time.Now() // 尽早获取时间。

	// 加载一次 prefix 和 flag，以便在此调用中它们的值保持一致，
	// 无论它们的值是否有任何并发更改。
	prefix := l.Prefix()
	flag := l.Flags()

	var file string
	var line int
	if flag&(Lshortfile|Llongfile) != 0 {
		if pc == 0 {
			var ok bool
			_, file, line, ok = runtime.Caller(calldepth)
			if !ok {
				file = "???"
				line = 0
			}
		} else {
			fs := runtime.CallersFrames([]uintptr{pc})
			f, _ := fs.Next()
			file = f.File
			if file == "" {
				file = "???"
			}
			line = f.Line
		}
	}

	buf := getBuffer()
	defer putBuffer(buf)
	formatHeader(buf, now, prefix, flag, file, line)
	*buf = appendOutput(*buf)
	if len(*buf) == 0 || (*buf)[len(*buf)-1] != '\n' {
		*buf = append(*buf, '\n')
	}

	l.outMu.Lock()
	defer l.outMu.Unlock()
	_, err := l.out.Write(*buf)
	return err
}

func init() {
	internal.DefaultOutput = func(pc uintptr, data []byte) error {
		return std.output(pc, 0, func(buf []byte) []byte {
			return append(buf, data...)
		})
	}
}

// Print 调用 l.Output 打印到日志记录器。
// 参数的处理方式与 [fmt.Print] 相同。
func (l *Logger) Print(v ...any) {
	l.output(0, 2, func(b []byte) []byte {
		return fmt.Append(b, v...)
	})
}

// Printf 调用 l.Output 打印到日志记录器。
// 参数的处理方式与 [fmt.Printf] 相同。
func (l *Logger) Printf(format string, v ...any) {
	l.output(0, 2, func(b []byte) []byte {
		return fmt.Appendf(b, format, v...)
	})
}

// Println 调用 l.Output 打印到日志记录器。
// 参数的处理方式与 [fmt.Println] 相同。
func (l *Logger) Println(v ...any) {
	l.output(0, 2, func(b []byte) []byte {
		return fmt.Appendln(b, v...)
	})
}

// Fatal 等同于 l.Print() 后跟调用 [os.Exit](1)。
func (l *Logger) Fatal(v ...any) {
	l.output(0, 2, func(b []byte) []byte {
		return fmt.Append(b, v...)
	})
	os.Exit(1)
}

// Fatalf 等同于 l.Printf() 后跟调用 [os.Exit](1)。
func (l *Logger) Fatalf(format string, v ...any) {
	l.output(0, 2, func(b []byte) []byte {
		return fmt.Appendf(b, format, v...)
	})
	os.Exit(1)
}

// Fatalln 等同于 l.Println() 后跟调用 [os.Exit](1)。
func (l *Logger) Fatalln(v ...any) {
	l.output(0, 2, func(b []byte) []byte {
		return fmt.Appendln(b, v...)
	})
	os.Exit(1)
}

// Panic 等同于 l.Print() 后跟调用 panic()。
func (l *Logger) Panic(v ...any) {
	s := fmt.Sprint(v...)
	l.output(0, 2, func(b []byte) []byte {
		return append(b, s...)
	})
	panic(s)
}

// Panicf 等同于 l.Printf() 后跟调用 panic()。
func (l *Logger) Panicf(format string, v ...any) {
	s := fmt.Sprintf(format, v...)
	l.output(0, 2, func(b []byte) []byte {
		return append(b, s...)
	})
	panic(s)
}

// Panicln 等同于 l.Println() 后跟调用 panic()。
func (l *Logger) Panicln(v ...any) {
	s := fmt.Sprintln(v...)
	l.output(0, 2, func(b []byte) []byte {
		return append(b, s...)
	})
	panic(s)
}

// Flags 返回日志记录器的输出标志。
// 标志位是 [Ldate]、[Ltime] 等。
func (l *Logger) Flags() int {
	return int(l.flag.Load())
}

// SetFlags 设置日志记录器的输出标志。
// 标志位是 [Ldate]、[Ltime] 等。
func (l *Logger) SetFlags(flag int) {
	l.flag.Store(int32(flag))
}

// Prefix 返回日志记录器的输出前缀。
func (l *Logger) Prefix() string {
	if p := l.prefix.Load(); p != nil {
		return *p
	}
	return ""
}

// SetPrefix 设置日志记录器的输出前缀。
func (l *Logger) SetPrefix(prefix string) {
	l.prefix.Store(&prefix)
}

// Writer 返回日志记录器的输出目标。
func (l *Logger) Writer() io.Writer {
	l.outMu.Lock()
	defer l.outMu.Unlock()
	return l.out
}

// SetOutput 设置标准日志记录器的输出目标。
func SetOutput(w io.Writer) {
	std.SetOutput(w)
}

// Flags 返回标准日志记录器的输出标志。
// 标志位是 [Ldate]、[Ltime] 等。
func Flags() int {
	return std.Flags()
}

// SetFlags 设置标准日志记录器的输出标志。
// 标志位是 [Ldate]、[Ltime] 等。
func SetFlags(flag int) {
	std.SetFlags(flag)
}

// Prefix 返回标准日志记录器的输出前缀。
func Prefix() string {
	return std.Prefix()
}

// SetPrefix 设置标准日志记录器的输出前缀。
func SetPrefix(prefix string) {
	std.SetPrefix(prefix)
}

// Writer 返回标准日志记录器的输出目标。
func Writer() io.Writer {
	return std.Writer()
}

// 这些函数写入标准日志记录器。

// Print 调用 Output 打印到标准日志记录器。
// 参数的处理方式与 [fmt.Print] 相同。
func Print(v ...any) {
	std.output(0, 2, func(b []byte) []byte {
		return fmt.Append(b, v...)
	})
}

// Printf 调用 Output 打印到标准日志记录器。
// 参数的处理方式与 [fmt.Printf] 相同。
func Printf(format string, v ...any) {
	std.output(0, 2, func(b []byte) []byte {
		return fmt.Appendf(b, format, v...)
	})
}

// Println 调用 Output 打印到标准日志记录器。
// 参数的处理方式与 [fmt.Println] 相同。
func Println(v ...any) {
	std.output(0, 2, func(b []byte) []byte {
		return fmt.Appendln(b, v...)
	})
}

// Fatal 等同于 [Print] 后跟调用 [os.Exit](1)。
func Fatal(v ...any) {
	std.output(0, 2, func(b []byte) []byte {
		return fmt.Append(b, v...)
	})
	os.Exit(1)
}

// Fatalf 等同于 [Printf] 后跟调用 [os.Exit](1)。
func Fatalf(format string, v ...any) {
	std.output(0, 2, func(b []byte) []byte {
		return fmt.Appendf(b, format, v...)
	})
	os.Exit(1)
}

// Fatalln 等同于 [Println] 后跟调用 [os.Exit](1)。
func Fatalln(v ...any) {
	std.output(0, 2, func(b []byte) []byte {
		return fmt.Appendln(b, v...)
	})
	os.Exit(1)
}

// Panic 等同于 [Print] 后跟调用 panic()。
func Panic(v ...any) {
	s := fmt.Sprint(v...)
	std.output(0, 2, func(b []byte) []byte {
		return append(b, s...)
	})
	panic(s)
}

// Panicf 等同于 [Printf] 后跟调用 panic()。
func Panicf(format string, v ...any) {
	s := fmt.Sprintf(format, v...)
	std.output(0, 2, func(b []byte) []byte {
		return append(b, s...)
	})
	panic(s)
}

// Panicln 等同于 [Println] 后跟调用 panic()。
func Panicln(v ...any) {
	s := fmt.Sprintln(v...)
	std.output(0, 2, func(b []byte) []byte {
		return append(b, s...)
	})
	panic(s)
}

// Output 为日志事件写入输出。字符串 s 包含要在 Logger 标志指定的
// 前缀之后打印的文本。如果 s 的最后一个字符不是换行符，则会追加一个换行符。
// 如果设置了 [Llongfile] 或 [Lshortfile]，calldepth 是计算文件名和行号时
// 要跳过的帧数；值为 1 将打印 Output 调用者的详细信息。
func Output(calldepth int, s string) error {
	return std.output(0, calldepth+1, func(b []byte) []byte { // +1 用于此帧。
		return append(b, s...)
	})
}
