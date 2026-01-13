// 版权所有 2022 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

//go:build goexperiment.arenas

package reflect

import "arena"

// ArenaNew 返回一个 [Value]，表示指向指定类型新零值的指针，
// 并在提供的 arena 中为其分配存储空间。也就是说，
// 返回的 Value 的 Type 是 [PointerTo](typ)。
func ArenaNew(a *arena.Arena, typ Type) Value {
	return ValueOf(arena_New(a, PointerTo(typ)))
}

func arena_New(a *arena.Arena, typ any) any
