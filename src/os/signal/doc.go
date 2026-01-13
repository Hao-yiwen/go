// 版权所有 2015 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

/*
包 signal 实现了对传入信号的访问。

信号主要用于类 Unix 系统。关于此包在 Windows 和 Plan 9 上的使用，请参见下文。

# 信号的类型

SIGKILL 和 SIGSTOP 信号无法被程序捕获，
因此无法受此包影响。

同步信号是由程序执行中的错误触发的信号：
SIGBUS、SIGFPE 和 SIGSEGV。这些仅在由程序执行引起时才被认为是
同步的，而不是通过 [os.Process.Kill] 或 kill 程序或类似机制发送时。
一般来说，除了下面讨论的情况外，Go 程序会将
同步信号转换为运行时 panic。

其余信号是异步信号。它们不是
由程序错误触发的，而是从内核或
其他程序发送的。

在异步信号中，当程序失去其控制终端时发送 SIGHUP 信号。
当用户在控制终端按下中断字符时发送 SIGINT 信号，
默认情况下是 ^C（Control-C）。当用户在控制终端按下 quit 字符时发送 SIGQUIT 信号，
默认情况下是 ^\（Control-Backslash）。
一般来说，你可以通过按 ^C 使程序简单退出，也可以通过按 ^\ 使其
退出并输出堆栈转储。

# Go 程序中信号的默认行为

默认情况下，同步信号被转换为运行时 panic。
SIGHUP、SIGINT 或 SIGTERM 信号导致程序退出。
SIGQUIT、SIGILL、SIGTRAP、SIGABRT、SIGSTKFLT、SIGEMT 或 SIGSYS 信号
导致程序退出并输出堆栈转储。SIGTSTP、SIGTTIN 或
SIGTTOU 信号获得系统默认行为（这些信号被
shell 用于作业控制）。SIGPROF 信号由 Go 运行时直接处理以实现 runtime.CPUProfile。
其他信号将被捕获但不采取任何操作。

如果 Go 程序启动时 SIGHUP 或 SIGINT 被忽略
（信号处理程序设置为 SIG_IGN），它们将保持忽略。

如果 Go 程序启动时信号掩码非空，那将
通常被遵守。但是，某些信号被显式取消阻止：
同步信号、SIGILL、SIGTRAP、SIGSTKFLT、SIGCHLD、SIGPROF，
以及在 Linux 上，信号 32（SIGCANCEL）和 33（SIGSETXID）
（SIGCANCEL 和 SIGSETXID 由 glibc 内部使用）。由 [os.Exec] 或 [os/exec]
启动的子进程将继承
修改后的信号掩码。

# 改变 Go 程序中信号的行为

此包中的函数允许程序改变 Go
程序处理信号的方式。

Notify 禁用给定异步信号集合的默认行为，并且
改为通过一个或多个已注册
通道发送它们。具体来说，它适用于 SIGHUP、SIGINT、
SIGQUIT、SIGABRT 和 SIGTERM 信号。它也适用于作业控制
信号 SIGTSTP、SIGTTIN 和 SIGTTOU，在这种情况下系统
默认行为不会发生。它也适用于一些原本不会导致任何操作的信号：
SIGUSR1、SIGUSR2、SIGPIPE、SIGALRM、
SIGCHLD、SIGCONT、SIGURG、SIGXCPU、SIGXFSZ、SIGVTALRM、SIGWINCH、
SIGIO、SIGPWR、SIGINFO、SIGTHR、SIGWAITING、SIGLWP、SIGFREEZE、
SIGTHAW、SIGLOST、SIGXRES、SIGJVM1、SIGJVM2 以及系统上使用的任何实时信号。
注意并非所有这些信号都在所有系统上可用。

如果程序启动时 SIGHUP 或 SIGINT 被忽略，并且对任一信号调用了 [Notify]，
将安装该信号的信号处理程序，它将不再被忽略。
如果后来对该信号调用了 [Reset] 或
[Ignore]，或对传递给 Notify 的所有通道调用了 [Stop]，
该信号将再次被忽略。Reset 将恢复该
信号的系统默认行为，而 Ignore 将导致系统
完全忽略该信号。

如果程序启动时信号掩码非空，某些信号
将被显式取消阻止，如上所述。如果对被阻止的信号调用 Notify，
它将被取消阻止。如果后来对该信号调用 Reset，
或对传递给 Notify 的所有通道调用 Stop，该信号
将再次被阻止。

# SIGPIPE

当 Go 程序写入已关闭的管道时，内核将引发
SIGPIPE 信号。

如果程序未调用 Notify 来接收 SIGPIPE 信号，那么
行为取决于文件描述符号。对文件描述符 1 或 2
（标准输出或标准错误）上的已关闭管道的写入将导致程序
以 SIGPIPE 信号退出。对某个其他文件描述符
上的已关闭管道的写入将不对 SIGPIPE 信号采取任何操作，
并且写入将失败，返回 [syscall.EPIPE]
错误。

如果程序已调用 Notify 来接收 SIGPIPE 信号，文件
描述符号无关紧要。SIGPIPE 信号将
在 Notify 通道上发送，写入将失败，返回
[syscall.EPIPE] 错误。

这意味着，默认情况下，命令行程序将表现得像
典型的 Unix 命令行程序，而其他程序在写入已关闭的网络连接时将不会
因 SIGPIPE 而崩溃。

# 使用 cgo 或 SWIG 的 Go 程序

在包含非 Go 代码的 Go 程序中，通常是 C/C++ 代码
通过 cgo 或 SWIG 访问，Go 的启动代码通常首先运行。它
在非 Go 启动代码运行之前配置 Go 运行时期望的
信号处理程序。如果非 Go 启动代码希望
安装其自己的信号处理程序，它必须采取某些步骤以保持 Go
正常工作。本节记录了这些步骤以及非 Go 代码对信号处理程序设置的整体
影响变更可能对 Go 程序产生的影响。在极少数情况下，
非 Go 代码可能在 Go 代码之前运行，在这种情况下下一节也适用。

如果 Go 程序调用的非 Go 代码不改变任何信号
处理程序或掩码，则行为与纯 Go
程序相同。

如果非 Go 代码安装任何信号处理程序，它必须使用
SA_ONSTACK 标志与 sigaction 配合。不这样做可能导致
程序在接收信号时崩溃。Go 程序通常
使用有限堆栈运行，因此设置了替代信号
堆栈。

如果非 Go 代码为任何
同步信号（SIGBUS、SIGFPE、SIGSEGV）安装了信号处理程序，
它应该记录现有的 Go 信号处理程序。
如果在执行 Go 代码时这些信号发生，它应该调用 Go 信号处理程序
（是否在执行 Go 代码时发生信号可以通过查看传递给信号处理程序的 PC 来确定）。
否则某些 Go 运行时 panic 将不会按预期发生。

如果非 Go 代码为任何
异步信号安装了信号处理程序，它可以选择是否调用 Go 信号处理程序。
自然地，如果它不调用 Go 信号处理程序，
上述 Go 行为将不会发生。这对于
SIGPROF 信号特别可能成为问题。

非 Go 代码不应改变 Go 运行时创建的任何线程上的信号掩码。
如果非 Go 代码启动新线程，那些线程可以随意设置信号掩码。

如果非 Go 代码启动新线程、更改信号掩码，然后
在该线程中调用 Go 函数，Go 运行时将
自动取消阻止某些信号：同步信号、
SIGILL、SIGTRAP、SIGSTKFLT、SIGCHLD、SIGPROF、SIGCANCEL 和
SIGSETXID。当 Go 函数返回时，非 Go 信号掩码将
被恢复。

如果 Go 信号处理程序在未运行 Go 代码的非 Go 线程上被调用，
处理程序通常将信号转发给非 Go 代码，如下所示。
如果信号是 SIGPROF，Go 处理程序不做任何事。否则，
Go 处理程序移除自身、取消阻止信号，然后再次引发它以调用
任何非 Go 处理程序或默认系统处理程序。如果程序不退出，
Go 处理程序会重新安装自身并继续程序执行。

如果接收到 SIGPIPE 信号，如果 SIGPIPE 在 Go
线程上被接收，Go 程序将调用上述特殊处理。如果 SIGPIPE 在非 Go 线程上被接收，
信号将被转发给非 Go 处理程序（如果有）；如果没有，
默认系统处理程序将导致程序终止。

# 调用 Go 代码的非 Go 程序

当 Go 代码使用 -buildmode=c-shared 等选项构建时，它将
作为现有非 Go 程序的一部分运行。非 Go 代码可能
在 Go 代码启动时已经安装了信号处理程序（在使用 cgo 或 SWIG 的不寻常情况下也可能发生；
在这种情况下下面的讨论适用）。对于 -buildmode=c-archive，
Go 运行时将在全局构造函数时初始化信号。对于
-buildmode=c-shared，Go 运行时将在加载共享库时初始化信号。

如果 Go 运行时看到 SIGCANCEL 或
SIGSETXID 信号的现有信号处理程序（仅在 Linux 上使用），
它将打开 SA_ONSTACK 标志并保持信号处理程序。

对于同步信号和 SIGPIPE，Go 运行时将安装
信号处理程序。它将保存任何现有的信号处理程序。如果
同步信号在执行非 Go 代码时到达，Go 运行时
将调用现有的信号处理程序而不是 Go 信号
处理程序。

使用 -buildmode=c-archive 或 -buildmode=c-shared 构建的 Go 代码将
默认不安装任何其他信号处理程序。如果存在
现有的信号处理程序，Go 运行时将打开 SA_ONSTACK
标志并保持信号处理程序。如果对异步信号调用 Notify，
将安装该信号的 Go 信号处理程序。如果后来对该信号调用 Reset，
原始的信号处理将被重新安装，恢复非 Go
信号处理程序（如果有）。

不使用 -buildmode=c-archive 或 -buildmode=c-shared 构建的 Go 代码将
为上述异步信号安装信号处理程序，
并保存任何现有的信号处理程序。如果信号被发送到
非 Go 线程，它将按上述方式进行，除了如果存在
现有的非 Go 信号处理程序，该处理程序将在引发信号之前被安装。

# Windows

在 Windows 上，^C（Control-C）或 ^BREAK（Control-Break）通常会导致
程序退出。如果对 [os.Interrupt] 调用 Notify，^C 或 ^BREAK
将导致 [os.Interrupt] 被发送到通道上，程序将
不会退出。如果调用 Reset，或对传递给 Notify 的所有通道
调用 Stop，则默认行为将被恢复。

此外，如果调用 Notify，并且 Windows 将 CTRL_CLOSE_EVENT、
CTRL_LOGOFF_EVENT 或 CTRL_SHUTDOWN_EVENT 发送给进程，Notify 将
返回 syscall.SIGTERM。与 Control-C 和 Control-Break 不同，Notify
在接收 CTRL_CLOSE_EVENT、
CTRL_LOGOFF_EVENT 或 CTRL_SHUTDOWN_EVENT 时不改变进程行为 - 进程将
仍然被终止，除非它退出。但接收 syscall.SIGTERM 将
给进程一个在终止之前清理的机会。

# Plan 9

在 Plan 9 上，信号的类型为 syscall.Note，即一个字符串。使用
syscall.Note 调用 Notify 将导致该值在该字符串被发布为
注释时在通道上被发送。
*/
package signal
