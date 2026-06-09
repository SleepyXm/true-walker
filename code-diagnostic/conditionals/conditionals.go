package conditionals

import (
	"log"
	"os"
	"regexp"
	"sort"
	"strings"
	"tree-sit/test/core/helpers"
	"tree-sit/test/core/syntax"
	"tree-sit/test/types"

	"github.com/goccy/go-yaml"
)

func CompileRules(defs []types.ConditionalRuleDef, exts map[string]bool) []types.ConditionalRule {
	var out []types.ConditionalRule
	for _, d := range defs {
		if d.Language != "" && len(exts) > 0 && !exts[d.Language] {
			continue
		}
		re, err := regexp.Compile(d.Pattern)
		if err != nil {
			log.Printf("skipping conditional rule %q: %v", d.Name, err)
			continue
		}
		out = append(out, types.ConditionalRule{
			Re:           re,
			Kind:         d.Kind,
			Language:     d.Language,
			ConditionIdx: re.SubexpIndex("condition"),
		})
	}
	return out
}

func LoadRules(path string, exts map[string]bool) []types.ConditionalRule {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Printf("conditionals: cannot read %s: %v", path, err)
		return nil
	}
	var f struct {
		ConditionalRules []types.ConditionalRuleDef `yaml:"conditional_rules"`
	}
	if err := yaml.Unmarshal(data, &f); err != nil {
		log.Printf("conditionals: cannot parse %s: %v", path, err)
		return nil
	}
	return CompileRules(f.ConditionalRules, exts)
}

// Extract scans f line-by-line and returns all matched conditional statements.
// Pass the already-extracted fns slice so each conditional is labelled with
// its enclosing function — mirrors how routes and assignments are handled.
func Extract(f types.SourceFile, rules []types.ConditionalRule, fns []types.FunctionDef) []types.ConditionalDef {
	var defs []types.ConditionalDef
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
			if m == nil {
				continue
			}
			condition := ""
			if r.ConditionIdx >= 0 {
				condition = strings.TrimSpace(helpers.Subgroup(line, m, r.ConditionIdx))
			}
			defs = append(defs, types.ConditionalDef{
				Kind:      r.Kind,
				Condition: condition,
				Line:      lineNum,
				Function:  containing(fns, lineNum),
			})
			break // first matching rule wins per line
		}
	}

	sort.Slice(defs, func(i, j int) bool { return defs[i].Line < defs[j].Line })
	return defs
}

// containing mirrors functions.Containing — returns the name of the last
// function whose StartLine is ≤ line (functions must be sorted by StartLine).
func containing(fns []types.FunctionDef, line int) string {
	name := ""
	for _, fn := range fns {
		if fn.StartLine > line {
			break
		}
		name = fn.Name
	}
	return name
}
