// Package syntax provides language-agnostic structural utilities for source
// analysis. It knows nothing about what you are looking for — only about the
// structural rules that govern how source text is laid out.
//
// Every function here operates on text. Language-specific behaviour is
// expressed entirely through LangSyntax values, which are data, not code.
package syntax

import (
	"bufio"
	"bytes"
	"strings"
)

// ── Language Description ──────────────────────────────────────────────────────

// BlockStyle describes how a language delimits blocks of code.
type BlockStyle uint8

const (
	BraceBlock   BlockStyle = iota // { } delimited — Go, Rust, Java, C, C++, JS/TS
	IndentBlock                    // indentation-delimited — Python
	KeywordBlock                   // open/close keywords — Ruby (def/end, do/end)
)

// RawString describes one raw/verbatim string form for a language.
// In raw strings escape sequences are not processed; the literal content
// extends until the exact closing token is found.
//
// Examples:
//
//	Go backtick:    Prefix="",   Open="`",  Close="`"
//	Python r-string: Prefix="r", Open=`"`,  Close=`"`
//	Rust r#:        Prefix="r#", Open=`"`,  Close=`"#`
//	C++ raw:        Prefix=`R"(`, Open="",  Close=`)"`
type RawString struct {
	Prefix string
	Open   string
	Close  string
}

// LangSyntax describes the structural characteristics of one language.
// It contains no patterns, no semantics — only the delimiters and rules
// needed for masking, joining, and block detection.
type LangSyntax struct {
	// Comment tokens.
	LineComment  string    // e.g. "//" or "#" or "--"
	BlockComment [2]string // open/close, e.g. ["/*", "*/"] — zero means none
	DocComment   string    // distinct doc prefix, e.g. "///" — treated as line comment

	// String tokens — checked in this order: RawStrings, Multiline, Strings.
	// Order matters: a triple-quote must be matched before a single quote.
	RawStrings []RawString // verbatim string forms
	Multiline  []string    // multi-line string delimiters, e.g. [`"""`, `'''`]
	Strings    []string    // single-line string delimiters, e.g. [`"`, `'`]

	// Structural tokens.
	Continuation byte       // explicit line-continuation char, e.g. '\\'; 0 if none
	BlockStyle   BlockStyle // how blocks are delimited
	BlockKeyword [2]string  // open/close keywords for KeywordBlock, e.g. ["do", "end"]
	AngleDepth   bool       // count <> as nesting (Java, C++, TS generics)

	// Decorator/annotation prefixes — lines whose trimmed text starts with
	// one of these immediately precede and belong to the next definition.
	//
	// Examples: "@" (Python, Java, TS), "#[" (Rust), "[Http" (C# attributes)
	DecoratorPrefixes []string
}

// ── Masking ───────────────────────────────────────────────────────────────────

// Mask returns a copy of src where the content of all string literals and
// comments is replaced with space characters. Newlines are preserved so line
// numbers remain valid. Delimiters are also blanked.
//
// The result is safe to run regex rules against: patterns will never match
// content that was inside a string or comment in the original source.
func Mask(src []byte, s LangSyntax) []byte {
	out := make([]byte, len(src))
	copy(out, src)
	i := 0
	for i < len(out) {
		// ── block comment ─────────────────────────────────────────────────
		if s.BlockComment[0] != "" && hasPrefix(out, i, s.BlockComment[0]) {
			open := len(s.BlockComment[0])
			end := indexFrom(out, i+open, s.BlockComment[1])
			if end < 0 {
				blankRange(out, i, len(out))
				break
			}
			closeEnd := end + len(s.BlockComment[1])
			blankRange(out, i, closeEnd)
			i = closeEnd
			continue
		}

		// ── raw strings ───────────────────────────────────────────────────
		if matched, next := maskRaw(out, i, s.RawStrings); matched {
			i = next
			continue
		}

		// ── multi-line strings ────────────────────────────────────────────
		if matched, next := maskDelimited(out, i, s.Multiline, true); matched {
			i = next
			continue
		}

		// ── single-line strings ───────────────────────────────────────────
		if matched, next := maskSingleLine(out, i, s.Strings); matched {
			i = next
			continue
		}

		// ── line / doc comment ────────────────────────────────────────────
		if isLineCommentStart(out, i, s) {
			j := i
			for j < len(out) && out[j] != '\n' {
				j++
			}
			blankRange(out, i, j)
			i = j
			continue
		}

		i++
	}
	return out
}

