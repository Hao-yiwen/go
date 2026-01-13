// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package buildcfg 提供对由当前环境描述的构建配置的访问。
// 它供构建工具使用，例如 cmd/go 或 cmd/compile，
// 以及设置 go/build 的 Default 上下文。
//
// 注意：它不提供对用于构建当前运行的二进制文件的构建配置的访问。
// 为此，请使用 runtime.GOOS 等
// 以及 internal/goexperiment。
package buildcfg

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

var (
	GOROOT    = os.Getenv("GOROOT") // cached for efficiency
	GOARCH    = envOr("GOARCH", defaultGOARCH)
	GOOS      = envOr("GOOS", defaultGOOS)
	GO386     = envOr("GO386", DefaultGO386)
	GOAMD64   = goamd64()
	GOARM     = goarm()
	GOARM64   = goarm64()
	GOMIPS    = gomips()
	GOMIPS64  = gomips64()
	GOPPC64   = goppc64()
	GORISCV64 = goriscv64()
	GOWASM    = gowasm()
	ToolTags  = toolTags()
	GO_LDSO   = defaultGO_LDSO
	GOFIPS140 = gofips140()
	Version   = version
)

// Error 是在构建配置中发现的错误之一（如果有的话）。
var Error error

// Check 如果 Error 非零则以致命错误退出程序。
func Check() {
	if Error != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", filepath.Base(os.Args[0]), Error)
		os.Exit(2)
	}
}

func envOr(key, value string) string {
	if x := os.Getenv(key); x != "" {
		return x
	}
	return value
}

func goamd64() int {
	switch v := envOr("GOAMD64", DefaultGOAMD64); v {
	case "v1":
		return 1
	case "v2":
		return 2
	case "v3":
		return 3
	case "v4":
		return 4
	}
	Error = fmt.Errorf("invalid GOAMD64: must be v1, v2, v3, v4")
	return int(DefaultGOAMD64[len("v")] - '0')
}

func gofips140() string {
	v := envOr("GOFIPS140", DefaultGOFIPS140)
	switch v {
	case "off", "latest", "inprocess", "certified":
		return v
	}
	if isFIPSVersion(v) {
		return v
	}
	Error = fmt.Errorf("invalid GOFIPS140: must be off, latest, inprocess, certified, or v1.Y.Z")
	return DefaultGOFIPS140
}

// isFIPSVersion 报告 v 是否是有效的 FIPS 版本，
// 形式为 v1.Y.Z 或 v1.Y.Z-hhhhhhhh 或 v1.Y.Z-rcN。
func isFIPSVersion(v string) bool {
	v, ok := strings.CutPrefix(v, "v1.")
	if !ok {
		return false
	}
	if v, ok = cutNum(v); !ok {
		return false
	}
	if v, ok = strings.CutPrefix(v, "."); !ok {
		return false
	}
	if v, ok = cutNum(v); !ok {
		return false
	}
	if v == "" {
		return true
	}
	if v, ok = strings.CutPrefix(v, "-rc"); ok {
		v, ok = cutNum(v)
		return ok && v == ""
	}
	if v, ok = strings.CutPrefix(v, "-"); ok {
		return len(v) == 8
	}
	return false
}

// cutNum 跳过与 [0-9]+ 匹配的前导文本
// 在 s 中，返回其余部分以及是否找到了这样的文本。
func cutNum(s string) (rest string, ok bool) {
	i := 0
	for i < len(s) && '0' <= s[i] && s[i] <= '9' {
		i++
	}
	return s[i:], i > 0
}

type GoarmFeatures struct {
	Version   int
	SoftFloat bool
}

func (g GoarmFeatures) String() string {
	armStr := strconv.Itoa(g.Version)
	if g.SoftFloat {
		armStr += ",softfloat"
	} else {
		armStr += ",hardfloat"
	}
	return armStr
}

func goarm() (g GoarmFeatures) {
	const (
		softFloatOpt = ",softfloat"
		hardFloatOpt = ",hardfloat"
	)
	def := DefaultGOARM
	if GOOS == "android" && GOARCH == "arm" {
		// Android arm 设备总是支持 GOARM=7。
		def = "7"
	}
	v := envOr("GOARM", def)

	floatSpecified := false
	if strings.HasSuffix(v, softFloatOpt) {
		g.SoftFloat = true
		floatSpecified = true
		v = v[:len(v)-len(softFloatOpt)]
	}
	if strings.HasSuffix(v, hardFloatOpt) {
		floatSpecified = true
		v = v[:len(v)-len(hardFloatOpt)]
	}

	switch v {
	case "5":
		g.Version = 5
	case "6":
		g.Version = 6
	case "7":
		g.Version = 7
	default:
		Error = fmt.Errorf("invalid GOARM: must start with 5, 6, or 7, and may optionally end in either %q or %q", hardFloatOpt, softFloatOpt)
		g.Version = int(def[0] - '0')
	}

	// 5 默认为 softfloat。6 和 7 默认为 hardfloat。
	if !floatSpecified && g.Version == 5 {
		g.SoftFloat = true
	}
	return
}

