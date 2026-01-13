// 版权所有 2022 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package slog

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
)

// Level 是日志事件的重要性或严重性。
// 级别越高，事件越重要或越严重。
type Level int

// 常见级别的名称。
//
// 级别数字本质上是任意的，
// 但我们选择它们以满足三个约束。
// 如果愿意，任何系统都可以将它们映射到另一个编号方案。
//
// 首先，我们希望默认级别是 Info，由于 Level 是 int，
// Info 是 int 的默认值，即零。
//
// 其次，我们希望使用级别来轻松指定日志记录器的详细程度。
// 由于更大的级别意味着更严重的事件，接受具有更小（或更负）级别事件的
// 日志记录器意味着更详细的日志记录器。因此，日志记录器的详细程度是
// 事件严重性的否定，默认详细程度 0 接受至少与 INFO 一样严重的所有事件。
//
// 第三，我们希望在级别之间留出一些空间，以容纳在我们的级别之间
// 有命名级别的方案。例如，Google Cloud Logging 在 Info 和 Warn 之间
// 定义了一个 Notice 级别。由于这些中间级别只有几个，
// 数字之间的间隔不需要很大。我们的间隔 4 与 OpenTelemetry 的映射匹配。
// 从 DEBUG、INFO、WARN 和 ERROR 范围内的 OpenTelemetry 级别减去 9
// 会将其转换为相应的 slog Level 范围。OpenTelemetry 还有 TRACE 和 FATAL
// 名称，而 slog 没有。但这些 OpenTelemetry 级别仍然可以通过使用
// 适当的整数表示为 slog Level。
const (
	LevelDebug Level = -4
	LevelInfo  Level = 0
	LevelWarn  Level = 4
	LevelError Level = 8
)

// String 返回级别的名称。
// 如果级别有名称，则返回该名称的大写形式。
// 如果级别在命名值之间，则将整数附加到大写名称后。
// 示例：
//
//	LevelWarn.String() => "WARN"
//	(LevelInfo+2).String() => "INFO+2"
func (l Level) String() string {
	str := func(base string, val Level) string {
		if val == 0 {
			return base
		}
		sval := strconv.Itoa(int(val))
		if val > 0 {
			sval = "+" + sval
		}
		return base + sval
	}

	switch {
	case l < LevelInfo:
		return str("DEBUG", l-LevelDebug)
	case l < LevelWarn:
		return str("INFO", l-LevelInfo)
	case l < LevelError:
		return str("WARN", l-LevelWarn)
	default:
		return str("ERROR", l-LevelError)
	}
}

// MarshalJSON 通过引用 [Level.String] 的输出实现 [encoding/json.Marshaler]。
func (l Level) MarshalJSON() ([]byte, error) {
	// AppendQuote 足以对所有 Level 字符串进行 JSON 编码。
	// 它们不包含任何在转义时会产生无效 JSON 的符文。
	return strconv.AppendQuote(nil, l.String()), nil
}

// UnmarshalJSON 实现 [encoding/json.Unmarshaler]。
// 它接受 [Level.MarshalJSON] 产生的任何字符串，忽略大小写。
// 它还接受会在输出中产生不同字符串的数字偏移量。
// 例如，"Error-8" 会被序列化为 "INFO"。
func (l *Level) UnmarshalJSON(data []byte) error {
	s, err := strconv.Unquote(string(data))
	if err != nil {
		return err
	}
	return l.parse(s)
}

// AppendText 通过调用 [Level.String] 实现 [encoding.TextAppender]。
func (l Level) AppendText(b []byte) ([]byte, error) {
	return append(b, l.String()...), nil
}

// MarshalText 通过调用 [Level.AppendText] 实现 [encoding.TextMarshaler]。
func (l Level) MarshalText() ([]byte, error) {
	return l.AppendText(nil)
}

// UnmarshalText 实现 [encoding.TextUnmarshaler]。
// 它接受 [Level.MarshalText] 产生的任何字符串，忽略大小写。
// 它还接受会在输出中产生不同字符串的数字偏移量。
// 例如，"Error-8" 会被序列化为 "INFO"。
func (l *Level) UnmarshalText(data []byte) error {
	return l.parse(string(data))
}

func (l *Level) parse(s string) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("slog: level string %q: %w", s, err)
		}
	}()

	name := s
	offset := 0
	if i := strings.IndexAny(s, "+-"); i >= 0 {
		name = s[:i]
		offset, err = strconv.Atoi(s[i:])
		if err != nil {
			return err
		}
	}
	switch strings.ToUpper(name) {
	case "DEBUG":
		*l = LevelDebug
	case "INFO":
		*l = LevelInfo
	case "WARN":
		*l = LevelWarn
	case "ERROR":
		*l = LevelError
	default:
		return errors.New("unknown name")
	}
	*l += Level(offset)
	return nil
}

// Level 返回接收者本身。
// 它实现了 [Leveler]。
func (l Level) Level() Level { return l }

// LevelVar 是一个 [Level] 变量，允许 [Handler] 的级别动态更改。
// 它实现了 [Leveler] 以及 Set 方法，
// 并且可以安全地被多个 goroutine 使用。
// 零值 LevelVar 对应于 [LevelInfo]。
type LevelVar struct {
	val atomic.Int64
}

// Level 返回 v 的级别。
func (v *LevelVar) Level() Level {
	return Level(int(v.val.Load()))
}

// Set 将 v 的级别设置为 l。
func (v *LevelVar) Set(l Level) {
	v.val.Store(int64(l))
}

func (v *LevelVar) String() string {
	return fmt.Sprintf("LevelVar(%s)", v.Level())
}

// AppendText 通过调用 [Level.AppendText] 实现 [encoding.TextAppender]。
func (v *LevelVar) AppendText(b []byte) ([]byte, error) {
	return v.Level().AppendText(b)
}

// MarshalText 通过调用 [LevelVar.AppendText] 实现 [encoding.TextMarshaler]。
func (v *LevelVar) MarshalText() ([]byte, error) {
	return v.AppendText(nil)
}

// UnmarshalText 通过调用 [Level.UnmarshalText] 实现 [encoding.TextUnmarshaler]。
func (v *LevelVar) UnmarshalText(data []byte) error {
	var l Level
	if err := l.UnmarshalText(data); err != nil {
		return err
	}
	v.Set(l)
	return nil
}

// Leveler 提供一个 [Level] 值。
//
// 由于 Level 本身实现了 Leveler，客户端通常在需要 Leveler 的地方
// 提供 Level 值，例如在 [HandlerOptions] 中。
// 需要动态改变级别的客户端可以提供更复杂的 Leveler 实现，
// 如 *[LevelVar]。
type Leveler interface {
	Level() Level
}
