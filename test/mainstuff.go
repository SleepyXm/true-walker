package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/javascript"
	"github.com/smacker/go-tree-sitter/python"
	"github.com/smacker/go-tree-sitter/rust"
	"gopkg.in/yaml.v3"
)

// ── Types ─────────────────────────────────────────────────────────────────────

type SourceFile struct {
	Path     string
	Content  []byte
	Ext      string
	Language *sitter.Language
}

type Primitive struct {
	Kind        string
	File        string
	StartLine   int
	StartColumn int
	EndLine     int
	EndColumn   int
	Data        map[string]string
}

type RoutePattern struct {
	Name        string
	Re          *regexp.Regexp
	MethodIdx   int
	PathIdx     int
	ReceiverIdx int // 0 if not captured
	Multi       bool
}

type PrefixRule struct {
	Re          *regexp.Regexp
	VarIdx      int
	ReceiverIdx int
	PrefixIdx   int
}

// resolvePrefixes does multiple passes so chained groups resolve correctly:
//
//	rg  := router.Group("/api")
//	v1  := rg.Group("/v1")      → /api/v1
//	v1.GET("/users")             → /api/v1/users
func resolvePrefixes(content []byte, rules []PrefixRule) map[string]string {
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

	// fixed-point: keep resolving until nothing new settles
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

// ── Config ────────────────────────────────────────────────────────────────────

type RuleDef struct {
	Name    string `yaml:"name"`
	Pattern string `yaml:"pattern"` // METHOD placeholder, named groups ?P<receiver> ?P<method> ?P<path>
	Multi   bool   `yaml:"multi"`
}

type PrefixRuleDef struct {
	Name    string `yaml:"name"`
	Pattern string `yaml:"pattern"` // named groups ?P<var> ?P<receiver> ?P<prefix>
}

type ImportRuleDef struct {
	Name     string `yaml:"name"`
	Pattern  string `yaml:"pattern"`
	Language string `yaml:"language"`
}

type ImportRule struct {
	Re       *regexp.Regexp
	Language string
}

type Config struct {
	Name         string          `yaml:"name"`
	RouteMethods []string        `yaml:"route_methods"`
	Rules        []RuleDef       `yaml:"route_rules"`
	PrefixRules  []PrefixRuleDef `yaml:"prefix_rules"`
	ImportRules  []ImportRuleDef `yaml:"import_rules"`
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func DefaultConfig() *Config {
	return &Config{
		Name:         "http_routes",
		RouteMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE"},
	}
}

func compileImportRules(defs []ImportRuleDef) []ImportRule {
	var out []ImportRule
	for _, d := range defs {
		re, err := regexp.Compile(d.Pattern)
		if err != nil {
			log.Printf("skipping import rule %q: %v", d.Name, err)
			continue
		}
		out = append(out, ImportRule{Re: re, Language: d.Language})
	}
	return out
}

func extractImports(f SourceFile, rules []ImportRule) []string {
	var imports []string
	sc := bufio.NewScanner(bytes.NewReader(f.Content))
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
			imp := subgroup(line, m, r.Re.SubexpIndex("import"))
			if imp != "" {
				imports = append(imports, imp)
			}
		}
	}
	return imports
}

type RegexExtractor struct {
	patterns    []RoutePattern
	prefixRules []PrefixRule
	allowed     map[string]bool
}

func NewRegexExtractor(cfg *Config) *RegexExtractor {
	allowed := make(map[string]bool, len(cfg.RouteMethods))
	for _, m := range cfg.RouteMethods {
		allowed[strings.ToUpper(m)] = true
	}

	methods := strings.ToLower(strings.Join(cfg.RouteMethods, "|"))

	var patterns []RoutePattern
	for _, rule := range cfg.Rules {
		p, err := compileRule(rule, methods)
		if err != nil {
			log.Printf("skipping route rule %q: %v", rule.Name, err)
			continue
		}
		patterns = append(patterns, p)
	}

	var prefixRules []PrefixRule
	for _, rule := range cfg.PrefixRules {
		p, err := compilePrefixRule(rule)
		if err != nil {
			log.Printf("skipping prefix rule %q: %v", rule.Name, err)
			continue
		}
		prefixRules = append(prefixRules, p)
	}

	log.Printf("loaded %d route rules, %d prefix rules", len(patterns), len(prefixRules))
	return &RegexExtractor{patterns: patterns, prefixRules: prefixRules, allowed: allowed}
}

