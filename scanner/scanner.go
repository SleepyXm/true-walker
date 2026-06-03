package scanner

import (
	"io/fs"
	"os"
	"path/filepath"
	"tree-sit/test/types"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/javascript"
	"github.com/smacker/go-tree-sitter/python"
	"github.com/smacker/go-tree-sitter/rust"
)

var languages = map[string]*sitter.Language{
	".go": golang.GetLanguage(),
	".py": python.GetLanguage(),
	".js": javascript.GetLanguage(),
	".rs": rust.GetLanguage(),
}

func ScanDir(root string) ([]types.SourceFile, error) {
	var files []types.SourceFile
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		ext := filepath.Ext(path)
		files = append(files, types.SourceFile{
			Path:     path,
			Content:  data,
			Ext:      ext,
			Language: languages[ext],
		})
		return nil
	})
	return files, err
}
