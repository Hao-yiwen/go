// 版权所有 2022 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package slog

import (
	"bytes"
	"context"
	"log"
	loginternal "log/internal"
	"log/slog/internal"
	"runtime"
	"sync/atomic"
	"time"
)

var defaultLogger atomic.Pointer[Logger]

var logLoggerLevel LevelVar

// SetLogLoggerLevel 控制到 [log] 包的桥接级别。
//
// 在调用 [SetDefault] 之前，slog 顶级日志记录函数会调用默认的 [log.Logger]。
// 在该模式下，SetLogLoggerLevel 设置这些调用的最小级别。
// 默认情况下，最小级别是 Info，因此对 [Debug] 的调用
// （以及较低级别的顶级日志记录调用）不会传递给 log.Logger。
// 在调用以下语句后：
//
//	slog.SetLogLoggerLevel(slog.LevelDebug)
//
// 对 [Debug] 的调用将传递给 log.Logger。
//
// 在调用 [SetDefault] 之后，对默认 [log.Logger] 的调用将传递给
// slog 默认处理程序。在该模式下，
// SetLogLoggerLevel 设置记录这些调用的级别。
// 也就是说，在调用以下语句后：
//
//	slog.SetLogLoggerLevel(slog.LevelDebug)
//
// 对 [log.Printf] 的调用将以 [LevelDebug] 级别输出。
//
// SetLogLoggerLevel 返回之前的值。
func SetLogLoggerLevel(level Level) (oldLevel Level) {
	oldLevel = logLoggerLevel.Level()
	logLoggerLevel.Set(level)
	return
}

func init() {
	defaultLogger.Store(New(newDefaultHandler(loginternal.DefaultOutput)))
}

// Default 返回默认的 [Logger]。
func Default() *Logger { return defaultLogger.Load() }

// SetDefault 将 l 设为默认 [Logger]，顶级函数 [Info]、[Debug] 等会使用它。
// 此调用之后，来自 log 包默认 Logger 的输出
// （如 [log.Print] 等）将使用 l 的 Handler 进行记录，
// 级别由 [SetLogLoggerLevel] 控制。
func SetDefault(l *Logger) {
	defaultLogger.Store(l)
	// 如果默认的处理程序是 defaultHandler，则不要使用 handleWriter，
	// 否则当它们都尝试获取 log 默认互斥锁时会死锁。
	// defaultHandler 将使用当前设置的 log 默认写入器，这是正确的。
	// 这可能发生在 SetDefault(Default()) 的情况下。
	// 参见 TestSetDefault。
	if _, ok := l.Handler().(*defaultHandler); !ok {
		capturePC := log.Flags()&(log.Lshortfile|log.Llongfile) != 0
		log.SetOutput(&handlerWriter{l.Handler(), &logLoggerLevel, capturePC})
		log.SetFlags(0) // 我们只需要日志消息，不需要时间或位置
	}
}

// handlerWriter 是一个调用 Handler 的 io.Writer。
// 它用于将默认的 log.Logger 链接到默认的 slog.Logger。
type handlerWriter struct {
	h         Handler
	level     Leveler
	capturePC bool
}

func (w *handlerWriter) Write(buf []byte) (int, error) {
	level := w.level.Level()
	if !w.h.Enabled(context.Background(), level) {
		return 0, nil
	}
	var pc uintptr
	if !internal.IgnorePC && w.capturePC {
		// 跳过 [runtime.Callers, w.Write, Logger.Output, log.Print]
		var pcs [1]uintptr
		runtime.Callers(4, pcs[:])
		pc = pcs[0]
	}

	// 移除末尾的换行符。
	origLen := len(buf) // 报告整个 buf 已被写入。
	buf = bytes.TrimSuffix(buf, []byte{'\n'})
	r := NewRecord(time.Now(), level, string(buf), pc)
	return origLen, w.h.Handle(context.Background(), r)
}

// Logger 记录每次调用其 Log、Debug、Info、Warn 和 Error 方法的结构化信息。
// 对于每次调用，它创建一个 [Record] 并将其传递给 [Handler]。
//
// 要创建新的 Logger，请调用 [New] 或以 "With" 开头的 Logger 方法。
type Logger struct {
	handler Handler // 用于结构化日志记录
}

