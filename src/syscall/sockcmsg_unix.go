// 版权所有 2011 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

//go:build unix

// 套接字控制消息

package syscall

import (
	"unsafe"
)

// CmsgLen 返回要存储在 [Cmsghdr] 结构的 Len 字段中的值，
// 考虑了任何必要的对齐。
func CmsgLen(datalen int) int {
	return cmsgAlignOf(SizeofCmsghdr) + datalen
}

// CmsgSpace 返回具有指定数据长度载荷的辅助元素占用的字节数。
func CmsgSpace(datalen int) int {
	return cmsgAlignOf(SizeofCmsghdr) + cmsgAlignOf(datalen)
}

func (h *Cmsghdr) data(offset uintptr) unsafe.Pointer {
	return unsafe.Pointer(uintptr(unsafe.Pointer(h)) + uintptr(cmsgAlignOf(SizeofCmsghdr)) + offset)
}

// SocketControlMessage 表示一个套接字控制消息。
type SocketControlMessage struct {
	Header Cmsghdr
	Data   []byte
}

// ParseSocketControlMessage 将 b 解析为套接字控制消息数组。
func ParseSocketControlMessage(b []byte) ([]SocketControlMessage, error) {
	var msgs []SocketControlMessage
	i := 0
	for i+CmsgLen(0) <= len(b) {
		h, dbuf, err := socketControlMessageHeaderAndData(b[i:])
		if err != nil {
			return nil, err
		}
		m := SocketControlMessage{Header: *h, Data: dbuf}
		msgs = append(msgs, m)
		i += cmsgAlignOf(int(h.Len))
	}
	return msgs, nil
}

func socketControlMessageHeaderAndData(b []byte) (*Cmsghdr, []byte, error) {
	h := (*Cmsghdr)(unsafe.Pointer(&b[0]))
	if h.Len < SizeofCmsghdr || uint64(h.Len) > uint64(len(b)) {
		return nil, nil, EINVAL
	}
	return h, b[cmsgAlignOf(SizeofCmsghdr):h.Len], nil
}

// UnixRights 将一组打开的文件描述符编码为套接字控制消息，
// 用于发送给另一个进程。
func UnixRights(fds ...int) []byte {
	datalen := len(fds) * 4
	b := make([]byte, CmsgSpace(datalen))
	h := (*Cmsghdr)(unsafe.Pointer(&b[0]))
	h.Level = SOL_SOCKET
	h.Type = SCM_RIGHTS
	h.SetLen(CmsgLen(datalen))
	for i, fd := range fds {
		*(*int32)(h.data(4 * uintptr(i))) = int32(fd)
	}
	return b
}

// ParseUnixRights 解码包含来自另一个进程的打开文件描述符整数数组的
// 套接字控制消息。
func ParseUnixRights(m *SocketControlMessage) ([]int, error) {
	if m.Header.Level != SOL_SOCKET {
		return nil, EINVAL
	}
	if m.Header.Type != SCM_RIGHTS {
		return nil, EINVAL
	}
	fds := make([]int, len(m.Data)>>2)
	for i, j := 0, 0; i < len(m.Data); i += 4 {
		fds[j] = int(*(*int32)(unsafe.Pointer(&m.Data[i])))
		j++
	}
	return fds, nil
}
