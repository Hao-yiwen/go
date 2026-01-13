// 版权所有 2012 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

//go:build unix || (js && wasm) || wasip1 || windows

package signal

import (
	"os"
	"syscall"
)

// 由运行时包定义。
func signal_disable(uint32)
func signal_enable(uint32)
func signal_ignore(uint32)
func signal_ignored(uint32) bool
func signal_recv() uint32

func loop() {
	for {
		process(syscall.Signal(signal_recv()))
	}
}

func init() {
	watchSignalLoop = loop
}

const (
	numSig = 65 // 所有系统中的最大值
)

func signum(sig os.Signal) int {
	switch sig := sig.(type) {
	case syscall.Signal:
		i := int(sig)
		if i < 0 || i >= numSig {
			return -1
		}
		return i
	default:
		return -1
	}
}

func enableSignal(sig int) {
	signal_enable(uint32(sig))
}

func disableSignal(sig int) {
	signal_disable(uint32(sig))
}

func ignoreSignal(sig int) {
	signal_ignore(uint32(sig))
}

func signalIgnored(sig int) bool {
	return signal_ignored(uint32(sig))
}
