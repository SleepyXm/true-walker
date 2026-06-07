package imports

import (
	"bufio"
	"bytes"
	"log"
	"regexp"
	"strings"
	"tree-sit/test/types"
)

var reImportBlockStart = regexp.MustCompile(`^(from\s+[\w.]+\s+import|import)\s*[\(\{]`)

func CompileRules(defs []types.ImportRuleDef) []types.ImportRule {
	var out []types.ImportRule
	for _, d := range defs {
		re, err := regexp.Compile(d.Pattern)
		if err != nil {
			log.Printf("skipping import rule %q: %v", d.Name, err)
			continue
		}
		out = append(out, types.ImportRule{Re: re, Language: d.Language})
	}
	return out
}

// Extract returns imports with their alias and usage line numbers.
func Extract(f types.SourceFile, rules []types.ImportRule) []types.Import {
	content := f.Content
	switch f.Ext {
	case ".py":
		content = collapse(content, '(', ')')
	case ".js", ".ts", ".tsx":
		content = collapse(content, '{', '}')
	}

	byPath := make(map[string]*types.Import)
	var order []string

	add := func(path, name, alias string) {
		if _, ok := byPath[path]; !ok {
			byPath[path] = &types.Import{Path: path, Usages: make(map[string][]types.UsageSite)}
			order = append(order, path)
		}
		imp := byPath[path]
		if alias != "" {
			imp.Alias = alias
		}
		if name != "" {
			imp.Names = append(imp.Names, name)
		}
	}

	sc := bufio.NewScanner(bytes.NewReader(content))
	for sc.Scan() {
		line := sc.Text()
		for _, r := range rules {
			if r.Language != f.Ext {
				continue
			}
			m := r.Re.FindStringSubmatchIndex(line)
			if m == nil {
				continue
			}
			switch f.Ext {
			case ".py":
				if idx := r.Re.SubexpIndex("imports"); idx > 0 {
					for _, token := range strings.Split(subgroup(line, m, idx), ",") {
						name, alias := parseAlias(strings.TrimSpace(token))
						if name != "" {
							add(name, "", alias)
						}
					}
				}
				if idxMod := r.Re.SubexpIndex("module"); idxMod > 0 {
					if idxNames := r.Re.SubexpIndex("names"); idxNames > 0 {
						mod := subgroup(line, m, idxMod)
						for _, token := range strings.Split(subgroup(line, m, idxNames), ",") {
							name, alias := parseAlias(strings.TrimSpace(token))
							if name != "" {
								add(mod, name, alias)
							}
						}
					}
				}
			default:
				if idx := r.Re.SubexpIndex("import"); idx > 0 {
					if path := subgroup(line, m, idx); path != "" {
						if idxNames := r.Re.SubexpIndex("names"); idxNames > 0 {
							for _, token := range strings.Split(subgroup(line, m, idxNames), ",") {
								name, alias := parseAlias(strings.TrimSpace(token))
								if name != "" {
									add(path, name, alias)
								}
							}
						} else {
							add(path, "", "")
						}
					}
				}
			}
		}
	}

	out := make([]types.Import, 0, len(order))
	for _, path := range order {
		out = append(out, *byPath[path])
	}
	return out
}

func Resolve(f types.SourceFile, imps []types.Import, fns []types.FunctionDef) []types.Import {
	for i, imp := range imps {
		if imp.Alias != "" {
			imps[i].Usages[imp.Alias] = findUsages(f.Content, imp.Alias, fns)
		}
		for _, name := range imp.Names {
			imps[i].Usages[name] = findUsages(f.Content, name, fns)
		}
		if imp.Alias == "" && len(imp.Names) == 0 {
			ident := resolveIdent(imp.Path, "")
			imps[i].Usages[imp.Path] = findUsages(f.Content, ident, fns)
		}
	}
	return imps
}