func (l *Logger) clone() *Logger {
	c := *l
	return &c
}

// Handler 返回 l 的 Handler。
func (l *Logger) Handler() Handler { return l.handler }

// With 返回一个在每次输出操作中包含给定属性的 Logger。
// 参数按照 [Logger.Log] 的方式转换为属性。
func (l *Logger) With(args ...any) *Logger {
	if len(args) == 0 {
		return l
	}
	c := l.clone()
	c.handler = l.handler.WithAttrs(argsToAttrSlice(args))
	return c
}

// WithGroup 如果 name 非空，则返回一个开始分组的 Logger。
// 添加到 Logger 的所有属性的键将由给定名称限定。
// （如何进行限定取决于 Logger 的 Handler 的 [Handler.WithGroup] 方法。）
//
// 如果 name 为空，WithGroup 返回接收者本身。
func (l *Logger) WithGroup(name string) *Logger {
	if name == "" {
		return l
	}
	c := l.clone()
	c.handler = l.handler.WithGroup(name)
	return c
}

// New 使用给定的非 nil Handler 创建新的 Logger。
func New(h Handler) *Logger {
	if h == nil {
		panic("nil Handler")
	}
	return &Logger{handler: h}
}

// With 在默认日志记录器上调用 [Logger.With]。
func With(args ...any) *Logger {
	return Default().With(args...)
}

// Enabled 报告 l 是否在给定的上下文和级别发出日志记录。
func (l *Logger) Enabled(ctx context.Context, level Level) bool {
	if ctx == nil {
		ctx = context.Background()
	}
	return l.Handler().Enabled(ctx, level)
}

// NewLogLogger 返回一个新的 [log.Logger]，使得对其 Output 方法的每次调用
// 都会将 Record 分派给指定的处理程序。该日志记录器充当从
// 旧版 log API 到新版结构化日志处理程序的桥梁。
func NewLogLogger(h Handler, level Level) *log.Logger {
	return log.New(&handlerWriter{h, level, true}, "", 0)
}

// Log 使用当前时间和给定的级别与消息发出日志记录。
// Record 的 Attrs 由 Logger 的属性加上 args 指定的 Attrs 组成。
//
// 属性参数按如下方式处理：
//   - 如果参数是 Attr，则按原样使用。
//   - 如果参数是字符串且不是最后一个参数，
//     则下一个参数被视为值，两者组合成一个 Attr。
//   - 否则，参数被视为键为 "!BADKEY" 的值。
func (l *Logger) Log(ctx context.Context, level Level, msg string, args ...any) {
	l.log(ctx, level, msg, args...)
}

// LogAttrs 是 [Logger.Log] 的更高效版本，只接受 Attrs。
func (l *Logger) LogAttrs(ctx context.Context, level Level, msg string, attrs ...Attr) {
	l.logAttrs(ctx, level, msg, attrs...)
}

// Debug 以 [LevelDebug] 级别记录日志。
func (l *Logger) Debug(msg string, args ...any) {
	l.log(context.Background(), LevelDebug, msg, args...)
}

// DebugContext 使用给定的上下文以 [LevelDebug] 级别记录日志。
func (l *Logger) DebugContext(ctx context.Context, msg string, args ...any) {
	l.log(ctx, LevelDebug, msg, args...)
}

// Info 以 [LevelInfo] 级别记录日志。
func (l *Logger) Info(msg string, args ...any) {
	l.log(context.Background(), LevelInfo, msg, args...)
}

// InfoContext 使用给定的上下文以 [LevelInfo] 级别记录日志。
func (l *Logger) InfoContext(ctx context.Context, msg string, args ...any) {
	l.log(ctx, LevelInfo, msg, args...)
}

// Warn 以 [LevelWarn] 级别记录日志。
func (l *Logger) Warn(msg string, args ...any) {
	l.log(context.Background(), LevelWarn, msg, args...)
}

