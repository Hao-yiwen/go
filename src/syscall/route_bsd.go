// 版权所有 2011 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package syscall

import (
	"runtime"
	"unsafe"
)

var (
	freebsdConfArch       string // freebsd 上 kern.conftxt 中的 "machine $arch" 行
	minRoutingSockaddrLen = rsaAlignOf(0)
)

// 将原始 sockaddr 的长度向上舍入以正确对齐。
func rsaAlignOf(salen int) int {
	salign := sizeofPtr
	if darwin64Bit {
		// Darwin 内核需要 32 位对齐访问路由设施。
		salign = 4
	} else if netbsd32Bit {
		// NetBSD 6 及更高版本内核需要 64 位对齐访问路由设施。
		salign = 8
	} else if runtime.GOOS == "freebsd" {
		// 在 kern.supported_archs="amd64 i386" 的情况下，
		// 我们需要知道底层内核的架构，因为路由设施的对齐
		// 是在内核构建时设置的。
		if freebsdConfArch == "amd64" {
			salign = 8
		}
	}
	if salen == 0 {
		return salign
	}
	return (salen + salign - 1) & ^(salign - 1)
}

// parseSockaddrLink 将 b 解析为数据链路套接字地址。
func parseSockaddrLink(b []byte) (*SockaddrDatalink, error) {
	if len(b) < 8 {
		return nil, EINVAL
	}
	sa, _, err := parseLinkLayerAddr(b[4:])
	if err != nil {
		return nil, err
	}
	rsa := (*RawSockaddrDatalink)(unsafe.Pointer(&b[0]))
	sa.Len = rsa.Len
	sa.Family = rsa.Family
	sa.Index = rsa.Index
	return sa, nil
}

// parseLinkLayerAddr 将 b 解析为传统 BSD 内核形式的数据链路套接字地址。
func parseLinkLayerAddr(b []byte) (*SockaddrDatalink, int, error) {
	// 编码格式如下：
	// +----------------------------+
	// | 类型             (1 字节)  |
	// +----------------------------+
	// | 名称长度         (1 字节)  |
	// +----------------------------+
	// | 地址长度         (1 字节)  |
	// +----------------------------+
	// | 选择器长度       (1 字节)  |
	// +----------------------------+
	// | 数据            (可变长度) |
	// +----------------------------+
	type linkLayerAddr struct {
		Type byte
		Nlen byte
		Alen byte
		Slen byte
	}
	lla := (*linkLayerAddr)(unsafe.Pointer(&b[0]))
	l := 4 + int(lla.Nlen) + int(lla.Alen) + int(lla.Slen)
	if len(b) < l {
		return nil, 0, EINVAL
	}
	b = b[4:]
	sa := &SockaddrDatalink{Type: lla.Type, Nlen: lla.Nlen, Alen: lla.Alen, Slen: lla.Slen}
	for i := 0; len(sa.Data) > i && i < l-4; i++ {
		sa.Data[i] = int8(b[i])
	}
	return sa, rsaAlignOf(l), nil
}

// parseSockaddrInet 将 b 解析为互联网套接字地址。
func parseSockaddrInet(b []byte, family byte) (Sockaddr, error) {
	switch family {
	case AF_INET:
		if len(b) < SizeofSockaddrInet4 {
			return nil, EINVAL
		}
		rsa := (*RawSockaddrAny)(unsafe.Pointer(&b[0]))
		return anyToSockaddr(rsa)
	case AF_INET6:
		if len(b) < SizeofSockaddrInet6 {
			return nil, EINVAL
		}
		rsa := (*RawSockaddrAny)(unsafe.Pointer(&b[0]))
		return anyToSockaddr(rsa)
	default:
		return nil, EINVAL
	}
}

const (
	offsetofInet4 = int(unsafe.Offsetof(RawSockaddrInet4{}.Addr))
	offsetofInet6 = int(unsafe.Offsetof(RawSockaddrInet6{}.Addr))
)

