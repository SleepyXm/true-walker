package routes

import (
	"bufio"
	"bytes"
	"log"
	"regexp"
	"strings"
	"tree-sit/test/types"
)

type routeKey struct {
	line   int
	method string
	path   string
}

type Extractor struct {
	patterns    []types.RoutePattern
	prefixRules []types.PrefixRule
	allowed     map[string]bool
}

func NewExtractor(cfg *types.Config) *Extractor {
	allowed := make(map[string]bool, len(cfg.RouteMethods))
	for _, m := range cfg.RouteMethods {
		allowed[strings.ToUpper(m)] = true
	}
	methods := strings.ToLower(strings.Join(cfg.RouteMethods, "|"))

	var patterns []types.RoutePattern
	for _, rule := range cfg.Rules {
		p, err := compileRule(rule, methods)
		if err != nil {
			log.Printf("skipping route rule %q: %v", rule.Name, err)
			continue
		}
		patterns = append(patterns, p)
	}

	var prefixRules []types.PrefixRule
	for _, rule := range cfg.PrefixRules {
		p, err := compilePrefixRule(rule)
		if err != nil {
			log.Printf("skipping prefix rule %q: %v", rule.Name, err)
			continue
		}
		prefixRules = append(prefixRules, p)
	}

	log.Printf("loaded %d route rules, %d prefix rules", len(patterns), len(prefixRules))
	return &Extractor{patterns: patterns, prefixRules: prefixRules, allowed: allowed}
}

func (e *Extractor) Extract(f types.SourceFile) []types.Primitive {
	prefixes := resolvePrefixes(f.Content, e.prefixRules)
	seen := make(map[routeKey]bool)
	var out []types.Primitive

	sc := bufio.NewScanner(bytes.NewReader(f.Content))
	lineNum := 0
	for sc.Scan() {
		lineNum++
		text := sc.Text()
		for _, p := range e.patterns {
			if p.Language != "" && p.Language != f.Ext {
				continue
			}
			for _, m := range p.Re.FindAllStringSubmatchIndex(text, -1) {
				method := subgroup(text, m, p.MethodIdx)
				path := subgroup(text, m, p.PathIdx)
				receiver := subgroup(text, m, p.ReceiverIdx)
				if method == "" || path == "" {
					continue
				}
				if prefix, ok := prefixes[receiver]; ok {
					path = prefix + path
				}
				for _, resolved := range expandMethods(method, p.Multi) {
					if !e.allowed[resolved] {
						continue
					}
					key := routeKey{lineNum, resolved, path}
					if seen[key] {
						continue
					}
					seen[key] = true
					out = append(out, types.Primitive{
						Kind:        "http_route",
						File:        f.Path,
						StartLine:   lineNum,
						StartColumn: m[0],
						EndLine:     lineNum,
						EndColumn:   m[1],
						Data: map[string]string{
							"method":  resolved,
							"path":    path,
							"pattern": p.Name,
						},
					})
				}
			}
		}
	}
	return out
}

func resolvePrefixes(content []byte, rules []types.PrefixRule) map[string]string {
	prefixes := make(map[string]string)
	sc := bufio.NewScanner(bytes.NewReader(content))
	type raw struct{ varName, receiver, prefix string }
	var found []raw

	for sc.Scan() {
		line := sc.Text()
		for _, r := range rules {
			m := r.Re.FindStringSubmatchIndex(line)
			if m == nil {
				continue
			}
			found = append(found, raw{
				varName:  subgroup(line, m, r.VarIdx),
				receiver: subgroup(line, m, r.ReceiverIdx),
				prefix:   subgroup(line, m, r.PrefixIdx),
			})
		}
	}
	for changed := true; changed; {
		changed = false
		for _, f := range found {
			if f.varName == "" || f.prefix == "" {
				continue
			}
			full := prefixes[f.receiver] + f.prefix
			if prefixes[f.varName] != full {
				prefixes[f.varName] = full
				changed = true
			}
		}
	}
	return prefixes
}

func compileRule(def types.RuleDef, methods string) (types.RoutePattern, error) {
	pattern := strings.ReplaceAll(def.Pattern, "METHOD", `(?i)(`+methods+`)`)
	re, err := regexp.Compile(pattern)
	if err != nil {
		return types.RoutePattern{}, err
	}
	return types.RoutePattern{
		Name:        def.Name,
		Re:          re,
		MethodIdx:   re.SubexpIndex("method"),
		PathIdx:     re.SubexpIndex("path"),
		ReceiverIdx: re.SubexpIndex("receiver"),
		Language:    def.Language,
		Multi:       def.Multi,
	}, nil
}

func compilePrefixRule(def types.PrefixRuleDef) (types.PrefixRule, error) {
	re, err := regexp.Compile(def.Pattern)
	if err != nil {
		return types.PrefixRule{}, err
	}
	return types.PrefixRule{
		Re:          re,
		VarIdx:      re.SubexpIndex("var"),
		ReceiverIdx: re.SubexpIndex("receiver"),
		PrefixIdx:   re.SubexpIndex("prefix"),
	}, nil
}

func expandMethods(raw string, multi bool) []string {
	if !multi {
		return []string{strings.ToUpper(raw)}
	}
	re := regexp.MustCompile(`(?i)(GET|POST|PUT|PATCH|DELETE|HEAD|OPTIONS)`)
	matches := re.FindAllString(raw, -1)
	for i, m := range matches {
		matches[i] = strings.ToUpper(m)
	}
	return matches
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