func maskRaw(out []byte, i int, raws []RawString) (bool, int) {
	for _, rs := range raws {
		full := rs.Prefix + rs.Open
		if !hasPrefix(out, i, full) {
			continue
		}
		start := i + len(full)
		end := indexFrom(out, start, rs.Close)
		if end < 0 {
			blankRange(out, i, len(out))
			return true, len(out)
		}
		closeEnd := end + len(rs.Close)
		blankRange(out, i, closeEnd)
		return true, closeEnd
	}
	return false, i
}

func maskDelimited(out []byte, i int, delims []string, crossLine bool) (bool, int) {
	for _, delim := range delims {
		if !hasPrefix(out, i, delim) {
			continue
		}
		start := i + len(delim)
		end := indexFrom(out, start, delim)
		if end < 0 {
			blankRange(out, i, len(out))
			return true, len(out)
		}
		closeEnd := end + len(delim)
		blankRange(out, i, closeEnd)
		return true, closeEnd
	}
	return false, i
}

func maskSingleLine(out []byte, i int, delims []string) (bool, int) {
	for _, delim := range delims {
		if !hasPrefix(out, i, delim) {
			continue
		}
		j := i + len(delim)
		for j < len(out) && out[j] != '\n' {
			if out[j] == '\\' { // escape sequence — skip two chars
				out[j] = ' '
				j++
				if j < len(out) && out[j] != '\n' {
					out[j] = ' '
					j++
				}
				continue
			}
			if hasPrefix(out, j, delim) {
				blankRange(out, i, j+len(delim))
				return true, j + len(delim)
			}
			j++
		}
		// unterminated — blank to end of line
		blankRange(out, i, j)
		return true, j
	}
	return false, i
}

func isLineCommentStart(out []byte, i int, s LangSyntax) bool {
	return (s.LineComment != "" && hasPrefix(out, i, s.LineComment)) ||
		(s.DocComment != "" && hasPrefix(out, i, s.DocComment))
}

// blankRange replaces out[lo:hi] with spaces, preserving newlines.
func blankRange(out []byte, lo, hi int) {
	for k := lo; k < hi && k < len(out); k++ {
		if out[k] != '\n' {
			out[k] = ' '
		}
	}
}

func hasPrefix(b []byte, i int, s string) bool {
	return i+len(s) <= len(b) && string(b[i:i+len(s)]) == s
}

func indexFrom(b []byte, from int, needle string) int {
	if from >= len(b) {
		return -1
	}
	idx := bytes.Index(b[from:], []byte(needle))
	if idx < 0 {
		return -1
	}
	return from + idx
}

// ── Line Joining ──────────────────────────────────────────────────────────────

// JoinContinuations merges lines ending with s.Continuation into a single
// logical line by replacing the continuation character and following newline
// with a space. No-op when s.Continuation == 0.
func JoinContinuations(src []byte, s LangSyntax) []byte {
	if s.Continuation == 0 {
		return src
	}
	cont := s.Continuation
	var buf bytes.Buffer
	for i := 0; i < len(src); i++ {
		b := src[i]
		if b == cont && i+1 < len(src) && src[i+1] == '\n' {
			buf.WriteByte(' ')
			i++ // consume the newline
			continue
		}
		buf.WriteByte(b)
	}
	return buf.Bytes()
}

// ── Depth Tracking ────────────────────────────────────────────────────────────

// DepthAt returns the bracket nesting depth at byte offset pos in line.
// Tracks (, [, { always. Tracks <, > when s.AngleDepth is true.
// Call on masked content — this function does not understand strings or comments.
func DepthAt(line string, pos int, s LangSyntax) int {
	d := 0
	for i, ch := range line {
		if i >= pos {
			break
		}
		d += depthDelta(ch, s)
	}
	return d
}

