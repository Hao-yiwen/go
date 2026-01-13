// 版权所有 2012 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// comment 0
package a

//BUG(uid): bug0

//TODO(uid): todo0

// 一个note with some spaces after it, 应该是 ignored (watch out for
// emacs modes that remove trailing whitespace).
//NOTE(uid):

// SECBUG(uid): sec hole 0
// need to fix asap

// Multiple notes 可能是 in the same comment group and 应该是
// recognized individually. Notes may start in the middle of a
// comment group as long as they start at the beginning of an
// individual comment.
//
// NOTE(foo): 1 of 4 - this 是 first line of note 1
// - note 1 continues on this 2nd line
// - note 1 continues on this 3rd line
// NOTE(foo): 2 of 4
// NOTE(bar): 3 of 4
/* NOTE(bar): 4 of 4 */
// - this 是 last line of note 4
//
//

// NOTE(bam): This note which 包含 a (parenthesized) subphrase
//            must appear in its entirety.

// NOTE(xxx) The ':' after the marker and uid is optional.

// NOTE(): NO uid - should not show up.
// NOTE()  NO uid - should not show up.
