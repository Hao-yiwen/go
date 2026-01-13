// 版权所有 2010 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package net

import (
	"context"
	"internal/bytealg"
	"internal/godebug"
	"internal/nettrace"
	"net/netip"
	"syscall"
	"time"
)

const (
	// defaultTCPKeepAliveIdle 是 TCP_KEEPIDLE 的默认常量值。
	// 详情参见 go.dev/issue/31510。
	defaultTCPKeepAliveIdle = 15 * time.Second

	// defaultTCPKeepAliveInterval 是 TCP_KEEPINTVL 的默认常量值。
	// 它与 defaultTCPKeepAliveIdle 相同，详情参见 go.dev/issue/31510。
	defaultTCPKeepAliveInterval = 15 * time.Second

	// defaultTCPKeepAliveCount 是 TCP_KEEPCNT 的默认常量值。
	defaultTCPKeepAliveCount = 9

	// 目前，如果可用，多路径 TCP 默认用于监听器，但不用于拨号器。
	// 参见 go.dev/issue/56539
	defaultMPTCPEnabledListen = true
	defaultMPTCPEnabledDial   = false
)

// 提供的服务类型
//
//	0 == MPTCP 禁用
//	1 == MPTCP 启用
//	2 == 仅在监听器上启用 MPTCP
//	3 == 仅在拨号器上启用 MPTCP
var multipathtcp = godebug.New("multipathtcp")

// mptcpStatusDial 是客户端上多路径 TCP 的三态值，
// 参见 go.dev/issue/56539
type mptcpStatusDial uint8

const (
	// 值 0 是系统默认值，与 defaultMPTCPEnabledDial 关联
	mptcpUseDefaultDial mptcpStatusDial = iota
	mptcpEnabledDial
	mptcpDisabledDial
)

func (m *mptcpStatusDial) get() bool {
	switch *m {
	case mptcpEnabledDial:
		return true
	case mptcpDisabledDial:
		return false
	}

	// 如果通过 GODEBUG=multipathtcp=1 强制启用 MPTCP
	if multipathtcp.Value() == "1" || multipathtcp.Value() == "3" {
		multipathtcp.IncNonDefault()

		return true
	}

	return defaultMPTCPEnabledDial
}

func (m *mptcpStatusDial) set(use bool) {
	if use {
		*m = mptcpEnabledDial
	} else {
		*m = mptcpDisabledDial
	}
}

// mptcpStatusListen 是服务器上多路径 TCP 的三态值，
// 参见 go.dev/issue/56539
type mptcpStatusListen uint8

const (
	// 值 0 是系统默认值，与 defaultMPTCPEnabledListen 关联
	mptcpUseDefaultListen mptcpStatusListen = iota
	mptcpEnabledListen
	mptcpDisabledListen
)

func (m *mptcpStatusListen) get() bool {
	switch *m {
	case mptcpEnabledListen:
		return true
	case mptcpDisabledListen:
		return false
	}

	// 如果通过 GODEBUG=multipathtcp=0 禁用 MPTCP 或
	// 仅在拨号器上启用，而不在监听器上启用。
	if multipathtcp.Value() == "0" || multipathtcp.Value() == "3" {
		multipathtcp.IncNonDefault()

		return false
	}

	return defaultMPTCPEnabledListen
}

func (m *mptcpStatusListen) set(use bool) {
	if use {
		*m = mptcpEnabledListen
	} else {
		*m = mptcpDisabledListen
	}
}

