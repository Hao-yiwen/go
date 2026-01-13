// 版权所有 2015 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package net

import (
	"errors"
	"internal/bytealg"
	"internal/godebug"
	"internal/stringslite"
	"io/fs"
	"os"
	"runtime"
	"sync"
)

// net 包的名称解析相当复杂。
// 有两种主要方法：go 和 cgo。
// cgo 解析器使用像 getaddrinfo 这样的 C 函数。
// go 解析器直接读取系统文件并直接向服务器发送 DNS 数据包。
//
// netgo 构建标签优先使用 go 解析器。
// netcgo 构建标签优先使用 cgo 解析器。
//
// netgo 构建标签也禁止使用 cgo 工具。
// 然而，在 Darwin、Plan 9 和 Windows 上，cgo 解析器仍然可用。
// 在这些系统上，cgo 解析器不需要 cgo 工具。
// （术语 "cgo 解析器" 是在 cgo 解析器确实需要 cgo 工具时
// 通过 GODEBUG 设置锁定的。）
//
// 在 GODEBUG 中添加 netdns=go 将优先使用 go 解析器。
// 在 GODEBUG 中添加 netdns=cgo 将优先使用 cgo 解析器。
//
// Resolver 结构体有一个 PreferGo 字段，用户代码可以设置它
// 来优先使用 go 解析器。它被记录为等同于在 GODEBUG 中添加 netdns=go。
//
// 在决定使用哪个解析器时，我们首先检查 PreferGo 字段。
// 如果未设置，我们检查 GODEBUG 设置。
// 如果未设置，我们检查 netgo 或 netcgo 构建标签。
// 如果这些都未设置，我们通常默认优先使用 go 解析器。
// 然而，如果 cgo 解析器可用，
// 有一组复杂的条件使我们优先使用 cgo 解析器。
//
// 其他文件定义了 netGoBuildTag、netCgoBuildTag 和 cgoAvailable 常量。

// conf 用于确定名称解析配置。
type conf struct {
	netGo  bool // 优先使用 go 方式，基于构建标签和 GODEBUG
	netCgo bool // 优先使用 cgo 方式，基于构建标签和 GODEBUG

	dnsDebugLevel int // 来自 GODEBUG

	preferCgo bool // 如果没有明确偏好，使用 cgo

	goos     string   // runtime.GOOS 的副本，用于测试
	mdnsTest mdnsTest // 假设 /etc/mdns.allow 存在，用于测试
}

// mdnsTest 仅用于测试。
type mdnsTest int

const (
	mdnsFromSystem mdnsTest = iota
	mdnsAssumeExists
	mdnsAssumeDoesNotExist
)

var (
	confOnce sync.Once // 通过 initConfVal 保护 confVal 的初始化
	confVal  = &conf{goos: runtime.GOOS}
)

// systemConf 返回机器的网络配置。
func systemConf() *conf {
	confOnce.Do(initConfVal)
	return confVal
}

// initConfVal 基于在程序执行期间不会改变的环境
// 初始化 confVal。
func initConfVal() {
	dnsMode, debugLevel := goDebugNetDNS()
	confVal.netGo = netGoBuildTag || dnsMode == "go"
	confVal.netCgo = netCgoBuildTag || dnsMode == "cgo"
	confVal.dnsDebugLevel = debugLevel

	if confVal.dnsDebugLevel > 0 {
		defer func() {
			if confVal.dnsDebugLevel > 1 {
				println("go package net: confVal.netCgo =", confVal.netCgo, " netGo =", confVal.netGo)
			}
			if dnsMode != "go" && dnsMode != "cgo" && dnsMode != "" {
				println("go package net: GODEBUG=netdns contains an invalid dns mode, ignoring it")
			}
			switch {
			case netGoBuildTag || !cgoAvailable:
				if dnsMode == "cgo" {
					println("go package net: ignoring GODEBUG=netdns=cgo as the binary was compiled without support for the cgo resolver")
				} else {
					println("go package net: using the Go DNS resolver")
				}
			case netCgoBuildTag:
				if dnsMode == "go" {
					println("go package net: GODEBUG setting forcing use of the Go resolver")
				} else {
					println("go package net: using the cgo DNS resolver")
				}
			default:
				if dnsMode == "go" {
					println("go package net: GODEBUG setting forcing use of the Go resolver")
				} else if dnsMode == "cgo" {
					println("go package net: GODEBUG setting forcing use of the cgo resolver")
				} else {
					println("go package net: dynamic selection of DNS resolver")
				}
			}
		}()
	}

	// 此函数的其余部分基于在程序执行期间不会改变的
	// 条件设置 preferCgo。

	// 默认情况下，优先使用 go 解析器。
	confVal.preferCgo = false

	// 如果 cgo 解析器不可用，我们就不能优先使用它。
	if !cgoAvailable {
		return
	}

	// 某些操作系统始终优先使用 cgo 解析器。
	if goosPrefersCgo() {
		confVal.preferCgo = true
		return
	}

	// 其余检查特定于 Unix 系统。
	switch runtime.GOOS {
	case "plan9", "windows", "js", "wasip1":
		return
	}

	// 如果指定了任何环境指定的解析器选项，
	// 则优先使用 cgo 解析器。
	// 注意 LOCALDOMAIN 仅通过用空字符串指定就可以改变行为。
	_, localDomainDefined := os.LookupEnv("LOCALDOMAIN")
	if localDomainDefined || os.Getenv("RES_OPTIONS") != "" || os.Getenv("HOSTALIASES") != "" {
		confVal.preferCgo = true
		return
	}

	// OpenBSD 显然允许你用 ASR_CONFIG 覆盖 resolv.conf 的位置。
	// 如果我们注意到这一点，就交给 libc 处理。
	if runtime.GOOS == "openbsd" && os.Getenv("ASR_CONFIG") != "" {
		confVal.preferCgo = true
		return
	}
}

