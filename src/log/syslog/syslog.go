// 版权所有 2009 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

//go:build !windows && !plan9

package syslog

import (
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"time"
)

// Priority 是 syslog 设施和严重性的组合。
// 例如，[LOG_ALERT] | [LOG_FTP] 从 FTP 设施发送警报严重性消息。
// 默认严重性是 [LOG_EMERG]；默认设施是 [LOG_KERN]。
type Priority int

const severityMask = 0x07
const facilityMask = 0xf8

const (
	// 严重性。

	// 来自 /usr/include/sys/syslog.h。
	// 在 Linux、BSD 和 OS X 上相同。
	LOG_EMERG Priority = iota
	LOG_ALERT
	LOG_CRIT
	LOG_ERR
	LOG_WARNING
	LOG_NOTICE
	LOG_INFO
	LOG_DEBUG
)

const (
	// 设施。

	// 来自 /usr/include/sys/syslog.h。
	// 在 Linux、BSD 和 OS X 上直到 LOG_FTP 都相同。
	LOG_KERN Priority = iota << 3
	LOG_USER
	LOG_MAIL
	LOG_DAEMON
	LOG_AUTH
	LOG_SYSLOG
	LOG_LPR
	LOG_NEWS
	LOG_UUCP
	LOG_CRON
	LOG_AUTHPRIV
	LOG_FTP
	_ // 未使用
	_ // 未使用
	_ // 未使用
	_ // 未使用
	LOG_LOCAL0
	LOG_LOCAL1
	LOG_LOCAL2
	LOG_LOCAL3
	LOG_LOCAL4
	LOG_LOCAL5
	LOG_LOCAL6
	LOG_LOCAL7
)

// Writer 是到 syslog 服务器的连接。
type Writer struct {
	priority Priority
	tag      string
	hostname string
	network  string
	raddr    string

	mu   sync.Mutex // 保护 conn
	conn serverConn
}

// 此接口和单独的 syslog_unix.go 文件是为了支持 gccgo 实现的 Solaris。
// 在 Solaris 上，你不能简单地打开到 syslog 守护进程的 TCP 连接。
// gccgo 源代码有一个 syslog_solaris.go 文件，它实现了 unixSyslog
// 来返回一个满足此接口的类型，并简单地调用 C 库的 syslog 函数。
type serverConn interface {
	writeString(p Priority, hostname, tag, s, nl string) error
	close() error
}

type netConn struct {
	local bool
	conn  net.Conn
}

// New 建立到系统日志守护进程的新连接。
// 每次写入返回的 writer 都会发送一条具有给定优先级
// （syslog 设施和严重性的组合）和前缀标签的日志消息。
// 如果 tag 为空，则使用 [os.Args][0]。
func New(priority Priority, tag string) (*Writer, error) {
	return Dial("", "", priority, tag)
}