// Dialer 包含连接到地址的选项。
//
// 每个字段的零值等同于不使用该选项进行拨号。
// 因此，使用 Dialer 的零值进行拨号等同于直接调用 [Dial] 函数。
//
// 并发调用 Dialer 的方法是安全的。
type Dialer struct {
	// Timeout 是拨号等待连接完成的最长时间。
	// 如果同时设置了 Deadline，可能会更早失败。
	//
	// 默认值是没有超时。
	//
	// 当使用 TCP 并拨号具有多个 IP 地址的主机名时，
	// 超时时间可能会在它们之间分配。
	//
	// 无论是否设置超时，操作系统可能会强加自己更早的超时。
	// 例如，TCP 超时通常约为 3 分钟。
	Timeout time.Duration

	// Deadline 是拨号将失败的绝对时间点。
	// 如果设置了 Timeout，可能会更早失败。
	// 零值表示没有截止时间，或者像 Timeout 选项一样取决于操作系统。
	Deadline time.Time

	// LocalAddr 是拨号地址时使用的本地地址。
	// 地址必须是与正在拨号的网络兼容的类型。
	// 如果为 nil，则自动选择本地地址。
	LocalAddr Addr

	// DualStack 以前用于启用 RFC 6555 快速回退支持，
	// 也称为 "Happy Eyeballs"，其中如果 IPv6 似乎配置错误
	// 并挂起，则很快尝试 IPv4。
	//
	// 已弃用：快速回退默认启用。
	// 要禁用，请将 FallbackDelay 设置为负值。
	DualStack bool

	// FallbackDelay 指定在生成 RFC 6555 快速回退连接之前
	// 等待的时间长度。也就是说，这是在假设 IPv6 配置错误
	// 并回退到 IPv4 之前等待 IPv6 成功的时间。
	//
	// 如果为零，则使用 300 毫秒的默认延迟。
	// 负值禁用快速回退支持。
	FallbackDelay time.Duration

	// KeepAlive 指定活动网络连接的保活探测之间的间隔。
	//
	// 如果 KeepAliveConfig.Enable 为 true，则忽略 KeepAlive。
	//
	// 如果为零，且协议和操作系统支持，则使用默认值
	// （当前为 15 秒）发送保活探测。
	// 不支持保活的网络协议或操作系统会忽略此字段。
	// 如果为负，则禁用保活探测。
	KeepAlive time.Duration

	// KeepAliveConfig 指定活动网络连接的保活探测配置，
	// 当协议和操作系统支持时。
	//
	// 如果 KeepAliveConfig.Enable 为 true，则启用保活探测。
	// 如果 KeepAliveConfig.Enable 为 false 且 KeepAlive 为负，
	// 则禁用保活探测。
	KeepAliveConfig KeepAliveConfig

	// Resolver 可选地指定要使用的备用解析器。
	Resolver *Resolver

	// Cancel 是一个可选通道，其关闭表示应取消拨号。
	// 并非所有类型的拨号都支持取消。
	//
	// 已弃用：请改用 DialContext。
	Cancel <-chan struct{}

	// 如果 Control 不为 nil，它在创建网络连接之后
	// 但在实际拨号之前被调用。
	//
	// 传递给 Control 函数的网络和地址参数不一定是
	// 传递给 Dial 的那些。使用 TCP 网络调用 Dial
	// 将导致 Control 函数被调用时使用 "tcp4" 或 "tcp6"，
	// UDP 网络变为 "udp4" 或 "udp6"，IP 网络变为 "ip4" 或 "ip6"，
	// 其他已知网络按原样传递。
	//
	// 如果 ControlContext 不为 nil，则忽略 Control。
	Control func(network, address string, c syscall.RawConn) error

	// 如果 ControlContext 不为 nil，它在创建网络连接之后
	// 但在实际拨号之前被调用。
	//
	// 传递给 ControlContext 函数的网络和地址参数不一定是
	// 传递给 Dial 的那些。使用 TCP 网络调用 Dial
	// 将导致 ControlContext 函数被调用时使用 "tcp4" 或 "tcp6"，
	// UDP 网络变为 "udp4" 或 "udp6"，IP 网络变为 "ip4" 或 "ip6"，
	// 其他已知网络按原样传递。
	//
	// 如果 ControlContext 不为 nil，则忽略 Control。
	ControlContext func(ctx context.Context, network, address string, c syscall.RawConn) error

	// 如果 mptcpStatus 设置为允许使用多路径 TCP (MPTCP) 的值，
	// 任何使用 "tcp(4|6)" 作为网络的 Dial 调用将使用 MPTCP
	// （如果操作系统支持）。
	mptcpStatus mptcpStatusDial
}

func (d *Dialer) dualStack() bool { return d.FallbackDelay >= 0 }

func minNonzeroTime(a, b time.Time) time.Time {
	if a.IsZero() {
		return b
	}
	if b.IsZero() || a.Before(b) {
		return a
	}
	return b
}

// deadline 返回以下最早的时间：
//   - now+Timeout
//   - d.Deadline
//   - 上下文的截止时间
//
// 如果 Timeout、Deadline 或上下文的截止时间都未设置，则返回零值。
func (d *Dialer) deadline(ctx context.Context, now time.Time) (earliest time.Time) {
	if d.Timeout != 0 { // 包括负值，出于历史原因
		earliest = now.Add(d.Timeout)
	}
	if d, ok := ctx.Deadline(); ok {
		earliest = minNonzeroTime(earliest, d)
	}
	return minNonzeroTime(earliest, d.Deadline)
}

