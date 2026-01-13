// 版权所有 2015 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// 最小化的 RFC 6724 地址选择实现。

package net

import (
	"net/netip"
	"slices"
)

func sortByRFC6724(addrs []IPAddr) {
	if len(addrs) < 2 {
		return
	}
	sortByRFC6724withSrcs(addrs, srcAddrs(addrs))
}

func sortByRFC6724withSrcs(addrs []IPAddr, srcs []netip.Addr) {
	if len(addrs) != len(srcs) {
		panic("internal error")
	}
	addrInfos := make([]byRFC6724Info, len(addrs))
	for i, v := range addrs {
		addrAttrIP, _ := netip.AddrFromSlice(v.IP)
		addrInfos[i] = byRFC6724Info{
			addr:     addrs[i],
			addrAttr: ipAttrOf(addrAttrIP),
			src:      srcs[i],
			srcAttr:  ipAttrOf(srcs[i]),
		}
	}
	slices.SortStableFunc(addrInfos, compareByRFC6724)
	for i := range addrInfos {
		addrs[i] = addrInfos[i].addr
	}
}

// srcAddrs 尝试通过 UDP 连接到每个地址，以检查是否有路由。
// （这不会发送任何数据包）。目标端口号无关紧要。
func srcAddrs(addrs []IPAddr) []netip.Addr {
	srcs := make([]netip.Addr, len(addrs))
	dst := UDPAddr{Port: 53}
	for i := range addrs {
		dst.IP = addrs[i].IP
		dst.Zone = addrs[i].Zone
		c, err := DialUDP("udp", nil, &dst)
		if err == nil {
			if src, ok := c.LocalAddr().(*UDPAddr); ok {
				srcs[i], _ = netip.AddrFromSlice(src.IP)
			}
			c.Close()
		}
	}
	return srcs
}

type ipAttr struct {
	Scope      scope
	Precedence uint8
	Label      uint8
}

func ipAttrOf(ip netip.Addr) ipAttr {
	if !ip.IsValid() {
		return ipAttr{}
	}
	match := rfc6724policyTable.Classify(ip)
	return ipAttr{
		Scope:      classifyScope(ip),
		Precedence: match.Precedence,
		Label:      match.Label,
	}
}

type byRFC6724Info struct {
	addr     IPAddr
	addrAttr ipAttr
	src      netip.Addr
	srcAttr  ipAttr
}

