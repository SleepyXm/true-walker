package functions

import (
	"log"
	"regexp"
	"sort"
	"strings"
	"tree-sit/test/helpers"
	"tree-sit/test/syntax"
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
		if syntax.IsComment(line, f.Syntax) || syntax.IsBlank(line) {
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
			name := helpers.Subgroup(line, m, r.NameIdx)
			if name == "" {
				continue
			}
			rawParams, endLine := syntax.CollectUntilClose(lines, i, '(', ')')
			defs = append(defs, types.FunctionDef{
				Name:      name,
				StartLine: lineNum,
				EndLine:   endLine,
				Params:    ParseParams(rawParams, f.Syntax),
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

func ParseParams(raw string, syn syntax.LangSyntax) []types.Param {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := syntax.SplitDepth(raw, ',', syn)
	var params []types.Param
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		// strip default value
		p = syntax.StripAfter(p, '=', syn)
		// strip variadic prefixes
		prefix := ""
		for _, pfx := range []string{"**", "*", "..."} {
			if strings.HasPrefix(p, pfx) {
				prefix, p = pfx, p[len(pfx):]
				break
			}
		}
		// name: type  or  name type  or  bare type
		if idx := syntax.FirstAt(p, ':', syn); idx >= 0 {
			name := strings.TrimSuffix(strings.TrimSpace(p[:idx]), "?")
			params = append(params, types.Param{
				Name: prefix + name,
				Type: strings.TrimSpace(p[idx+1:]),
			})
		} else {
			fields := strings.Fields(p)
			switch len(fields) {
			case 2:
				params = append(params, types.Param{Name: prefix + fields[0], Type: fields[1]})
			case 1:
				params = append(params, types.Param{Name: prefix + fields[0]})
			default:
				params = append(params, types.Param{Raw: prefix + p})
			}
		}
	}
	return params
}