func (d *Dialer) resolver() *Resolver {
	if d.Resolver != nil {
		return d.Resolver
	}
	return DefaultResolver
}

// partialDeadline 返回当有多个地址待处理时，
// 单个地址使用的截止时间。
func partialDeadline(now, deadline time.Time, addrsRemaining int) (time.Time, error) {
	if deadline.IsZero() {
		return deadline, nil
	}
	timeRemaining := deadline.Sub(now)
	if timeRemaining <= 0 {
		return time.Time{}, errTimeout
	}
	// 暂时为每个剩余地址分配相等的时间。
	timeout := timeRemaining / time.Duration(addrsRemaining)
	// 如果每个地址的时间太短，从列表末尾借用。
	const saneMinimum = 2 * time.Second
	if timeout < saneMinimum {
		if timeRemaining < saneMinimum {
			timeout = timeRemaining
		} else {
			timeout = saneMinimum
		}
	}
	return now.Add(timeout), nil
}

func (d *Dialer) fallbackDelay() time.Duration {
	if d.FallbackDelay > 0 {
		return d.FallbackDelay
	} else {
		return 300 * time.Millisecond
	}
}

func parseNetwork(ctx context.Context, network string, needsProto bool) (afnet string, proto int, err error) {
	i := bytealg.LastIndexByteString(network, ':')
	if i < 0 { // 没有冒号
		switch network {
		case "tcp", "tcp4", "tcp6":
		case "udp", "udp4", "udp6":
		case "ip", "ip4", "ip6":
			if needsProto {
				return "", 0, UnknownNetworkError(network)
			}
		case "unix", "unixgram", "unixpacket":
		default:
			return "", 0, UnknownNetworkError(network)
		}
		return network, 0, nil
	}
	afnet = network[:i]
	switch afnet {
	case "ip", "ip4", "ip6":
		protostr := network[i+1:]
		proto, i, ok := dtoi(protostr)
		if !ok || i != len(protostr) {
			proto, err = lookupProtocol(ctx, protostr)
			if err != nil {
				return "", 0, err
			}
		}
		return afnet, proto, nil
	}
	return "", 0, UnknownNetworkError(network)
}

// resolveAddrList 使用 hint 解析 addr 并返回地址列表。
// 当 error 为 nil 时，结果至少包含一个地址。
func (r *Resolver) resolveAddrList(ctx context.Context, op, network, addr string, hint Addr) (addrList, error) {
	afnet, _, err := parseNetwork(ctx, network, true)
	if err != nil {
		return nil, err
	}
	if op == "dial" && addr == "" {
		return nil, errMissingAddress
	}
	switch afnet {
	case "unix", "unixgram", "unixpacket":
		addr, err := ResolveUnixAddr(afnet, addr)
		if err != nil {
			return nil, err
		}
		if op == "dial" && hint != nil && addr.Network() != hint.Network() {
			return nil, &AddrError{Err: "mismatched local address type", Addr: hint.String()}
		}
		return addrList{addr}, nil
	}
	addrs, err := r.internetAddrList(ctx, afnet, addr)
	if err != nil || op != "dial" || hint == nil {
		return addrs, err
	}
	var (
		tcp      *TCPAddr
		udp      *UDPAddr
		ip       *IPAddr
		wildcard bool
	)
	switch hint := hint.(type) {
	case *TCPAddr:
		tcp = hint
		wildcard = tcp.isWildcard()
	case *UDPAddr:
		udp = hint
		wildcard = udp.isWildcard()
	case *IPAddr:
		ip = hint
		wildcard = ip.isWildcard()
	}
	naddrs := addrs[:0]
	for _, addr := range addrs {
		if addr.Network() != hint.Network() {
			return nil, &AddrError{Err: "mismatched local address type", Addr: hint.String()}
		}
		switch addr := addr.(type) {
		case *TCPAddr:
			if !wildcard && !addr.isWildcard() && !addr.IP.matchAddrFamily(tcp.IP) {
				continue
			}
			naddrs = append(naddrs, addr)
		case *UDPAddr:
			if !wildcard && !addr.isWildcard() && !addr.IP.matchAddrFamily(udp.IP) {
				continue
			}
			naddrs = append(naddrs, addr)
		case *IPAddr:
			if !wildcard && !addr.isWildcard() && !addr.IP.matchAddrFamily(ip.IP) {
				continue
			}
			naddrs = append(naddrs, addr)
		}
	}
	if len(naddrs) == 0 {
		return nil, &AddrError{Err: errNoSuitableAddress.Error(), Addr: hint.String()}
	}
	return naddrs, nil
}