// compareByRFC6724 比较两个 byRFC6724Info 记录并返回一个表示顺序的整数。
// 它遵循 RFC 6724 第 6 节中的算法和变量名。
// 如果优先选择 a 则返回 -1，如果优先选择 b 则返回 1，如果相等则返回 0。
func compareByRFC6724(a, b byRFC6724Info) int {
	DA := a.addr.IP
	DB := b.addr.IP
	SourceDA := a.src
	SourceDB := b.src
	attrDA := &a.addrAttr
	attrDB := &b.addrAttr
	attrSourceDA := &a.srcAttr
	attrSourceDB := &b.srcAttr

	const preferDA = -1
	const preferDB = 1

	// 规则 1：避免不可用的目的地。
	// 如果 DB 已知不可达或 Source(DB) 未定义，则优先选择 DA。
	// 类似地，如果 DA 已知不可达或 Source(DA) 未定义，则优先选择 DB。
	if !SourceDA.IsValid() && !SourceDB.IsValid() {
		return 0 // "相等"
	}
	if !SourceDB.IsValid() {
		return preferDA
	}
	if !SourceDA.IsValid() {
		return preferDB
	}

	// 规则 2：优先选择匹配的作用域。
	// 如果 Scope(DA) = Scope(Source(DA)) 且 Scope(DB) <> Scope(Source(DB))，
	// 则优先选择 DA。类似地，如果 Scope(DA) <> Scope(Source(DA)) 且
	// Scope(DB) = Scope(Source(DB))，则优先选择 DB。
	if attrDA.Scope == attrSourceDA.Scope && attrDB.Scope != attrSourceDB.Scope {
		return preferDA
	}
	if attrDA.Scope != attrSourceDA.Scope && attrDB.Scope == attrSourceDB.Scope {
		return preferDB
	}

	// 规则 3：避免已弃用的地址。
	// 如果 Source(DA) 已弃用而 Source(DB) 未弃用，则优先选择 DB。
	// 类似地，如果 Source(DA) 未弃用而 Source(DB) 已弃用，则优先选择 DA。

	// TODO(bradfitz): 是否实现？目前优先级较低。

	// 规则 4：优先选择归属地址。
	// 如果 Source(DA) 同时是归属地址和转交地址而 Source(DB) 不是，
	// 则优先选择 DA。类似地，如果 Source(DB) 同时是归属地址和转交地址
	// 而 Source(DA) 不是，则优先选择 DB。

	// TODO(bradfitz): 是否实现？目前优先级较低。

	// 规则 5：优先选择匹配的标签。
	// 如果 Label(Source(DA)) = Label(DA) 且 Label(Source(DB)) <> Label(DB)，
	// 则优先选择 DA。类似地，如果 Label(Source(DA)) <> Label(DA) 且
	// Label(Source(DB)) = Label(DB)，则优先选择 DB。
	if attrSourceDA.Label == attrDA.Label &&
		attrSourceDB.Label != attrDB.Label {
		return preferDA
	}
	if attrSourceDA.Label != attrDA.Label &&
		attrSourceDB.Label == attrDB.Label {
		return preferDB
	}

	// 规则 6：优先选择更高优先级。
	// 如果 Precedence(DA) > Precedence(DB)，则优先选择 DA。
	// 类似地，如果 Precedence(DA) < Precedence(DB)，则优先选择 DB。
	if attrDA.Precedence > attrDB.Precedence {
		return preferDA
	}
	if attrDA.Precedence < attrDB.Precedence {
		return preferDB
	}

	// 规则 7：优先选择原生传输。
	// 如果 DA 通过封装过渡机制到达（例如 IPv6 in IPv4）而 DB 不是，
	// 则优先选择 DB。类似地，如果 DB 通过封装到达而 DA 不是，则优先选择 DA。

	// TODO(bradfitz): 是否实现？目前优先级较低。

	// 规则 8：优先选择较小的作用域。
	// 如果 Scope(DA) < Scope(DB)，则优先选择 DA。
	// 类似地，如果 Scope(DA) > Scope(DB)，则优先选择 DB。
	if attrDA.Scope < attrDB.Scope {
		return preferDA
	}
	if attrDA.Scope > attrDB.Scope {
		return preferDB
	}

	// 规则 9：使用最长匹配前缀。
	// 当 DA 和 DB 属于同一地址族（都是 IPv6 或都是 IPv4 [但见下文]）时：
	// 如果 CommonPrefixLen(Source(DA), DA) > CommonPrefixLen(Source(DB), DB)，
	// 则优先选择 DA。类似地，如果 CommonPrefixLen(Source(DA), DA) <
	// CommonPrefixLen(Source(DB), DB)，则优先选择 DB。
	//
	// 然而，将此规则应用于 IPv4 地址会导致问题
	// （参见 issues 13283 和 18518），因此仅限于 IPv6。
	if DA.To4() == nil && DB.To4() == nil {
		commonA := commonPrefixLen(SourceDA, DA)
		commonB := commonPrefixLen(SourceDB, DB)

		if commonA > commonB {
			return preferDA
		}
		if commonA < commonB {
			return preferDB
		}
	}

	// 规则 10：否则，保持顺序不变。
	// 如果 DA 在原始列表中位于 DB 之前，则优先选择 DA。
	// 否则，优先选择 DB。
	return 0 // "相等"
}

type policyTableEntry struct {
	Prefix     netip.Prefix
	Precedence uint8
	Label      uint8
}

type policyTable []policyTableEntry

