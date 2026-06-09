package imports

import (
	"bufio"
	"bytes"
	"log"
	"os"
	"regexp"
	"strings"
	"tree-sit/test/core/helpers"
	"tree-sit/test/core/syntax"
	"tree-sit/test/types"

	"github.com/goccy/go-yaml"
)

var reImportBlockStart = regexp.MustCompile(`^(from\s+[\w.]+\s+import|import)\s*[\(\{]`)

// LoadRules reads imports.yml and compiles only the rules that match exts.
// Replaces the old CompileRules(defs, exts) call.
func LoadRules(path string, exts map[string]bool) []types.ImportRule {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Printf("imports: cannot read %s: %v", path, err)
		return nil
	}
	var f struct {
		ImportRules []types.ImportRuleDef `yaml:"import_rules"`
	}
	if err := yaml.Unmarshal(data, &f); err != nil {
		log.Printf("imports: cannot parse %s: %v", path, err)
		return nil
	}
	return CompileRules(f.ImportRules, exts)
}

// CompileRules compiles import rule defs, pre-computing named group indices and
// filtering out rules that don't apply to any of the worker's extensions.
// Language-agnostic rules (Language == "") are always included.
func CompileRules(defs []types.ImportRuleDef, exts map[string]bool) []types.ImportRule {
	var out []types.ImportRule
	for _, d := range defs {
		if d.Language != "" && len(exts) > 0 && !exts[d.Language] {
			continue
		}
		re, err := regexp.Compile(d.Pattern)
		if err != nil {
			log.Printf("skipping import rule %q: %v", d.Name, err)
			continue
		}
		out = append(out, types.ImportRule{
			Re:         re,
			Language:   d.Language,
			PathIdx:    re.SubexpIndex("import"),
			NamesIdx:   re.SubexpIndex("names"),
			AliasIdx:   re.SubexpIndex("alias"),
			ModuleIdx:  re.SubexpIndex("module"),
			ImportsIdx: re.SubexpIndex("imports"),
		})
	}
	return out
}

// Extract returns imports found in f using pre-compiled rules.
// Multi-line import blocks are flattened before scanning so each rule
// only ever sees a single logical line.
func Extract(f types.SourceFile, rules []types.ImportRule) []types.Import {
	content := f.Content
	lines := strings.Split(string(content), "\n")
	switch f.Ext {
	case ".py":
		content = flattenMultiLine(lines, '(', ')')
	case ".js", ".ts", ".tsx":
		content = flattenMultiLine(lines, '{', '}')
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

	sc := syntax.NewScanner(content)
	for sc.Scan() {
		line := sc.Text()
		for _, r := range rules {
			if r.Language != "" && r.Language != f.Ext {
				continue
			}
			m := r.Re.FindStringSubmatchIndex(line)
			if m == nil {
				continue
			}

			// Python bare: import os, sys
			if imps := helpers.Subgroup(line, m, r.ImportsIdx); imps != "" {
				for _, token := range strings.Split(imps, ",") {
					if name, alias := parseAlias(strings.TrimSpace(token)); name != "" {
						add(name, "", alias)
					}
				}
				break
			}

			// Python from: from X import Y, Z
			if mod := helpers.Subgroup(line, m, r.ModuleIdx); mod != "" {
				for _, token := range strings.Split(helpers.Subgroup(line, m, r.NamesIdx), ",") {
					if name, alias := parseAlias(strings.TrimSpace(token)); name != "" {
						add(mod, name, alias)
					}
				}
				break
			}

			// Default: import "path" or import { names } from "path"
			if path := helpers.Subgroup(line, m, r.PathIdx); path != "" {
				if names := helpers.Subgroup(line, m, r.NamesIdx); names != "" {
					for _, token := range strings.Split(names, ",") {
						if name, alias := parseAlias(strings.TrimSpace(token)); name != "" {
							add(path, name, alias)
						}
					}
				} else {
					add(path, "", helpers.Subgroup(line, m, r.AliasIdx))
				}
				break
			}
		}
	}

	out := make([]types.Import, 0, len(order))
	for _, path := range order {
		out = append(out, *byPath[path])
	}
	return out
}

// Resolve finds where each imported identifier is used within f.
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
				Function: helpers.Containing(fns, lineNum),
			})
		}
	}
	return sites
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

// resolveIdent returns alias if set, otherwise the last meaningful segment of path.
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

func flattenMultiLine(lines []string, open, close byte) []byte {
	var buf bytes.Buffer
	i := 0
	for i < len(lines) {
		line := lines[i]
		if reImportBlockStart.MatchString(line) && !strings.Contains(line, string(close)) {
			body, nextIdx := syntax.CollectUntilClose(lines, i+1, open, close)
			prefixEnd := strings.IndexByte(line, open)
			if prefixEnd >= 0 {
				buf.WriteString(line[:prefixEnd])
			}
			for _, token := range strings.Split(body, ",") {
				if t := strings.TrimSpace(token); t != "" && !strings.HasPrefix(t, "#") {
					buf.WriteByte(' ')
					buf.WriteString(t)
				}
			}
			buf.WriteByte('\n')
			i = nextIdx
		} else {
			buf.WriteString(line)
			buf.WriteByte('\n')
			i++
		}
	}
	return buf.Bytes()
}
