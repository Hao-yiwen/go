// 版权所有 2013 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package signal_test

import (
	"fmt"
	"os"
	"os/signal"
)

func ExampleNotify() {
	// 在其上设置通道以发送信号通知。
	// 我们必须使用缓冲通道或冒着丢失信号的风险
	// 如果我们在发送信号时还未准备好接收。
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	// 阻塞直到接收到信号。
	s := <-c
	fmt.Println("Got signal:", s)
}

func ExampleNotify_allSignals() {
	// 在其上设置通道以发送信号通知。
	// 我们必须使用缓冲通道或冒着丢失信号的风险
	// 如果我们在发送信号时还未准备好接收。
	c := make(chan os.Signal, 1)

	// 不向 Notify 传递信号意味着
	// 所有信号都将被发送到通道。
	signal.Notify(c)

	// 阻塞直到接收到任何信号。
	s := <-c
	fmt.Println("Got signal:", s)
}