// MultipathTCP 报告是否将使用 MPTCP。
//
// 此方法不检查操作系统是否支持 MPTCP。
func (d *Dialer) MultipathTCP() bool {
	return d.mptcpStatus.get()
}

// SetMultipathTCP 指示 [Dial] 方法使用或不使用 MPTCP
// （如果操作系统支持）。此方法覆盖系统默认值
// 和 GODEBUG=multipathtcp=... 设置（如果有）。
//
// 如果主机上 MPTCP 不可用或服务器不支持，
// Dial 方法将回退到 TCP。
func (d *Dialer) SetMultipathTCP(use bool) {
	d.mptcpStatus.set(use)
}

// Dial connects to the address on the named network.
//
// Known networks are "tcp", "tcp4" (IPv4-only), "tcp6" (IPv6-only),
// "udp", "udp4" (IPv4-only), "udp6" (IPv6-only), "ip", "ip4"
// (IPv4-only), "ip6" (IPv6-only), "unix", "unixgram" and
// "unixpacket".
//
// For TCP and UDP networks, the address has the form "host:port".
// The host must be a literal IP address, or a host name that can be
// resolved to IP addresses.
// The port must be a literal port number or a service name.
// If the host is a literal IPv6 address it must be enclosed in square
// brackets, as in "[2001:db8::1]:80" or "[fe80::1%zone]:80".
// The zone specifies the scope of the literal IPv6 address as defined
// in RFC 4007.
// The functions [JoinHostPort] and [SplitHostPort] manipulate a pair of
// host and port in this form.
// When using TCP, and the host resolves to multiple IP addresses,
// Dial will try each IP address in order until one succeeds.
//
// Examples:
//
//	Dial("tcp", "golang.org:http")
//	Dial("tcp", "192.0.2.1:http")
//	Dial("tcp", "198.51.100.1:80")
//	Dial("udp", "[2001:db8::1]:domain")
//	Dial("udp", "[fe80::1%lo0]:53")
//	Dial("tcp", ":80")
//
// For IP networks, the network must be "ip", "ip4" or "ip6" followed
// by a colon and a literal protocol number or a protocol name, and
// the address has the form "host". The host must be a literal IP
// address or a literal IPv6 address with zone.
// It depends on each operating system how the operating system
// behaves with a non-well known protocol number such as "0" or "255".
//
// Examples:
//
//	Dial("ip4:1", "192.0.2.1")
//	Dial("ip6:ipv6-icmp", "2001:db8::1")
//	Dial("ip6:58", "fe80::1%lo0")
//
// For TCP, UDP and IP networks, if the host is empty or a literal
// unspecified IP address, as in ":80", "0.0.0.0:80" or "[::]:80" for
// TCP and UDP, "", "0.0.0.0" or "::" for IP, the local system is
// assumed.
//
// For Unix networks, the address must be a file system path.
func Dial(network, address string) (Conn, error) {
	var d Dialer
	return d.Dial(network, address)
}

// DialTimeout acts like [Dial] but takes a timeout.
//
// The timeout includes name resolution, if required.
// When using TCP, and the host in the address parameter resolves to
// multiple IP addresses, the timeout is spread over each consecutive
// dial, such that each is given an appropriate fraction of the time
// to connect.
//
// See func Dial for a description of the network and address
// parameters.
func DialTimeout(network, address string, timeout time.Duration) (Conn, error) {
	d := Dialer{Timeout: timeout}
	return d.Dial(network, address)
}

// sysDialer contains a Dial's parameters and configuration.
type sysDialer struct {
	Dialer
	network, address string
	testHookDialTCP  func(ctx context.Context, net string, laddr, raddr *TCPAddr) (*TCPConn, error)
}

// Dial connects to the address on the named network.
//
// See func Dial for a description of the network and address
// parameters.
//
// Dial uses [context.Background] internally; to specify the context, use
// [Dialer.DialContext].
func (d *Dialer) Dial(network, address string) (Conn, error) {
	return d.DialContext(context.Background(), network, address)
}