type Goarm64Features struct {
	Version string
	// 大型系统扩展
	LSE bool
	// ARM v8.0 加密扩展。它包括以下功能：
	// * FEAT_AES，包括 AESD 和 AESE 指令。
	// * FEAT_PMULL，包括 PMULL、PMULL2 指令。
	// * FEAT_SHA1，包括 SHA1* 指令。
	// * FEAT_SHA256，包括 SHA256* 指令。
	Crypto bool
}

func (g Goarm64Features) String() string {
	arm64Str := g.Version
	if g.LSE {
		arm64Str += ",lse"
	}
	if g.Crypto {
		arm64Str += ",crypto"
	}
	return arm64Str
}

func ParseGoarm64(v string) (g Goarm64Features, e error) {
	const (
		lseOpt    = ",lse"
		cryptoOpt = ",crypto"
	)

	g.LSE = false
	g.Crypto = false
	// 我们允许任何后缀组合，按任何顺序
	for {
		if strings.HasSuffix(v, lseOpt) {
			g.LSE = true
			v = v[:len(v)-len(lseOpt)]
			continue
		}

		if strings.HasSuffix(v, cryptoOpt) {
			g.Crypto = true
			v = v[:len(v)-len(cryptoOpt)]
			continue
		}

		break
	}

	switch v {
	case "v8.0":
		g.Version = v
	case "v8.1", "v8.2", "v8.3", "v8.4", "v8.5", "v8.6", "v8.7", "v8.8", "v8.9",
		"v9.0", "v9.1", "v9.2", "v9.3", "v9.4", "v9.5":
		g.Version = v
		// LSE 扩展从 8.1 开始是强制的
		g.LSE = true
	default:
		e = fmt.Errorf("invalid GOARM64: must start with v8.{0-9} or v9.{0-5} and may optionally end in %q and/or %q",
			lseOpt, cryptoOpt)
		g.Version = DefaultGOARM64
	}

	return
}

func goarm64() (g Goarm64Features) {
	g, Error = ParseGoarm64(envOr("GOARM64", DefaultGOARM64))
	return
}

// 如果 g 支持给定的 ARM64 ISA，则返回 true
// 注意：此函数不接受/测试后缀（如",lse"或",crypto"）
func (g Goarm64Features) Supports(s string) bool {
	// 我们只接受 "v{8-9}.{0-9}"。其他都是格式不正确的。
	if len(s) != 4 {
		return false
	}

	major := s[1]
	minor := s[3]

	// 我们只接受 "v{8-9}.{0-9}"。其他都是格式不正确的。
	if major < '8' || major > '9' ||
		minor < '0' || minor > '9' ||
		s[0] != 'v' || s[2] != '.' {
		return false
	}

	g_major := g.Version[1]
	g_minor := g.Version[3]

	if major == g_major {
		return minor <= g_minor
	} else if g_major == '9' {
		// v9.0 与 v8.5 分离。这意味着我们应该与 g_minor 增加五进行比较。
		return minor <= g_minor+5
	} else {
		return false
	}
}

func gomips() string {
	switch v := envOr("GOMIPS", DefaultGOMIPS); v {
	case "hardfloat", "softfloat":
		return v
	}
	Error = fmt.Errorf("invalid GOMIPS: must be hardfloat, softfloat")
	return DefaultGOMIPS
}

func gomips64() string {
	switch v := envOr("GOMIPS64", DefaultGOMIPS64); v {
	case "hardfloat", "softfloat":
		return v
	}
	Error = fmt.Errorf("invalid GOMIPS64: must be hardfloat, softfloat")
	return DefaultGOMIPS64
}

func goppc64() int {
	switch v := envOr("GOPPC64", DefaultGOPPC64); v {
	case "power8":
		return 8
	case "power9":
		return 9
	case "power10":
		return 10
	}
	Error = fmt.Errorf("invalid GOPPC64: must be power8, power9, power10")
	return int(DefaultGOPPC64[len("power")] - '0')
}

func goriscv64() int {
	switch v := envOr("GORISCV64", DefaultGORISCV64); v {
	case "rva20u64":
		return 20
	case "rva22u64":
		return 22
	case "rva23u64":
		return 23
	}
	Error = fmt.Errorf("invalid GORISCV64: must be rva20u64, rva22u64, rva23u64")
	v := DefaultGORISCV64[len("rva"):]
	i := strings.IndexFunc(v, func(r rune) bool {
		return r < '0' || r > '9'
	})
	year, _ := strconv.Atoi(v[:i])
	return year
}

