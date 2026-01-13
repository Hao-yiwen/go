// 版权所有 2011 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

//go:build aix || darwin

package syscall

// forkExecPipe 打开一个管道，并非原子地在两个文件描述符上设置 O_CLOEXEC。
func forkExecPipe(p []int) error {
	err := Pipe(p)
	if err != nil {
		return err
	}
	_, err = fcntl(p[0], F_SETFD, FD_CLOEXEC)
	if err != nil {
		return err
	}
	_, err = fcntl(p[1], F_SETFD, FD_CLOEXEC)
	return err
}

func acquireForkLock() {
	ForkLock.Lock()
}

func releaseForkLock() {
	ForkLock.Unlock()
}