// RFC 6724 第 2.1 节。
// 条目按其 Prefix.Mask.Size 的大小排序，
var rfc6724policyTable = policyTable{
	{
		// "::1/128"
		Prefix:     netip.PrefixFrom(netip.AddrFrom16([16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x01}), 128),
		Precedence: 50,
		Label:      0,
	},
	{
		// "::ffff:0:0/96"
		// IPv4 兼容地址等。
		Prefix:     netip.PrefixFrom(netip.AddrFrom16([16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xff, 0xff}), 96),
		Precedence: 35,
		Label:      4,
	},
	{
		// "::/96"
		Prefix:     netip.PrefixFrom(netip.AddrFrom16([16]byte{}), 96),
		Precedence: 1,
		Label:      3,
	},
	{
		// "2001::/32"
		// Teredo 隧道
		Prefix:     netip.PrefixFrom(netip.AddrFrom16([16]byte{0x20, 0x01}), 32),
		Precedence: 5,
		Label:      5,
	},
	{
		// "2002::/16"
		// 6to4 隧道
		Prefix:     netip.PrefixFrom(netip.AddrFrom16([16]byte{0x20, 0x02}), 16),
		Precedence: 30,
		Label:      2,
	},
	{
		// "3ffe::/16"
		Prefix:     netip.PrefixFrom(netip.AddrFrom16([16]byte{0x3f, 0xfe}), 16),
		Precedence: 1,
		Label:      12,
	},
	{
		// "fec0::/10"
		Prefix:     netip.PrefixFrom(netip.AddrFrom16([16]byte{0xfe, 0xc0}), 10),
		Precedence: 1,
		Label:      11,
	},
	{
		// "fc00::/7"
		Prefix:     netip.PrefixFrom(netip.AddrFrom16([16]byte{0xfc}), 7),
		Precedence: 3,
		Label:      13,
	},
	{
		// "::/0"
		Prefix:     netip.PrefixFrom(netip.AddrFrom16([16]byte{}), 0),
		Precedence: 40,
		Label:      1,
	},
}

// Classify 返回包含 ip 的最长匹配前缀的 policyTableEntry 条目。
// 表 t 必须按掩码大小从大到小排序。
func (t policyTable) Classify(ip netip.Addr) policyTableEntry {
	// Prefix.Contains() 不会将 IPv6 前缀与 IPv4 地址匹配。
	if ip.Is4() {
		ip = netip.AddrFrom16(ip.As16())
	}
	for _, ent := range t {
		if ent.Prefix.Contains(ip) {
			return ent
		}
	}
	return policyTableEntry{}
}

// RFC 6724 第 3.1 节。
type scope uint8

const (
	scopeInterfaceLocal scope = 0x1
	scopeLinkLocal      scope = 0x2
	scopeAdminLocal     scope = 0x4
	scopeSiteLocal      scope = 0x5
	scopeOrgLocal       scope = 0x8
	scopeGlobal         scope = 0xe
)

func classifyScope(ip netip.Addr) scope {
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return scopeLinkLocal
	}
	ipv6 := ip.Is6() && !ip.Is4In6()
	ipv6AsBytes := ip.As16()
	if ipv6 && ip.IsMulticast() {
		return scope(ipv6AsBytes[1] & 0xf)
	}
	// 站点本地地址在 RFC 3513 第 2.5.6 节中定义
	// （并在 RFC 3879 中被弃用）。
	if ipv6 && ipv6AsBytes[0] == 0xfe && ipv6AsBytes[1]&0xc0 == 0xc0 {
		return scopeSiteLocal
	}
	return scopeGlobal
}

// commonPrefixLen 报告两个地址共有的最长前缀的长度（从最高有效位
// 或最左边的位开始），最多到 a 的前缀长度（即不包括接口 ID 的地址部分）。
//
// 如果 a 或 b 是作为 IPv6 地址的 IPv4 地址，则比较 IPv4 地址
// （最大公共前缀长度为 32）。
// 如果 a 和 b 是不同的 IP 版本，则返回 0。
//
// 参见 https://tools.ietf.org/html/rfc6724#section-2.2
func commonPrefixLen(a netip.Addr, b IP) (cpl int) {
	if b4 := b.To4(); b4 != nil {
		b = b4
	}
	aAsSlice := a.AsSlice()
	if len(aAsSlice) != len(b) {
		return 0
	}
	// 如果是 IPv6，只比较前缀部分（前 64 位）
	if len(aAsSlice) > 8 {
		aAsSlice = aAsSlice[:8]
		b = b[:8]
	}
	for len(aAsSlice) > 0 {
		if aAsSlice[0] == b[0] {
			cpl += 8
			aAsSlice = aAsSlice[1:]
			b = b[1:]
			continue
		}
		bits := 8
		ab, bb := aAsSlice[0], b[0]
		for {
			ab >>= 1
			bb >>= 1
			bits--
			if ab == bb {
				cpl += bits
				return
			}
		}
	}
	return
}
