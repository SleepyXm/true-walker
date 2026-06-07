package functions

import (
	"log"
	"regexp"
	"sort"
	"strings"
	"tree-sit/test/types"
)

func CompileRules(defs []types.FunctionRuleDef) []types.FunctionRule {
	var out []types.FunctionRule
	for _, d := range defs {
		re, err := regexp.Compile(d.Pattern)
		if err != nil {
			log.Printf("skipping function rule %q: %v", d.Name, err)
			continue
		}
		out = append(out, types.FunctionRule{
			Re:       re,
			NameIdx:  re.SubexpIndex("function"),
			Language: d.Language,
		})
	}
	return out
}

func Extract(f types.SourceFile, rules []types.FunctionRule) []types.FunctionDef {
	var defs []types.FunctionDef
	seen := make(map[int]bool)
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
			if m == nil || seen[lineNum] {
				continue
			}
			name := subgroup(line, m, r.NameIdx)
			if name == "" {
				continue
			}
			rawParams, endLine := collectParams(lines, i)
			defs = append(defs, types.FunctionDef{
				Name:      name,
				StartLine: lineNum,
				EndLine:   endLine,
				Params:    ParseParams(rawParams, f.Ext),
				RawParams: rawParams,
			})
			seen[lineNum] = true
		}
	}

	sort.Slice(defs, func(i, j int) bool { return defs[i].StartLine < defs[j].StartLine })
	return defs
}

func Containing(defs []types.FunctionDef, line int) string {
	name := ""
	for _, d := range defs {
		if d.StartLine > line {
			break
		}
		name = d.Name
	}
	return name
}

// collectParams walks lines from startIdx, tracks paren depth,
// returns the raw content inside the first (...) and the line it closed on.
func collectParams(lines []string, startIdx int) (string, int) {
	var buf strings.Builder
	depth := 0

	for i := startIdx; i < len(lines); i++ {
		for _, ch := range lines[i] {
			switch ch {
			case '(':
				depth++
				if depth > 1 {
					buf.WriteRune(ch)
				}
			case ')':
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
			buf.WriteRune(' ')
		}
	}
	return strings.TrimSpace(buf.String()), startIdx + 1
}

func ParseParams(raw, ext string) []types.Param {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	switch ext {
	case ".py":
		return parsePython(raw)
	case ".go":
		return parseGo(raw)
	case ".ts", ".tsx", ".js", ".jsx":
		return parseTS(raw)
	}
	return []types.Param{{Raw: raw}}
}

// splitComma splits on commas at depth 0, respecting () [] {} <>
func splitComma(s string) []string {
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

// stripDefault removes everything from the first = at depth 0
func stripDefault(s string) string {
	depth := 0
	for i, ch := range s {
		switch ch {
		case '(', '[', '{', '<':
			depth++
		case ')', ']', '}', '>':
			if depth > 0 {
				depth--
			}
		case '=':
			if depth == 0 {
				return strings.TrimSpace(s[:i])
			}
		}
	}
	return s
}

func firstColon(s string) int {
	depth := 0
	for i, ch := range s {
		switch ch {
		case '(', '[', '{', '<':
			depth++
		case ')', ']', '}', '>':
			if depth > 0 {
				depth--
			}
		case ':':
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func parsePython(raw string) []types.Param {
	var params []types.Param
	for _, p := range splitComma(raw) {
		p = strings.TrimSpace(p)
		if p == "" || p == "*" || p == "/" {
			continue
		}
		p = stripDefault(p)

		prefix := ""
		if strings.HasPrefix(p, "**") {
			prefix, p = "**", p[2:]
		} else if strings.HasPrefix(p, "*") {
			prefix, p = "*", p[1:]
		}

		if idx := firstColon(p); idx >= 0 {
			params = append(params, types.Param{
				Name: prefix + strings.TrimSpace(p[:idx]),
				Type: strings.TrimSpace(p[idx+1:]),
			})
		} else {
			params = append(params, types.Param{Name: prefix + p, Raw: p})
		}
	}
	return params
}

func parseGo(raw string) []types.Param {
	var params []types.Param
	for _, p := range splitComma(raw) {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		fields := strings.Fields(p)
		switch len(fields) {
		case 1:
			params = append(params, types.Param{Type: fields[0], Raw: p})
		case 2:
			params = append(params, types.Param{Name: fields[0], Type: fields[1]})
		default:
			params = append(params, types.Param{Raw: p})
		}
	}
	return params
}

func parseTS(raw string) []types.Param {
	var params []types.Param
	for _, p := range splitComma(raw) {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		p = stripDefault(p)
		prefix := ""
		if strings.HasPrefix(p, "...") {
			prefix, p = "...", p[3:]
		}
		if idx := firstColon(p); idx >= 0 {
			name := strings.TrimSuffix(strings.TrimSpace(p[:idx]), "?")
			params = append(params, types.Param{
				Name: prefix + name,
				Type: strings.TrimSpace(p[idx+1:]),
			})
		} else {
			params = append(params, types.Param{Name: prefix + p, Raw: p})
		}
	}
	return params
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