// DialContext connects to the address on the named network using
// the provided context.
//
// The provided Context must be non-nil. If the context expires before
// the connection is complete, an error is returned. Once successfully
// connected, any expiration of the context will not affect the
// connection.
//
// When using TCP, and the host in the address parameter resolves to multiple
// network addresses, any dial timeout (from d.Timeout or ctx) is spread
// over each consecutive dial, such that each is given an appropriate
// fraction of the time to connect.
// For example, if a host has 4 IP addresses and the timeout is 1 minute,
// the connect to each single address will be given 15 seconds to complete
// before trying the next one.
//
// See func [Dial] for a description of the network and address
// parameters.
func (d *Dialer) DialContext(ctx context.Context, network, address string) (Conn, error) {
	ctx, cancel := d.dialCtx(ctx)
	defer cancel()

	// Shadow the nettrace (if any) during resolve so Connect events don't fire for DNS lookups.
	resolveCtx := ctx
	if trace, _ := ctx.Value(nettrace.TraceKey{}).(*nettrace.Trace); trace != nil {
		shadow := *trace
		shadow.ConnectStart = nil
		shadow.ConnectDone = nil
		resolveCtx = context.WithValue(resolveCtx, nettrace.TraceKey{}, &shadow)
	}

	addrs, err := d.resolver().resolveAddrList(resolveCtx, "dial", network, address, d.LocalAddr)
	if err != nil {
		return nil, &OpError{Op: "dial", Net: network, Source: nil, Addr: nil, Err: err}
	}

	sd := &sysDialer{
		Dialer:  *d,
		network: network,
		address: address,
	}

	var primaries, fallbacks addrList
	if d.dualStack() && network == "tcp" {
		primaries, fallbacks = addrs.partition(isIPv4)
	} else {
		primaries = addrs
	}

	return sd.dialParallel(ctx, primaries, fallbacks)
}

func (d *Dialer) dialCtx(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		panic("nil context")
	}
	deadline := d.deadline(ctx, time.Now())
	var cancel1, cancel2 context.CancelFunc
	if !deadline.IsZero() {
		testHookStepTime()
		if d, ok := ctx.Deadline(); !ok || deadline.Before(d) {
			var subCtx context.Context
			subCtx, cancel1 = context.WithDeadline(ctx, deadline)
			ctx = subCtx
		}
	}
	if oldCancel := d.Cancel; oldCancel != nil {
		subCtx, cancel2 := context.WithCancel(ctx)
		go func() {
			select {
			case <-oldCancel:
				cancel2()
			case <-subCtx.Done():
			}
		}()
		ctx = subCtx
	}
	return ctx, func() {
		if cancel1 != nil {
			cancel1()
		}
		if cancel2 != nil {
			cancel2()
		}
	}
}

// DialTCP acts like Dial for TCP networks using the provided context.
//
// The provided Context must be non-nil. If the context expires before
// the connection is complete, an error is returned. Once successfully
// connected, any expiration of the context will not affect the
// connection.
//
// The network must be a TCP network name; see func Dial for details.
func (d *Dialer) DialTCP(ctx context.Context, network string, laddr netip.AddrPort, raddr netip.AddrPort) (*TCPConn, error) {
	ctx, cancel := d.dialCtx(ctx)
	defer cancel()
	return dialTCP(ctx, d, network, TCPAddrFromAddrPort(laddr), TCPAddrFromAddrPort(raddr))
}

// DialUDP acts like Dial for UDP networks using the provided context.
//
// The provided Context must be non-nil. If the context expires before
// the connection is complete, an error is returned. Once successfully
// connected, any expiration of the context will not affect the
// connection.
//
// The network must be a UDP network name; see func Dial for details.
func (d *Dialer) DialUDP(ctx context.Context, network string, laddr netip.AddrPort, raddr netip.AddrPort) (*UDPConn, error) {
	ctx, cancel := d.dialCtx(ctx)
	defer cancel()
	return dialUDP(ctx, d, network, UDPAddrFromAddrPort(laddr), UDPAddrFromAddrPort(raddr))
}

// DialIP acts like Dial for IP networks using the provided context.
//
// The provided Context must be non-nil. If the context expires before
// the connection is complete, an error is returned. Once successfully
// connected, any expiration of the context will not affect the
// connection.
//
// The network must be an IP network name; see func Dial for details.
func (d *Dialer) DialIP(ctx context.Context, network string, laddr netip.Addr, raddr netip.Addr) (*IPConn, error) {
	ctx, cancel := d.dialCtx(ctx)
	defer cancel()
	return dialIP(ctx, d, network, ipAddrFromAddr(laddr), ipAddrFromAddr(raddr))
}