// goosPrefersCgo 报告传入的 GOOS 值是否优先使用 cgo 解析器。
func goosPrefersCgo() bool {
	switch runtime.GOOS {
	// 历史上在 Windows 和 Plan 9 上，我们优先使用
	// cgo 解析器（它不使用 cgo 工具）而不是 go 解析器。
	// 这是因为最初这些系统不支持 go 解析器。
	// 为了更好的兼容性保持这种方式。
	// 也许将来某天我们可以重新审视这个问题。
	case "windows", "plan9":
		return true

	// 如果程序尝试自己进行 DNS 请求，Darwin 会弹出烦人的对话框，
	// 所以优先使用 cgo。
	case "darwin", "ios":
		return true

	// DNS 请求在 Android 上不起作用，所以优先使用 cgo 解析器。
	// Issue #10714。
	case "android":
		return true

	default:
		return false
	}
}

// mustUseGoResolver 报告任何类型的 DNS 查找是否需要使用 go 解析器。
// 提供的 Resolver 是可选的。
// 如果 cgo 解析器不可用，这将返回 true。
func (c *conf) mustUseGoResolver(r *Resolver) bool {
	if !cgoAvailable {
		return true
	}

	if runtime.GOOS == "plan9" {
		// TODO(bradfitz): 目前我们只允许在有非 nil Resolver 和
		// 非 nil Dialer 时使用 PreferGo 实现。这表明代码正在尝试
		// 使用它们自己的 DNS 通信 net.Conn（例如内存中的 DNS 缓存），
		// 并且它们不想实际访问网络。
		// 但是，一旦我们添加了从 plan9 查找默认 DNS 服务器的支持，
		// 我们就可以放宽这个限制。
		if r == nil || r.Dial == nil {
			return false
		}
	}

	return c.netGo || r.preferGo()
}

// addrLookupOrder 确定使用哪种策略来解析地址。
// 提供的 Resolver 是可选的。nil 表示不考虑其选项。
// 当用于确定查找顺序时，它还返回 dnsConfig。
func (c *conf) addrLookupOrder(r *Resolver, addr string) (ret hostLookupOrder, dnsConf *dnsConfig) {
	if c.dnsDebugLevel > 1 {
		defer func() {
			print("go package net: addrLookupOrder(", addr, ") = ", ret.String(), "\n")
		}()
	}
	return c.lookupOrder(r, "")
}

// hostLookupOrder 确定使用哪种策略来解析主机名。
// 提供的 Resolver 是可选的。nil 表示不考虑其选项。
// 当用于确定查找顺序时，它还返回 dnsConfig。
func (c *conf) hostLookupOrder(r *Resolver, hostname string) (ret hostLookupOrder, dnsConf *dnsConfig) {
	if c.dnsDebugLevel > 1 {
		defer func() {
			print("go package net: hostLookupOrder(", hostname, ") = ", ret.String(), "\n")
		}()
	}
	return c.lookupOrder(r, hostname)
}