// parseNetworkLayerAddr 将 b 解析为传统 BSD 内核形式的互联网套接字地址。
func parseNetworkLayerAddr(b []byte, family byte) (Sockaddr, error) {
	// 编码格式类似于 NLRI 编码。
	// +----------------------------+
	// | 长度             (1 字节)  |
	// +----------------------------+
	// | 地址前缀        (可变长度) |
	// +----------------------------+
	//
	// 内核形式与 NLRI 编码之间的区别：
	//
	// - 内核形式的长度字段表示前缀长度（以字节为单位），而不是位
	//
	// - 在内核形式中，长度字段的零值不表示 0.0.0.0/0 或 ::/0
	//
	// - 内核形式在前缀字段前附加前导字节，
	//   使 <长度, 前缀> 元组符合路由消息边界
	l := int(rsaAlignOf(int(b[0])))
	if len(b) < l {
		return nil, EINVAL
	}
	// 不要重新排序 case 表达式。
	// IPv6 的 case 表达式必须放在前面。
	switch {
	case b[0] == SizeofSockaddrInet6:
		sa := &SockaddrInet6{}
		copy(sa.Addr[:], b[offsetofInet6:])
		return sa, nil
	case family == AF_INET6:
		sa := &SockaddrInet6{}
		if l-1 < offsetofInet6 {
			copy(sa.Addr[:], b[1:l])
		} else {
			copy(sa.Addr[:], b[l-offsetofInet6:l])
		}
		return sa, nil
	case b[0] == SizeofSockaddrInet4:
		sa := &SockaddrInet4{}
		copy(sa.Addr[:], b[offsetofInet4:])
		return sa, nil
	default: // 旧方式，AF_UNSPEC 或未知表示 AF_INET
		sa := &SockaddrInet4{}
		if l-1 < offsetofInet4 {
			copy(sa.Addr[:], b[1:l])
		} else {
			copy(sa.Addr[:], b[l-offsetofInet4:l])
		}
		return sa, nil
	}
}

// RouteRIB 返回路由信息库（RIB），它包含网络设施信息、状态和参数。
//
// 已弃用：请使用 golang.org/x/net/route 代替。
func RouteRIB(facility, param int) ([]byte, error) {
	mib := []_C_int{CTL_NET, AF_ROUTE, 0, 0, _C_int(facility), _C_int(param)}
	// 查找大小。
	n := uintptr(0)
	if err := sysctl(mib, nil, &n, nil, 0); err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, nil
	}
	tab := make([]byte, n)
	if err := sysctl(mib, &tab[0], &n, nil, 0); err != nil {
		return nil, err
	}
	return tab[:n], nil
}

// RoutingMessage 表示一个路由消息。
//
// 已弃用：请使用 golang.org/x/net/route 代替。
type RoutingMessage interface {
	sockaddr() ([]Sockaddr, error)
}

const anyMessageLen = int(unsafe.Sizeof(anyMessage{}))

type anyMessage struct {
	Msglen  uint16
	Version uint8
	Type    uint8
}

// RouteMessage 表示包含路由条目的路由消息。
//
// 已弃用：请使用 golang.org/x/net/route 代替。
type RouteMessage struct {
	Header RtMsghdr
	Data   []byte
}

func (m *RouteMessage) sockaddr() ([]Sockaddr, error) {
	var sas [RTAX_MAX]Sockaddr
	b := m.Data[:]
	family := uint8(AF_UNSPEC)
	for i := uint(0); i < RTAX_MAX && len(b) >= minRoutingSockaddrLen; i++ {
		if m.Header.Addrs&(1<<i) == 0 {
			continue
		}
		rsa := (*RawSockaddr)(unsafe.Pointer(&b[0]))
		switch rsa.Family {
		case AF_LINK:
			sa, err := parseSockaddrLink(b)
			if err != nil {
				return nil, err
			}
			sas[i] = sa
			b = b[rsaAlignOf(int(rsa.Len)):]
		case AF_INET, AF_INET6:
			sa, err := parseSockaddrInet(b, rsa.Family)
			if err != nil {
				return nil, err
			}
			sas[i] = sa
			b = b[rsaAlignOf(int(rsa.Len)):]
			family = rsa.Family
		default:
			sa, err := parseNetworkLayerAddr(b, family)
			if err != nil {
				return nil, err
			}
			sas[i] = sa
			b = b[rsaAlignOf(int(b[0])):]
		}
	}
	return sas[:], nil
}