// DialUnix acts like Dial for Unix networks using the provided context.
//
// The provided Context must be non-nil. If the context expires before
// the connection is complete, an error is returned. Once successfully
// connected, any expiration of the context will not affect the
// connection.
//
// The network must be a Unix network name; see func Dial for details.
func (d *Dialer) DialUnix(ctx context.Context, network string, laddr *UnixAddr, raddr *UnixAddr) (*UnixConn, error) {
	ctx, cancel := d.dialCtx(ctx)
	defer cancel()
	return dialUnix(ctx, d, network, laddr, raddr)
}

// dialParallel races two copies of dialSerial, giving the first a
// head start. It returns the first established connection and
// closes the others. Otherwise it returns an error from the first
// primary address.
func (sd *sysDialer) dialParallel(ctx context.Context, primaries, fallbacks addrList) (Conn, error) {
	if len(fallbacks) == 0 {
		return sd.dialSerial(ctx, primaries)
	}

	returned := make(chan struct{})
	defer close(returned)

	type dialResult struct {
		Conn
		error
		primary bool
		done    bool
	}
	results := make(chan dialResult) // unbuffered

	startRacer := func(ctx context.Context, primary bool) {
		ras := primaries
		if !primary {
			ras = fallbacks
		}
		c, err := sd.dialSerial(ctx, ras)
		select {
		case results <- dialResult{Conn: c, error: err, primary: primary, done: true}:
		case <-returned:
			if c != nil {
				c.Close()
			}
		}
	}

	var primary, fallback dialResult

	// Start the main racer.
	primaryCtx, primaryCancel := context.WithCancel(ctx)
	defer primaryCancel()
	go startRacer(primaryCtx, true)

	// Start the timer for the fallback racer.
	fallbackTimer := time.NewTimer(sd.fallbackDelay())
	defer fallbackTimer.Stop()

	for {
		select {
		case <-fallbackTimer.C:
			fallbackCtx, fallbackCancel := context.WithCancel(ctx)
			defer fallbackCancel()
			go startRacer(fallbackCtx, false)

		case res := <-results:
			if res.error == nil {
				return res.Conn, nil
			}
			if res.primary {
				primary = res
			} else {
				fallback = res
			}
			if primary.done && fallback.done {
				return nil, primary.error
			}
			if res.primary && fallbackTimer.Stop() {
				// If we were able to stop the timer, that means it
				// was running (hadn't yet started the fallback), but
				// we just got an error on the primary path, so start
				// the fallback immediately (in 0 nanoseconds).
				fallbackTimer.Reset(0)
			}
		}
	}
}

// dialSerial connects to a list of addresses in sequence, returning
// either the first successful connection, or the first error.
func (sd *sysDialer) dialSerial(ctx context.Context, ras addrList) (Conn, error) {
	var firstErr error // The error from the first address is most relevant.

	for i, ra := range ras {
		select {
		case <-ctx.Done():
			return nil, &OpError{Op: "dial", Net: sd.network, Source: sd.LocalAddr, Addr: ra, Err: mapErr(ctx.Err())}
		default:
		}

		dialCtx := ctx
		if deadline, hasDeadline := ctx.Deadline(); hasDeadline {
			partialDeadline, err := partialDeadline(time.Now(), deadline, len(ras)-i)
			if err != nil {
				// Ran out of time.
				if firstErr == nil {
					firstErr = &OpError{Op: "dial", Net: sd.network, Source: sd.LocalAddr, Addr: ra, Err: err}
				}
				break
			}
			if partialDeadline.Before(deadline) {
				var cancel context.CancelFunc
				dialCtx, cancel = context.WithDeadline(ctx, partialDeadline)
				defer cancel()
			}
		}

		c, err := sd.dialSingle(dialCtx, ra)
		if err == nil {
			return c, nil
		}
		if firstErr == nil {
			firstErr = err
		}
	}

	if firstErr == nil {
		firstErr = &OpError{Op: "dial", Net: sd.network, Source: nil, Addr: nil, Err: errMissingAddress}
	}
	return nil, firstErr
}

