// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package os

import (
	"runtime"
	"sync"
	"syscall"
)

var getwdCache struct {
	sync.Mutex
	dir string
}

// Getwd 返回对应当前目录的绝对路径名。如果当前目录可以通过
// 多个路径到达（由于符号链接），Getwd 可能会返回其中任何一个。
//
// 在 Unix 平台上，如果环境变量 PWD 提供绝对名称，
// 且它是当前目录的名称，则返回它。
func Getwd() (dir string, err error) {
	if runtime.GOOS == "windows" || runtime.GOOS == "plan9" {
		// 直接使用 syscall.Getwd，原因如下：
		//   - plan9: 请参阅 CL 89575 中的原因；
		//   - windows: syscall 实现已足够，
		//     我们不应该依赖 $PWD。
		dir, err = syscall.Getwd()
		return dir, NewSyscallError("getwd", err)
	}

	// 笨拙但广泛使用的技巧：
	// 如果设置了 $PWD 并且与 "." 匹配，使用它。
	var dot FileInfo
	dir = Getenv("PWD")
	if len(dir) > 0 && dir[0] == '/' {
		dot, err = statNolog(".")
		if err != nil {
			return "", err
		}
		d, err := statNolog(dir)
		if err == nil && SameFile(dot, d) {
			return dir, nil
		}
		// 如果这里 err 是 ENAMETOOLONG，下面的 syscall.Getwd
		// 也会因相同错误而失败，但让我们尝试一下，
		// 因为回退代码慢得多。
	}

	// 如果操作系统提供了 Getwd 调用，使用它。
	if syscall.ImplementsGetwd {
		dir, err = ignoringEINTR2(syscall.Getwd)
		// Linux 在结果过长时返回 ENAMETOOLONG。
		// 一些 BSD 系统似乎返回 EINVAL。
		// FreeBSD 系统似乎使用 ENOMEM。
		// Solaris 似乎使用 ERANGE。
		if err != syscall.ENAMETOOLONG && err != syscall.EINVAL && err != errERANGE && err != errENOMEM {
			return dir, NewSyscallError("getwd", err)
		}
	}

	// 我们在尽力找到回到 "." 的路径。
	if dot == nil {
		dot, err = statNolog(".")
		if err != nil {
			return "", err
		}
	}
	// 应用相同的技巧，但使用缓存的 dir 而不是 $PWD。
	getwdCache.Lock()
	dir = getwdCache.dir
	getwdCache.Unlock()
	if len(dir) > 0 {
		d, err := statNolog(dir)
		if err == nil && SameFile(dot, d) {
			return dir, nil
		}
	}

	// Root 是特殊情况，因为它没有父目录
	// 且以斜杠结尾。
	root, err := statNolog("/")
	if err != nil {
		// 无法 stat root——无法继续。
		return "", err
	}
	if SameFile(root, dot) {
		return "/", nil
	}

	// 通用算法：在父目录中查找名称
	// 然后找到父目录的名称。每次迭代
	// 在 dir 的开头添加 /name。
	dir = ""
	for parent := ".."; ; parent = "../" + parent {
		if len(parent) >= 1024 { // 理智检查
			return "", NewSyscallError("getwd", syscall.ENAMETOOLONG)
		}
		fd, err := openDirNolog(parent)
		if err != nil {
			return "", err
		}

		for {
			names, err := fd.Readdirnames(100)
			if err != nil {
				fd.Close()
				// Readdirnames 可能返回 io.EOF 或其他错误。
				// 无论如何，我们在这里是因为 syscall.Getwd
				// 未被实现或因 ENAMETOOLONG 而失败，
				// 所以返回最合理的错误。
				if syscall.ImplementsGetwd {
					return "", NewSyscallError("getwd", syscall.ENAMETOOLONG)
				}
				return "", NewSyscallError("getwd", errENOSYS)
			}
			for _, name := range names {
				d, _ := lstatNolog(parent + "/" + name)
				if SameFile(d, dot) {
					dir = "/" + name + dir
					goto Found
				}
			}
		}

	Found:
		pd, err := fd.Stat()
		fd.Close()
		if err != nil {
			return "", err
		}
		if SameFile(pd, root) {
			break
		}
		// 为下一轮做准备。
		dot = pd
	}

	// 将答案保存为提示，以避免下次的昂贵路径。
	getwdCache.Lock()
	getwdCache.dir = dir
	getwdCache.Unlock()

	return dir, nil
}
