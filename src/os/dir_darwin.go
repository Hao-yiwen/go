// 版权所有 2009 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package os

import (
	"io"
	"runtime"
	"syscall"
	"unsafe"
)

// 如果 File 描述目录的辅助信息
type dirInfo struct {
	dir uintptr // 指向 dirent.h 中 DIR 结构的指针
}

func (d *dirInfo) close() {
	if d.dir == 0 {
		return
	}
	closedir(d.dir)
	d.dir = 0
}

func (f *File) readdir(n int, mode readdirMode) (names []string, dirents []DirEntry, infos []FileInfo, err error) {
	// 如果此文件没有 dirinfo，创建一个。
	var d *dirInfo
	for {
		d = f.dirinfo.Load()
		if d != nil {
			break
		}
		dir, call, errno := f.pfd.OpenDir()
		if errno != nil {
			return nil, nil, nil, &PathError{Op: call, Path: f.name, Err: errno}
		}
		d = &dirInfo{dir: dir}
		if f.dirinfo.CompareAndSwap(nil, d) {
			break
		}
		// 我们在竞争中失败了：再试一次。
		d.close()
	}

	size := n
	if size <= 0 {
		size = 100
		n = -1
	}

	var dirent syscall.Dirent
	var entptr *syscall.Dirent
	for len(names)+len(dirents)+len(infos) < size || n == -1 {
		if errno := readdir_r(d.dir, &dirent, &entptr); errno != 0 {
			if errno == syscall.EINTR {
				continue
			}
			return names, dirents, infos, &PathError{Op: "readdir", Path: f.name, Err: errno}
		}
		if entptr == nil { // EOF
			break
		}
		// Darwin 在目录条目已被删除但尚未从目录中移除时可能返回零 inode。
		// getdirentries(2) 的手册页面指出程序负责跳过这些条目：
		//
		//   getdirentries() 的用户应该跳过 d_fileno = 0 的条目，
		//   因为这些条目表示已被删除但尚未从目录条目中删除的文件。
		//
		if dirent.Ino == 0 {
			continue
		}
		name := (*[len(syscall.Dirent{}.Name)]byte)(unsafe.Pointer(&dirent.Name))[:]
		for i, c := range name {
			if c == 0 {
				name = name[:i]
				break
			}
		}
		// Check for useless names before allocating a string.
		if string(name) == "." || string(name) == ".." {
			continue
		}
		if mode == readdirName {
			names = append(names, string(name))
		} else if mode == readdirDirEntry {
			de, err := newUnixDirent(f.name, string(name), dtToType(dirent.Type))
			if IsNotExist(err) {
				// File disappeared between readdir and stat.
				// Treat as if it didn't exist.
				continue
			}
			if err != nil {
				return nil, dirents, nil, err
			}
			dirents = append(dirents, de)
		} else {
			info, err := lstat(f.name + "/" + string(name))
			if IsNotExist(err) {
				// File disappeared between readdir + stat.
				// Treat as if it didn't exist.
				continue
			}
			if err != nil {
				return nil, nil, infos, err
			}
			infos = append(infos, info)
		}
		runtime.KeepAlive(f)
	}

	if n > 0 && len(names)+len(dirents)+len(infos) == 0 {
		return nil, nil, nil, io.EOF
	}
	return names, dirents, infos, nil
}

func dtToType(typ uint8) FileMode {
	switch typ {
	case syscall.DT_BLK:
		return ModeDevice
	case syscall.DT_CHR:
		return ModeDevice | ModeCharDevice
	case syscall.DT_DIR:
		return ModeDir
	case syscall.DT_FIFO:
		return ModeNamedPipe
	case syscall.DT_LNK:
		return ModeSymlink
	case syscall.DT_REG:
		return 0
	case syscall.DT_SOCK:
		return ModeSocket
	}
	return ^FileMode(0)
}

// Implemented in syscall/syscall_darwin.go.

//go:linkname closedir syscall.closedir
func closedir(dir uintptr) (err error)

//go:linkname readdir_r syscall.readdir_r
func readdir_r(dir uintptr, entry *syscall.Dirent, result **syscall.Dirent) (res syscall.Errno)
