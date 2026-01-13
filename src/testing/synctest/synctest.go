// 版权所有 2024 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// Package synctest 提供对测试并发代码的支持。
//
// [Test] 函数在隔离的"泡泡"中运行一个函数。
// 在泡泡内启动的任何 goroutine 也是泡泡的一部分。
//
// 每个测试应该完全自包含：
// 以下指南应适用于大多数测试：
//
//   - 避免与不是从测试中启动的 goroutine 交互。
//   - 避免使用网络。根据需要使用虚假网络实现。
//   - 避免与外部进程交互。
//   - 避免在后台任务中泄露 goroutine。
//
// # 时间
//
// 在泡泡内，[time] 包使用虚假时钟。
// 每个泡泡都有自己的时钟。
// 初始时间是 2000-01-01 UTC 午夜。
//
// 泡泡中的时间仅在泡泡中的每个 goroutine 都
// 持久阻塞时才会前进。
// 有关"持久阻塞"的确切定义，请参见下面。
//
// 例如，此测试立即运行，而不是花费
// 两秒钟：
//
//	func TestTime(t *testing.T) {
//		synctest.Test(t, func(t *testing.T) {
//			start := time.Now() // 总是 2000-01-01 UTC 午夜
//			go func() {
//				time.Sleep(1 * time.Second)
//				t.Log(time.Since(start)) // 总是记录 "1s"
//			}()
//			time.Sleep(2 * time.Second) // 上面的 goroutine 将在此 Sleep 返回之前运行
//			t.Log(time.Since(start))    // 总是记录 "2s"
//		})
//	}
//
// 当泡泡的根 goroutine 退出时，时间停止前进。
//
// # 阻塞
//
// 泡泡中的 goroutine 是"持久阻塞"当它被阻塞
// 并且只能由同一泡泡中的另一个 goroutine 取消阻塞。
// 可以由来自泡泡外的事件取消阻塞的 goroutine
// 不是持久阻塞。
//
// [Wait] 函数阻塞直到泡泡中的所有其他 goroutine
// 都持久阻塞。
//
// 例如：
//
//	func TestWait(t *testing.T) {
//		synctest.Test(t, func(t *testing.T) {
//			done := false
//			go func() {
//				done = true
//			}()
//			// Wait 将阻塞直到上面的 goroutine 完成。
//			synctest.Wait()
//			t.Log(done) // 总是记录 "true"
//		})
//	}
//
// 当泡泡中的每个 goroutine 都持久阻塞时：
//
//   - [Wait] 返回（如果已被调用）。
//   - 否则，时间前进到将至少取消
//     一个 goroutine 阻塞的下一时间（如果存在
//     这样的时间并且泡泡的根 goroutine 尚未退出）。
//   - 否则，存在死锁，[Test] panic。
//
// 以下操作持久阻塞 goroutine：
//
//   - 在泡泡中创建的通道上的阻塞发送或接收
//   - 阻塞的 select 语句，其中每个 case 都是在
//     泡泡中创建的通道
//   - [sync.Cond.Wait]
//   - [sync.WaitGroup.Wait]，当 [sync.WaitGroup.Add] 在泡泡中调用时
//   - [time.Sleep]
//
// 上述列表中没有的操作不是持久阻塞。
// 特别是，以下操作可能会阻塞 goroutine，
// 但不是持久阻塞，因为 goroutine 可以被
// 泡泡外发生的事件取消阻塞：
//
//   - 锁定 [sync.Mutex] 或 [sync.RWMutex]
//   - 阻塞 I/O，如从网络套接字读取
//   - 系统调用
//
// # 隔离
//
// 在泡泡中创建的通道、[time.Timer] 或 [time.Ticker]
// 与其相关联。从泡泡外操作泡泡化的通道、计时器或
// 计时器会 panic。
//
// [sync.WaitGroup] 在第一次调用 Add 或 Go 时与泡泡相关联。
// 一旦 WaitGroup 与泡泡相关联，从泡泡外调用 Add 或 Go 是致命错误。
// (作为技术限制，定义为包变量的 WaitGroup，
// 例如 "var wg sync.WaitGroup"，不能与泡泡关联，
// 对其的操作可能不是持久阻塞。
// 此限制不适用于存储在包变量中的 *WaitGroup，
// 例如 "var wg = new(sync.WaitGroup)"。)
//
// [sync.Cond.Wait] 是持久阻塞。从泡泡外唤醒阻塞在
// Cond.Wait 上的泡泡中的 goroutine 是致命错误。
//
// 用 [runtime.AddCleanup] 和 [runtime.SetFinalizer]
// 注册的清理函数和终结器
// 运行在任何泡泡之外。
//
// # Example: Context.AfterFunc
//
// This example demonstrates testing the [context.AfterFunc] function.
//
// AfterFunc registers a function to execute in a new goroutine
// after a context is canceled.
//
// The test verifies that the function is not run before the context is canceled,
// and is run after the context is canceled.
//
//	func TestContextAfterFunc(t *testing.T) {
//		synctest.Test(t, func(t *testing.T) {
//			// Create a context.Context which can be canceled.
//			ctx, cancel := context.WithCancel(t.Context())
//
//			// context.AfterFunc registers a function to be called
//			// when a context is canceled.
//			afterFuncCalled := false
//			context.AfterFunc(ctx, func() {
//				afterFuncCalled = true
//			})
//
//			// The context has not been canceled, so the AfterFunc is not called.
//			synctest.Wait()
//			if afterFuncCalled {
//				t.Fatalf("before context is canceled: AfterFunc called")
//			}
//
//			// Cancel the context and wait for the AfterFunc to finish executing.
//			// Verify that the AfterFunc ran.
//			cancel()
//			synctest.Wait()
//			if !afterFuncCalled {
//				t.Fatalf("after context is canceled: AfterFunc not called")
//			}
//		})
//	}
//
// # Example: Context.WithTimeout
//
// This example demonstrates testing the [context.WithTimeout] function.
//
// WithTimeout creates a context which is canceled after a timeout.
//
// The test verifies that the context is not canceled before the timeout expires,
// and is canceled after the timeout expires.
//
//	func TestContextWithTimeout(t *testing.T) {
//		synctest.Test(t, func(t *testing.T) {
//			// Create a context.Context which is canceled after a timeout.
//			const timeout = 5 * time.Second
//			ctx, cancel := context.WithTimeout(t.Context(), timeout)
//			defer cancel()
//
//			// Wait just less than the timeout.
//			time.Sleep(timeout - time.Nanosecond)
//			synctest.Wait()
//			if err := ctx.Err(); err != nil {
//				t.Fatalf("before timeout: ctx.Err() = %v, want nil\n", err)
//			}
//
//			// Wait the rest of the way until the timeout.
//			time.Sleep(time.Nanosecond)
//			synctest.Wait()
//			if err := ctx.Err(); err != context.DeadlineExceeded {
//				t.Fatalf("after timeout: ctx.Err() = %v, want DeadlineExceeded\n", err)
//			}
//		})
//	}
//
// # Example: HTTP 100 Continue
//
// This example demonstrates testing [http.Transport]'s 100 Continue handling.
//
// An HTTP client sending a request can include an "Expect: 100-continue" header
// to tell the server that the client has additional data to send.
// The server may then respond with an 100 Continue information response
// to request the data, or some other status to tell the client the data is not needed.
// For example, a client uploading a large file might use this feature to confirm
// that the server is willing to accept the file before sending it.
//
// This test confirms that when sending an "Expect: 100-continue" header
// the HTTP client does not send a request's content before the server requests it,
// and that it does send the content after receiving a 100 Continue response.
//
//	func TestHTTPTransport100Continue(t *testing.T) {
//		synctest.Test(t, func(*testing.T) {
//			// Create an in-process fake network connection.
//			// We cannot use a loopback network connection for this test,
//			// because goroutines blocked on network I/O prevent a synctest
//			// bubble from becoming idle.
//			srvConn, cliConn := net.Pipe()
//			defer cliConn.Close()
//			defer srvConn.Close()
//
//			tr := &http.Transport{
//				// Use the fake network connection created above.
//				DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
//					return cliConn, nil
//				},
//				// Enable "Expect: 100-continue" handling.
//				ExpectContinueTimeout: 5 * time.Second,
//			}
//
//			// Send a request with the "Expect: 100-continue" header set.
//			// Send it in a new goroutine, since it won't complete until the end of the test.
//			body := "request body"
//			go func() {
//				req, _ := http.NewRequest("PUT", "http://test.tld/", strings.NewReader(body))
//				req.Header.Set("Expect", "100-continue")
//				resp, err := tr.RoundTrip(req)
//				if err != nil {
//					t.Errorf("RoundTrip: unexpected error %v\n", err)
//				} else {
//					resp.Body.Close()
//				}
//			}()
//
//			// Read the request headers sent by the client.
//			req, err := http.ReadRequest(bufio.NewReader(srvConn))
//			if err != nil {
//				t.Fatalf("ReadRequest: %v\n", err)
//			}
//
//			// Start a new goroutine copying the body sent by the client into a buffer.
//			// Wait for all goroutines in the bubble to block and verify that we haven't
//			// read anything from the client yet.
//			var gotBody bytes.Buffer
//			go io.Copy(&gotBody, req.Body)
//			synctest.Wait()
//			if got, want := gotBody.String(), ""; got != want {
//				t.Fatalf("before sending 100 Continue, read body: %q, want %q\n", got, want)
//			}
//
//			// Write a "100 Continue" response to the client and verify that
//			// it sends the request body.
//			srvConn.Write([]byte("HTTP/1.1 100 Continue\r\n\r\n"))
//			synctest.Wait()
//			if got, want := gotBody.String(), body; got != want {
//				t.Fatalf("after sending 100 Continue, read body: %q, want %q\n", got, want)
//			}
//
//			// Finish up by sending the "200 OK" response to conclude the request.
//			srvConn.Write([]byte("HTTP/1.1 200 OK\r\n\r\n"))
//
//			// We started several goroutines during the test.
//			// The synctest.Test call will wait for all of them to exit before returning.
//		})
//	}
package synctest