type gowasmFeatures struct {
	// 遗留功能，现在总是启用
	//SatConv bool
	//SignExt bool
}

func (f gowasmFeatures) String() string {
	var flags []string
	return strings.Join(flags, ",")
}

func gowasm() (f gowasmFeatures) {
	for opt := range strings.SplitSeq(envOr("GOWASM", ""), ",") {
		switch opt {
		case "satconv":
			// 忽略，总是启用
		case "signext":
			// 忽略，总是启用
		case "":
			// 忽略
		default:
			Error = fmt.Errorf("invalid GOWASM: no such feature %q", opt)
		}
	}
	return
}

func Getgoextlinkenabled() string {
	return envOr("GO_EXTLINK_ENABLED", defaultGO_EXTLINK_ENABLED)
}

func toolTags() []string {
	tags := experimentTags()
	tags = append(tags, gogoarchTags()...)
	return tags
}

func experimentTags() []string {
	var list []string
	// 对于工具链中已启用的每个实验，定义一个
	// 具有相同名称但以 "goexperiment." 为前缀的构建标签，该标签可以
	// 用于为实验编译替代文件。这允许
	// 对实验进行更改，如运行时中的额外结构字段，
	// 而完全不影响基础非实验代码。
	for _, exp := range Experiment.Enabled() {
		list = append(list, "goexperiment."+exp)
	}
	return list
}

// GOGOARCH 返回 GO$GOARCH 设置的名称和值。
// 例如，如果 GOARCH 是 "amd64"，它可能返回 "GOAMD64"、"v2"。
func GOGOARCH() (name, value string) {
	switch GOARCH {
	case "386":
		return "GO386", GO386
	case "amd64":
		return "GOAMD64", fmt.Sprintf("v%d", GOAMD64)
	case "arm":
		return "GOARM", GOARM.String()
	case "arm64":
		return "GOARM64", GOARM64.String()
	case "mips", "mipsle":
		return "GOMIPS", GOMIPS
	case "mips64", "mips64le":
		return "GOMIPS64", GOMIPS64
	case "ppc64", "ppc64le":
		return "GOPPC64", fmt.Sprintf("power%d", GOPPC64)
	case "riscv64":
		return "GORISCV64", fmt.Sprintf("rva%du64", GORISCV64)
	case "wasm":
		return "GOWASM", GOWASM.String()
	}
	return "", ""
}

func gogoarchTags() []string {
	switch GOARCH {
	case "386":
		return []string{GOARCH + "." + GO386}
	case "amd64":
		var list []string
		for i := 1; i <= GOAMD64; i++ {
			list = append(list, fmt.Sprintf("%s.v%d", GOARCH, i))
		}
		return list
	case "arm":
		var list []string
		for i := 5; i <= GOARM.Version; i++ {
			list = append(list, fmt.Sprintf("%s.%d", GOARCH, i))
		}
		return list
	case "arm64":
		var list []string
		major := int(GOARM64.Version[1] - '0')
		minor := int(GOARM64.Version[3] - '0')
		for i := 0; i <= minor; i++ {
			list = append(list, fmt.Sprintf("%s.v%d.%d", GOARCH, major, i))
		}
		// ARM64 v9.x 也包括对 v8.x+5 的支持（即 v9.1 包括 v8.(1+5) = v8.6）。
		if major == 9 {
			for i := 0; i <= minor+5 && i <= 9; i++ {
				list = append(list, fmt.Sprintf("%s.v%d.%d", GOARCH, 8, i))
			}
		}
		return list
	case "mips", "mipsle":
		return []string{GOARCH + "." + GOMIPS}
	case "mips64", "mips64le":
		return []string{GOARCH + "." + GOMIPS64}
	case "ppc64", "ppc64le":
		var list []string
		for i := 8; i <= GOPPC64; i++ {
			list = append(list, fmt.Sprintf("%s.power%d", GOARCH, i))
		}
		return list
	case "riscv64":
		list := []string{GOARCH + "." + "rva20u64"}
		if GORISCV64 >= 22 {
			list = append(list, GOARCH+"."+"rva22u64")
		}
		if GORISCV64 >= 23 {
			list = append(list, GOARCH+"."+"rva23u64")
		}
		return list
	case "wasm":
		var list []string
		// SatConv 总是启用的
		list = append(list, GOARCH+".satconv")
		// SignExt 总是启用的
		list = append(list, GOARCH+".signext")
		return list
	}
	return nil
}
