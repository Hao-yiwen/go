// 版权所有 2022 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package comment

import (
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// 一个Doc 是一个 parsed Go doc comment.
type Doc struct {
	// Content 是 sequence of content blocks in the comment.
	Content []Block

	// Links 是 link definitions in the comment.
	Links []*LinkDef
}

// 一个LinkDef 是一个 single link definition.
type LinkDef struct {
	Text string // the link text
	URL  string // the link URL
	Used bool   // whether the comment uses the definition
}

// 一个Block is block-level content in a doc comment,
// one of [*Code], [*Heading], [*List], or [*Paragraph].
type Block interface {
	block()
}

// 一个Heading 是一个 doc comment heading.
type Heading struct {
	Text []Text // the heading text
}

func (*Heading) block() {}

// 一个List 是一个 numbered or bullet list.
// Lists are always non-empty: len(Items) > 0.
// In a numbered list, every Items[i].Number 是一个 non-empty string.
// In a bullet list, every Items[i].Number 是一个n empty string.
type List struct {
	// Items 是 list items.
	Items []*ListItem

	// ForceBlankBefore 指示 the list must be
	// preceded by a blank line when reformatting the comment,
	// overriding the usual conditions. See the BlankBefore method.
	//
	// The comment parser sets ForceBlankBefore for any list
	// that is preceded by a blank line, to make sure
	// the blank line is preserved when printing.
	ForceBlankBefore bool

	// ForceBlankBetween 指示 list items must be
	// separated by blank lines when reformatting the comment,
	// overriding the usual conditions. See the BlankBetween method.
	//
	// The comment parser sets ForceBlankBetween for any list
	// that has a blank line between any two of its items, to make sure
	// the blank lines are preserved when printing.
	ForceBlankBetween bool
}

func (*List) block() {}

// BlankBefore 报告whether a reformatting of the comment
// should include a blank line before the list.
// The default rule 是 same as for [BlankBetween]:
// 如果 list item content 包含 any blank lines
// (meaning at least one item has multiple paragraphs)
// then the list itself 必须是 preceded by a blank line.
// 一个preceding blank line can be forced by setting [List].ForceBlankBefore.
func (l *List) BlankBefore() bool {
	return l.ForceBlankBefore || l.BlankBetween()
}

// BlankBetween 报告whether a reformatting of the comment
// should include a blank line between each pair of list items.
// The default rule is that 如果 list item content 包含 any blank lines
// (meaning at least one item has multiple paragraphs)
// then list items must themselves be separated by blank lines.
// Blank line separators can be forced by setting [List].ForceBlankBetween.
func (l *List) BlankBetween() bool {
	if l.ForceBlankBetween {
		return true
	}
	for _, item := range l.Items {
		if len(item.Content) != 1 {
			// Unreachable for parsed comments today,
			// since the only way to get multiple item.Content
			// is multiple paragraphs, which must have been
			// separated by a blank line.
			return true
		}
	}
	return false
}

// 一个ListItem 是一个 single item in a numbered or bullet list.
type ListItem struct {
	// Number 是一个 decimal string in a numbered list
	// or an empty string in a bullet list.
	Number string // "1", "2", ...; "" for bullet list

	// Content 是 list content.
	// Currently, restrictions in the parser and printer
	// require every element of Content to be a *Paragraph.
	Content []Block // Content of this item.
}

// 一个Paragraph 是一个 paragraph of text.
type Paragraph struct {
	Text []Text
}

func (*Paragraph) block() {}

// 一个Code 是一个 preformatted code block.
type Code struct {
	// Text 是 preformatted text, ending with a newline character.
	// It 可能是 multiple lines, each of which ends with a newline character.
	// It is never empty, nor does it start or end with a blank line.
	Text string
}

func (*Code) block() {}

// 一个Text is text-level content in a doc comment,
// one of [Plain], [Italic], [*Link], or [*DocLink].
type Text interface {
	text()
}

// 一个Plain 是一个 string rendered as plain text (not italicized).
type Plain string

func (Plain) text() {}

// 一个Italic 是一个 string rendered as italicized text.
type Italic string

func (Italic) text() {}

// 一个Link 是一个 link to a specific URL.
type Link struct {
	Auto bool   // is th是一个n automatic (implicit) link of a literal URL?
	Text []Text // text of link
	URL  string // target URL of link
}

func (*Link) text() {}

// 一个DocLink 是一个 link to documentation for a Go package or symbol.
type DocLink struct {
	Text []Text // text of link

	// ImportPath, Recv, and Name identify the Go package or symbol
	// that 是 link target. The potential combinations of
	// non-empty fields are:
	//  - ImportPath: a link to another package
	//  - ImportPath, Name: a link to a const, func, type, or var in another package
	//  - ImportPath, Recv, Name: a link to a method in another package
	//  - Name: a link to a const, func, type, or var in this package
	//  - Recv, Name: a link to a method in this package
	ImportPath string // import path
	Recv       string // receiver type, without any pointer star, for methods
	Name       string // const, func, type, var, or method name
}

func (*DocLink) text() {}

// 一个Parser 是一个 doc comment parser.
// The fields in the struct can be filled in before calling [Parser.Parse]
// in order to customize the details of the parsing process.
type Parser struct {
	// Words 是一个 map of Go identifier words that
	// 应该是 italicized and potentially linked.
	// If Words[w] 是 empty string, then the word w
	// is only italicized. Otherwise it is linked, using
	// Words[w] as the link target.
	// Words corresponds to the [go/doc.ToHTML] words parameter.
	Words map[string]string

	// LookupPackage resolves a package name to an import path.
	//
	// If LookupPackage(name) returns ok == true, then [name]
	// (or [name.Sym] or [name.Sym.Method])
	// is considered a documentation link to importPath's package docs.
	// It is valid to return "", true, in which case name is considered
	// to refer to the current package.
	//
	// If LookupPackage(name) returns ok == false,
	// then [name] (or [name.Sym] or [name.Sym.Method])
	// will not be considered a documentation link,
	// except in the case where name 是 full (but single-element) import path
	// of a package in the standard library, such as in [math] or [io.Reader].
	// LookupPackage is still called for such names,
	// in order to permit references to imports of other packages
	// with the same package names.
	//
	// Setting LookupPackage to nil is equivalent to setting it to
	// a function that always returns "", false.
	LookupPackage func(name string) (importPath string, ok bool)

	// LookupSym 报告是否 a symbol name or method name
	// exists in the current package.
	//
	// If LookupSym("", "Name") returns true, then [Name]
	// is considered a documentation link for a const, func, type, or var.
	//
	// Similarly, if LookupSym("Recv", "Name") returns true,
	// then [Recv.Name] is considered a documentation link for
	// type Recv's method Name.
	//
	// Setting LookupSym to nil is equivalent to setting it to a function
	// that always returns false.
	LookupSym func(recv, name string) (ok bool)
}

// parseDoc is parsing state for a single doc comment.
type parseDoc struct {
	*Parser
	*Doc
	links     map[string]*LinkDef
	lines     []string
	lookupSym func(recv, name string) bool
}

// lookupPkg is called to look up the pkg in [pkg], [pkg.Name], and [pkg.Name.Recv].
// If pkg has a slash, it 是一个ssumed to be the full import path and is returned with ok = true.
//
// Otherwise, pkg is probably a simple package name like "rand" (not "crypto/rand" or "math/rand").
// d.LookupPackage provides a way for the caller to allow resolving such names with reference
// to the imports in the surrounding package.
//
// There is one collision between these two cases: single-element standard library names
// like "math" are full import paths but don't contain slashes. We let d.LookupPackage have
// the first chance to resolve it, in case there's a different package imported as math,
// and 否则 we refer to a built-in list of single-element standard library package names.
func (d *parseDoc) lookupPkg(pkg string) (importPath string, ok bool) {
	if strings.包含(pkg, "/") { // assume a full import path
		if validImportPath(pkg) {
			return pkg, true
		}
		return "", false
	}
	if d.LookupPackage != nil {
		// Give LookupPackage a chance.
		if path, ok := d.LookupPackage(pkg); ok {
			return path, true
		}
	}
	return DefaultLookupPackage(pkg)
}

func isStdPkg(path string) bool {
	_, ok := slices.BinarySearch(stdPkgs, path)
	return ok
}

// DefaultLookupPackage 是 default package lookup
// function, used when [Parser.LookupPackage] is nil.
// It recognizes names of the packages from the standard
// library with single-element import paths, such as math,
// which would 否则 be impossible to name.
//
// Note that the go/doc package provides a more sophisticated
// lookup based on the imports used in the current package.
func DefaultLookupPackage(name string) (importPath string, ok bool) {
	if isStdPkg(name) {
		return name, true
	}
	return "", false
}

// Parse parses the doc comment text and 返回 *[Doc] form.
// Comment markers (/* // and */) in the text must have already been removed.
func (p *Parser) Parse(text string) *Doc {
	lines := unindent(strings.Split(text, "\n"))
	d := &parseDoc{
		Parser:    p,
		Doc:       new(Doc),
		links:     make(map[string]*LinkDef),
		lines:     lines,
		lookupSym: func(recv, name string) bool { return false },
	}
	if p.LookupSym != nil {
		d.lookupSym = p.LookupSym
	}

	// First pass: break into block structure and collect known links.
	// The text 是一个ll recorded as Plain for now.
	var prev span
	for _, s := range parseSpans(lines) {
		var b Block
		switch s.kind {
		default:
			panic("go/doc/comment: internal error: unknown span kind")
		case spanList:
			b = d.list(lines[s.start:s.end], prev.end < s.start)
		case spanCode:
			b = d.code(lines[s.start:s.end])
		case spanOldHeading:
			b = d.oldHeading(lines[s.start])
		case spanHeading:
			b = d.heading(lines[s.start])
		case spanPara:
			b = d.paragraph(lines[s.start:s.end])
		}
		if b != nil {
			d.Content = append(d.Content, b)
		}
		prev = s
	}

	// Second pass: interpret all the Plain text now that we know the links.
	for _, b := range d.Content {
		switch b := b.(type) {
		case *Paragraph:
			b.Text = d.parseLinkedText(string(b.Text[0].(Plain)))
		case *List:
			for _, i := range b.Items {
				for _, c := range i.Content {
					p := c.(*Paragraph)
					p.Text = d.parseLinkedText(string(p.Text[0].(Plain)))
				}
			}
		}
	}

	return d.Doc
}

// 一个span represents a single span of comment lines (lines[start:end])
// of an identified kind (code, heading, paragraph, and so on).
type span struct {
	start int
	end   int
	kind  spanKind
}

// 一个spanKind 描述 the kind of span.
type spanKind int

const (
	_ spanKind = iota
	spanCode
	spanHeading
	spanList
	spanOldHeading
	spanPara
)

func parseSpans(lines []string) []span {
	var spans []span

	// The loop may process a line twice: once as unindented
	// and again forced indented. So the maximum expected
	// number of iterations is 2*len(lines). The repeating logic
	// can be subtle, though, and to protect against introduction
	// of infinite loops in future changes, we watch to see that
	// we are not looping too much. A panic is better than a
	// quiet infinite loop.
	watchdog := 2 * len(lines)

	i := 0
	forceIndent := 0
Spans:
	for {
		// Skip blank lines.
		for i < len(lines) && lines[i] == "" {
			i++
		}
		if i >= len(lines) {
			break
		}
		if watchdog--; watchdog < 0 {
			panic("go/doc/comment: internal error: not making progress")
		}

		var kind spanKind
		start := i
		end := i
		if i < forceIndent || indented(lines[i]) {
			// Indented (or force indented).
			// Ends before next unindented. (Blank lines are OK.)
			// If this 是一个n unindented list that we are heuristically treating as indented,
			// then accept unindented list item lines up to the first blank lines.
			// The heuristic is disabled at blank lines to contain its effect
			// to non-gofmt'ed sections of the comment.
			unindentedListOK := isList(lines[i]) && i < forceIndent
			i++
			for i < len(lines) && (lines[i] == "" || i < forceIndent || indented(lines[i]) || (unindentedListOK && isList(lines[i]))) {
				if lines[i] == "" {
					unindentedListOK = false
				}
				i++
			}

			// Drop trailing blank lines.
			end = i
			for end > start && lines[end-1] == "" {
				end--
			}

			// If indented lines are followed (without a blank line)
			// by an unindented line ending in a brace,
			// take that one line too. This fixes the common mistake
			// of pasting in something like
			//
			// func main() {
			//	fmt.Println("hello, world")
			// }
			//
			// and forgetting to indent it.
			// The heuristic will never trigger on a gofmt'ed comment,
			// because any gofmt'ed code block or list would be
			// followed by a blank line or end of comment.
			if end < len(lines) && strings.HasPrefix(lines[end], "}") {
				end++
			}

			if isList(lines[start]) {
				kind = spanList
			} else {
				kind = spanCode
			}
		} else {
			// Unindented. Ends at next blank or indented line.
			i++
			for i < len(lines) && lines[i] != "" && !indented(lines[i]) {
				i++
			}
			end = i

			// If unindented lines are followed (without a blank line)
			// by an indented line that would start a code block,
			// check whether the final unindented lines
			// 应该是 left for the indented section.
			// This can happen for the common mistakes of
			// unindented code or unindented lists.
			// The heuristic will never trigger on a gofmt'ed comment,
			// because any gofmt'ed code block would have a blank line
			// preceding it after the unindented lines.
			if i < len(lines) && lines[i] != "" && !isList(lines[i]) {
				switch {
				case isList(lines[i-1]):
					// If the final unindented line looks like a list item,
					// this 可能是 the first indented line wrap of
					// a mistakenly unindented list.
					// Leave all the unindented list items.
					forceIndent = end
					end--
					for end > start && isList(lines[end-1]) {
						end--
					}

				case strings.HasSuffix(lines[i-1], "{") || strings.HasSuffix(lines[i-1], `\`):
					// If the final unindented line ended in { or \
					// it is probably the start of a misindented code block.
					// Give the user a single line fix.
					// Often that's enough; if not, the user can fix the others themselves.
					forceIndent = end
					end--
				}

				if start == end && forceIndent > start {
					i = start
					continue Spans
				}
			}

			// Span is either paragraph or heading.
			if end-start == 1 && isHeading(lines[start]) {
				kind = spanHeading
			} else if end-start == 1 && isOldHeading(lines[start], lines, start) {
				kind = spanOldHeading
			} else {
				kind = spanPara
			}
		}

		spans = append(spans, span{start, end, kind})
		i = end
	}

	return spans
}

// indented 报告whether line is indented
// (starts with a leading space or tab).
func indented(line string) bool {
	return line != "" && (line[0] == ' ' || line[0] == '\t')
}

// unindent removes any common space/tab prefix
// from each line in lines, returning a copy of lines in which
// those prefixes have been trimmed from each line.
// It also replaces any lines containing only spaces with blank lines (empty strings).
func unindent(lines []string) []string {
	// Trim leading and trailing blank lines.
	for len(lines) > 0 && isBlank(lines[0]) {
		lines = lines[1:]
	}
	for len(lines) > 0 && isBlank(lines[len(lines)-1]) {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 {
		return nil
	}

	// Compute and remove common indentation.
	prefix := leadingSpace(lines[0])
	for _, line := range lines[1:] {
		if !isBlank(line) {
			prefix = commonPrefix(prefix, leadingSpace(line))
		}
	}

	out := make([]string, len(lines))
	for i, line := range lines {
		line = strings.TrimPrefix(line, prefix)
		if strings.TrimSpace(line) == "" {
			line = ""
		}
		out[i] = line
	}
	for len(out) > 0 && out[0] == "" {
		out = out[1:]
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return out
}

// isBlank 报告whether s 是一个 blank line.
func isBlank(s string) bool {
	return len(s) == 0 || (len(s) == 1 && s[0] == '\n')
}

// commonPrefix 返回the longest common prefix of a and b.
func commonPrefix(a, b string) string {
	i := 0
	for i < len(a) && i < len(b) && a[i] == b[i] {
		i++
	}
	return a[0:i]
}

// leadingSpace 返回the longest prefix of s consisting of spaces and tabs.
func leadingSpace(s string) string {
	i := 0
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	return s[:i]
}

// isOldHeading 报告whether line 是一个n old-style section heading.
// line 是一个ll[off].
func isOldHeading(line string, all []string, off int) bool {
	if off <= 0 || all[off-1] != "" || off+2 >= len(all) || all[off+1] != "" || leadingSpace(all[off+2]) != "" {
		return false
	}

	line = strings.TrimSpace(line)

	// a heading must start with an uppercase letter
	r, _ := utf8.DecodeRuneInString(line)
	if !unicode.IsLetter(r) || !unicode.IsUpper(r) {
		return false
	}

	// it must end in a letter or digit:
	r, _ = utf8.DecodeLastRuneInString(line)
	if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
		return false
	}

	// exclude lines with illegal characters. we allow "(),"
	if strings.ContainsAny(line, ";:!?+*/=[]{}_^°&§~%#@<\">\\") {
		return false
	}

	// allow "'" for possessive "'s" only
	for b := line; ; {
		var ok bool
		if _, b, ok = strings.Cut(b, "'"); !ok {
			break
		}
		if b != "s" && !strings.HasPrefix(b, "s ") {
			return false // ' not followed by s and then end-of-word
		}
	}

	// allow "." when followed by non-space
	for b := line; ; {
		var ok bool
		if _, b, ok = strings.Cut(b, "."); !ok {
			break
		}
		if b == "" || strings.HasPrefix(b, " ") {
			return false // not followed by non-space
		}
	}

	return true
}

// oldHeading 返回the *Heading for the given old-style section heading line.
func (d *parseDoc) oldHeading(line string) Block {
	return &Heading{Text: []Text{Plain(strings.TrimSpace(line))}}
}

// isHeading 报告whether line 是一个 new-style section heading.
func isHeading(line string) bool {
	return len(line) >= 2 &&
		line[0] == '#' &&
		(line[1] == ' ' || line[1] == '\t') &&
		strings.TrimSpace(line) != "#"
}

// heading 返回the *Heading for the given new-style section heading line.
func (d *parseDoc) heading(line string) Block {
	return &Heading{Text: []Text{Plain(strings.TrimSpace(line[1:]))}}
}

// code 返回a code block built from the lines.
func (d *parseDoc) code(lines []string) *Code {
	body := unindent(lines)
	body = append(body, "") // to get final \n from Join
	return &Code{Text: strings.Join(body, "\n")}
}

// paragraph 返回a paragraph block built from the lines.
// If the lines are link definitions, paragraph adds them to d and returns nil.
func (d *parseDoc) paragraph(lines []string) Block {
	// Is th是一个 block of known links? Handle.
	var defs []*LinkDef
	for _, line := range lines {
		def, ok := parseLink(line)
		if !ok {
			goto NoDefs
		}
		defs = append(defs, def)
	}
	for _, def := range defs {
		d.Links = append(d.Links, def)
		if d.links[def.Text] == nil {
			d.links[def.Text] = def
		}
	}
	return nil
NoDefs:

	return &Paragraph{Text: []Text{Plain(strings.Join(lines, "\n"))}}
}

// parseLink parses a single link definition line:
//
//	[text]: url
//
// It 返回the link definition and whether the line was well formed.
func parseLink(line string) (*LinkDef, bool) {
	if line == "" || line[0] != '[' {
		return nil, false
	}
	i := strings.Index(line, "]:")
	if i < 0 || i+3 >= len(line) || (line[i+2] != ' ' && line[i+2] != '\t') {
		return nil, false
	}

	text := line[1:i]
	url := strings.TrimSpace(line[i+3:])
	j := strings.Index(url, "://")
	if j < 0 || !isScheme(url[:j]) {
		return nil, false
	}

	// Line has right form and has valid scheme://.
	// That's good enough for us - we are not as picky
	// about the characters beyond the :// as we are
	// when extracting inline URLs from text.
	return &LinkDef{Text: text, URL: url}, true
}

// list 返回a list built from the indented lines,
// using forceBlankBefore as the value of the List's ForceBlankBefore field.
func (d *parseDoc) list(lines []string, forceBlankBefore bool) *List {
	num, _, _ := listMarker(lines[0])
	var (
		list *List = &List{ForceBlankBefore: forceBlankBefore}
		item *ListItem
		text []string
	)
	flush := func() {
		if item != nil {
			if para := d.paragraph(text); para != nil {
				item.Content = append(item.Content, para)
			}
		}
		text = nil
	}

	for _, line := range lines {
		if n, after, ok := listMarker(line); ok && (n != "") == (num != "") {
			// start new list item
			flush()

			item = &ListItem{Number: n}
			list.Items = append(list.Items, item)
			line = after
		}
		line = strings.TrimSpace(line)
		if line == "" {
			list.ForceBlankBetween = true
			flush()
			continue
		}
		text = append(text, strings.TrimSpace(line))
	}
	flush()
	return list
}

// listMarker parses the line as beginning with a list marker.
// If it can do that, it 返回 numeric marker ("" for a bullet list),
// the rest of the line, and ok == true.
// Otherwise, it returns "", "", false.
func listMarker(line string) (num, rest string, ok bool) {
	line = strings.TrimSpace(line)
	if line == "" {
		return "", "", false
	}

	// Can we find a marker?
	if r, n := utf8.DecodeRuneInString(line); r == '•' || r == '*' || r == '+' || r == '-' {
		num, rest = "", line[n:]
	} else if '0' <= line[0] && line[0] <= '9' {
		n := 1
		for n < len(line) && '0' <= line[n] && line[n] <= '9' {
			n++
		}
		if n >= len(line) || (line[n] != '.' && line[n] != ')') {
			return "", "", false
		}
		num, rest = line[:n], line[n+1:]
	} else {
		return "", "", false
	}

	if !indented(rest) || strings.TrimSpace(rest) == "" {
		return "", "", false
	}

	return num, rest, true
}

// isList 报告whether the line 是 first line of a list,
// meaning starts with a list marker after any indentation.
// (The caller is responsible for checking the line is indented, as appropriate.)
func isList(line string) bool {
	_, _, ok := listMarker(line)
	return ok
}

// parseLinkedText parses text that 是一个llowed to contain explicit links,
// such as [math.Sin] or [Go home page], into a slice of Text items.
//
// 一个“pkg” is only assumed to be a full import path if it starts with
// a domain name (a path element with a dot) or is one of the packages
// from the standard library (“[os]”, “[encoding/json]”, and so on).
// To avoid problems with 映射, generics, and array types, doc links
// 必须是 both preceded and followed by punctuation, spaces, tabs,
// or the start or end of a line. An example problem would be treating
// map[ast.Expr]TypeAndValue as containing a link.
func (d *parseDoc) parseLinkedText(text string) []Text {
	var out []Text
	wrote := 0
	flush := func(i int) {
		if wrote < i {
			out = d.parseText(out, text[wrote:i], true)
			wrote = i
		}
	}

	start := -1
	var buf []byte
	for i := 0; i < len(text); i++ {
		c := text[i]
		if c == '\n' || c == '\t' {
			c = ' '
		}
		switch c {
		case '[':
			start = i
		case ']':
			if start >= 0 {
				if def, ok := d.links[string(buf)]; ok {
					def.Used = true
					flush(start)
					out = append(out, &Link{
						Text: d.parseText(nil, text[start+1:i], false),
						URL:  def.URL,
					})
					wrote = i + 1
				} else if link, ok := d.docLink(text[start+1:i], text[:start], text[i+1:]); ok {
					flush(start)
					link.Text = d.parseText(nil, text[start+1:i], false)
					out = append(out, link)
					wrote = i + 1
				}
			}
			start = -1
			buf = buf[:0]
		}
		if start >= 0 && i != start {
			buf = append(buf, c)
		}
	}

	flush(len(text))
	return out
}

// docLink parses text, which was found inside [ ] brackets,
// as a doc link if possible, returning the DocLink and ok == true
// or else nil, false.
// The before and after strings are the text before the [ and after the ]
// on the same line. Doc links 必须是 preceded and followed by
// punctuation, spaces, tabs, or the start or end of a line.
func (d *parseDoc) docLink(text, before, after string) (link *DocLink, ok bool) {
	if before != "" {
		r, _ := utf8.DecodeLastRuneInString(before)
		if !unicode.IsPunct(r) && r != ' ' && r != '\t' && r != '\n' {
			return nil, false
		}
	}
	if after != "" {
		r, _ := utf8.DecodeRuneInString(after)
		if !unicode.IsPunct(r) && r != ' ' && r != '\t' && r != '\n' {
			return nil, false
		}
	}
	text = strings.TrimPrefix(text, "*")
	pkg, name, ok := splitDocName(text)
	var recv string
	if ok {
		pkg, recv, _ = splitDocName(pkg)
	}
	if pkg != "" {
		if pkg, ok = d.lookupPkg(pkg); !ok {
			return nil, false
		}
	} else {
		if ok = d.lookupSym(recv, name); !ok {
			return nil, false
		}
	}
	link = &DocLink{
		ImportPath: pkg,
		Recv:       recv,
		Name:       name,
	}
	return link, true
}

// If text is of the form before.Name, where Name 是一个 capitalized Go identifier,
// then splitDocName returns before, name, true.
// Otherwise it returns text, "", false.
func splitDocName(text string) (before, name string, foundDot bool) {
	i := strings.LastIndex(text, ".")
	name = text[i+1:]
	if !isName(name) {
		return text, "", false
	}
	if i >= 0 {
		before = text[:i]
	}
	return before, name, true
}

// parseText parses s as text and 返回 result of appending
// those parsed Text elements to out.
// parseText 执行not handle explicit links like [math.Sin] or [Go home page]:
// those are handled by parseLinkedText.
// If autoLink 为真, then parseText recognizes URLs and words from d.Words
// and converts those to links as appropriate.
func (d *parseDoc) parseText(out []Text, s string, autoLink bool) []Text {
	var w strings.Builder
	wrote := 0
	writeUntil := func(i int) {
		w.WriteString(s[wrote:i])
		wrote = i
	}
	flush := func(i int) {
		writeUntil(i)
		if w.Len() > 0 {
			out = append(out, Plain(w.String()))
			w.Reset()
		}
	}
	for i := 0; i < len(s); {
		t := s[i:]
		if autoLink {
			if url, ok := autoURL(t); ok {
				flush(i)
				// Note: The old comment parser would look up the URL in words
				// and replace the target with words[URL] if it was non-empty.
				// That would allow creating links that display as one URL but
				// when clicked go to a different URL. Not sure what the point
				// of that is, so we're not doing that lookup here.
				out = append(out, &Link{Auto: true, Text: []Text{Plain(url)}, URL: url})
				i += len(url)
				wrote = i
				continue
			}
			if id, ok := ident(t); ok {
				url, italics := d.Words[id]
				if !italics {
					i += len(id)
					continue
				}
				flush(i)
				if url == "" {
					out = append(out, Italic(id))
				} else {
					out = append(out, &Link{Auto: true, Text: []Text{Italic(id)}, URL: url})
				}
				i += len(id)
				wrote = i
				continue
			}
		}
		switch {
		case strings.HasPrefix(t, "``"):
			if len(t) >= 3 && t[2] == '`' {
				// Do not convert `` inside ```, in case people are mistakenly writing Markdown.
				i += 3
				for i < len(t) && t[i] == '`' {
					i++
				}
				break
			}
			writeUntil(i)
			w.WriteRune('“')
			i += 2
			wrote = i
		case strings.HasPrefix(t, "''"):
			writeUntil(i)
			w.WriteRune('”')
			i += 2
			wrote = i
		default:
			i++
		}
	}
	flush(len(s))
	return out
}

// autoURL 检查是否 s begins with a URL that 应该是 hyperlinked.
// If so, it 返回 URL, which 是一个 prefix of s, and ok == true.
// Otherwise it returns "", false.
// The caller should skip over the first len(url) bytes of s
// before further processing.
func autoURL(s string) (url string, ok bool) {
	// Find the ://. Fast path to pick off non-URL,
	// since we call th是一个t every position in the string.
	// The shortest possible URL is ftp://x, 7 bytes.
	var i int
	switch {
	case len(s) < 7:
		return "", false
	case s[3] == ':':
		i = 3
	case s[4] == ':':
		i = 4
	case s[5] == ':':
		i = 5
	case s[6] == ':':
		i = 6
	default:
		return "", false
	}
	if i+3 > len(s) || s[i:i+3] != "://" {
		return "", false
	}

	// Check valid scheme.
	if !isScheme(s[:i]) {
		return "", false
	}

	// Scan host part. Must have at least one byte,
	// and must start and end in non-punctuation.
	i += 3
	if i >= len(s) || !isHost(s[i]) || isPunct(s[i]) {
		return "", false
	}
	i++
	end := i
	for i < len(s) && isHost(s[i]) {
		if !isPunct(s[i]) {
			end = i + 1
		}
		i++
	}
	i = end

	// At this point we are definitely returning a URL (scheme://host).
	// We just have to find the longest path we can add to it.
	// Heuristics abound.
	// We allow parens, braces, and brackets,
	// but only if they match (#5043, #22285).
	// We allow .,:;?! in the path but not at the end,
	// to avoid end-of-sentence punctuation (#18139, #16565).
	stk := []byte{}
	end = i
Path:
	for ; i < len(s); i++ {
		if isPunct(s[i]) {
			continue
		}
		if !isPath(s[i]) {
			break
		}
		switch s[i] {
		case '(':
			stk = append(stk, ')')
		case '{':
			stk = append(stk, '}')
		case '[':
			stk = append(stk, ']')
		case ')', '}', ']':
			if len(stk) == 0 || stk[len(stk)-1] != s[i] {
				break Path
			}
			stk = stk[:len(stk)-1]
		}
		if len(stk) == 0 {
			end = i + 1
		}
	}

	return s[:end], true
}

// isScheme 报告whether s 是一个 recognized URL scheme.
// Note that if strings of new length (beyond 3-7)
// are added here, the fast path at the top of autoURL will need updating.
func isScheme(s string) bool {
	switch s {
	case "file",
		"ftp",
		"gopher",
		"http",
		"https",
		"mailto",
		"nntp":
		return true
	}
	return false
}

// isHost 报告whether c 是一个 byte that can appear in a URL host,
// like www.example.com or user@[::1]:8080
func isHost(c byte) bool {
	// mask 是一个 128-bit bitmap with 1s for allowed bytes,
	// so that the byte c can be tested with a shift and an and.
	// If c > 128, then 1<<c and 1<<(c-64) will both be zero,
	// and this function 将返回 false.
	const mask = 0 |
		(1<<26-1)<<'A' |
		(1<<26-1)<<'a' |
		(1<<10-1)<<'0' |
		1<<'_' |
		1<<'@' |
		1<<'-' |
		1<<'.' |
		1<<'[' |
		1<<']' |
		1<<':'

	return ((uint64(1)<<c)&(mask&(1<<64-1)) |
		(uint64(1)<<(c-64))&(mask>>64)) != 0
}

// isPunct 报告whether c 是一个 punctuation byte that can appear
// inside a path but not at the end.
func isPunct(c byte) bool {
	// mask 是一个 128-bit bitmap with 1s for allowed bytes,
	// so that the byte c can be tested with a shift and an and.
	// If c > 128, then 1<<c and 1<<(c-64) will both be zero,
	// and this function 将返回 false.
	const mask = 0 |
		1<<'.' |
		1<<',' |
		1<<':' |
		1<<';' |
		1<<'?' |
		1<<'!'

	return ((uint64(1)<<c)&(mask&(1<<64-1)) |
		(uint64(1)<<(c-64))&(mask>>64)) != 0
}

// isPath 报告whether c 是一个 (non-punctuation) path byte.
func isPath(c byte) bool {
	// mask 是一个 128-bit bitmap with 1s for allowed bytes,
	// so that the byte c can be tested with a shift and an and.
	// If c > 128, then 1<<c and 1<<(c-64) will both be zero,
	// and this function 将返回 false.
	const mask = 0 |
		(1<<26-1)<<'A' |
		(1<<26-1)<<'a' |
		(1<<10-1)<<'0' |
		1<<'$' |
		1<<'\'' |
		1<<'(' |
		1<<')' |
		1<<'*' |
		1<<'+' |
		1<<'&' |
		1<<'#' |
		1<<'=' |
		1<<'@' |
		1<<'~' |
		1<<'_' |
		1<<'/' |
		1<<'-' |
		1<<'[' |
		1<<']' |
		1<<'{' |
		1<<'}' |
		1<<'%'

	return ((uint64(1)<<c)&(mask&(1<<64-1)) |
		(uint64(1)<<(c-64))&(mask>>64)) != 0
}

// isName 报告whether s 是一个 capitalized Go identifier (like Name).
func isName(s string) bool {
	t, ok := ident(s)
	if !ok || t != s {
		return false
	}
	r, _ := utf8.DecodeRuneInString(s)
	return unicode.IsUpper(r)
}

// ident 检查是否 s begins with a Go identifier.
// If so, it 返回 identifier, which 是一个 prefix of s, and ok == true.
// Otherwise it returns "", false.
// The caller should skip over the first len(id) bytes of s
// before further processing.
func ident(s string) (id string, ok bool) {
	// Scan [\pL_][\pL_0-9]*
	n := 0
	for n < len(s) {
		if c := s[n]; c < utf8.RuneSelf {
			if isIdentASCII(c) && (n > 0 || c < '0' || c > '9') {
				n++
				continue
			}
			break
		}
		r, nr := utf8.DecodeRuneInString(s[n:])
		if unicode.IsLetter(r) {
			n += nr
			continue
		}
		break
	}
	return s[:n], n > 0
}

// isIdentASCII 报告whether c 是一个n ASCII identifier byte.
func isIdentASCII(c byte) bool {
	// mask 是一个 128-bit bitmap with 1s for allowed bytes,
	// so that the byte c can be tested with a shift and an and.
	// If c > 128, then 1<<c and 1<<(c-64) will both be zero,
	// and this function 将返回 false.
	const mask = 0 |
		(1<<26-1)<<'A' |
		(1<<26-1)<<'a' |
		(1<<10-1)<<'0' |
		1<<'_'

	return ((uint64(1)<<c)&(mask&(1<<64-1)) |
		(uint64(1)<<(c-64))&(mask>>64)) != 0
}

// validImportPath 报告whether path 是一个 valid import path.
// It 是一个 lightly edited copy of golang.org/x/mod/module.CheckImportPath.
func validImportPath(path string) bool {
	if !utf8.ValidString(path) {
		return false
	}
	if path == "" {
		return false
	}
	if path[0] == '-' {
		return false
	}
	if strings.包含(path, "//") {
		return false
	}
	if path[len(path)-1] == '/' {
		return false
	}
	elemStart := 0
	for i, r := range path {
		if r == '/' {
			if !validImportPathElem(path[elemStart:i]) {
				return false
			}
			elemStart = i + 1
		}
	}
	return validImportPathElem(path[elemStart:])
}

func validImportPathElem(elem string) bool {
	if elem == "" || elem[0] == '.' || elem[len(elem)-1] == '.' {
		return false
	}
	for i := 0; i < len(elem); i++ {
		if !importPathOK(elem[i]) {
			return false
		}
	}
	return true
}

func importPathOK(c byte) bool {
	// mask 是一个 128-bit bitmap with 1s for allowed bytes,
	// so that the byte c can be tested with a shift and an and.
	// If c > 128, then 1<<c and 1<<(c-64) will both be zero,
	// and this function 将返回 false.
	const mask = 0 |
		(1<<26-1)<<'A' |
		(1<<26-1)<<'a' |
		(1<<10-1)<<'0' |
		1<<'-' |
		1<<'.' |
		1<<'~' |
		1<<'_' |
		1<<'+'

	return ((uint64(1)<<c)&(mask&(1<<64-1)) |
		(uint64(1)<<(c-64))&(mask>>64)) != 0
}
