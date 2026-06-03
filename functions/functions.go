package functions

import (
	"bufio"
	"bytes"
	"log"
	"regexp"
	"sort"
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

	sc := bufio.NewScanner(bytes.NewReader(f.Content))
	lineNum := 0
	for sc.Scan() {
		lineNum++
		line := sc.Text()
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
			defs = append(defs, types.FunctionDef{Name: name, StartLine: lineNum})
			seen[lineNum] = true
		}
	}

	sort.Slice(defs, func(i, j int) bool {
		return defs[i].StartLine < defs[j].StartLine
	})
	return defs
}

// Containing returns the name of the innermost function that contains line,
// using a simple "largest start ≤ line" heuristic.
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
