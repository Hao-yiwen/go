// 版权所有 2018 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

//go:build darwin || dragonfly || freebsd || netbsd || openbsd

// BSD 变体的伯克利包过滤器

package syscall

import (
	"unsafe"
)

// 已弃用：请使用 golang.org/x/net/bpf 代替。
func BpfStmt(code, k int) *BpfInsn {
	return &BpfInsn{Code: uint16(code), K: uint32(k)}
}

// 已弃用：请使用 golang.org/x/net/bpf 代替。
func BpfJump(code, k, jt, jf int) *BpfInsn {
	return &BpfInsn{Code: uint16(code), Jt: uint8(jt), Jf: uint8(jf), K: uint32(k)}
}

// 已弃用：请使用 golang.org/x/net/bpf 代替。
func BpfBuflen(fd int) (int, error) {
	var l int
	err := ioctlPtr(fd, BIOCGBLEN, unsafe.Pointer(&l))
	if err != nil {
		return 0, err
	}
	return l, nil
}

// 已弃用：请使用 golang.org/x/net/bpf 代替。
func SetBpfBuflen(fd, l int) (int, error) {
	err := ioctlPtr(fd, BIOCSBLEN, unsafe.Pointer(&l))
	if err != nil {
		return 0, err
	}
	return l, nil
}

// 已弃用：请使用 golang.org/x/net/bpf 代替。
func BpfDatalink(fd int) (int, error) {
	var t int
	err := ioctlPtr(fd, BIOCGDLT, unsafe.Pointer(&t))
	if err != nil {
		return 0, err
	}
	return t, nil
}

// 已弃用：请使用 golang.org/x/net/bpf 代替。
func SetBpfDatalink(fd, t int) (int, error) {
	err := ioctlPtr(fd, BIOCSDLT, unsafe.Pointer(&t))
	if err != nil {
		return 0, err
	}
	return t, nil
}

// 已弃用：请使用 golang.org/x/net/bpf 代替。
func SetBpfPromisc(fd, m int) error {
	err := ioctlPtr(fd, BIOCPROMISC, unsafe.Pointer(&m))
	if err != nil {
		return err
	}
	return nil
}

// 已弃用：请使用 golang.org/x/net/bpf 代替。
func FlushBpf(fd int) error {
	err := ioctlPtr(fd, BIOCFLUSH, nil)
	if err != nil {
		return err
	}
	return nil
}

type ivalue struct {
	name  [IFNAMSIZ]byte
	value int16
}

// 已弃用：请使用 golang.org/x/net/bpf 代替。
func BpfInterface(fd int, name string) (string, error) {
	var iv ivalue
	err := ioctlPtr(fd, BIOCGETIF, unsafe.Pointer(&iv))
	if err != nil {
		return "", err
	}
	return name, nil
}

// 已弃用：请使用 golang.org/x/net/bpf 代替。
func SetBpfInterface(fd int, name string) error {
	var iv ivalue
	copy(iv.name[:], []byte(name))
	err := ioctlPtr(fd, BIOCSETIF, unsafe.Pointer(&iv))
	if err != nil {
		return err
	}
	return nil
}

// 已弃用：请使用 golang.org/x/net/bpf 代替。
func BpfTimeout(fd int) (*Timeval, error) {
	var tv Timeval
	err := ioctlPtr(fd, BIOCGRTIMEOUT, unsafe.Pointer(&tv))
	if err != nil {
		return nil, err
	}
	return &tv, nil
}

// 已弃用：请使用 golang.org/x/net/bpf 代替。
func SetBpfTimeout(fd int, tv *Timeval) error {
	err := ioctlPtr(fd, BIOCSRTIMEOUT, unsafe.Pointer(tv))
	if err != nil {
		return err
	}
	return nil
}

// 已弃用：请使用 golang.org/x/net/bpf 代替。
func BpfStats(fd int) (*BpfStat, error) {
	var s BpfStat
	err := ioctlPtr(fd, BIOCGSTATS, unsafe.Pointer(&s))
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// 已弃用：请使用 golang.org/x/net/bpf 代替。
func SetBpfImmediate(fd, m int) error {
	err := ioctlPtr(fd, BIOCIMMEDIATE, unsafe.Pointer(&m))
	if err != nil {
		return err
	}
	return nil
}

// 已弃用：请使用 golang.org/x/net/bpf 代替。
func SetBpf(fd int, i []BpfInsn) error {
	var p BpfProgram
	p.Len = uint32(len(i))
	p.Insns = (*BpfInsn)(unsafe.Pointer(&i[0]))
	err := ioctlPtr(fd, BIOCSETF, unsafe.Pointer(&p))
	if err != nil {
		return err
	}
	return nil
}

// 已弃用：请使用 golang.org/x/net/bpf 代替。
func CheckBpfVersion(fd int) error {
	var v BpfVersion
	err := ioctlPtr(fd, BIOCVERSION, unsafe.Pointer(&v))
	if err != nil {
		return err
	}
	if v.Major != BPF_MAJOR_VERSION || v.Minor != BPF_MINOR_VERSION {
		return EINVAL
	}
	return nil
}

// 已弃用：请使用 golang.org/x/net/bpf 代替。
func BpfHeadercmpl(fd int) (int, error) {
	var f int
	err := ioctlPtr(fd, BIOCGHDRCMPLT, unsafe.Pointer(&f))
	if err != nil {
		return 0, err
	}
	return f, nil
}

// 已弃用：请使用 golang.org/x/net/bpf 代替。
func SetBpfHeadercmpl(fd, f int) error {
	err := ioctlPtr(fd, BIOCSHDRCMPLT, unsafe.Pointer(&f))
	if err != nil {
		return err
	}
	return nil
}
