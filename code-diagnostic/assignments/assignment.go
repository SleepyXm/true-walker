package assignments

import (
	"log"
	"os"
	"regexp"
	"sort"
	"strings"
	"tree-sit/test/code-diagnostic/functions"
	"tree-sit/test/core/helpers"
	"tree-sit/test/core/syntax"
	"tree-sit/test/types"

	"github.com/goccy/go-yaml"
)

func LoadRules(path string, exts map[string]bool) []types.AssignmentRule {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Printf("assignments: cannot read %s: %v", path, err)
		return nil
	}
	var f struct {
		AssignmentRules []types.AssignmentRuleDef `yaml:"assignment_rules"`
	}
	if err := yaml.Unmarshal(data, &f); err != nil {
		log.Printf("assignments: cannot parse %s: %v", path, err)
		return nil
	}
	return CompileRules(f.AssignmentRules, exts)
}

func CompileRules(defs []types.AssignmentRuleDef, exts map[string]bool) []types.AssignmentRule {
	var out []types.AssignmentRule

	for _, d := range defs {
		if d.Language != "" && len(exts) > 0 && !exts[d.Language] {
			continue
		}

		re, err := regexp.Compile(d.Pattern)
		if err != nil {
			log.Printf("skipping assignment rule %q: %v", d.Name, err)
			continue
		}

		out = append(out, types.AssignmentRule{
			Re:       re,
			VarIdx:   re.SubexpIndex("var"),
			ValueIdx: re.SubexpIndex("value"),
			Language: d.Language,
		})
	}

	return out
}

func Extract(
	f types.SourceFile,
	rules []types.AssignmentRule,
	fns []types.FunctionDef,
) []types.AssignmentDef {

	var defs []types.AssignmentDef

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

			varName := strings.TrimSpace(
				helpers.Subgroup(line, m, r.VarIdx),
			)

			if varName == "" {
				continue
			}

			value := ""
			if r.ValueIdx >= 0 {
				value = strings.TrimSpace(
					helpers.Subgroup(line, m, r.ValueIdx),
				)
			}

			defs = append(defs, types.AssignmentDef{
				Var:      varName,
				Value:    value,
				Line:     lineNum,
				Function: functions.Containing(fns, lineNum),
			})

			break
		}
	}

	sort.Slice(defs, func(i, j int) bool {
		return defs[i].Line < defs[j].Line
	})

	return defs
}

// Attach correlates extracted assignments back onto their FunctionDefs.
func Attach(
	fns []types.FunctionDef,
	assignments []types.AssignmentDef,
) []types.FunctionDef {

	for i := range fns {
		for _, a := range assignments {
			if a.Function == fns[i].Name {
				fns[i].Assignments = append(
					fns[i].Assignments,
					a,
				)
			}
		}
	}

	return fns
}

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
