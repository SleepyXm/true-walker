package returns

import (
	"log"
	"regexp"
	"strings"
	"tree-sit/test/types"
)

// signals are mechanisms that exit abnormally — they never produce a value.
var signals = map[string]bool{
	"throw": true, "raise": true, "panic": true,
	"die": true, "abort": true, "exit": true,
}

// structLiteralRe matches Go/Rust/TS struct literals: TypeName{ or pkg.Type{
var structLiteralRe = regexp.MustCompile(`^\w[\w.]*\s*\{`)

// Object-key vocabularies — checked against the lower-cased value string.
var (
	dataKeys = []string{
		"data", "result", "results", "payload",
		"items", "records", "content", "resource",
		"response", "resp", "entity", "dto",
	}
	msgKeys = []string{
		"message", "msg", "description", "text",
		"detail", "details", "info", "reason",
		"cause", "note", "title", "label", "summary",
	}
	errKeys = []string{
		"error", "err", "errors", "exception", "fault",
	}
)

func CompileRules(defs []types.ReturnRuleDef) []types.ReturnRule {
	var out []types.ReturnRule
	for _, d := range defs {
		re, err := regexp.Compile(d.Pattern)
		if err != nil {
			log.Printf("skipping return rule %q: %v", d.Name, err)
			continue
		}
		out = append(out, types.ReturnRule{
			Re:       re,
			Name:     d.Name,
			ValueIdx: re.SubexpIndex("value"),
			Language: d.Language,
		})
	}
	return out
}

func Extract(f types.SourceFile, rules []types.ReturnRule, fns []types.FunctionDef) []types.ReturnDef {
	var defs []types.ReturnDef
	lines := strings.Split(string(f.Content), "\n")

	for i, line := range lines {
		lineNum := i + 1
		trimmed := strings.TrimSpace(line)

		if (f.Ext == ".py" || f.Ext == ".rb") && strings.HasPrefix(trimmed, "#") {
			continue
		}
		if (f.Ext == ".go" || f.Ext == ".ts" || f.Ext == ".js" || f.Ext == ".tsx" || f.Ext == ".jsx") && strings.HasPrefix(trimmed, "//") {
			continue
		}

		for _, r := range rules {
			if r.Language != "" && r.Language != f.Ext {
				continue
			}
			m := r.Re.FindStringSubmatchIndex(line)
			if m == nil {
				continue
			}
			value := strings.TrimSpace(subgroup(line, m, r.ValueIdx))
			mech := mechanism(r.Name)
			defs = append(defs, types.ReturnDef{
				Mechanism: mech,
				Shape:     classify(mech, value),
				Value:     value,
				Line:      lineNum,
				Function:  containing(fns, lineNum),
			})
			break
		}
	}
	return defs
}

// Attach correlates a file's returns back onto their FunctionDefs.
// Call this after both Extract passes for the same file.
func Attach(fns []types.FunctionDef, rets []types.ReturnDef) []types.FunctionDef {
	for i, fn := range fns {
		for _, r := range rets {
			if r.Function == fn.Name {
				fns[i].Returns = append(fns[i].Returns, r)
			}
		}
	}
	return fns
}

// ── classification ────────────────────────────────────────────────────────────

func mechanism(name string) string {
	for _, sig := range []string{"throw", "raise", "panic", "yield", "propagate", "die", "abort", "exit", "break"} {
		if strings.Contains(name, sig) {
			return sig
		}
	}
	return "return"
}

func classify(mech, value string) string {
	if signals[mech] {
		return "signal"
	}
	return classifyValue(value)
}

func classifyValue(v string) string {
	v = strings.TrimSpace(v)

	// ── void ──────────────────────────────────────────────────────────────────
	switch v {
	case "", "nil", "None", "null", "undefined", "()", "void":
		return "void"
	}

	// ── multi-value: Go `return a, b` or Python tuple ────────────────────────
	parts := topLevelSplit(v)
	if len(parts) > 1 {
		return classifyMulti(parts)
	}

	// ── object / struct literal: inspect keys for semantics ──────────────────
	if looksLikeObject(v) {
		return classifyObjectKeys(v)
	}

	// ── single-value heuristics ───────────────────────────────────────────────
	if looksLikeError(v) {
		return "error"
	}
	if looksLikeCollection(v) {
		return "collection"
	}
	if looksLikeMessage(v) {
		return "message"
	}
	if looksLikeData(v) {
		return "data"
	}
	return "primitive"
}

// classifyMulti handles Go-style (result, err) and Python tuple returns.
func classifyMulti(parts []string) string {
	hasData, hasMsg, hasErr := false, false, false
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if isNilLike(p) {
			continue
		}
		switch {
		case looksLikeError(p):
			hasErr = true
		case looksLikeMessage(p):
			hasMsg = true
		case looksLikeObject(p) || looksLikeCollection(p) || looksLikeData(p):
			hasData = true
		}
	}
	return compositeShape(hasData, hasMsg, hasErr)
}

