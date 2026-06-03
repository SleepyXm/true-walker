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

func Extract(f types.SourceFile, rules []types.ImportRule) []string {
	content := f.Content
	switch f.Ext {
	case ".py":
		content = collapse(content, '(', ')')
	case ".js", ".ts", ".tsx":
		content = collapse(content, '{', '}')
	}

	var imports []string
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
					for _, name := range strings.Split(subgroup(line, m, idx), ",") {
						if t := strings.TrimSpace(name); t != "" {
							imports = append(imports, t)
						}
					}
				}
				if idxMod := r.Re.SubexpIndex("module"); idxMod > 0 {
					if idxNames := r.Re.SubexpIndex("names"); idxNames > 0 {
						mod := subgroup(line, m, idxMod)
						for _, name := range strings.Split(subgroup(line, m, idxNames), ",") {
							if t := strings.TrimSpace(name); t != "" {
								imports = append(imports, mod+"."+t)
							}
						}
					}
				}
			default:
				if idx := r.Re.SubexpIndex("import"); idx > 0 {
					if imp := subgroup(line, m, idx); imp != "" {
						imports = append(imports, imp)
					}
				}
			}
		}
	}

	seen := make(map[string]bool)
	var out []string
	for _, imp := range imports {
		if !seen[imp] {
			seen[imp] = true
			out = append(out, imp)
		}
	}
	return out
}

func collapse(src []byte, open, close byte) []byte {
	var buf bytes.Buffer
	sc := bufio.NewScanner(bytes.NewReader(src))
	var prefix string
	var names []string
	inBlock := false

	flush := func() {
		buf.WriteString(prefix)
		buf.WriteByte(' ')
		buf.WriteString(strings.Join(names, ", "))
		buf.WriteByte('\n')
		names = nil
		inBlock = false
	}

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
				inBlock = true
				prefix = strings.TrimRight(line[:openIdx], " \t")
				after := line[openIdx+1:]
				if strings.IndexByte(after, close) >= 0 {
					buf.WriteString(line)
					buf.WriteByte('\n')
					inBlock = false
					names = nil
				} else {
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
			flush()
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
