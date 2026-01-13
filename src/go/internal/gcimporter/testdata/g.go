// 版权所有 2016 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// Input for TestTypeNamingOrder

// ensures that the order in which "type A B" declarations are
// processed is correct; this was a problem for unified IR imports.

package g

type Client struct {
	common service
	A      *AService
	B      *BService
}

type service struct {
	client *Client
}

type AService service
type BService service
