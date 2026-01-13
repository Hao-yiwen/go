// 版权所有 2010 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package textproto

import (
	"sync"
)

// Pipeline 管理流水线有序请求/响应序列。
//
// 要使用 Pipeline p 在连接上管理多个客户端，
// 每个客户端应运行：
//
//	id := p.Next()	// 取一个号码
//
//	p.StartRequest(id)	// 等待轮次发送请求
//	«send request»
//	p.EndRequest(id)	// 通知 Pipeline 请求已发送
//
//	p.StartResponse(id)	// 等待轮次读取响应
//	«read response»
//	p.EndResponse(id)	// 通知 Pipeline 响应已读取
//
// 流水线服务器可以使用相同的调用来确保
// 并行计算的响应以正确的顺序写入。
type Pipeline struct {
	mu       sync.Mutex
	id       uint
	request  sequencer
	response sequencer
}

// Next 返回请求/响应对的下一个 id。
func (p *Pipeline) Next() uint {
	p.mu.Lock()
	id := p.id
	p.id++
	p.mu.Unlock()
	return id
}

// StartRequest 阻塞直到是时候发送（或者，如果这是一个服务器，接收）
// 具有给定 id 的请求。
func (p *Pipeline) StartRequest(id uint) {
	p.request.Start(id)
}

// EndRequest 通知 p 具有给定 id 的请求已被发送
// （或者，如果这是一个服务器，已接收）。
func (p *Pipeline) EndRequest(id uint) {
	p.request.End(id)
}

// StartResponse 阻塞直到是时候接收（或者，如果这是一个服务器，发送）
// 具有给定 id 的请求。
func (p *Pipeline) StartResponse(id uint) {
	p.response.Start(id)
}

// EndResponse 通知 p 具有给定 id 的响应已被接收
// （或者，如果这是一个服务器，已发送）。
func (p *Pipeline) EndResponse(id uint) {
	p.response.End(id)
}

// sequencer 调度一系列必须
// 按顺序一个接一个发生的编号事件。事件编号必须从
// 0 开始并逐步递增，不跳过。事件号会环绕
// 安全地，只要没有 2^32 个同时待处理的事件。
type sequencer struct {
	mu   sync.Mutex
	id   uint
	wait map[uint]chan struct{}
}

// Start 等待直到编号为 id 的事件开始的时刻。
// 也就是说，除了第一个事件，它等待直到 End(id-1) 已
// 被调用。
func (s *sequencer) Start(id uint) {
	s.mu.Lock()
	if s.id == id {
		s.mu.Unlock()
		return
	}
	c := make(chan struct{})
	if s.wait == nil {
		s.wait = make(map[uint]chan struct{})
	}
	s.wait[id] = c
	s.mu.Unlock()
	<-c
}

// End 通知 sequencer 编号为 id 的事件已完成，
// 允许它调度编号为 id+1 的事件。调用
// End 使用不是活动事件号的 id 是一个运行时错误。
func (s *sequencer) End(id uint) {
	s.mu.Lock()
	if s.id != id {
		s.mu.Unlock()
		panic("out of sync")
	}
	id++
	s.id = id
	if s.wait == nil {
		s.wait = make(map[uint]chan struct{})
	}
	c, ok := s.wait[id]
	if ok {
		delete(s.wait, id)
	}
	s.mu.Unlock()
	if ok {
		close(c)
	}
}
