// 版权所有 2018 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package server

import (
	"context"
	"errors"
)

type A struct {
	x int
}

func (a *A) AMethod(y int) *Server {
	return nil
}

// FooServer 是一个 server that provides Foo services
type FooServer Server

func (f *FooServer) WriteEvents(ctx context.Context, x int) error {
	return errors.New("hey!")
}

type Server struct {
	FooServer *FooServer
	user      string
	ctx       context.Context
}

func New(sctx context.Context, u string) (*Server, error) {
	s := &Server{user: u, ctx: sctx}
	s.FooServer = (*FooServer)(s)
	return s, nil
}