func (c *conf) lookupOrder(r *Resolver, hostname string) (ret hostLookupOrder, dnsConf *dnsConfig) {
	// fallbackOrder 是当我们无法确定时返回的顺序。
	var fallbackOrder hostLookupOrder

	var canUseCgo bool
	if c.mustUseGoResolver(r) {
		// Go 解析器被明确请求
		// 或 cgo 解析器不可用。
		// 在下面确定顺序。
		fallbackOrder = hostLookupFilesDNS
		canUseCgo = false
	} else if c.netCgo {
		// Cgo 解析器被明确请求。
		return hostLookupCgo, nil
	} else if c.preferCgo {
		// 有选择时，我们优先使用 cgo 解析器。
		return hostLookupCgo, nil
	} else {
		// 两个解析器都没有被明确请求，
		// 我们也没有偏好。

		if bytealg.IndexByteString(hostname, '\\') != -1 || bytealg.IndexByteString(hostname, '%') != -1 {
			// 不处理带有反斜杠或 '%' 的
			// 特殊形式主机名。
			return hostLookupCgo, nil
		}

		// 如果有什么不认识的，使用 cgo。
		fallbackOrder = hostLookupCgo
		canUseCgo = true
	}

	// 在不使用 /etc/resolv.conf 或 /etc/nsswitch.conf 的系统上，我们已完成。
	switch c.goos {
	case "windows", "plan9", "android", "ios":
		return fallbackOrder, nil
	}

	// 尝试确定搜索使用的顺序。
	// 如果我们不认识某些内容，使用 fallbackOrder。
	// 除非明确请求 Go 解析器，否则将使用 cgo。
	// 如果我们确定了顺序，返回 fallbackOrder 以外的值
	// 以使用带有该顺序的 Go 解析器。

	dnsConf = getSystemDNSConfig()

	if canUseCgo && dnsConf.err != nil && !errors.Is(dnsConf.err, fs.ErrNotExist) && !errors.Is(dnsConf.err, fs.ErrPermission) {
		// 我们无法读取 resolv.conf 文件，所以如果可以就使用 cgo。
		return hostLookupCgo, dnsConf
	}

	if canUseCgo && dnsConf.unknownOpt {
		// 我们不认识 resolv.conf 中的某些内容，
		// 所以如果可以就使用 cgo。
		return hostLookupCgo, dnsConf
	}

	// OpenBSD 是独特的，不使用 nsswitch.conf。
	// 它也不支持 mDNS。
	if c.goos == "openbsd" {
		// OpenBSD 的 resolv.conf 手册页说，
		// 不存在的 resolv.conf 意味着 "lookup" 默认
		// 仅为 "files"，不进行 DNS 查找。
		if errors.Is(dnsConf.err, fs.ErrNotExist) {
			return hostLookupFiles, dnsConf
		}

		lookup := dnsConf.lookup
		if len(lookup) == 0 {
			// https://www.openbsd.org/cgi-bin/man.cgi/OpenBSD-current/man5/resolv.conf.5
			// "如果系统的 resolv.conf 文件中没有使用 lookup 关键字，
			// 则假定顺序为 'bind file'"
			return hostLookupDNSFiles, dnsConf
		}
		if len(lookup) < 1 || len(lookup) > 2 {
			// 我们不认识这种格式。
			return fallbackOrder, dnsConf
		}
		switch lookup[0] {
		case "bind":
			if len(lookup) == 2 {
				if lookup[1] == "file" {
					return hostLookupDNSFiles, dnsConf
				}
				// 不认识。
				return fallbackOrder, dnsConf
			}
			return hostLookupDNS, dnsConf
		case "file":
			if len(lookup) == 2 {
				if lookup[1] == "bind" {
					return hostLookupFilesDNS, dnsConf
				}
				// 不认识。
				return fallbackOrder, dnsConf
			}
			return hostLookupFiles, dnsConf
		default:
			// 不认识。
			return fallbackOrder, dnsConf
		}

		// 我们总是在此之前返回。
		// 下面的代码是为非 OpenBSD 系统准备的。
	}

	// 通过移除尾部的点来规范化主机名。
	hostname = stringslite.TrimSuffix(hostname, ".")

	nss := getSystemNSS()
	srcs := nss.sources["hosts"]
	// 如果 /etc/nsswitch.conf 不存在或没有为 "hosts" 指定任何源，
	// 假设 Go 的 DNS 会正常工作。
	if errors.Is(nss.err, fs.ErrNotExist) || (nss.err == nil && len(srcs) == 0) {
		if canUseCgo && c.goos == "solaris" {
			// illumos 默认为
			// "nis [NOTFOUND=return] files"，
			// go 解析器不支持这个。
			return hostLookupCgo, dnsConf
		}

		return hostLookupFilesDNS, dnsConf
	}
	if nss.err != nil {
		// 我们无法解析或打开 nsswitch.conf，所以
		// 我们没有确定顺序的依据。
		return fallbackOrder, dnsConf
	}

	var hasDNSSource bool
	var hasDNSSourceChecked bool

	var filesSource, dnsSource bool
	var first string
	for i, src := range srcs {
		if src.source == "files" || src.source == "dns" {
			if canUseCgo && !src.standardCriteria() {
				// 非标准；让 libc 处理它。
				return hostLookupCgo, dnsConf
			}
			if src.source == "files" {
				filesSource = true
			} else {
				hasDNSSource = true
				hasDNSSourceChecked = true
				dnsSource = true
			}
			if first == "" {
				first = src.source
			}
			continue
		}

		if canUseCgo {
			switch {
			case hostname != "" && src.source == "myhostname":
				// 如果我们正在查找本地主机名，
				// 让 cgo 解析器处理 myhostname。
				if isLocalhost(hostname) || isGateway(hostname) || isOutbound(hostname) {
					return hostLookupCgo, dnsConf
				}
				hn, err := getHostname()
				if err != nil || stringsEqualFold(hostname, hn) {
					return hostLookupCgo, dnsConf
				}
				continue
			case hostname != "" && stringslite.HasPrefix(src.source, "mdns"):
				if stringsHasSuffixFold(hostname, ".local") {
					// 根据 RFC 6762，".local" TLD 是特殊的。
					// 因为 Go 的原生解析器不支持 mDNS 或
					// 类似的本地解析机制，假设
					// libc 可能支持（通过 Avahi 等）并使用 cgo。
					return hostLookupCgo, dnsConf
				}

				// 我们不解析 mdns.allow 文件。它们很少见。如果存在，
				// 它可能列出其他 TLD（除了 .local）甚至 '*'，
				// 所以直接让 libc 处理它。
				var haveMDNSAllow bool
				switch c.mdnsTest {
				case mdnsFromSystem:
					_, err := os.Stat("/etc/mdns.allow")
					if err != nil && !errors.Is(err, fs.ErrNotExist) {
						// 让 libc 弄清楚发生了什么。
						return hostLookupCgo, dnsConf
					}
					haveMDNSAllow = err == nil
				case mdnsAssumeExists:
					haveMDNSAllow = true
				case mdnsAssumeDoesNotExist:
					haveMDNSAllow = false
				}
				if haveMDNSAllow {
					return hostLookupCgo, dnsConf
				}
				continue
			default:
				// 某个我们不知道如何处理的源。
				return hostLookupCgo, dnsConf
			}
		}

		if !hasDNSSourceChecked {
			hasDNSSourceChecked = true
			for _, v := range srcs[i+1:] {
				if v.source == "dns" {
					hasDNSSource = true
					break
				}
			}
		}

		// 如果我们看到一个不认识的源，这只能发生在我们无法使用
		// cgo 解析器时，将其视为 DNS，
		// 但仅当所有其他源中没有 dns 时。
		if !hasDNSSource {
			dnsSource = true
			if first == "" {
				first = "dns"
			}
		}
	}

	// Go 可以处理的情况，无需 cgo 和 C 线程开销，
	// 或者 Go 解析器被强制使用的情况。
	switch {
	case filesSource && dnsSource:
		if first == "files" {
			return hostLookupFilesDNS, dnsConf
		} else {
			return hostLookupDNSFiles, dnsConf
		}
	case filesSource:
		return hostLookupFiles, dnsConf
	case dnsSource:
		return hostLookupDNS, dnsConf
	}

	// 一些奇怪的情况。回退到默认值。
	return fallbackOrder, dnsConf
}

