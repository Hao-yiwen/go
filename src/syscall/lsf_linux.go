// 版权所有 2011 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// Linux 套接字过滤器

package syscall

import (
	"unsafe"
)

// 已弃用：请使用 golang.org/x/net/bpf 代替。
func LsfStmt(code, k int) *SockFilter {
	return &SockFilter{Code: uint16(code), K: uint32(k)}
}

// 已弃用：请使用 golang.org/x/net/bpf 代替。
func LsfJump(code, k, jt, jf int) *SockFilter {
	return &SockFilter{Code: uint16(code), Jt: uint8(jt), Jf: uint8(jf), K: uint32(k)}
}

// 已弃用：请使用 golang.org/x/net/bpf 代替。
func LsfSocket(ifindex, proto int) (int, error) {
	var lsall SockaddrLinklayer
	// 这里缺少 SOCK_CLOEXEC，但添加该标志可能会破坏调用者。
	s, e := Socket(AF_PACKET, SOCK_RAW, proto)
	if e != nil {
		return 0, e
	}
	p := (*[2]byte)(unsafe.Pointer(&lsall.Protocol))
	p[0] = byte(proto >> 8)
	p[1] = byte(proto)
	lsall.Ifindex = ifindex
	e = Bind(s, &lsall)
	if e != nil {
		Close(s)
		return 0, e
	}
	return s, nil
}

type iflags struct {
	name  [IFNAMSIZ]byte
	flags uint16
}

// 已弃用：请使用 golang.org/x/net/bpf 代替。
func SetLsfPromisc(name string, m bool) error {
	s, e := Socket(AF_INET, SOCK_DGRAM|SOCK_CLOEXEC, 0)
	if e != nil {
		return e
	}
	defer Close(s)
	var ifl iflags
	copy(ifl.name[:], []byte(name))
	_, _, ep := Syscall(SYS_IOCTL, uintptr(s), SIOCGIFFLAGS, uintptr(unsafe.Pointer(&ifl)))
	if ep != 0 {
		return Errno(ep)
	}
	if m {
		ifl.flags |= uint16(IFF_PROMISC)
	} else {
		ifl.flags &^= uint16(IFF_PROMISC)
	}
	_, _, ep = Syscall(SYS_IOCTL, uintptr(s), SIOCSIFFLAGS, uintptr(unsafe.Pointer(&ifl)))
	if ep != 0 {
		return Errno(ep)
	}
	return nil
}

// 已弃用：请使用 golang.org/x/net/bpf 代替。
func AttachLsf(fd int, i []SockFilter) error {
	var p SockFprog
	p.Len = uint16(len(i))
	p.Filter = (*SockFilter)(unsafe.Pointer(&i[0]))
	return setsockopt(fd, SOL_SOCKET, SO_ATTACH_FILTER, unsafe.Pointer(&p), unsafe.Sizeof(p))
}

// 已弃用：请使用 golang.org/x/net/bpf 代替。
func DetachLsf(fd int) error {
	var dummy int
	return setsockopt(fd, SOL_SOCKET, SO_DETACH_FILTER, unsafe.Pointer(&dummy), unsafe.Sizeof(dummy))
}