// classifyObjectKeys inspects object/map literal content for semantic keys.
func classifyObjectKeys(v string) string {
	lower := strings.ToLower(v)
	hasData := containsAnyKey(lower, dataKeys)
	hasMsg := containsAnyKey(lower, msgKeys)
	hasErr := containsAnyKey(lower, errKeys)
	if !hasData && !hasMsg && !hasErr {
		return "object" // generic — no recognised semantic keys
	}
	return compositeShape(hasData, hasMsg, hasErr)
}

func compositeShape(hasData, hasMsg, hasErr bool) string {
	switch {
	case hasData && hasMsg && hasErr:
		return "data+message+error"
	case hasData && hasErr:
		return "data+error"
	case hasData && hasMsg:
		return "data+message"
	case hasMsg && hasErr:
		return "message+error"
	case hasData:
		return "data"
	case hasMsg:
		return "message"
	case hasErr:
		return "error"
	default:
		return "object"
	}
}

// containsAnyKey checks v for key appearances in object-key position.
// Handles: "key", 'key', key:, key :
func containsAnyKey(lower string, keys []string) bool {
	for _, k := range keys {
		if strings.Contains(lower, `"`+k+`"`) ||
			strings.Contains(lower, `'`+k+`'`) ||
			strings.Contains(lower, k+":") ||
			strings.Contains(lower, k+" :") {
			return true
		}
	}
	return false
}

// ── value-level heuristics ────────────────────────────────────────────────────

func isNilLike(v string) bool {
	switch strings.TrimSpace(v) {
	case "nil", "null", "None", "undefined", "()":
		return true
	}
	return false
}

func looksLikeObject(v string) bool {
	return strings.HasPrefix(v, "{") || // JS/TS literal
		strings.HasPrefix(v, "&") || // Go pointer-to-struct
		strings.HasPrefix(v, "map[") || // Go map
		structLiteralRe.MatchString(v) // TypeName{ / pkg.Type{
}

func looksLikeCollection(v string) bool {
	return strings.HasPrefix(v, "[]") ||
		strings.HasPrefix(v, "[") ||
		strings.HasPrefix(v, "Array(") ||
		strings.Contains(v, "append(") ||
		strings.Contains(v, "make([]") ||
		strings.Contains(v, ".collect()") ||
		strings.Contains(v, ".toList()") ||
		strings.Contains(v, ".toArray(")
}

func looksLikeError(v string) bool {
	lower := strings.ToLower(v)
	if lower == "err" || lower == "error" {
		return true
	}
	// CamelCase convention: errFoo, ErrNotFound, parseError, ValidationErr
	if strings.HasPrefix(lower, "err") ||
		strings.HasSuffix(lower, "error") ||
		strings.HasSuffix(lower, "err") {
		return true
	}
	// Construction calls
	return strings.Contains(v, "errors.New(") ||
		strings.Contains(v, "fmt.Errorf(") ||
		strings.Contains(v, "new Error(") ||
		strings.Contains(v, "new TypeError(") ||
		strings.Contains(v, "new ValueError(") ||
		strings.Contains(v, "Exception(")
}

func looksLikeMessage(v string) bool {
	// String literals
	if strings.HasPrefix(v, `"`) || strings.HasPrefix(v, `'`) || strings.HasPrefix(v, "`") {
		return true
	}
	// String-building calls
	if strings.Contains(v, "fmt.Sprintf(") ||
		strings.Contains(v, "strings.Join(") ||
		strings.Contains(v, "strconv.") ||
		strings.Contains(v, "String.format(") ||
		strings.Contains(v, "sprintf(") ||
		strings.HasPrefix(v, `f"`) || strings.HasPrefix(v, `f'`) { // Python f-string
		return true
	}
	// Variable name conventions
	lower := strings.ToLower(v)
	for _, k := range msgKeys {
		if lower == k || strings.HasSuffix(lower, k) || strings.HasPrefix(lower, k) {
			return true
		}
	}
	return false
}

func looksLikeData(v string) bool {
	lower := strings.ToLower(v)
	for _, k := range dataKeys {
		if lower == k || strings.HasSuffix(lower, k) || strings.HasPrefix(lower, k) {
			return true
		}
	}
	return false
}

// ── utilities ─────────────────────────────────────────────────────────────────

// topLevelSplit splits on commas at brace/bracket depth 0.
func topLevelSplit(s string) []string {
	var parts []string
	depth, start := 0, 0
	for i, ch := range s {
		switch ch {
		case '(', '[', '{', '<':
			depth++
		case ')', ']', '}', '>':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				parts = append(parts, strings.TrimSpace(s[start:i]))
				start = i + 1
			}
		}
	}
	if last := strings.TrimSpace(s[start:]); last != "" {
		parts = append(parts, last)
	}
	return parts
}

func containing(fns []types.FunctionDef, line int) string {
	name := ""
	for _, d := range fns {
		if d.StartLine > line {
			break
		}
		name = d.Name
	}
	return name
}

func subgroup(s string, m []int, idx int) string {
	if idx <= 0 {
		return ""
	}
	i := idx * 2
	if i+1 >= len(m) || m[i] < 0 {
		return ""
	}
	return s[m[i]:m[i+1]]
}
