// 版权所有 2022 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

/*
slog 包提供结构化日志记录，
其中日志记录包含消息、严重级别以及以键值对形式表达的各种其他属性。

它定义了一个 [Logger] 类型，
该类型提供了几个方法（如 [Logger.Info] 和 [Logger.Error]）
用于报告感兴趣的事件。

每个 Logger 都与一个 [Handler] 关联。
Logger 的输出方法从方法参数创建一个 [Record]，
并将其传递给 Handler，由 Handler 决定如何处理它。
有一个默认的 Logger 可以通过顶级函数访问
（如 [Info] 和 [Error]），这些函数调用相应的 Logger 方法。

日志记录由时间、级别、消息和一组键值对组成，
其中键是字符串，值可以是任何类型。
例如，

	slog.Info("hello", "count", 3)

创建一个包含调用时间、Info 级别、消息 "hello" 以及
键为 "count"、值为 3 的单个键值对的记录。

[Info] 顶级函数在默认 Logger 上调用 [Logger.Info] 方法。
除了 [Logger.Info]，还有用于 Debug、Warn 和 Error 级别的方法。
除了这些常用级别的便捷方法外，
还有一个 [Logger.Log] 方法，它将级别作为参数。
每个方法都有一个对应的顶级函数，使用默认日志记录器。

默认处理程序将日志记录的消息、时间、级别和属性格式化为字符串，
并将其传递给 [log] 包。

	2022/11/08 15:28:26 INFO hello count=3

要更好地控制输出格式，请使用不同的处理程序创建日志记录器。
此语句使用 [New] 创建一个新的日志记录器，该记录器使用 [TextHandler]
将结构化记录以文本形式写入标准错误输出：

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

[TextHandler] 的输出是一系列 key=value 对，
易于被机器明确解析。此语句：

	logger.Info("hello", "count", 3)

产生此输出：

	time=2022-11-08T15:28:26.000-05:00 level=INFO msg=hello count=3

该包还提供了 [JSONHandler]，其输出是行分隔的 JSON：

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	logger.Info("hello", "count", 3)

产生此输出：

	{"time":"2022-11-08T15:28:26.000000000-05:00","level":"INFO","msg":"hello","count":3}

[TextHandler] 和 [JSONHandler] 都可以通过 [HandlerOptions] 进行配置。
有设置最小级别的选项（参见下面的"级别"部分）、
显示日志调用的源文件和行号、以及在记录之前修改属性的选项。

使用以下方式将日志记录器设置为默认值：

	slog.SetDefault(logger)

将导致像 [Info] 这样的顶级函数使用它。
[SetDefault] 还会更新 [log] 包使用的默认日志记录器，
这样使用 [log.Printf] 和相关函数的现有应用程序
无需重写即可将日志记录发送到日志记录器的处理程序。

某些属性在许多日志调用中是通用的。
例如，您可能希望将服务器请求的 URL 或跟踪标识符
包含在该请求产生的所有日志事件中。
与其在每个日志调用中重复该属性，您可以使用 [Logger.With]
构造一个包含这些属性的新 Logger：

	logger2 := logger.With("url", r.URL)

With 的参数与 [Logger.Info] 中使用的键值对相同。
结果是一个与原始日志记录器具有相同处理程序的新 Logger，
但具有将出现在每次调用输出中的附加属性。

# 级别

[Level] 是表示日志事件重要性或严重性的整数。
级别越高，事件越严重。
此包为最常见的级别定义了常量，但任何 int 都可以用作级别。

在应用程序中，您可能希望仅记录某个级别或更高级别的消息。
一种常见的配置是记录 Info 或更高级别的消息，
在需要时才启用调试日志记录。
可以通过设置 [HandlerOptions.Level] 来配置内置处理程序的最小输出级别。
程序的 `main` 函数通常会执行此操作。
默认值是 LevelInfo。

将 [HandlerOptions.Level] 字段设置为 [Level] 值
会在处理程序的整个生命周期内固定其最小级别。
将其设置为 [LevelVar] 允许动态改变级别。
LevelVar 持有一个 Level，可以安全地从多个 goroutine 读取或写入。
要为整个程序动态改变级别，首先初始化一个全局 LevelVar：

	var programLevel = new(slog.LevelVar) // 默认为 Info

然后使用 LevelVar 构造处理程序，并将其设为默认值：

	h := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: programLevel})
	slog.SetDefault(slog.New(h))

现在程序可以用一条语句改变其日志级别：

	programLevel.Set(slog.LevelDebug)

# 分组

属性可以收集到分组中。
分组有一个名称，用于限定其属性的名称。
这种限定如何显示取决于处理程序。
[TextHandler] 用点分隔分组名和属性名。
[JSONHandler] 将每个分组视为单独的 JSON 对象，分组名作为键。

使用 [Group] 从名称和键值对列表创建 Group 属性：

	slog.Group("request",
	    "method", r.Method,
	    "url", r.URL)

TextHandler 会将此分组显示为：

	request.method=GET request.url=http://example.com

JSONHandler 会将其显示为：

	"request":{"method":"GET","url":"http://example.com"}

使用 [Logger.WithGroup] 用分组名限定 Logger 的所有输出。
在 Logger 上调用 WithGroup 会产生一个
与原始处理程序相同的新 Logger，
但其所有属性都由分组名限定。

这可以帮助防止大型系统中的重复属性键，
因为子系统可能使用相同的键。
为每个子系统传递一个具有自己分组名的不同 Logger，
以便限定潜在的重复项：

	logger := slog.Default().With("id", systemID)
	parserLogger := logger.WithGroup("parser")
	parseInput(input, parserLogger)

当 parseInput 使用 parserLogger 记录日志时，其键将用 "parser" 限定，
因此即使它使用通用键 "id"，日志行也将具有不同的键。

# 上下文

某些处理程序可能希望包含调用站点可用的 [context.Context] 中的信息。
此类信息的一个示例是启用跟踪时当前 span 的标识符。

[Logger.Log] 和 [Logger.LogAttrs] 方法将上下文作为第一个参数，
它们对应的顶级函数也是如此。

虽然 Logger 上的便捷方法（Info 等）和相应的顶级函数不接受上下文，
但以 "Context" 结尾的替代方法可以。例如，

	slog.InfoContext(ctx, "message")

如果有可用的上下文，建议将其传递给输出方法。

# Attr 和 Value

[Attr] 是一个键值对。Logger 输出方法接受 Attr 以及交替的键和值。语句

	slog.Info("hello", slog.Int("count", 3))

与以下语句行为相同：

	slog.Info("hello", "count", 3)

对于常见类型，有 [Attr] 的便捷构造函数，如 [Int]、[String] 和 [Bool]，
以及用于构造任何类型 Attr 的函数 [Any]。

Attr 的值部分是一个名为 [Value] 的类型。
像 [any] 一样，Value 可以持有任何 Go 值，
但它可以在不分配内存的情况下表示典型值，包括所有数字和字符串。

为了获得最高效的日志输出，请使用 [Logger.LogAttrs]。
它类似于 [Logger.Log]，但只接受 Attr，不接受交替的键和值；
这也使它能够避免分配。

调用

	logger.LogAttrs(ctx, slog.LevelInfo, "hello", slog.Int("count", 3))

是实现与以下语句相同输出的最高效方式：

	slog.InfoContext(ctx, "hello", "count", 3)

# 自定义类型的日志记录行为

如果类型实现了 [LogValuer] 接口，则其 LogValue 方法返回的 [Value] 将用于日志记录。
您可以使用它来控制该类型的值在日志中的显示方式。
例如，您可以编辑密码等机密信息，或将结构体的字段收集到一个 Group 中。
有关详细信息，请参阅 [LogValuer] 下的示例。

LogValue 方法可能返回一个本身实现 [LogValuer] 的 Value。
[Value.Resolve] 方法会仔细处理这些情况，避免无限循环和无界递归。
Handler 作者和其他人可能希望使用 [Value.Resolve] 而不是直接调用 LogValue。

# 包装输出方法

日志记录器函数使用调用栈反射来查找应用程序中日志调用的文件名和行号。
这可能会为包装 slog 的函数产生不正确的源信息。
例如，如果您在文件 mylog.go 中定义此函数：

	func Infof(logger *slog.Logger, format string, args ...any) {
	    logger.Info(fmt.Sprintf(format, args...))
	}

并在 main.go 中这样调用它：

	Infof(slog.Default(), "hello, %s", "world")

那么 slog 将报告源文件为 mylog.go，而不是 main.go。

正确的 Infof 实现将获取源位置 (pc) 并将其传递给 NewRecord。
名为 "wrapping" 的包级别示例中的 Infof 函数演示了如何做到这一点。

# 处理 Record

有时 Handler 需要在将 Record 传递给另一个 Handler 或后端之前修改它。
Record 包含简单公共字段（如 Time、Level、Message）和
间接引用状态（如属性）的隐藏字段的混合。
这意味着修改 Record 的简单副本（例如通过调用 [Record.Add] 或
[Record.AddAttrs] 添加属性）可能会对原始记录产生意外影响。
在修改 Record 之前，使用 [Record.Clone] 创建一个
与原始记录不共享状态的副本，
或者使用 [NewRecord] 创建一个新的 Record，
并通过使用 [Record.Attrs] 遍历旧属性来构建其 Attr。

# 性能考虑

如果分析您的应用程序表明日志记录占用了大量时间，
以下建议可能会有所帮助。

如果许多日志行具有通用属性，请使用 [Logger.With] 创建具有该属性的 Logger。
内置处理程序将仅在调用 [Logger.With] 时格式化该属性一次。
[Handler] 接口旨在允许这种优化，编写良好的 Handler 应该利用它。

日志调用的参数总是会被求值，即使日志事件被丢弃。
如果可能，推迟计算，使其仅在值实际被记录时才发生。
例如，考虑以下调用：

	slog.Info("starting request", "url", r.URL.String())  // 可能不必要地计算 String

即使日志记录器丢弃 Info 级别的事件，URL.String 方法也会被调用。
相反，直接传递 URL：

	slog.Info("starting request", "url", &r.URL) // 仅在需要时调用 URL.String

内置的 [TextHandler] 会调用其 String 方法，但仅在日志事件启用时才会调用。
避免调用 String 还可以保留底层值的结构。
例如，[JSONHandler] 将解析的 URL 的组件作为 JSON 对象发出。
如果您想避免急切地支付 String 调用的成本，
同时不让处理程序可能检查值的结构，
请将值包装在隐藏其 Marshal 方法的 fmt.Stringer 实现中。

您还可以使用 [LogValuer] 接口来避免在禁用的日志调用中进行不必要的工作。
假设您需要记录一些昂贵的值：

	slog.Debug("frobbing", "value", computeExpensiveValue(arg))

即使此行被禁用，computeExpensiveValue 也会被调用。
为避免这种情况，定义一个实现 LogValuer 的类型：

	type expensive struct { arg int }

	func (e expensive) LogValue() slog.Value {
	    return slog.AnyValue(computeExpensiveValue(e.arg))
	}

然后在日志调用中使用该类型的值：

	slog.Debug("frobbing", "value", expensive{arg})

现在 computeExpensiveValue 仅在该行启用时才会被调用。

内置处理程序在调用 [io.Writer.Write] 之前获取锁，
以确保一次完整地写入一个 [Record]。
虽然每个日志记录都有时间戳，
但内置处理程序不使用该时间来对写入的记录进行排序。
用户定义的处理程序负责自己的锁定和排序。

# 编写处理程序

有关编写自定义处理程序的指南，请参阅 https://golang.org/s/slog-handler-guide。
*/
package slog