// WarnContext 使用给定的上下文以 [LevelWarn] 级别记录日志。
func (l *Logger) WarnContext(ctx context.Context, msg string, args ...any) {
	l.log(ctx, LevelWarn, msg, args...)
}

// Error 以 [LevelError] 级别记录日志。
func (l *Logger) Error(msg string, args ...any) {
	l.log(context.Background(), LevelError, msg, args...)
}

// ErrorContext 使用给定的上下文以 [LevelError] 级别记录日志。
func (l *Logger) ErrorContext(ctx context.Context, msg string, args ...any) {
	l.log(ctx, LevelError, msg, args...)
}

// log 是接受 ...any 的方法的低级日志记录方法。
// 它必须总是由导出的日志记录方法或函数直接调用，
// 因为它使用固定的调用深度来获取 pc。
func (l *Logger) log(ctx context.Context, level Level, msg string, args ...any) {
	if ctx == nil {
		ctx = context.Background()
	}
	if !l.Enabled(ctx, level) {
		return
	}
	var pc uintptr
	if !internal.IgnorePC {
		var pcs [1]uintptr
		// 跳过 [runtime.Callers, 此函数, 此函数的调用者]
		runtime.Callers(3, pcs[:])
		pc = pcs[0]
	}
	r := NewRecord(time.Now(), level, msg, pc)
	r.Add(args...)
	_ = l.Handler().Handle(ctx, r)
}

// logAttrs 类似于 [Logger.log]，但用于接受 ...Attr 的方法。
func (l *Logger) logAttrs(ctx context.Context, level Level, msg string, attrs ...Attr) {
	if ctx == nil {
		ctx = context.Background()
	}
	if !l.Enabled(ctx, level) {
		return
	}
	var pc uintptr
	if !internal.IgnorePC {
		var pcs [1]uintptr
		// 跳过 [runtime.Callers, 此函数, 此函数的调用者]
		runtime.Callers(3, pcs[:])
		pc = pcs[0]
	}
	r := NewRecord(time.Now(), level, msg, pc)
	r.AddAttrs(attrs...)
	_ = l.Handler().Handle(ctx, r)
}

// Debug 在默认日志记录器上调用 [Logger.Debug]。
func Debug(msg string, args ...any) {
	Default().log(context.Background(), LevelDebug, msg, args...)
}

// DebugContext 在默认日志记录器上调用 [Logger.DebugContext]。
func DebugContext(ctx context.Context, msg string, args ...any) {
	Default().log(ctx, LevelDebug, msg, args...)
}

// Info 在默认日志记录器上调用 [Logger.Info]。
func Info(msg string, args ...any) {
	Default().log(context.Background(), LevelInfo, msg, args...)
}

// InfoContext 在默认日志记录器上调用 [Logger.InfoContext]。
func InfoContext(ctx context.Context, msg string, args ...any) {
	Default().log(ctx, LevelInfo, msg, args...)
}

// Warn 在默认日志记录器上调用 [Logger.Warn]。
func Warn(msg string, args ...any) {
	Default().log(context.Background(), LevelWarn, msg, args...)
}

// WarnContext 在默认日志记录器上调用 [Logger.WarnContext]。
func WarnContext(ctx context.Context, msg string, args ...any) {
	Default().log(ctx, LevelWarn, msg, args...)
}

// Error 在默认日志记录器上调用 [Logger.Error]。
func Error(msg string, args ...any) {
	Default().log(context.Background(), LevelError, msg, args...)
}

// ErrorContext 在默认日志记录器上调用 [Logger.ErrorContext]。
func ErrorContext(ctx context.Context, msg string, args ...any) {
	Default().log(ctx, LevelError, msg, args...)
}

// Log 在默认日志记录器上调用 [Logger.Log]。
func Log(ctx context.Context, level Level, msg string, args ...any) {
	Default().log(ctx, level, msg, args...)
}

// LogAttrs 在默认日志记录器上调用 [Logger.LogAttrs]。
func LogAttrs(ctx context.Context, level Level, msg string, attrs ...Attr) {
	Default().logAttrs(ctx, level, msg, attrs...)
}