func compileRule(def RuleDef, methods string) (RoutePattern, error) {
	pattern := strings.ReplaceAll(def.Pattern, "METHOD", `(?i)(`+methods+`)`)
	re, err := regexp.Compile(pattern)
	if err != nil {
		return RoutePattern{}, err
	}

	return RoutePattern{
		Name:        def.Name,
		Re:          re,
		MethodIdx:   re.SubexpIndex("method"),
		PathIdx:     re.SubexpIndex("path"),
		ReceiverIdx: re.SubexpIndex("receiver"),
		Multi:       def.Multi,
	}, nil
}

func compilePrefixRule(def PrefixRuleDef) (PrefixRule, error) {
	re, err := regexp.Compile(def.Pattern)
	if err != nil {
		return PrefixRule{}, err
	}
	return PrefixRule{
		Re:          re,
		VarIdx:      re.SubexpIndex("var"),
		ReceiverIdx: re.SubexpIndex("receiver"),
		PrefixIdx:   re.SubexpIndex("prefix"),
	}, nil
}

func (e *RegexExtractor) Extract(f SourceFile) []Primitive {
	prefixes := resolvePrefixes(f.Content, e.prefixRules)

	var out []Primitive
	sc := bufio.NewScanner(bytes.NewReader(f.Content))
	line := 0

	for sc.Scan() {
		line++
		text := sc.Text()

		for _, p := range e.patterns {
			for _, m := range p.Re.FindAllStringSubmatchIndex(text, -1) {
				method := subgroup(text, m, p.MethodIdx)
				path := subgroup(text, m, p.PathIdx)
				receiver := subgroup(text, m, p.ReceiverIdx)
				if method == "" || path == "" {
					continue
				}

				// prepend any known prefix for this receiver
				if prefix, ok := prefixes[receiver]; ok {
					path = prefix + path
				}

				for _, resolved := range expandMethods(method, p.Multi) {
					if !e.allowed[resolved] {
						continue
					}
					out = append(out, Primitive{
						Kind:        "http_route",
						File:        f.Path,
						StartLine:   line,
						StartColumn: m[0],
						EndLine:     line,
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

// ── Tree-sitter (kept for precision when needed) ──────────────────────────────

var Languages = map[string]*sitter.Language{
	".go": golang.GetLanguage(),
	".py": python.GetLanguage(),
	".js": javascript.GetLanguage(),
	".rs": rust.GetLanguage(),
}

func ParseFile(content []byte, lang *sitter.Language) (*sitter.Tree, error) {
	parser := sitter.NewParser()
	parser.SetLanguage(lang)
	return parser.ParseCtx(context.Background(), nil, content)
}

func Walk(node *sitter.Node, source []byte, depth int) {
	fmt.Printf("%s%s: %s\n", strings.Repeat("  ", depth), node.Type(), node.Content(source))
	for i := 0; i < int(node.ChildCount()); i++ {
		Walk(node.Child(i), source, depth+1)
	}
}

// ── File Scanner ──────────────────────────────────────────────────────────────

func ScanDir(root string) ([]SourceFile, error) {
	var files []SourceFile

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		ext := filepath.Ext(path)
		lang := Languages[ext] // nil is fine, regex doesn't need it

		files = append(files, SourceFile{
			Path:     path,
			Content:  data,
			Ext:      ext,
			Language: lang,
		})
		return nil
	})

	return files, err
}

// ── Main ──────────────────────────────────────────────────────────────────────

func main() {
	_, err := os.Stat("/Users/percedoutprince/Desktop/VSCodeProjects/Backend/Go/tree-sit/test/http.yml")
	fmt.Println(err)

	cfg, err := LoadConfig("/Users/percedoutprince/Desktop/VSCodeProjects/Backend/Go/tree-sit/test/http.yml")
	if err != nil {
		log.Printf("config error: %v", err)
		cfg = DefaultConfig()

	} else {
		log.Printf("config loaded: %d route rules, %d prefix rules, %d import rules",
			len(cfg.Rules), len(cfg.PrefixRules), len(cfg.ImportRules))
	}

	extractor := NewRegexExtractor(cfg)
	importRules := compileImportRules(cfg.ImportRules)

	files, err := ScanDir("/Users/percedoutprince/Desktop/VSCodeProjects/Backend/Go/tree-sit/samples/fastAPIdemo-master")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("found %d files\n", len(files))

	for _, f := range files {
		routes := extractor.Extract(f)
		imports := extractImports(f, importRules)
		if len(routes) == 0 {
			continue
		}
		fmt.Println("\n===", f.Path)
		if len(imports) > 0 {
			fmt.Printf("  imports: %v\n", imports)
		}
		for _, r := range routes {
			fmt.Printf("  [%s] %s  (line %d)\n",
				r.Data["method"], r.Data["path"], r.StartLine)
		}
	}
}
