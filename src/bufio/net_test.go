// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build unix

package bufio_test

import (
	"bufio"
	"io"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// TestCopyUnixpacket 测试我们可以在跨 unixpacket socket 复制时使用 bufio。
// 由于不必要的空 Write 调用被解释为 EOF，这曾经失败。
func TestCopyUnixpacket(t *testing.T) {
	tmpDir := t.TempDir()
	socket := filepath.Join(tmpDir, "unixsock")

	// 启动一个 unixpacket 服务器。
	addr := &net.UnixAddr{
		Name: socket,
		Net:  "unixpacket",
	}
	server, err := net.ListenUnix("unixpacket", addr)
	if err != nil {
		t.Skipf("skipping test because opening a unixpacket socket failed: %v", err)
	}

	// 为服务器启动一个 goroutine 以接受一个连接
	// 并读取在连接上发送的所有数据，
	// 在 ch 上报告读取的字节数。
	ch := make(chan int, 1)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()

		tot := 0
		defer func() {
			ch <- tot
		}()

		serverConn, err := server.Accept()
		if err != nil {
			t.Error(err)
			return
		}

		buf := make([]byte, 1024)
		for {
			n, err := serverConn.Read(buf)
			tot += n
			if err == io.EOF {
				return
			}
			if err != nil {
				t.Error(err)
				return
			}
		}
	}()

	clientConn, err := net.DialUnix("unixpacket", nil, addr)
	if err != nil {
		// 让服务器 goroutine 挂起。没关系。
		t.Fatal(err)
	}

	defer wg.Wait()
	defer clientConn.Close()

	const data = "data"
	r := bufio.NewReader(strings.NewReader(data))
	n, err := io.Copy(clientConn, r)
	if err != nil {
		t.Fatal(err)
	}

	if n != int64(len(data)) {
		t.Errorf("io.Copy returned %d, want %d", n, len(data))
	}

	clientConn.Close()
	tot := <-ch

	if tot != len(data) {
		t.Errorf("server read %d, want %d", tot, len(data))
	}
}
