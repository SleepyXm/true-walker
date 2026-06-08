package scanner

import (
	"io/fs"
	"path/filepath"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/c"
	"github.com/smacker/go-tree-sitter/cpp"
	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/java"
	"github.com/smacker/go-tree-sitter/javascript"
	"github.com/smacker/go-tree-sitter/python"
	"github.com/smacker/go-tree-sitter/ruby"
	"github.com/smacker/go-tree-sitter/rust"
)

// extGroup maps file extensions to a canonical language group name.
// Extensions in the same group share a single worker.
var extGroup = map[string]string{
	".go":   "go",
	".py":   "python",
	".js":   "javascript",
	".ts":   "javascript",
	".tsx":  "javascript",
	".jsx":  "javascript",
	".rs":   "rust",
	".c":    "c",
	".h":    "c",
	".cpp":  "cpp",
	".hpp":  "cpp",
	".rb":   "ruby",
	".java": "java",
}

var groupLanguage = map[string]*sitter.Language{
	"go":         golang.GetLanguage(),
	"python":     python.GetLanguage(),
	"javascript": javascript.GetLanguage(),
	"rust":       rust.GetLanguage(),
	"c":          c.GetLanguage(),
	"cpp":        cpp.GetLanguage(),
	"ruby":       ruby.GetLanguage(),
	"java":       java.GetLanguage(),
}

// LangGroup holds metadata and all file paths for one language group.
// No file content is read here.
type LangGroup struct {
	Name     string
	Language *sitter.Language // nil if no tree-sitter grammar registered
	Paths    []string
}

// GroupByLanguage walks root and buckets file paths by language group.
// It is the only place that touches the filesystem during the scan phase.
func GroupByLanguage(root string) (map[string]*LangGroup, error) {
	groups := make(map[string]*LangGroup)

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		name, ok := extGroup[filepath.Ext(path)]
		if !ok {
			return nil
		}
		if _, exists := groups[name]; !exists {
			groups[name] = &LangGroup{
				Name:     name,
				Language: groupLanguage[name],
			}
		}
		groups[name].Paths = append(groups[name].Paths, path)
		return nil
	})

	return groups, err
}