func depthDelta(ch rune, s LangSyntax) int {
	switch ch {
	case '(', '[', '{':
		return 1
	case ')', ']', '}':
		return -1
	case '<':
		if s.AngleDepth {
			return 1
		}
	case '>':
		if s.AngleDepth {
			return -1
		}
	}
	return 0
}

// SplitDepth splits s on sep only at nesting depth 0, respecting all bracket
// pairs. Replaces splitComma, topLevelSplit, and the ad-hoc comma-splitters
// scattered across functions.go, returns.go, imports.go, and classes.go.
func SplitDepth(s string, sep rune, syn LangSyntax) []string {
	var parts []string
	depth, start := 0, 0
	for i, ch := range s {
		if ch == sep && depth == 0 {
			parts = append(parts, strings.TrimSpace(s[start:i]))
			start = i + 1
			continue
		}
		d := depthDelta(ch, syn)
		depth += d
		if depth < 0 {
			depth = 0
		}
	}
	if last := strings.TrimSpace(s[start:]); last != "" {
		parts = append(parts, last)
	}
	return parts
}

// FirstAt returns the index of the first occurrence of ch at depth 0 in s.
// Returns -1 if not found. Replaces firstColon in functions.go.
func FirstAt(s string, ch rune, syn LangSyntax) int {
	depth := 0
	for i, c := range s {
		if c == ch && depth == 0 {
			return i
		}
		d := depthDelta(c, syn)
		depth += d
		if depth < 0 {
			depth = 0
		}
	}
	return -1
}

// StripAfter removes everything from the first occurrence of ch at depth 0.
// Replaces stripDefault in functions.go.
func StripAfter(s string, ch rune, syn LangSyntax) string {
	if i := FirstAt(s, ch, syn); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}

// ── Block Collection ──────────────────────────────────────────────────────────

// CollectBlock dispatches to the appropriate block collector based on
// s.BlockStyle. Returns the body content and the line index immediately
// after the block closes.
func CollectBlock(lines []string, startIdx int, s LangSyntax) (string, int) {
	switch s.BlockStyle {
	case IndentBlock:
		return CollectIndentBlock(lines, startIdx)
	case KeywordBlock:
		return CollectKeywordBlock(lines, startIdx, s.BlockKeyword)
	default:
		return CollectBraceBlock(lines, startIdx)
	}
}

// CollectBraceBlock collects the body of a { } delimited block.
func CollectBraceBlock(lines []string, startIdx int) (string, int) {
	var buf strings.Builder
	depth, opened := 0, false
	for i := startIdx; i < len(lines); i++ {
		for _, ch := range lines[i] {
			switch ch {
			case '{':
				depth++
				opened = true
			case '}':
				depth--
				if opened && depth == 0 {
					return strings.TrimSpace(buf.String()), i + 1
				}
			default:
				if opened && depth > 0 {
					buf.WriteRune(ch)
				}
			}
		}
		if opened && depth > 0 {
			buf.WriteByte('\n')
		}
	}
	return strings.TrimSpace(buf.String()), startIdx + 1
}

// CollectIndentBlock collects an indentation-delimited block (Python).
func CollectIndentBlock(lines []string, startIdx int) (string, int) {
	base := leadingWS(lines[startIdx])
	var buf strings.Builder
	for i := startIdx + 1; i < len(lines); i++ {
		line := lines[i]
		if strings.TrimSpace(line) == "" {
			continue
		}
		if leadingWS(line) <= base {
			return strings.TrimSpace(buf.String()), i
		}
		buf.WriteString(line)
		buf.WriteByte('\n')
	}
	return strings.TrimSpace(buf.String()), len(lines)
}

