package treesitter

import (
	"context"
	"fmt"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/javascript"
	"github.com/smacker/go-tree-sitter/python"
	"github.com/smacker/go-tree-sitter/rust"
)

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
