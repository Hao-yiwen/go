// 版权所有 2011 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// 此文件名为 cgo_unix.go，但为了允许基于系统调用到 libc 的
// 实现共享代码，它不直接使用 cgo。
// 它使用 _C_foo 而不是 C.foo，这在 cgo_unix_cgo.go
// 或 cgo_unix_syscall.go 中定义。

//go:build !netgo && ((cgo && unix) || darwin)

package net

import (
	"context"
	"errors"
	"internal/bytealg"
	"net/netip"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/net/dns/dnsmessage"
)

// cgoAvailable 设置为 true 表示此系统上
// cgo 解析器可用。
const cgoAvailable = true

// addrinfoErrno 表示 getaddrinfo、getnameinfo 特定的
// 错误号。它是一个有符号数，按惯例零值表示无错误。
type addrinfoErrno int

func (eai addrinfoErrno) Error() string   { return _C_gai_strerror(_C_int(eai)) }
func (eai addrinfoErrno) Temporary() bool { return eai == _C_EAI_AGAIN }
func (eai addrinfoErrno) Timeout() bool   { return false }

// isAddrinfoErrno 仅用于测试目的。
func (eai addrinfoErrno) isAddrinfoErrno() {}

// doBlockingWithCtx 当提供的上下文可取消时，在单独的 goroutine 中执行阻塞函数。
// 它用于不支持上下文取消的调用（cgo、系统调用）。
// 此函数结束后，阻塞函数可能仍在运行。
// 在阻塞函数执行期间，使用 [acquireThread] "获取" 线程，
// 当上下文提前取消时，阻塞函数可能不会被执行。
func doBlockingWithCtx[T any](ctx context.Context, lookupName string, blocking func() (T, error)) (T, error) {
	if err := acquireThread(ctx); err != nil {
		var zero T
		return zero, newDNSError(mapErr(err), lookupName, "")
	}

	if ctx.Done() == nil {
		defer releaseThread()
		return blocking()
	}

	type result struct {
		res T
		err error
	}

	res := make(chan result, 1)
	go func() {
		defer releaseThread()
		var r result
		r.res, r.err = blocking()
		res <- r
	}()

	select {
	case r := <-res:
		return r.res, r.err
	case <-ctx.Done():
		var zero T
		return zero, newDNSError(mapErr(ctx.Err()), lookupName, "")
	}
}

func cgoLookupHost(ctx context.Context, name string) (hosts []string, err error) {
	addrs, err := cgoLookupIP(ctx, "ip", name)
	if err != nil {
		return nil, err
	}
	for _, addr := range addrs {
		hosts = append(hosts, addr.String())
	}
	return hosts, nil
}

func cgoLookupPort(ctx context.Context, network, service string) (port int, err error) {
	var hints _C_struct_addrinfo
	switch network {
	case "ip": // 无提示
	case "tcp", "tcp4", "tcp6":
		*_C_ai_socktype(&hints) = _C_SOCK_STREAM
		*_C_ai_protocol(&hints) = _C_IPPROTO_TCP
	case "udp", "udp4", "udp6":
		*_C_ai_socktype(&hints) = _C_SOCK_DGRAM
		*_C_ai_protocol(&hints) = _C_IPPROTO_UDP
	default:
		return 0, &DNSError{Err: "unknown network", Name: network + "/" + service}
	}
	switch ipVersion(network) {
	case '4':
		*_C_ai_family(&hints) = _C_AF_INET
	case '6':
		*_C_ai_family(&hints) = _C_AF_INET6
	}

	return doBlockingWithCtx(ctx, network+"/"+service, func() (int, error) {
		return cgoLookupServicePort(&hints, network, service)
	})
}

func cgoLookupServicePort(hints *_C_struct_addrinfo, network, service string) (port int, err error) {
	cservice, err := syscall.ByteSliceFromString(service)
	if err != nil {
		return 0, &DNSError{Err: err.Error(), Name: network + "/" + service}
	}
	// 将 C 服务名转换为小写。
	for i, b := range cservice[:len(service)] {
		cservice[i] = lowerASCII(b)
	}
	var res *_C_struct_addrinfo
	gerrno, err := _C_getaddrinfo(nil, (*_C_char)(unsafe.Pointer(&cservice[0])), hints, &res)
	if gerrno != 0 {
		switch gerrno {
		case _C_EAI_SYSTEM:
			if err == nil { // 参见 golang.org/issue/6232
				err = syscall.EMFILE
			}
			return 0, newDNSError(err, network+"/"+service, "")
		case _C_EAI_SERVICE, _C_EAI_NONAME: // Darwin 返回 EAI_NONAME。
			return 0, newDNSError(errUnknownPort, network+"/"+service, "")
		default:
			return 0, newDNSError(addrinfoErrno(gerrno), network+"/"+service, "")
		}
	}
	defer _C_freeaddrinfo(res)

	for r := res; r != nil; r = *_C_ai_next(r) {
		switch *_C_ai_family(r) {
		case _C_AF_INET:
			sa := (*syscall.RawSockaddrInet4)(unsafe.Pointer(*_C_ai_addr(r)))
			p := (*[2]byte)(unsafe.Pointer(&sa.Port))
			return int(p[0])<<8 | int(p[1]), nil
		case _C_AF_INET6:
			sa := (*syscall.RawSockaddrInet6)(unsafe.Pointer(*_C_ai_addr(r)))
			p := (*[2]byte)(unsafe.Pointer(&sa.Port))
			return int(p[0])<<8 | int(p[1]), nil
		}
	}
	return 0, newDNSError(errUnknownPort, network+"/"+service, "")
}