// dialSingle attempts to establish and returns a single connection to
// the destination address.
func (sd *sysDialer) dialSingle(ctx context.Context, ra Addr) (c Conn, err error) {
	trace, _ := ctx.Value(nettrace.TraceKey{}).(*nettrace.Trace)
	if trace != nil {
		raStr := ra.String()
		if trace.ConnectStart != nil {
			trace.ConnectStart(sd.network, raStr)
		}
		if trace.ConnectDone != nil {
			defer func() { trace.ConnectDone(sd.network, raStr, err) }()
		}
	}
	la := sd.LocalAddr
	switch ra := ra.(type) {
	case *TCPAddr:
		la, _ := la.(*TCPAddr)
		if sd.MultipathTCP() {
			c, err = sd.dialMPTCP(ctx, la, ra)
		} else {
			c, err = sd.dialTCP(ctx, la, ra)
		}
	case *UDPAddr:
		la, _ := la.(*UDPAddr)
		c, err = sd.dialUDP(ctx, la, ra)
	case *IPAddr:
		la, _ := la.(*IPAddr)
		c, err = sd.dialIP(ctx, la, ra)
	case *UnixAddr:
		la, _ := la.(*UnixAddr)
		c, err = sd.dialUnix(ctx, la, ra)
	default:
		return nil, &OpError{Op: "dial", Net: sd.network, Source: la, Addr: ra, Err: &AddrError{Err: "unexpected address type", Addr: sd.address}}
	}
	if err != nil {
		return nil, &OpError{Op: "dial", Net: sd.network, Source: la, Addr: ra, Err: err} // c is non-nil interface containing nil pointer
	}
	return c, nil
}

// ListenConfig contains options for listening to an address.
type ListenConfig struct {
	// If Control is not nil, it is called after creating the network
	// connection but before binding it to the operating system.
	//
	// Network and address parameters passed to Control function are not
	// necessarily the ones passed to Listen. Calling Listen with TCP networks
	// will cause the Control function to be called with "tcp4" or "tcp6",
	// UDP networks become "udp4" or "udp6", IP networks become "ip4" or "ip6",
	// and other known networks are passed as-is.
	Control func(network, address string, c syscall.RawConn) error

	// KeepAlive specifies the keep-alive period for network
	// connections accepted by this listener.
	//
	// KeepAlive is ignored if KeepAliveConfig.Enable is true.
	//
	// If zero, keep-alive are enabled if supported by the protocol
	// and operating system. Network protocols or operating systems
	// that do not support keep-alive ignore this field.
	// If negative, keep-alive are disabled.
	KeepAlive time.Duration

	// KeepAliveConfig specifies the keep-alive probe configuration
	// for an active network connection, when supported by the
	// protocol and operating system.
	//
	// If KeepAliveConfig.Enable is true, keep-alive probes are enabled.
	// If KeepAliveConfig.Enable is false and KeepAlive is negative,
	// keep-alive probes are disabled.
	KeepAliveConfig KeepAliveConfig

	// If mptcpStatus is set to a value allowing Multipath TCP (MPTCP) to be
	// used, any call to Listen with "tcp(4|6)" as network will use MPTCP if
	// supported by the operating system.
	mptcpStatus mptcpStatusListen
}

// MultipathTCP reports whether MPTCP will be used.
//
// This method doesn't check if MPTCP is supported by the operating
// system or not.
func (lc *ListenConfig) MultipathTCP() bool {
	return lc.mptcpStatus.get()
}

// SetMultipathTCP directs the [Listen] method to use, or not use, MPTCP,
// if supported by the operating system. This method overrides the
// system default and the GODEBUG=multipathtcp=... setting if any.
//
// If MPTCP is not available on the host or not supported by the client,
// the Listen method will fall back to TCP.
func (lc *ListenConfig) SetMultipathTCP(use bool) {
	lc.mptcpStatus.set(use)
}

