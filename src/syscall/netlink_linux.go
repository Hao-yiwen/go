// 版权所有 2011 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// Netlink 套接字和消息

package syscall

import (
	"sync"
	"unsafe"
)

// 将 netlink 消息的长度向上舍入以正确对齐。
func nlmAlignOf(msglen int) int {
	return (msglen + NLMSG_ALIGNTO - 1) & ^(NLMSG_ALIGNTO - 1)
}

// 将 netlink 路由属性的长度向上舍入以正确对齐。
func rtaAlignOf(attrlen int) int {
	return (attrlen + RTA_ALIGNTO - 1) & ^(RTA_ALIGNTO - 1)
}

// NetlinkRouteRequest 表示从内核接收路由和链路状态的请求消息。
type NetlinkRouteRequest struct {
	Header NlMsghdr
	Data   RtGenmsg
}

func (rr *NetlinkRouteRequest) toWireFormat() []byte {
	b := make([]byte, rr.Header.Len)
	*(*uint32)(unsafe.Pointer(&b[0:4][0])) = rr.Header.Len
	*(*uint16)(unsafe.Pointer(&b[4:6][0])) = rr.Header.Type
	*(*uint16)(unsafe.Pointer(&b[6:8][0])) = rr.Header.Flags
	*(*uint32)(unsafe.Pointer(&b[8:12][0])) = rr.Header.Seq
	*(*uint32)(unsafe.Pointer(&b[12:16][0])) = rr.Header.Pid
	b[16] = rr.Data.Family
	return b
}

func newNetlinkRouteRequest(proto, seq, family int) []byte {
	rr := &NetlinkRouteRequest{}
	rr.Header.Len = uint32(NLMSG_HDRLEN + SizeofRtGenmsg)
	rr.Header.Type = uint16(proto)
	rr.Header.Flags = NLM_F_DUMP | NLM_F_REQUEST
	rr.Header.Seq = uint32(seq)
	rr.Data.Family = uint8(family)
	return rr.toWireFormat()
}

var pageBufPool = &sync.Pool{New: func() any {
	b := make([]byte, Getpagesize())
	return &b
}}

// NetlinkRIB 返回路由信息库（RIB），它包含网络设施信息、状态和参数。
func NetlinkRIB(proto, family int) ([]byte, error) {
	s, err := Socket(AF_NETLINK, SOCK_RAW|SOCK_CLOEXEC, NETLINK_ROUTE)
	if err != nil {
		return nil, err
	}
	defer Close(s)
	sa := &SockaddrNetlink{Family: AF_NETLINK}
	if err := Bind(s, sa); err != nil {
		return nil, err
	}
	wb := newNetlinkRouteRequest(proto, 1, family)
	if err := Sendto(s, wb, 0, sa); err != nil {
		return nil, err
	}
	lsa, err := Getsockname(s)
	if err != nil {
		return nil, err
	}
	lsanl, ok := lsa.(*SockaddrNetlink)
	if !ok {
		return nil, EINVAL
	}
	var tab []byte

	rbNew := pageBufPool.Get().(*[]byte)
	defer pageBufPool.Put(rbNew)
done:
	for {
		rb := *rbNew
		nr, _, err := Recvfrom(s, rb, 0)
		if err != nil {
			return nil, err
		}
		if nr < NLMSG_HDRLEN {
			return nil, EINVAL
		}
		rb = rb[:nr]
		tab = append(tab, rb...)
		msgs, err := ParseNetlinkMessage(rb)
		if err != nil {
			return nil, err
		}
		for _, m := range msgs {
			if m.Header.Seq != 1 || m.Header.Pid != lsanl.Pid {
				return nil, EINVAL
			}
			if m.Header.Type == NLMSG_DONE {
				break done
			}
			if m.Header.Type == NLMSG_ERROR {
				return nil, EINVAL
			}
		}
	}
	return tab, nil
}

// NetlinkMessage 表示一个 netlink 消息。
type NetlinkMessage struct {
	Header NlMsghdr
	Data   []byte
}

// ParseNetlinkMessage 将 b 解析为 netlink 消息数组，
// 并返回包含 NetlinkMessage 结构的切片。
func ParseNetlinkMessage(b []byte) ([]NetlinkMessage, error) {
	var msgs []NetlinkMessage
	for len(b) >= NLMSG_HDRLEN {
		h, dbuf, dlen, err := netlinkMessageHeaderAndData(b)
		if err != nil {
			return nil, err
		}
		m := NetlinkMessage{Header: *h, Data: dbuf[:int(h.Len)-NLMSG_HDRLEN]}
		msgs = append(msgs, m)
		b = b[dlen:]
	}
	return msgs, nil
}

func netlinkMessageHeaderAndData(b []byte) (*NlMsghdr, []byte, int, error) {
	h := (*NlMsghdr)(unsafe.Pointer(&b[0]))
	l := nlmAlignOf(int(h.Len))
	if int(h.Len) < NLMSG_HDRLEN || l > len(b) {
		return nil, nil, 0, EINVAL
	}
	return h, b[NLMSG_HDRLEN:], l, nil
}

// NetlinkRouteAttr 表示一个 netlink 路由属性。
type NetlinkRouteAttr struct {
	Attr  RtAttr
	Value []byte
}

// ParseNetlinkRouteAttr 将 m 的载荷解析为 netlink 路由属性数组，
// 并返回包含 NetlinkRouteAttr 结构的切片。
func ParseNetlinkRouteAttr(m *NetlinkMessage) ([]NetlinkRouteAttr, error) {
	var b []byte
	switch m.Header.Type {
	case RTM_NEWLINK, RTM_DELLINK:
		b = m.Data[SizeofIfInfomsg:]
	case RTM_NEWADDR, RTM_DELADDR:
		b = m.Data[SizeofIfAddrmsg:]
	case RTM_NEWROUTE, RTM_DELROUTE:
		b = m.Data[SizeofRtMsg:]
	default:
		return nil, EINVAL
	}
	var attrs []NetlinkRouteAttr
	for len(b) >= SizeofRtAttr {
		a, vbuf, alen, err := netlinkRouteAttrAndValue(b)
		if err != nil {
			return nil, err
		}
		ra := NetlinkRouteAttr{Attr: *a, Value: vbuf[:int(a.Len)-SizeofRtAttr]}
		attrs = append(attrs, ra)
		b = b[alen:]
	}
	return attrs, nil
}

func netlinkRouteAttrAndValue(b []byte) (*RtAttr, []byte, int, error) {
	a := (*RtAttr)(unsafe.Pointer(&b[0]))
	if int(a.Len) < SizeofRtAttr || int(a.Len) > len(b) {
		return nil, nil, 0, EINVAL
	}
	return a, b[SizeofRtAttr:], rtaAlignOf(int(a.Len)), nil
}
