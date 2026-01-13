// 版权所有 2009 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

//go:build unix

// Fork、exec、wait 等。

package syscall

import (
	errorspkg "errors"
	"internal/bytealg"
	"runtime"
	"sync"
	"unsafe"
)

// ForkLock 用于同步新文件描述符的创建与 fork 操作。
//
// 我们希望 fork/exec 序列中的子进程只继承我们预期的文件描述符。
// 为此，我们将所有文件描述符标记为 close-on-exec，
// 然后在子进程中显式取消标记我们希望 exec 的程序保留的那些。
// Unix 不容易做到这一点：通常没有办法分配一个 close-on-exec 的新文件描述符。
// 相反，你必须先分配描述符，然后将其标记为 close-on-exec。
// 如果在这两个事件之间发生 fork，子进程的 exec 将继承一个不需要的文件描述符。
//
// 这个锁解决了这个竞态：创建新 fd/标记 close-on-exec 的操作
// 在持有 ForkLock 读锁的情况下完成，而 fork 本身
// 在持有 ForkLock 写锁的情况下完成。至少，这是我们的想法。
// 但有一些复杂情况。
//
// 一些创建新文件描述符的系统调用可能会阻塞任意长的时间：
// 在挂起的 NFS 服务器或命名管道上 open，在套接字上 accept，等等。
// 我们不能合理地在这些操作期间持有锁。
//
// 继承某些文件描述符比其他的更糟糕。
// 如果一个非恶意的子进程意外继承了一个打开的普通文件，那不是什么大问题。
// 另一方面，如果一个长寿命的子进程意外继承了管道的写端，
// 那么该管道的读者直到该子进程退出才会看到 EOF，
// 这可能导致父程序挂起。这在使用 popen 的多线程 C 程序中是一个常见问题。
//
// 幸运的是，最重要的不应该被继承的文件描述符并不是那些
// 创建需要任意长时间的：pipe 立即返回，net 包使用
// 非阻塞 I/O 在监听套接字上 accept。
// 哪些创建文件描述符的操作使用 ForkLock 的规则如下：
//
//   - [Pipe]。如果可用则使用 pipe2。否则，不会阻塞，所以使用 ForkLock。
//   - [Socket]。如果可用则使用 SOCK_CLOEXEC。否则，不会阻塞，所以使用 ForkLock。
//   - [Open]。如果可用则使用 [O_CLOEXEC]。否则，可能阻塞，所以接受竞态。
//   - [Dup]。如果可用则使用 [F_DUPFD_CLOEXEC] 或 dup3。否则，
//     不会阻塞，所以使用 ForkLock。
var ForkLock sync.RWMutex

// StringSlicePtr 将字符串切片转换为指向以 NUL 结尾的字节数组的指针切片。
// 如果任何字符串包含 NUL 字节，此函数会 panic 而不是返回错误。
//
// 已弃用：请使用 [SlicePtrFromStrings] 代替。
func StringSlicePtr(ss []string) []*byte {
	bb := make([]*byte, len(ss)+1)
	for i := 0; i < len(ss); i++ {
		bb[i] = StringBytePtr(ss[i])
	}
	bb[len(ss)] = nil
	return bb
}

// SlicePtrFromStrings 将字符串切片转换为指向以 NUL 结尾的字节数组的指针切片。
// 如果任何字符串包含 NUL 字节，它返回 (nil, [EINVAL])。
func SlicePtrFromStrings(ss []string) ([]*byte, error) {
	n := 0
	for _, s := range ss {
		if bytealg.IndexByteString(s, 0) != -1 {
			return nil, EINVAL
		}
		n += len(s) + 1 // +1 用于 NUL
	}
	bb := make([]*byte, len(ss)+1)
	b := make([]byte, n)
	n = 0
	for i, s := range ss {
		bb[i] = &b[n]
		copy(b[n:], s)
		n += len(s) + 1
	}
	return bb, nil
}

func CloseOnExec(fd int) { fcntl(fd, F_SETFD, FD_CLOEXEC) }

func SetNonblock(fd int, nonblocking bool) (err error) {
	flag, err := fcntl(fd, F_GETFL, 0)
	if err != nil {
		return err
	}
	if (flag&O_NONBLOCK != 0) == nonblocking {
		return nil
	}
	if nonblocking {
		flag |= O_NONBLOCK
	} else {
		flag &^= O_NONBLOCK
	}
	_, err = fcntl(fd, F_SETFL, flag)
	return err
}

// Credential 保存由 [StartProcess] 启动的子进程将要使用的用户和组身份。
type Credential struct {
	Uid         uint32   // 用户 ID。
	Gid         uint32   // 组 ID。
	Groups      []uint32 // 附加组 ID。
	NoSetGroups bool     // 如果为 true，则不设置附加组
}

// ProcAttr 保存将应用于由 [StartProcess] 启动的新进程的属性。
type ProcAttr struct {
	Dir   string    // 当前工作目录。
	Env   []string  // 环境变量。
	Files []uintptr // 文件描述符。
	Sys   *SysProcAttr
}