var netdns = godebug.New("netdns")

// goDebugNetDNS 解析 GODEBUG "netdns" 值。
// netdns 值可以是以下形式：
//
//	1       // 调试级别 1
//	2       // 调试级别 2
//	cgo     // 使用 cgo 进行 DNS 查找
//	go      // 使用 go 进行 DNS 查找
//	cgo+1   // 使用 cgo 进行 DNS 查找 + 调试级别 1
//	1+cgo   // 同上
//	cgo+2   // 同上，但调试级别 2
//
// 等等。
func goDebugNetDNS() (dnsMode string, debugLevel int) {
	goDebug := netdns.Value()
	parsePart := func(s string) {
		if s == "" {
			return
		}
		if '0' <= s[0] && s[0] <= '9' {
			debugLevel, _, _ = dtoi(s)
		} else {
			dnsMode = s
		}
	}
	if i := bytealg.IndexByteString(goDebug, '+'); i != -1 {
		parsePart(goDebug[:i])
		parsePart(goDebug[i+1:])
		return
	}
	parsePart(goDebug)
	return
}

// isLocalhost 报告 h 是否应被视为 myhostname NSS 模块的 "localhost" 名称。
func isLocalhost(h string) bool {
	return stringsEqualFold(h, "localhost") || stringsEqualFold(h, "localhost.localdomain") || stringsHasSuffixFold(h, ".localhost") || stringsHasSuffixFold(h, ".localhost.localdomain")
}

// isGateway 报告 h 是否应被视为 myhostname NSS 模块的 "gateway" 名称。
func isGateway(h string) bool {
	return stringsEqualFold(h, "_gateway")
}

// isOutbound 报告 h 是否应被视为 myhostname NSS 模块的 "outbound" 名称。
func isOutbound(h string) bool {
	return stringsEqualFold(h, "_outbound")
}
