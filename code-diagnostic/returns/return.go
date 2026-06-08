package returns

import (
	"log"
	"regexp"
	"sort"
	"strings"
	"tree-sit/test/syntax"
	"tree-sit/test/types"
)

// signals are mechanisms that cause abnormal function exit — they never
// produce a value for the caller. Any rule whose name contains one of these
// tokens is classified as "signal" regardless of the value expression that
// follows, because the function never returns normally.
var signals = map[string]bool{
	"throw": true, "raise": true, "panic": true,
	"die": true, "abort": true, "exit": true,
}

// structLiteralRe matches Go/Rust/TS struct literals of the form TypeName{
// or pkg.Type{ — catches object returns that start with a type name rather
// than a bare {.
var structLiteralRe = regexp.MustCompile(`^\w[\w.]*\s*\{`)

// dataKeys are object-key names and variable-name fragments that indicate
// a structured data payload. When these appear as keys inside an object
// literal, or as a variable name being returned, the shape carries "data".
var dataKeys = []string{
	"data", "result", "results", "payload",
	"items", "records", "content", "resource",
	"response", "resp", "entity", "dto",
}

// msgKeys are object-key names and variable-name fragments that indicate
// a human-readable text value — API response messages, descriptions,
// labels, reasons, etc. Also used to detect message-named variables
// being returned directly.
var msgKeys = []string{
	"message", "msg", "description", "text",
	"detail", "details", "info", "reason",
	"cause", "note", "title", "label", "summary",
}

// errKeys are object-key names indicating an error field is present inside
// a returned object — distinct from a function that returns an error value
// directly. Used only for object key inspection in classifyObjectKeys.
var errKeys = []string{
	"error", "err", "errors", "exception", "fault",
}

// boolLiterals covers the boolean literal spellings across all supported
// languages. Go/JS/TS/Rust use lowercase, Python uses titlecase.
var boolLiterals = map[string]bool{
	"true": true, "false": true,
	"True": true, "False": true,
}

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

// helper to get a default LangSyntax based on file extension
func langSyntaxForExt(ext string) syntax.LangSyntax {
	switch ext {
	case ".py", ".rb":
		return syntax.LangSyntax{LineComment: "#", BlockStyle: syntax.IndentBlock}
	case ".go":
		return syntax.LangSyntax{LineComment: "//"}
	case ".js", ".ts", ".tsx", ".jsx":
		return syntax.LangSyntax{LineComment: "//", AngleDepth: true}
	default:
		return syntax.LangSyntax{}
	}
}

func Extract(f types.SourceFile, rules []types.ReturnRule, fns []types.FunctionDef) []types.ReturnDef {
	var defs []types.ReturnDef
	syn := langSyntaxForExt(f.Ext)

	sc := syntax.NewScanner(f.Content)
	for sc.Scan() {
		lineNum := sc.Line
		trimmed := strings.TrimSpace(sc.Text())

		// skip comment lines — they can contain return-like syntax in docs
		if syntax.IsComment(trimmed, syn) {
			continue
		}

		for _, r := range rules {
			if r.Language != "" && r.Language != f.Ext {
				continue
			}
			m := r.Re.FindStringSubmatchIndex(trimmed)
			if m == nil {
				continue
			}
			value := strings.TrimSpace(subgroup(trimmed, m, r.ValueIdx))
			mech := mechanism(r.Name)
			defs = append(defs, types.ReturnDef{
				Mechanism: mech,
				Shape:     classify(mech, value),
				Value:     value,
				Line:      lineNum,
				Function:  containing(fns, lineNum),
			})
			// first matching rule wins — language-specific rules must be
			// listed before generics in the yaml to take priority
			break
		}
	}
	return defs
}

// Attach correlates a file's extracted returns back onto their FunctionDefs.
// Call after Extract for the same file.
// Functions with zero matched return statements receive a synthetic void
// entry — async/event functions that never explicitly return still have a
// complete shape.
func Attach(fns []types.FunctionDef, rets []types.ReturnDef) []types.FunctionDef {
	for i, fn := range fns {
		for _, r := range rets {
			if r.Function == fn.Name {
				fns[i].Returns = append(fns[i].Returns, r)
			}
		}
		if len(fns[i].Returns) == 0 {
			fns[i].Returns = []types.ReturnDef{{
				Mechanism: "return",
				Shape:     "void",
			}}
		}
	}
	return fns
}