func cgoLookupHostIP(network, name string) (addrs []IPAddr, err error) {
	var hints _C_struct_addrinfo
	*_C_ai_flags(&hints) = cgoAddrInfoFlags
	*_C_ai_socktype(&hints) = _C_SOCK_STREAM
	*_C_ai_family(&hints) = _C_AF_UNSPEC
	switch ipVersion(network) {
	case '4':
		*_C_ai_family(&hints) = _C_AF_INET
	case '6':
		*_C_ai_family(&hints) = _C_AF_INET6
	}

	h, err := syscall.BytePtrFromString(name)
	if err != nil {
		return nil, &DNSError{Err: err.Error(), Name: name}
	}
	var res *_C_struct_addrinfo
	gerrno, err := _C_getaddrinfo((*_C_char)(unsafe.Pointer(h)), nil, &hints, &res)
	if gerrno != 0 {
		switch gerrno {
		case _C_EAI_SYSTEM:
			if err == nil {
				// err 不应该为 nil，但有时在 Linux 上 getaddrinfo 返回
				// gerrno == _C_EAI_SYSTEM 且 err == nil。
				// 报告称这发生在我们打开的文件太多时，
				// 所以使用 syscall.EMFILE（系统中打开的文件太多）。
				// 大多数系统调用会返回 ENFILE（打开的文件太多），
				// 所以至少如果再次出现这种情况，EMFILE 应该容易识别。
				// golang.org/issue/6232。
				err = syscall.EMFILE
			}
			return nil, newDNSError(err, name, "")
		case _C_EAI_NONAME, _C_EAI_NODATA:
			return nil, newDNSError(errNoSuchHost, name, "")
		case _C_EAI_ADDRFAMILY:
			if runtime.GOOS == "freebsd" {
				// FreeBSD 在 13.2 版本开始对没有 A 记录的有效主机
				// 返回 EAI_ADDRFAMILY。我们之前对这种情况返回
				// "no such host"。
				//
				// https://bugs.freebsd.org/bugzilla/show_bug.cgi?id=273912
				return nil, newDNSError(errNoSuchHost, name, "")
			}
			fallthrough
		default:
			return nil, newDNSError(addrinfoErrno(gerrno), name, "")
		}

	}
	defer _C_freeaddrinfo(res)

	for r := res; r != nil; r = *_C_ai_next(r) {
		// 我们只请求了 SOCK_STREAM，但还是检查一下。
		if *_C_ai_socktype(r) != _C_SOCK_STREAM {
			continue
		}
		switch *_C_ai_family(r) {
		case _C_AF_INET:
			sa := (*syscall.RawSockaddrInet4)(unsafe.Pointer(*_C_ai_addr(r)))
			addr := IPAddr{IP: copyIP(sa.Addr[:])}
			addrs = append(addrs, addr)
		case _C_AF_INET6:
			sa := (*syscall.RawSockaddrInet6)(unsafe.Pointer(*_C_ai_addr(r)))
			addr := IPAddr{IP: copyIP(sa.Addr[:]), Zone: zoneCache.name(int(sa.Scope_id))}
			addrs = append(addrs, addr)
		}
	}
	return addrs, nil
}

func cgoLookupIP(ctx context.Context, network, name string) (addrs []IPAddr, err error) {
	return doBlockingWithCtx(ctx, name, func() ([]IPAddr, error) {
		return cgoLookupHostIP(network, name)
	})
}

// 这些大致足够用于以下情况：
//
//	 来源		编码				单个名称条目的最大长度
//	 单播 DNS		ASCII 或			<=253 + NUL 终止符
//				RFC 5892 中的 Unicode		252 * 标签总数 + 分隔符 + NUL 终止符
//	 多播 DNS	RFC 5198 中的 UTF-8 或		<=253 + NUL 终止符
//				与单播 DNS ASCII 相同		<=253 + NUL 终止符
//	 本地数据库	各种				取决于实现
const (
	nameinfoLen    = 64
	maxNameinfoLen = 4096
)

func cgoLookupPTR(ctx context.Context, addr string) (names []string, err error) {
	ip, err := netip.ParseAddr(addr)
	if err != nil {
		return nil, &DNSError{Err: "invalid address", Name: addr}
	}
	sa, salen := cgoSockaddr(IP(ip.AsSlice()), ip.Zone())
	if sa == nil {
		return nil, &DNSError{Err: "invalid address " + ip.String(), Name: addr}
	}

	return doBlockingWithCtx(ctx, addr, func() ([]string, error) {
		return cgoLookupAddrPTR(addr, sa, salen)
	})
}

