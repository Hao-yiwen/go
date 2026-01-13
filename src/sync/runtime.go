// 版权所有 2012 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package sync

import "unsafe"

// 在 package runtime 中定义

// Semacquire 等待直到 *s > 0，然后原子地递减它。
// 它旨在作为同步库使用的简单睡眠原语，不应直接使用。
func runtime_Semacquire(s *uint32)

// SemacquireWaitGroup 类似于 Semacquire，但用于 WaitGroup.Wait。
func runtime_SemacquireWaitGroup(s *uint32, synctestDurable bool)

// Semacquire(RW)Mutex(R) 类似于 Semacquire，但用于分析竞争的
// Mutex 和 RWMutex。
// 如果 lifo 为 true，则将等待者排队到等待队列的头部。
// skipframes 是跟踪期间要省略的帧数，从 runtime_SemacquireMutex 的调用者开始计数。
// 此函数的不同形式只是告诉运行时如何在回溯中呈现等待原因，
// 并用于计算一些指标。否则它们在功能上是相同的。
func runtime_SemacquireRWMutexR(s *uint32, lifo bool, skipframes int)
func runtime_SemacquireRWMutex(s *uint32, lifo bool, skipframes int)

// Semrelease 原子地递增 *s，并在有 goroutine 在 Semacquire 中阻塞时通知它。
// 它旨在作为同步库使用的简单唤醒原语，不应直接使用。
// 如果 handoff 为 true，则直接将计数传递给第一个等待者。
// skipframes 是跟踪期间要省略的帧数，从 runtime_Semrelease 的调用者开始计数。
func runtime_Semrelease(s *uint32, handoff bool, skipframes int)

// 文档请参阅 runtime/sema.go。
func runtime_notifyListAdd(l *notifyList) uint32

// 文档请参阅 runtime/sema.go。
func runtime_notifyListWait(l *notifyList, t uint32)

// 文档请参阅 runtime/sema.go。
func runtime_notifyListNotifyAll(l *notifyList)

// 文档请参阅 runtime/sema.go。
func runtime_notifyListNotifyOne(l *notifyList)

// 确保 sync 和 runtime 对 notifyList 大小的认知一致。
func runtime_notifyListCheck(size uintptr)
func init() {
	var n notifyList
	runtime_notifyListCheck(unsafe.Sizeof(n))
}

func throw(string)
func fatal(string)
