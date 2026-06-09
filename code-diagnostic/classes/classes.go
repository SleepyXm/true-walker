package classes

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

func CompileClassRules(defs []types.ClassRuleDef, exts map[string]bool) []types.ClassRule {
	var out []types.ClassRule
	for _, d := range defs {
		if d.Language != "" && len(exts) > 0 && !exts[d.Language] {
			continue
		}
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

func CompileFieldRules(defs []types.FieldRuleDef, exts map[string]bool) []types.FieldRule {
	var out []types.FieldRule
	for _, d := range defs {
		if d.Language != "" && len(exts) > 0 && !exts[d.Language] {
			continue
		}
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

// LoadClassRules reads types.yml and compiles only the class rules that match exts.
func LoadClassRules(path string, exts map[string]bool) []types.ClassRule {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Printf("classes: cannot read %s: %v", path, err)
		return nil
	}
	var f struct {
		ClassRules []types.ClassRuleDef `yaml:"class_rules"`
	}
	if err := yaml.Unmarshal(data, &f); err != nil {
		log.Printf("classes: cannot parse %s: %v", path, err)
		return nil
	}
	return CompileClassRules(f.ClassRules, exts)
}

// LoadFieldRules reads types.yml and compiles only the field rules that match exts.
func LoadFieldRules(path string, exts map[string]bool) []types.FieldRule {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Printf("classes: cannot read %s: %v", path, err)
		return nil
	}
	var f struct {
		FieldRules []types.FieldRuleDef `yaml:"field_rules"`
	}
	if err := yaml.Unmarshal(data, &f); err != nil {
		log.Printf("classes: cannot parse %s: %v", path, err)
		return nil
	}
	return CompileFieldRules(f.FieldRules, exts)
}

func Extract(f types.SourceFile, classRules []types.ClassRule, fieldRules []types.FieldRule) []types.ClassDef {
	var defs []types.ClassDef
	seen := make(map[int]bool)
	lines := strings.Split(string(f.Content), "\n")

	for i, line := range lines {
		lineNum := i + 1
		if syntax.IsComment(line, f.Syntax) || syntax.IsBlank(line) {
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
			name := helpers.Subgroup(line, m, r.NameIdx)
			if name == "" {
				continue
			}
			bases := parseBases(helpers.Subgroup(line, m, r.BasesIdx))
			body, endLine := syntax.CollectBlock(lines, i, f.Syntax)
			defs = append(defs, types.ClassDef{
				Name:      name,
				Bases:     bases,
				StartLine: lineNum,
				EndLine:   endLine,
				Fields:    parseFields(body, f.Ext, fieldRules, f.Syntax),
			})
			seen[lineNum] = true
		}
	}

	sort.Slice(defs, func(i, j int) bool { return defs[i].StartLine < defs[j].StartLine })
	return defs
}

func parseFields(body, ext string, rules []types.FieldRule, syn syntax.LangSyntax) []types.FieldDef {
	var fields []types.FieldDef
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || syntax.IsComment(line, syn) {
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
				Name:    helpers.Subgroup(line, m, r.NameIdx),
				Type:    helpers.Subgroup(line, m, r.TypeIdx),
				Tag:     helpers.Subgroup(line, m, r.TagIdx),
				Default: helpers.Subgroup(line, m, r.DefaultIdx),
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
