// 版权所有 2009 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package scanner

import (
	"fmt"
	"go/token"
	"io"
	"sort"
)

// In an [ErrorList], an error is represented by an *Error.
// The position Pos, if valid, points to the beginning of
// the offending token, and the error condition is described
// by Msg.
type Error struct {
	Pos token.Position
	Msg string
}

// Error 实现the error interface.
func (e Error) Error() string {
	if e.Pos.Filename != "" || e.Pos.IsValid() {
		// don't print "<unknown position>"
		// TODO(gri) reconsider the semantics of Position.IsValid
		return e.Pos.String() + ": " + e.Msg
	}
	return e.Msg
}

// ErrorList 是一个 list of *Errors.
// The zero value for an ErrorList 是一个n empty ErrorList ready to use.
type ErrorList []*Error

// Add adds an [Error] with given position and error message to an [ErrorList].
func (p *ErrorList) Add(pos token.Position, msg string) {
	*p = append(*p, &Error{pos, msg})
}

// Reset resets an [ErrorList] to no errors.
func (p *ErrorList) Reset() { *p = (*p)[0:0] }

// [ErrorList] implements the sort Interface.
func (p ErrorList) Len() int      { return len(p) }
func (p ErrorList) Swap(i, j int) { p[i], p[j] = p[j], p[i] }

func (p ErrorList) Less(i, j int) bool {
	e := &p[i].Pos
	f := &p[j].Pos
	// Note that it is not sufficient to simply compare file offsets because
	// the offsets do not reflect modified line information (through //line
	// comments).
	if e.Filename != f.Filename {
		return e.Filename < f.Filename
	}
	if e.Line != f.Line {
		return e.Line < f.Line
	}
	if e.Column != f.Column {
		return e.Column < f.Column
	}
	return p[i].Msg < p[j].Msg
}

// Sort sorts an [ErrorList]. *[Error] entries are sorted by position,
// other errors are sorted by error message, and before any *[Error]
// entry.
func (p ErrorList) Sort() {
	sort.Sort(p)
}

// RemoveMultiples sorts an [ErrorList] and removes all but the first error per line.
func (p *ErrorList) RemoveMultiples() {
	sort.Sort(p)
	var last token.Position // initial last.Line is != any legal error line
	i := 0
	for _, e := range *p {
		if e.Pos.Filename != last.Filename || e.Pos.Line != last.Line {
			last = e.Pos
			(*p)[i] = e
			i++
		}
	}
	*p = (*p)[0:i]
}

// 一个[ErrorList] implements the error interface.
func (p ErrorList) Error() string {
	switch len(p) {
	case 0:
		return "no errors"
	case 1:
		return p[0].Error()
	}
	return fmt.Sprintf("%s (and %d more errors)", p[0], len(p)-1)
}

// Err 返回an error equivalent to this error list.
// If the list is empty, Err returns nil.
func (p ErrorList) Err() error {
	if len(p) == 0 {
		return nil
	}
	return p
}

// PrintError 是一个 utility function that prints a list of errors to w,
// one error per line, 如果 err parameter 是一个n [ErrorList]. Otherwise
// it prints the err string.
func PrintError(w io.Writer, err error) {
	if list, ok := err.(ErrorList); ok {
		for _, e := range list {
			fmt.Fprintf(w, "%s\n", e)
		}
	} else if err != nil {
		fmt.Fprintf(w, "%s\n", err)
	}
}
