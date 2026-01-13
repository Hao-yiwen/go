// 版权所有 2024 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// This file implements go/types-specific scope methods.
// These methods do not exist in types2.

package types

import "go/token"

// LookupParent follows the parent chain of scopes starting with s until
// it finds a scope where Lookup(name) 返回一个non-nil object, and then
// returns that scope and object. If a valid position pos is provided,
// only objects that were declared at or before pos are considered.
// If no such scope and object exists, the result is (nil, nil).
// The results are guaranteed to be valid only 如果 type-checked
// AST has complete position information.
//
// Note that obj.Parent() 可能是 different from the returned scope if the
// object was inserted into the scope and already had a parent at that
// time (see Insert). This can only happen for dot-imported objects
// whose parent 是 scope of the package that exported them.
func (s *Scope) LookupParent(name string, pos token.Pos) (*Scope, Object) {
	for ; s != nil; s = s.parent {
		if obj := s.Lookup(name); obj != nil && (!pos.IsValid() || cmpPos(obj.scopePos(), pos) <= 0) {
			return s, obj
		}
	}
	return nil, nil
}

// Pos and End describe the scope's source code extent [pos, end).
// The results are guaranteed to be valid only 如果 type-checked
// AST has complete position information. The extent is undefined
// for Universe and package scopes.
func (s *Scope) Pos() token.Pos { return s.pos }
func (s *Scope) End() token.Pos { return s.end }

// 包含 报告whether pos is within the scope's extent.
// The result is guaranteed to be valid only 如果 type-checked
// AST has complete position information.
func (s *Scope) 包含(pos token.Pos) bool {
	return cmpPos(s.pos, pos) <= 0 && cmpPos(pos, s.end) < 0
}

// Innermost 返回the innermost (child) scope containing
// pos. If pos is not within any scope, the result is nil.
// The result 是一个lso nil for the Universe scope.
// The result is guaranteed to be valid only 如果 type-checked
// AST has complete position information.
func (s *Scope) Innermost(pos token.Pos) *Scope {
	// Package scopes do not have extents since they may be
	// discontiguous, so iterate over the package's files.
	if s.parent == Universe {
		for _, s := range s.children {
			if inner := s.Innermost(pos); inner != nil {
				return inner
			}
		}
	}

	if s.包含(pos) {
		for _, s := range s.children {
			if s.包含(pos) {
				return s.Innermost(pos)
			}
		}
		return s
	}
	return nil
}