// InterfaceMessage 表示包含网络接口条目的路由消息。
//
// 已弃用：请使用 golang.org/x/net/route 代替。
type InterfaceMessage struct {
	Header IfMsghdr
	Data   []byte
}

func (m *InterfaceMessage) sockaddr() ([]Sockaddr, error) {
	var sas [RTAX_MAX]Sockaddr
	if m.Header.Addrs&RTA_IFP == 0 {
		return nil, nil
	}
	sa, err := parseSockaddrLink(m.Data[:])
	if err != nil {
		return nil, err
	}
	sas[RTAX_IFP] = sa
	return sas[:], nil
}

// InterfaceAddrMessage 表示包含网络接口地址条目的路由消息。
//
// 已弃用：请使用 golang.org/x/net/route 代替。
type InterfaceAddrMessage struct {
	Header IfaMsghdr
	Data   []byte
}

func (m *InterfaceAddrMessage) sockaddr() ([]Sockaddr, error) {
	var sas [RTAX_MAX]Sockaddr
	b := m.Data[:]
	family := uint8(AF_UNSPEC)
	for i := uint(0); i < RTAX_MAX && len(b) >= minRoutingSockaddrLen; i++ {
		if m.Header.Addrs&(1<<i) == 0 {
			continue
		}
		rsa := (*RawSockaddr)(unsafe.Pointer(&b[0]))
		switch rsa.Family {
		case AF_LINK:
			sa, err := parseSockaddrLink(b)
			if err != nil {
				return nil, err
			}
			sas[i] = sa
			b = b[rsaAlignOf(int(rsa.Len)):]
		case AF_INET, AF_INET6:
			sa, err := parseSockaddrInet(b, rsa.Family)
			if err != nil {
				return nil, err
			}
			sas[i] = sa
			b = b[rsaAlignOf(int(rsa.Len)):]
			family = rsa.Family
		default:
			sa, err := parseNetworkLayerAddr(b, family)
			if err != nil {
				return nil, err
			}
			sas[i] = sa
			b = b[rsaAlignOf(int(b[0])):]
		}
	}
	return sas[:], nil
}

// ParseRoutingMessage 将 b 解析为路由消息，并返回包含 [RoutingMessage] 接口的切片。
//
// 已弃用：请使用 golang.org/x/net/route 代替。
func ParseRoutingMessage(b []byte) (msgs []RoutingMessage, err error) {
	nmsgs, nskips := 0, 0
	for len(b) >= anyMessageLen {
		nmsgs++
		any := (*anyMessage)(unsafe.Pointer(&b[0]))
		if any.Version != RTM_VERSION {
			b = b[any.Msglen:]
			continue
		}
		if m := any.toRoutingMessage(b); m == nil {
			nskips++
		} else {
			msgs = append(msgs, m)
		}
		b = b[any.Msglen:]
	}
	// 我们未能解析任何消息 - 版本不匹配？
	if nmsgs != len(msgs)+nskips {
		return nil, EINVAL
	}
	return msgs, nil
}

// ParseRoutingSockaddr 将 msg 的载荷解析为原始 sockaddrs，
// 并返回包含 [Sockaddr] 接口的切片。
//
// 已弃用：请使用 golang.org/x/net/route 代替。
func ParseRoutingSockaddr(msg RoutingMessage) ([]Sockaddr, error) {
	sas, err := msg.sockaddr()
	if err != nil {
		return nil, err
	}
	return sas, nil
}