var zeroProcAttr ProcAttr
var zeroSysProcAttr SysProcAttr

func forkExec(argv0 string, argv []string, attr *ProcAttr) (pid int, err error) {
	var p [2]int
	var n int
	var err1 Errno
	var wstatus WaitStatus

	if attr == nil {
		attr = &zeroProcAttr
	}
	sys := attr.Sys
	if sys == nil {
		sys = &zeroSysProcAttr
	}

	// 将参数转换为 C 形式。
	argv0p, err := BytePtrFromString(argv0)
	if err != nil {
		return 0, err
	}
	argvp, err := SlicePtrFromStrings(argv)
	if err != nil {
		return 0, err
	}
	envvp, err := SlicePtrFromStrings(attr.Env)
	if err != nil {
		return 0, err
	}

	if (runtime.GOOS == "freebsd" || runtime.GOOS == "dragonfly") && len(argv) > 0 && len(argv[0]) > len(argv0) {
		argvp[0] = argv0p
	}

	var chroot *byte
	if sys.Chroot != "" {
		chroot, err = BytePtrFromString(sys.Chroot)
		if err != nil {
			return 0, err
		}
	}
	var dir *byte
	if attr.Dir != "" {
		dir, err = BytePtrFromString(attr.Dir)
		if err != nil {
			return 0, err
		}
	}

	// Setctty 和 Foreground 都使用 Ctty 字段，
	// 但它们赋予它略有不同的含义。
	if sys.Setctty && sys.Foreground {
		return 0, errorspkg.New("both Setctty and Foreground set in SysProcAttr")
	}
	if sys.Setctty && sys.Ctty >= len(attr.Files) {
		return 0, errorspkg.New("Setctty set but Ctty not valid in child")
	}

	acquireForkLock()

	// 分配子进程状态管道，在 exec 时关闭。
	if err = forkExecPipe(p[:]); err != nil {
		releaseForkLock()
		return 0, err
	}

	// 启动子进程。
	pid, err1 = forkAndExecInChild(argv0p, argvp, envvp, chroot, dir, attr, sys, p[1])
	if err1 != 0 {
		Close(p[0])
		Close(p[1])
		releaseForkLock()
		return 0, Errno(err1)
	}
	releaseForkLock()

	// 从管道读取子进程错误状态。
	Close(p[1])
	for {
		n, err = readlen(p[0], (*byte)(unsafe.Pointer(&err1)), int(unsafe.Sizeof(err1)))
		if err != EINTR {
			break
		}
	}
	Close(p[0])
	if err != nil || n != 0 {
		if n == int(unsafe.Sizeof(err1)) {
			err = Errno(err1)
		}
		if err == nil {
			err = EPIPE
		}

		// 子进程失败；等待它退出，确保不会积累僵尸进程。
		_, err1 := Wait4(pid, &wstatus, 0, nil)
		for err1 == EINTR {
			_, err1 = Wait4(pid, &wstatus, 0, nil)
		}

		// 失败时的操作系统特定清理。
		forkAndExecFailureCleanup(attr, sys)

		return 0, err
	}

	// 读取到 EOF，说明管道在 exec 时关闭，即 exec 成功。
	return pid, nil
}

// fork 和 exec 的组合，注意线程安全。
func ForkExec(argv0 string, argv []string, attr *ProcAttr) (pid int, err error) {
	return forkExec(argv0, argv, attr)
}

// StartProcess 为 os 包封装了 [ForkExec]。
func StartProcess(argv0 string, argv []string, attr *ProcAttr) (pid int, handle uintptr, err error) {
	pid, err = forkExec(argv0, argv, attr)
	return pid, 0, err
}

// 在 runtime 包中实现。
func runtime_BeforeExec()
func runtime_AfterExec()

// execveLibc 在使用 libc 系统调用的操作系统上非空，在 exec_libc.go 中设置为 execve；
// 这避免了其他平台的构建依赖。
var execveLibc func(path *byte, argv **byte, envp **byte) error

// Exec 调用 execve(2) 系统调用。
func Exec(argv0 string, argv []string, envv []string) (err error) {
	argv0p, err := BytePtrFromString(argv0)
	if err != nil {
		return err
	}
	argvp, err := SlicePtrFromStrings(argv)
	if err != nil {
		return err
	}
	envvp, err := SlicePtrFromStrings(envv)
	if err != nil {
		return err
	}
	runtime_BeforeExec()

	rlim := origRlimitNofile.Load()
	if rlim != nil {
		Setrlimit(RLIMIT_NOFILE, rlim)
	}

	var err1 error
	switch runtime.GOOS {
	case "aix", "darwin", "illumos", "ios", "openbsd", "solaris":
		// 在这些平台上不应该使用 RawSyscall。
		err1 = execveLibc(argv0p, &argvp[0], &envvp[0])

	default:
		_, _, err1 = RawSyscall(SYS_EXECVE,
			uintptr(unsafe.Pointer(argv0p)),
			uintptr(unsafe.Pointer(&argvp[0])),
			uintptr(unsafe.Pointer(&envvp[0])))
	}
	runtime_AfterExec()
	return err1
}