// Listen announces on the local network address.
//
// See func Listen for a description of the network and address
// parameters.
//
// The ctx argument is used while resolving the address on which to listen;
// it does not affect the returned Listener.
func (lc *ListenConfig) Listen(ctx context.Context, network, address string) (Listener, error) {
	addrs, err := DefaultResolver.resolveAddrList(ctx, "listen", network, address, nil)
	if err != nil {
		return nil, &OpError{Op: "listen", Net: network, Source: nil, Addr: nil, Err: err}
	}
	sl := &sysListener{
		ListenConfig: *lc,
		network:      network,
		address:      address,
	}
	var l Listener
	la := addrs.first(isIPv4)
	switch la := la.(type) {
	case *TCPAddr:
		if sl.MultipathTCP() {
			l, err = sl.listenMPTCP(ctx, la)
		} else {
			l, err = sl.listenTCP(ctx, la)
		}
	case *UnixAddr:
		l, err = sl.listenUnix(ctx, la)
	default:
		return nil, &OpError{Op: "listen", Net: sl.network, Source: nil, Addr: la, Err: &AddrError{Err: "unexpected address type", Addr: address}}
	}
	if err != nil {
		return nil, &OpError{Op: "listen", Net: sl.network, Source: nil, Addr: la, Err: err} // l is non-nil interface containing nil pointer
	}
	return l, nil
}

// ListenPacket announces on the local network address.
//
// See func ListenPacket for a description of the network and address
// parameters.
//
// The ctx argument is used while resolving the address on which to listen;
// it does not affect the returned PacketConn.
func (lc *ListenConfig) ListenPacket(ctx context.Context, network, address string) (PacketConn, error) {
	addrs, err := DefaultResolver.resolveAddrList(ctx, "listen", network, address, nil)
	if err != nil {
		return nil, &OpError{Op: "listen", Net: network, Source: nil, Addr: nil, Err: err}
	}
	sl := &sysListener{
		ListenConfig: *lc,
		network:      network,
		address:      address,
	}
	var c PacketConn
	la := addrs.first(isIPv4)
	switch la := la.(type) {
	case *UDPAddr:
		c, err = sl.listenUDP(ctx, la)
	case *IPAddr:
		c, err = sl.listenIP(ctx, la)
	case *UnixAddr:
		c, err = sl.listenUnixgram(ctx, la)
	default:
		return nil, &OpError{Op: "listen", Net: sl.network, Source: nil, Addr: la, Err: &AddrError{Err: "unexpected address type", Addr: address}}
	}
	if err != nil {
		return nil, &OpError{Op: "listen", Net: sl.network, Source: nil, Addr: la, Err: err} // c is non-nil interface containing nil pointer
	}
	return c, nil
}

// sysListener contains a Listen's parameters and configuration.
type sysListener struct {
	ListenConfig
	network, address string
}

// Listen announces on the local network address.
//
// The network must be "tcp", "tcp4", "tcp6", "unix" or "unixpacket".
//
// For TCP networks, if the host in the address parameter is empty or
// a literal unspecified IP address, Listen listens on all available
// unicast and anycast IP addresses of the local system.
// To only use IPv4, use network "tcp4".
// The address can use a host name, but this is not recommended,
// because it will create a listener for at most one of the host's IP
// addresses.
// If the port in the address parameter is empty or "0", as in
// "127.0.0.1:" or "[::1]:0", a port number is automatically chosen.
// The [Addr] method of [Listener] can be used to discover the chosen
// port.
//
// See func [Dial] for a description of the network and address
// parameters.
//
// Listen uses context.Background internally; to specify the context, use
// [ListenConfig.Listen].
func Listen(network, address string) (Listener, error) {
	var lc ListenConfig
	return lc.Listen(context.Background(), network, address)
}

// ListenPacket announces on the local network address.
//
// The network must be "udp", "udp4", "udp6", "unixgram", or an IP
// transport. The IP transports are "ip", "ip4", or "ip6" followed by
// a colon and a literal protocol number or a protocol name, as in
// "ip:1" or "ip:icmp".
//
// For UDP and IP networks, if the host in the address parameter is
// empty or a literal unspecified IP address, ListenPacket listens on
// all available IP addresses of the local system except multicast IP
// addresses.
// To only use IPv4, use network "udp4" or "ip4:proto".
// The address can use a host name, but this is not recommended,
// because it will create a listener for at most one of the host's IP
// addresses.
// If the port in the address parameter is empty or "0", as in
// "127.0.0.1:" or "[::1]:0", a port number is automatically chosen.
// The LocalAddr method of [PacketConn] can be used to discover the
// chosen port.
//
// See func [Dial] for a description of the network and address
// parameters.
//
// ListenPacket uses context.Background internally; to specify the context, use
// [ListenConfig.ListenPacket].
func ListenPacket(network, address string) (PacketConn, error) {
	var lc ListenConfig
	return lc.ListenPacket(context.Background(), network, address)
}