func cgoLookupAddrPTR(addr string, sa *_C_struct_sockaddr, salen _C_socklen_t) (names []string, err error) {
	var gerrno int
	var b []byte
	for l := nameinfoLen; l <= maxNameinfoLen; l *= 2 {
		b = make([]byte, l)
		gerrno, err = cgoNameinfoPTR(b, sa, salen)
		if gerrno == 0 || gerrno != _C_EAI_OVERFLOW {
			break
		}
	}
	if gerrno != 0 {
		switch gerrno {
		case _C_EAI_SYSTEM:
			if err == nil { // 参见 golang.org/issue/6232
				err = syscall.EMFILE
			}
			return nil, newDNSError(err, addr, "")
		case _C_EAI_NONAME:
			return nil, newDNSError(errNoSuchHost, addr, "")
		default:
			return nil, newDNSError(addrinfoErrno(gerrno), addr, "")
		}
	}
	if i := bytealg.IndexByte(b, 0); i != -1 {
		b = b[:i]
	}
	return []string{absDomainName(string(b))}, nil
}

func cgoSockaddr(ip IP, zone string) (*_C_struct_sockaddr, _C_socklen_t) {
	if ip4 := ip.To4(); ip4 != nil {
		return cgoSockaddrInet4(ip4), _C_socklen_t(syscall.SizeofSockaddrInet4)
	}
	if ip6 := ip.To16(); ip6 != nil {
		return cgoSockaddrInet6(ip6, zoneCache.index(zone)), _C_socklen_t(syscall.SizeofSockaddrInet6)
	}
	return nil, 0
}

func cgoLookupCNAME(ctx context.Context, name string) (cname string, err error) {
	resources, err := resSearch(ctx, name, int(dnsmessage.TypeCNAME), int(dnsmessage.ClassINET))
	if err != nil {
		return "", err
	}
	cname, err = parseCNAMEFromResources(resources)
	if err != nil {
		return "", err
	}
	return cname, nil
}

// resSearch 将调用 C 库中的 'res_nsearch' 例程
// 并将输出解析为 DNS 资源切片。
func resSearch(ctx context.Context, hostname string, rtype, class int) ([]dnsmessage.Resource, error) {
	return doBlockingWithCtx(ctx, hostname, func() ([]dnsmessage.Resource, error) {
		return cgoResSearch(hostname, rtype, class)
	})
}

func cgoResSearch(hostname string, rtype, class int) ([]dnsmessage.Resource, error) {
	resStateSize := unsafe.Sizeof(_C_struct___res_state{})
	var state *_C_struct___res_state
	if resStateSize > 0 {
		mem := _C_malloc(resStateSize)
		defer _C_free(mem)
		memSlice := unsafe.Slice((*byte)(mem), resStateSize)
		clear(memSlice)
		state = (*_C_struct___res_state)(unsafe.Pointer(&memSlice[0]))
	}
	if err := _C_res_ninit(state); err != nil {
		return nil, errors.New("res_ninit failure: " + err.Error())
	}
	defer _C_res_nclose(state)

	// 一些 res_nsearch 实现（如 macOS）不设置 errno。
	// 它们设置 h_errno，这不是线程特定的，对我们没用。
	// res_nsearch 返回 DNS 响应包的大小。
	// 但如果 DNS 响应包包含类似失败的响应代码，
	// res_search 返回 -1，即使它已将包复制到 buf 中，
	// 使我们无法知道包有多大。
	// 目前，我们愿意相信 res_search 的判断，认为响应中没有
	// 有用的内容，即使确实有响应。
	bufSize := maxDNSPacketSize
	buf := (*_C_uchar)(_C_malloc(uintptr(bufSize)))
	defer _C_free(unsafe.Pointer(buf))

	s, err := syscall.BytePtrFromString(hostname)
	if err != nil {
		return nil, err
	}

	var size int
	for {
		size = _C_res_nsearch(state, (*_C_char)(unsafe.Pointer(s)), class, rtype, buf, bufSize)
		if size <= 0 || size > 0xffff {
			return nil, errors.New("res_nsearch failure")
		}
		if size <= bufSize {
			break
		}

		// 分配更大的缓冲区以容纳整个消息。
		_C_free(unsafe.Pointer(buf))
		bufSize = size
		buf = (*_C_uchar)(_C_malloc(uintptr(bufSize)))
	}

	var p dnsmessage.Parser
	if _, err := p.Start(unsafe.Slice((*byte)(unsafe.Pointer(buf)), size)); err != nil {
		return nil, err
	}
	p.SkipAllQuestions()
	resources, err := p.AllAnswers()
	if err != nil {
		return nil, err
	}
	return resources, nil
}
