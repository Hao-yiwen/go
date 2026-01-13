// 版权所有 2010 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package mime

// isTSpecial 报告 c 是否属于 RFC 1521 和 RFC 2045 定义的 'tspecials' 字符集。
func isTSpecial(c byte) bool {
	// tspecials :=  "(" / ")" / "<" / ">" / "@" /
	//               "," / ";" / ":" / "\" / <">
	//               "/" / "[" / "]" / "?" / "="
	//
	// mask 是一个 128 位的位图，允许的字节对应位为 1，
	// 这样可以通过位移和与运算来测试字节 c。
	// 如果 c >= 128，则 1<<c 和 1<<(c-64) 都将为零，
	// 此函数将返回 false。
	const mask = 0 |
		1<<'(' |
		1<<')' |
		1<<'<' |
		1<<'>' |
		1<<'@' |
		1<<',' |
		1<<';' |
		1<<':' |
		1<<'\\' |
		1<<'"' |
		1<<'/' |
		1<<'[' |
		1<<']' |
		1<<'?' |
		1<<'='
	return ((uint64(1)<<c)&(mask&(1<<64-1)) |
		(uint64(1)<<(c-64))&(mask>>64)) != 0
}

// isTokenChar 报告 c 是否属于 RFC 1521 和 RFC 2045 定义的 'token' 字符集。
func isTokenChar(c byte) bool {
	// token := 1*<any (US-ASCII) CHAR except SPACE, CTLs,
	//             or tspecials>
	// token := 1*<除 SPACE、CTLs 或 tspecials 之外的任意 (US-ASCII) 字符>
	//
	// mask 是一个 128 位的位图，允许的字节对应位为 1，
	// 这样可以通过位移和与运算来测试字节 c。
	// 如果 c >= 128，则 1<<c 和 1<<(c-64) 都将为零，
	// 此函数将返回 false。
	const mask = 0 |
		(1<<(10)-1)<<'0' |
		(1<<(26)-1)<<'a' |
		(1<<(26)-1)<<'A' |
		1<<'!' |
		1<<'#' |
		1<<'$' |
		1<<'%' |
		1<<'&' |
		1<<'\'' |
		1<<'*' |
		1<<'+' |
		1<<'-' |
		1<<'.' |
		1<<'^' |
		1<<'_' |
		1<<'`' |
		1<<'{' |
		1<<'|' |
		1<<'}' |
		1<<'~'
	return ((uint64(1)<<c)&(mask&(1<<64-1)) |
		(uint64(1)<<(c-64))&(mask>>64)) != 0
}

// isToken 报告 s 是否是 RFC 1521 和 RFC 2045 定义的 'token'。
func isToken(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range []byte(s) {
		if !isTokenChar(c) {
			return false
		}
	}
	return true
}