import (
	"internal/synctest"
	"testing"
	_ "unsafe" // for linkname
)

// Test executes f in a new bubble.
//
// Test waits for all goroutines in the bubble to exit before returning.
// If the goroutines in the bubble become deadlocked, the test fails.
//
// Test must not be called from within a bubble.
//
// The [*testing.T] provided to f has the following properties:
//
//   - T.Cleanup functions run inside the bubble,
//     immediately before Test returns.
//   - T.Context returns a [context.Context] with a Done channel
//     associated with the bubble.
//   - T.Run, T.Parallel, and T.Deadline must not be called.
func Test(t *testing.T, f func(*testing.T)) {
	var ok bool
	synctest.Run(func() {
		ok = testingSynctestTest(t, f)
	})
	if !ok {
		// Fail the test outside the bubble,
		// so test durations get set using real time.
		t.FailNow()
	}
}

//go:linkname testingSynctestTest testing/synctest.testingSynctestTest
func testingSynctestTest(t *testing.T, f func(*testing.T)) bool

// Wait blocks until every goroutine within the current bubble,
// other than the current goroutine, is durably blocked.
//
// Wait must not be called from outside a bubble.
// Wait must not be called concurrently by multiple goroutines
// in the same bubble.
func Wait() {
	synctest.Wait()
}