// Dial 通过连接到指定网络上的地址 raddr 来建立到日志守护进程的连接。
// 每次写入返回的 writer 都会发送一条具有设施和严重性
// （来自 priority）以及 tag 的日志消息。如果 tag 为空，则使用 [os.Args][0]。
// 如果 network 为空，Dial 将连接到本地 syslog 服务器。
// 否则，请参阅 net.Dial 的文档以获取 network 和 raddr 的有效值。
func Dial(network, raddr string, priority Priority, tag string) (*Writer, error) {
	if priority < 0 || priority > LOG_LOCAL7|LOG_DEBUG {
		return nil, errors.New("log/syslog: invalid priority")
	}

	if tag == "" {
		tag = os.Args[0]
	}
	hostname, _ := os.Hostname()

	w := &Writer{
		priority: priority,
		tag:      tag,
		hostname: hostname,
		network:  network,
		raddr:    raddr,
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	err := w.connect()
	if err != nil {
		return nil, err
	}
	return w, nil
}

// connect 建立到 syslog 服务器的连接。
// 必须在持有 w.mu 的情况下调用。
func (w *Writer) connect() (err error) {
	if w.conn != nil {
		// 忽略 close 的错误，无论如何继续是有意义的
		w.conn.close()
		w.conn = nil
	}

	if w.network == "" {
		w.conn, err = unixSyslog()
		if w.hostname == "" {
			w.hostname = "localhost"
		}
	} else {
		var c net.Conn
		c, err = net.Dial(w.network, w.raddr)
		if err == nil {
			w.conn = &netConn{
				conn:  c,
				local: w.network == "unixgram" || w.network == "unix",
			}
			if w.hostname == "" {
				w.hostname = c.LocalAddr().String()
			}
		}
	}
	return
}

// Write 向 syslog 守护进程发送日志消息。
func (w *Writer) Write(b []byte) (int, error) {
	return w.writeAndRetry(w.priority, string(b))
}

// Close 关闭到 syslog 守护进程的连接。
func (w *Writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.conn != nil {
		err := w.conn.close()
		w.conn = nil
		return err
	}
	return nil
}

// Emerg 以 [LOG_EMERG] 严重性记录消息，忽略传递给 New 的严重性。
func (w *Writer) Emerg(m string) error {
	_, err := w.writeAndRetry(LOG_EMERG, m)
	return err
}

// Alert 以 [LOG_ALERT] 严重性记录消息，忽略传递给 New 的严重性。
func (w *Writer) Alert(m string) error {
	_, err := w.writeAndRetry(LOG_ALERT, m)
	return err
}

// Crit 以 [LOG_CRIT] 严重性记录消息，忽略传递给 New 的严重性。
func (w *Writer) Crit(m string) error {
	_, err := w.writeAndRetry(LOG_CRIT, m)
	return err
}

// Err 以 [LOG_ERR] 严重性记录消息，忽略传递给 New 的严重性。
func (w *Writer) Err(m string) error {
	_, err := w.writeAndRetry(LOG_ERR, m)
	return err
}

// Warning 以 [LOG_WARNING] 严重性记录消息，忽略传递给 New 的严重性。
func (w *Writer) Warning(m string) error {
	_, err := w.writeAndRetry(LOG_WARNING, m)
	return err
}

// Notice 以 [LOG_NOTICE] 严重性记录消息，忽略传递给 New 的严重性。
func (w *Writer) Notice(m string) error {
	_, err := w.writeAndRetry(LOG_NOTICE, m)
	return err
}

// Info 以 [LOG_INFO] 严重性记录消息，忽略传递给 New 的严重性。
func (w *Writer) Info(m string) error {
	_, err := w.writeAndRetry(LOG_INFO, m)
	return err
}

// Debug 以 [LOG_DEBUG] 严重性记录消息，忽略传递给 New 的严重性。
func (w *Writer) Debug(m string) error {
	_, err := w.writeAndRetry(LOG_DEBUG, m)
	return err
}

func (w *Writer) writeAndRetry(p Priority, s string) (int, error) {
	pr := (w.priority & facilityMask) | (p & severityMask)

	w.mu.Lock()
	defer w.mu.Unlock()

	if w.conn != nil {
		if n, err := w.write(pr, s); err == nil {
			return n, nil
		}
	}
	if err := w.connect(); err != nil {
		return 0, err
	}
	return w.write(pr, s)
}

// write 生成并写入 syslog 格式的字符串。
// 格式如下：<PRI>TIMESTAMP HOSTNAME TAG[PID]: MSG
func (w *Writer) write(p Priority, msg string) (int, error) {
	// 确保以 \n 结尾
	nl := ""
	if !strings.HasSuffix(msg, "\n") {
		nl = "\n"
	}

	err := w.conn.writeString(p, w.hostname, w.tag, msg, nl)
	if err != nil {
		return 0, err
	}
	// 注意：返回输入的长度，而不是 Fprintf 打印的字节数，
	// 因为这必须像 io.Writer 一样工作。
	return len(msg), nil
}

func (n *netConn) writeString(p Priority, hostname, tag, msg, nl string) error {
	if n.local {
		// 与下面的网络形式相比，更改如下：
		//	1. 使用 time.Stamp 而不是 time.RFC3339。
		//	2. 从 Fprintf 中删除主机名字段。
		timestamp := time.Now().Format(time.Stamp)
		_, err := fmt.Fprintf(n.conn, "<%d>%s %s[%d]: %s%s",
			p, timestamp,
			tag, os.Getpid(), msg, nl)
		return err
	}
	timestamp := time.Now().Format(time.RFC3339)
	_, err := fmt.Fprintf(n.conn, "<%d>%s %s %s[%d]: %s%s",
		p, timestamp, hostname,
		tag, os.Getpid(), msg, nl)
	return err
}

func (n *netConn) close() error {
	return n.conn.Close()
}

// NewLogger 创建一个 [log.Logger]，其输出以指定的优先级
// （syslog 设施和严重性的组合）写入系统日志服务。
// logFlag 参数是传递给 [log.New] 以创建 Logger 的标志集。
func NewLogger(p Priority, logFlag int) (*log.Logger, error) {
	s, err := New(p, "")
	if err != nil {
		return nil, err
	}
	return log.New(s, "", logFlag), nil
}
