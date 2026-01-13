// 版权所有 2012 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// syslog 包提供了一个简单的系统日志服务接口。
// 它可以使用 UNIX 域套接字、UDP 或 TCP 向 syslog 守护进程发送消息。
//
// 只需要调用一次 Dial。在写入失败时，
// syslog 客户端会尝试重新连接到服务器并再次写入。
//
// syslog 包已被冻结，不再接受新功能。
// 一些外部包提供了更多功能。参见：
//
//	https://godoc.org/?q=syslog
package syslog

// BUG(brainman): 此包未在 Windows 上实现。由于 syslog 包已被冻结，
// 建议 Windows 用户使用标准库之外的包。有关背景信息，
// 请参见 https://golang.org/issue/1108。

// BUG(akumar): 此包未在 Plan 9 上实现。