// ── classification ────────────────────────────────────────────────────────────

// mechanism normalises a rule name to a canonical exit keyword.
// Rule names are prefixed (py-raise, go-return-single) so we scan for the
// base token rather than exact-matching.
func mechanism(name string) string {
	for _, sig := range []string{
		"throw", "raise", "panic", "yield",
		"propagate", "die", "abort", "exit",
	} {
		if strings.Contains(name, sig) {
			return sig
		}
	}
	return "return"
}

// classify assigns a shape to a return site.
// Signals short-circuit — value is irrelevant when the function exits
// abnormally via throw/raise/panic/die/abort/exit.
func classify(mech, value string) string {
	if signals[mech] {
		return "signal"
	}
	return classifyValue(value)
}

// classifyValue determines the atomic or composite shape of a raw return
// value expression. Order of checks is intentional:
//  1. void   — catch nil/None/null/empty before anything else
//  2. multi  — split Go tuples / Python tuples, classify each part
//  3. object — inspect struct/map literals for semantic keys
//  4. single-value heuristics in priority order
func classifyValue(v string) string {
	v = strings.TrimSpace(v)

	// void — null/none/empty across all languages
	switch v {
	case "", "nil", "None", "null", "undefined", "()", "void":
		return "void"
	}

	// multi-value: Go `return result, err` or Python `return a, b`
	parts := topLevelSplit(v)
	if len(parts) > 1 {
		return classifyMulti(parts)
	}

	// object/struct literal — key inspection determines semantic content
	if looksLikeObject(v) {
		return classifyObjectKeys(v)
	}

	// single-value heuristics — order matters, error before bool before message
	if looksLikeError(v) {
		return "error"
	}
	if looksLikeBool(v) {
		return "bool"
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

// classifyMulti classifies multi-value returns by detecting the atomic shape
// of each part and composing them.
// Nil-like parts are skipped — `return result, nil` reads as "data" not
// "data+void", since nil is the absence of an error, not a meaningful value.
func classifyMulti(parts []string) string {
	var detected []string
	seen := make(map[string]bool)

	add := func(s string) {
		if !seen[s] {
			seen[s] = true
			detected = append(detected, s)
		}
	}

	for _, p := range parts {
		p = strings.TrimSpace(p)
		if isNilLike(p) {
			continue
		}
		switch {
		case looksLikeError(p):
			add("error")
		case looksLikeBool(p):
			add("bool")
		case looksLikeMessage(p):
			add("message")
		case looksLikeCollection(p):
			add("collection")
		case looksLikeObject(p):
			add("object")
		case looksLikeData(p):
			add("data")
		default:
			add("primitive")
		}
	}
	return compositeShape(detected)
}

// classifyObjectKeys inspects an object literal's content for semantic keys
// from the data/message/error vocabularies.
// Combinations are emergent — data+message, data+error, message+error, etc.
// are all valid and never hardcoded. If no recognised keys are found the
// shape is "object" — structured but semantically opaque.
func classifyObjectKeys(v string) string {
	lower := strings.ToLower(v)
	var detected []string
	seen := make(map[string]bool)

	add := func(s string) {
		if !seen[s] {
			seen[s] = true
			detected = append(detected, s)
		}
	}

	if containsAnyKey(lower, dataKeys) {
		add("data")
	}
	if containsAnyKey(lower, msgKeys) {
		add("message")
	}
	if containsAnyKey(lower, errKeys) {
		add("error")
	}
	if len(detected) == 0 {
		return "object"
	}
	return compositeShape(detected)
}

// compositeShape joins atomic shape labels into a composite string.
// Sorted so that data+message and message+data produce identical output.
// Combinations are never defined in advance — they emerge from detection.
func compositeShape(detected []string) string {
	if len(detected) == 0 {
		return "object"
	}
	sort.Strings(detected)
	return strings.Join(detected, "+")
}

// containsAnyKey checks v for key appearances in object-key position.
// Handles all common styles: "key", 'key', key:, key :
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

// isNilLike returns true for the null/none/empty representations across
// languages. Used to skip placeholder parts in multi-value returns so that
// `return result, nil` reads as "data" rather than "data+void".
func isNilLike(v string) bool {
	switch strings.TrimSpace(v) {
	case "nil", "null", "None", "undefined", "()":
		return true
	}
	return false
}

// looksLikeObject returns true for any struct/map/object literal.
// Semantic inspection of keys is done separately in classifyObjectKeys.
func looksLikeObject(v string) bool {
	return strings.HasPrefix(v, "{") || // JS/TS object literal
		strings.HasPrefix(v, "&") || // Go pointer-to-struct: &Type{...}
		strings.HasPrefix(v, "map[") || // Go map literal
		structLiteralRe.MatchString(v) // TypeName{ or pkg.Type{
}

// looksLikeCollection returns true for array/slice/list expressions.
func looksLikeCollection(v string) bool {
	return strings.HasPrefix(v, "[]") || // Go typed slice
		strings.HasPrefix(v, "[") || // array literal
		strings.HasPrefix(v, "Array(") ||
		strings.Contains(v, "append(") ||
		strings.Contains(v, "make([]") ||
		strings.Contains(v, ".collect()") || // Rust iterator
		strings.Contains(v, ".toList()") || // Kotlin/Java
		strings.Contains(v, ".toArray(")
}

// looksLikeError returns true for error values by naming convention and
// constructor call patterns. Covers:
//   - Go: err, ErrFoo, errors.New(...), fmt.Errorf(...)
//   - Python: Exception(...), ValueError(...)
//   - JS/TS: new Error(...), new TypeError(...)
func looksLikeError(v string) bool {
	lower := strings.ToLower(v)
	if lower == "err" || lower == "error" {
		return true
	}
	if strings.HasPrefix(lower, "err") ||
		strings.HasSuffix(lower, "error") ||
		strings.HasSuffix(lower, "err") {
		return true
	}
	return strings.Contains(v, "errors.New(") ||
		strings.Contains(v, "fmt.Errorf(") ||
		strings.Contains(v, "new Error(") ||
		strings.Contains(v, "new TypeError(") ||
		strings.Contains(v, "new ValueError(") ||
		strings.Contains(v, "Exception(")
}

// looksLikeBool returns true for boolean literals and boolean-named
// variables. Checked before message and data — status flags like isValid,
// hasAccess, ok are common return values and should not be misclassified.
func looksLikeBool(v string) bool {
	if boolLiterals[v] {
		return true
	}
	lower := strings.ToLower(v)
	for _, prefix := range []string{
		"is", "has", "can", "should",
		"ok", "found", "exists", "valid", "enabled", "active",
	} {
		if lower == prefix || strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

// looksLikeMessage returns true for string literals, string-building calls,
// and variables named after common message/text conventions from msgKeys.
func looksLikeMessage(v string) bool {
	// string literals
	if strings.HasPrefix(v, `"`) || strings.HasPrefix(v, `'`) || strings.HasPrefix(v, "`") {
		return true
	}
	// Python f-strings
	if strings.HasPrefix(v, `f"`) || strings.HasPrefix(v, `f'`) {
		return true
	}
	// string-building calls
	if strings.Contains(v, "fmt.Sprintf(") ||
		strings.Contains(v, "strings.Join(") ||
		strings.Contains(v, "strconv.") ||
		strings.Contains(v, "String.format(") ||
		strings.Contains(v, "sprintf(") {
		return true
	}
	// variable name conventions
	lower := strings.ToLower(v)
	for _, k := range msgKeys {
		if lower == k || strings.HasSuffix(lower, k) || strings.HasPrefix(lower, k) {
			return true
		}
	}
	return false
}

// looksLikeData returns true for variables named after common data payload
// conventions from dataKeys. The broadest positive heuristic — checked last
// so error/bool/message/collection all get priority over it.
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
// Prevents splitting inside nested function calls, struct literals, or
// generic type parameters.
func topLevelSplit(s string) []string {
	// Uses the shared depth tracker with an empty struct for standard bracket rules
	return syntax.SplitDepth(s, ',', syntax.LangSyntax{})
}

// containing returns the name of the innermost function that contains the
// given line number. Relies on fns being sorted by StartLine ascending,
// which functions.Extract guarantees.
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
