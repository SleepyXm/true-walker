package classes

import (
	"log"
	"regexp"
	"sort"
	"strings"
	"tree-sit/test/types"
)

func CompileClassRules(defs []types.ClassRuleDef) []types.ClassRule {
	var out []types.ClassRule
	for _, d := range defs {
		re, err := regexp.Compile(d.Pattern)
		if err != nil {
			log.Printf("skipping class rule %q: %v", d.Name, err)
			continue
		}
		out = append(out, types.ClassRule{
			Re:       re,
			NameIdx:  re.SubexpIndex("class"),
			BasesIdx: re.SubexpIndex("bases"),
			Language: d.Language,
		})
	}
	return out
}

func CompileFieldRules(defs []types.FieldRuleDef) []types.FieldRule {
	var out []types.FieldRule
	for _, d := range defs {
		re, err := regexp.Compile(d.Pattern)
		if err != nil {
			log.Printf("skipping field rule %q: %v", d.Name, err)
			continue
		}
		out = append(out, types.FieldRule{
			Re:         re,
			NameIdx:    re.SubexpIndex("field"),
			TypeIdx:    re.SubexpIndex("type"),
			TagIdx:     re.SubexpIndex("tag"),
			DefaultIdx: re.SubexpIndex("default"),
			Language:   d.Language,
		})
	}
	return out
}

func Extract(f types.SourceFile, classRules []types.ClassRule, fieldRules []types.FieldRule) []types.ClassDef {
	var defs []types.ClassDef
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
		for _, r := range classRules {
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
			bases := parseBases(subgroup(line, m, r.BasesIdx))
			body, endLine := collectBody(lines, i, f.Ext)
			defs = append(defs, types.ClassDef{
				Name:      name,
				Bases:     bases,
				StartLine: lineNum,
				EndLine:   endLine,
				Fields:    parseFields(body, f.Ext, fieldRules),
			})
			seen[lineNum] = true
		}
	}

	sort.Slice(defs, func(i, j int) bool { return defs[i].StartLine < defs[j].StartLine })
	return defs
}

// collectBody collects the body of a class definition.
// For brace languages it tracks { } depth.
// For Python it tracks indentation.
func collectBody(lines []string, startIdx int, ext string) (string, int) {
	if ext == ".py" {
		return collectPythonBody(lines, startIdx)
	}
	return collectBraceBody(lines, startIdx)
}

func collectBraceBody(lines []string, startIdx int) (string, int) {
	var buf strings.Builder
	depth := 0
	opened := false

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

func collectPythonBody(lines []string, startIdx int) (string, int) {
	// find the base indentation from the class line
	baseIndent := len(lines[startIdx]) - len(strings.TrimLeft(lines[startIdx], " \t"))
	var buf strings.Builder

	for i := startIdx + 1; i < len(lines); i++ {
		line := lines[i]
		if strings.TrimSpace(line) == "" {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " \t"))
		if indent <= baseIndent {
			return strings.TrimSpace(buf.String()), i
		}
		buf.WriteString(line)
		buf.WriteByte('\n')
	}
	return strings.TrimSpace(buf.String()), len(lines)
}

func parseFields(body, ext string, rules []types.FieldRule) []types.FieldDef {
	var fields []types.FieldDef
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		for _, r := range rules {
			if r.Language != "" && r.Language != ext {
				continue
			}
			m := r.Re.FindStringSubmatchIndex(line)
			if m == nil {
				continue
			}
			fields = append(fields, types.FieldDef{
				Name:    subgroup(line, m, r.NameIdx),
				Type:    subgroup(line, m, r.TypeIdx),
				Tag:     subgroup(line, m, r.TagIdx),
				Default: subgroup(line, m, r.DefaultIdx),
			})
			break
		}
	}
	return fields
}

func parseBases(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var bases []string
	for _, b := range strings.Split(raw, ",") {
		if t := strings.TrimSpace(b); t != "" {
			bases = append(bases, t)
		}
	}
	return bases
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