func findUsages(content []byte, ident string, fns []types.FunctionDef) []types.UsageSite {
	if ident == "" {
		return nil
	}
	re, err := regexp.Compile(`\b` + regexp.QuoteMeta(ident) + `\b`)
	if err != nil {
		return nil
	}
	var sites []types.UsageSite
	sc := bufio.NewScanner(bytes.NewReader(content))
	lineNum := 0
	for sc.Scan() {
		lineNum++
		if re.MatchString(sc.Text()) {
			sites = append(sites, types.UsageSite{
				Line:     lineNum,
				Function: containing(fns, lineNum),
			})
		}
	}
	return sites
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

// parseAlias splits "pandas as pd" → ("pandas", "pd"), or "pandas" → ("pandas", "").
func parseAlias(s string) (path, alias string) {
	parts := strings.Fields(s)
	for i, p := range parts {
		if strings.EqualFold(p, "as") && i+1 < len(parts) {
			return strings.Join(parts[:i], " "), parts[i+1]
		}
	}
	return s, ""
}

// resolveIdent returns alias if set, otherwise the last segment of path.
func resolveIdent(path, alias string) string {
	if alias != "" {
		return alias
	}
	path = strings.TrimRight(path, "/")
	parts := strings.FieldsFunc(path, func(r rune) bool {
		return r == '/' || r == '.' || r == '@'
	})
	if len(parts) == 0 {
		return path
	}
	for len(parts) > 1 {
		if matched, _ := regexp.MatchString(`^v\d+$`, parts[len(parts)-1]); matched {
			parts = parts[:len(parts)-1]
		} else {
			break
		}
	}
	last := parts[len(parts)-1]
	if idx := strings.LastIndex(last, "-"); idx != -1 {
		last = last[idx+1:]
	}
	return last
}

// findUsageLines scans content for lines where ident appears as a word.
func findUsageLines(content []byte, ident string) []int {
	if ident == "" {
		return nil
	}
	re, err := regexp.Compile(`\b` + regexp.QuoteMeta(ident) + `\b`)
	if err != nil {
		return nil
	}
	var lines []int
	sc := bufio.NewScanner(bytes.NewReader(content))
	lineNum := 0
	for sc.Scan() {
		lineNum++
		if re.MatchString(sc.Text()) {
			lines = append(lines, lineNum)
		}
	}
	return lines
}

func collapse(src []byte, open, close byte) []byte {
	var buf bytes.Buffer
	sc := bufio.NewScanner(bytes.NewReader(src))
	var prefix string
	var names []string
	inBlock := false

	collect := func(s string) {
		for _, n := range strings.Split(s, ",") {
			if t := strings.TrimSpace(n); t != "" && !strings.HasPrefix(t, "#") {
				names = append(names, t)
			}
		}
	}

	for sc.Scan() {
		line := sc.Text()
		if !inBlock {
			openIdx := strings.IndexByte(line, open)
			if openIdx >= 0 && reImportBlockStart.MatchString(line) {
				after := line[openIdx+1:]
				if strings.IndexByte(after, close) >= 0 {
					// already single-line, write as-is
					buf.WriteString(line)
					buf.WriteByte('\n')
				} else {
					inBlock = true
					prefix = strings.TrimRight(line[:openIdx], " \t")
					collect(after)
				}
			} else {
				buf.WriteString(line)
				buf.WriteByte('\n')
			}
			continue
		}
		if closeIdx := strings.IndexByte(line, close); closeIdx >= 0 {
			collect(line[:closeIdx])
			remainder := strings.TrimSpace(line[closeIdx+1:])
			buf.WriteString(prefix)
			buf.WriteString(" { ")
			buf.WriteString(strings.Join(names, ", "))
			buf.WriteString(" }")
			if remainder != "" {
				buf.WriteByte(' ')
				buf.WriteString(remainder)
			}
			buf.WriteByte('\n')
			names = nil
			inBlock = false
		} else {
			collect(line)
		}
	}
	return buf.Bytes()
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
