// 版权所有 2011 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

/*
Package http 提供 HTTP 客户端和服务器实现。

[Get]、[Head]、[Post] 和 [PostForm] 执行 HTTP（或 HTTPS）请求：

	resp, err := http.Get("http://example.com/")
	...
	resp, err := http.Post("http://example.com/upload", "image/jpeg", &buf)
	...
	resp, err := http.PostForm("http://example.com/form",
		url.Values{"key": {"Value"}, "id": {"123"}})

调用者在完成响应体后必须关闭它：

	resp, err := http.Get("http://example.com/")
	if err != nil {
		// 处理错误
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	// ...

# 客户端和传输

要控制 HTTP 客户端标头、重定向策略和其他
设置，创建一个 [Client]：

	client := &http.Client{
		CheckRedirect: redirectPolicyFunc,
	}

	resp, err := client.Get("http://example.com")
	// ...

	req, err := http.NewRequest("GET", "http://example.com", nil)
	// ...
	req.Header.Add("If-None-Match", `W/"wyzzy"`)
	resp, err := client.Do(req)
	// ...

要控制代理、TLS 配置、keep-alive、
压缩和其他设置，创建一个 [Transport]：

	tr := &http.Transport{
		MaxIdleConns:       10,
		IdleConnTimeout:    30 * time.Second,
		DisableCompression: true,
	}
	client := &http.Client{Transport: tr}
	resp, err := client.Get("https://example.com")

客户端和传输对于多个
goroutine 的并发使用是安全的，为了效率应该仅创建一次并重新使用。

# 服务器

ListenAndServe 启动一个具有给定地址和处理程序的 HTTP 服务器。
处理程序通常为 nil，这意味着使用 [DefaultServeMux]。
[Handle] 和 [HandleFunc] 将处理程序添加到 [DefaultServeMux]：

	http.Handle("/foo", fooHandler)

	http.HandleFunc("/bar", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Hello, %q", html.EscapeString(r.URL.Path))
	})

	log.Fatal(http.ListenAndServe(":8080", nil))

通过创建自定义
Server 可以获得对服务器行为的更多控制：

	s := &http.Server{
		Addr:           ":8080",
		Handler:        myHandler,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
	log.Fatal(s.ListenAndServe())

# HTTP/2

http 包对 HTTP/2 协议提供透明支持。

[Server] 和 [DefaultTransport] 在
使用 HTTPS 时自动启用 HTTP/2 支持。[Transport] 默认不启用 HTTP/2。

要启用或禁用对 HTTP/1、HTTP/2 和/或未加密 HTTP/2 的支持，
请参阅 [Server.Protocols] 和 [Transport.Protocols] 配置字段。

要配置高级 HTTP/2 功能，请参阅 [Server.HTTP2] 和
[Transport.HTTP2] 配置字段。

或者，目前支持以下 GODEBUG 设置：

	GODEBUG=http2client=0  # 禁用 HTTP/2 客户端支持
	GODEBUG=http2server=0  # 禁用 HTTP/2 服务器支持
	GODEBUG=http2debug=1   # 启用详细 HTTP/2 调试日志
	GODEBUG=http2debug=2   # ... 更详细，包括帧转储

"omithttp2" 构建标记可用于禁用 http 包中包含的 HTTP/2 实现。
*/

package http
