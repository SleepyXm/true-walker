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
	"tree-sit/test/types"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/javascript"
	"github.com/smacker/go-tree-sitter/python"
	"github.com/smacker/go-tree-sitter/rust"
	"gopkg.in/yaml.v3"
)

// ── Types ─────────────────────────────────────────────────────────────────────

type routeKey struct {
	line   int
	method string
	path   string
}

// resolvePrefixes does multiple passes so chained groups resolve correctly:
//
//	rg  := router.Group("/api")
//	v1  := rg.Group("/v1")      → /api/v1
//	v1.GET("/users")             → /api/v1/users
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

func LoadConfig(path string) (*types.Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg types.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func DefaultConfig() *types.Config {
	return &types.Config{
		Name:         "http_routes",
		RouteMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE"},
	}
}

func compileImportRules(defs []types.ImportRuleDef) []types.ImportRule {
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

func extractImports(f types.SourceFile, rules []types.ImportRule) []string {
	content := f.Content
	switch f.Ext {
	case ".py":
		content = collapseImports(content, '(', ')')
	case ".js", ".ts", ".tsx":
		content = collapseImports(content, '{', '}')
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
				// handle "import a, b, c"
				if idx := r.Re.SubexpIndex("imports"); idx > 0 {
					text := subgroup(line, m, idx)
					for _, name := range strings.Split(text, ",") {
						trimmed := strings.TrimSpace(name)
						if trimmed != "" {
							imports = append(imports, trimmed)
						}
					}
				}

				// handle "from x import y, z"
				if idxModule := r.Re.SubexpIndex("module"); idxModule > 0 {
					if idxNames := r.Re.SubexpIndex("names"); idxNames > 0 {
						module := subgroup(line, m, idxModule)
						names := subgroup(line, m, idxNames)
						for _, name := range strings.Split(names, ",") {
							trimmed := strings.TrimSpace(name)
							if trimmed != "" {
								imports = append(imports, module+"."+trimmed)
							}
						}
					}
				}

			default:
				// fallback for .go, .js, .ts, etc.
				if idx := r.Re.SubexpIndex("import"); idx > 0 {
					imp := subgroup(line, m, idx)
					if imp != "" {
						imports = append(imports, imp)
					}
				}
			}
		}

	}

	seen := make(map[string]bool)
	var deduped []string
	for _, imp := range imports {
		if !seen[imp] {
			seen[imp] = true
			deduped = append(deduped, imp)
		}
	}
	return deduped
}

// ── Add this at package level ─────────────────────────────────────────────────

var reImportBlockStart = regexp.MustCompile(`^(from\s+[\w.]+\s+import|import)\s*[\(\{]`)

// collapseImports joins multi-line import blocks into single lines.
// open/close are the delimiter pair: '('/')' for Python, '{'/'}' for JS/TS.
func collapseImports(src []byte, open, close byte) []byte {
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
			t := strings.TrimSpace(n)
			if t != "" && !strings.HasPrefix(t, "#") {
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
				if closeIdx := strings.IndexByte(after, close); closeIdx >= 0 {
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

type RegexExtractor struct {
	patterns    []types.RoutePattern
	prefixRules []types.PrefixRule
	allowed     map[string]bool
}

func NewRegexExtractor(cfg *types.Config) *RegexExtractor {
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
	return &RegexExtractor{patterns: patterns, prefixRules: prefixRules, allowed: allowed}
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

func (e *RegexExtractor) Extract(f types.SourceFile) []types.Primitive {
	prefixes := resolvePrefixes(f.Content, e.prefixRules)
	seen := make(map[routeKey]bool)

	var out []types.Primitive
	sc := bufio.NewScanner(bytes.NewReader(f.Content))
	line := 0

	for sc.Scan() {
		line++
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
					key := routeKey{line, resolved, path}
					if seen[key] {
						continue
					}
					seen[key] = true
					out = append(out, types.Primitive{
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

func ScanDir(root string) ([]types.SourceFile, error) {
	var files []types.SourceFile

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

		files = append(files, types.SourceFile{
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
	_, err := os.Stat("/Users/percedoutprince/Desktop/VSCodeProjects/Backend/Go/tree-sit/http.yml")
	fmt.Println(err)

	cfg, err := LoadConfig("/Users/percedoutprince/Desktop/VSCodeProjects/Backend/Go/tree-sit/http.yml")
	if err != nil {
		log.Printf("config error: %v", err)
		cfg = DefaultConfig()

	} else {
		log.Printf("config loaded: %d route rules, %d prefix rules, %d import rules",
			len(cfg.Rules), len(cfg.PrefixRules), len(cfg.ImportRules))
	}

	extractor := NewRegexExtractor(cfg)
	importRules := compileImportRules(cfg.ImportRules)

	files, err := ScanDir("/Users/percedoutprince/Desktop/VSCodeProjects/Webapps/Nextjs/finsec/app")
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