// CollectKeywordBlock collects a keyword-delimited block (Ruby: def/end, do/end).
// Nesting is tracked so inner def/end pairs are included in the body.
func CollectKeywordBlock(lines []string, startIdx int, kw [2]string) (string, int) {
	open, close := kw[0], kw[1]
	var buf strings.Builder
	depth := 0
	for i := startIdx; i < len(lines); i++ {
		t := strings.TrimSpace(lines[i])
		if strings.HasPrefix(t, open) {
			depth++
			if depth > 1 {
				buf.WriteString(lines[i])
				buf.WriteByte('\n')
			}
			continue
		}
		if t == close || strings.HasPrefix(t, close+" ") {
			depth--
			if depth == 0 {
				return strings.TrimSpace(buf.String()), i + 1
			}
		}
		if depth > 0 {
			buf.WriteString(lines[i])
			buf.WriteByte('\n')
		}
	}
	return strings.TrimSpace(buf.String()), startIdx + 1
}

// CollectUntilClose collects content from startIdx until the open/close byte
// pair returns to depth 0. Used for multi-line function parameters, import
// blocks, annotation arguments, and anywhere a matched delimiter spans lines.
// Replaces collectParams in functions.go.
func CollectUntilClose(lines []string, startIdx int, open, close byte) (string, int) {
	var buf strings.Builder
	depth := 0
	for i := startIdx; i < len(lines); i++ {
		for _, ch := range lines[i] {
			switch byte(ch) {
			case open:
				depth++
				if depth > 1 {
					buf.WriteRune(ch)
				}
			case close:
				depth--
				if depth == 0 {
					return strings.TrimSpace(buf.String()), i + 1
				}
				buf.WriteRune(ch)
			default:
				if depth > 0 {
					buf.WriteRune(ch)
				}
			}
		}
		if depth > 0 {
			buf.WriteByte(' ')
		}
	}
	return strings.TrimSpace(buf.String()), startIdx + 1
}

// ── Decorator / Annotation Lookbehind ────────────────────────────────────────

// LookBehind returns the decorator/annotation lines immediately preceding
// the definition at lineIdx (0-indexed slice index, not line number).
// Lines are returned in source order. A line qualifies as a decorator if its
// trimmed form starts with one of s.DecoratorPrefixes. Blank lines break
// the chain.
func LookBehind(lines []string, lineIdx int, s LangSyntax) []string {
	if len(s.DecoratorPrefixes) == 0 || lineIdx == 0 {
		return nil
	}
	var out []string
	for i := lineIdx - 1; i >= 0; i-- {
		t := strings.TrimSpace(lines[i])
		if t == "" {
			break
		}
		if !hasDecoratorPrefix(t, s.DecoratorPrefixes) {
			break
		}
		out = append([]string{lines[i]}, out...)
	}
	return out
}

func hasDecoratorPrefix(line string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(line, p) {
			return true
		}
	}
	return false
}

// ── Line Utilities ────────────────────────────────────────────────────────────

// IsComment returns true if line (trimmed) is entirely a comment.
func IsComment(line string, s LangSyntax) bool {
	t := strings.TrimSpace(line)
	return (s.LineComment != "" && strings.HasPrefix(t, s.LineComment)) ||
		(s.DocComment != "" && strings.HasPrefix(t, s.DocComment)) ||
		(s.BlockComment[0] != "" && strings.HasPrefix(t, s.BlockComment[0]))
}

// IsBlank returns true for lines that are empty or only whitespace.
func IsBlank(line string) bool { return strings.TrimSpace(line) == "" }

func leadingWS(line string) int {
	return len(line) - len(strings.TrimLeft(line, " \t"))
}

// ── Line-counted Scanner ──────────────────────────────────────────────────────

// Scanner wraps bufio.Scanner with a 1-based line counter.
// Replaces the repeated bufio.NewScanner + manual lineNum++ pattern
// across functions.go, imports.go, classes.go, returns.go, and routes.go.
type Scanner struct {
	*bufio.Scanner
	Line int
}

func NewScanner(src []byte) *Scanner {
	return &Scanner{Scanner: bufio.NewScanner(bytes.NewReader(src))}
}

func (s *Scanner) Scan() bool {
	if ok := s.Scanner.Scan(); ok {
		s.Line++
		return true
	}
	return false
}
