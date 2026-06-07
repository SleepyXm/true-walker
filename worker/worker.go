package worker

import (
	"os"
	"path/filepath"
	"sync"

	"tree-sit/test/code-diagnostic/classes"
	"tree-sit/test/code-diagnostic/functions"
	"tree-sit/test/code-diagnostic/imports"
	"tree-sit/test/code-diagnostic/returns"
	"tree-sit/test/code-diagnostic/routes"
	"tree-sit/test/code-diagnostic/scanner"
	"tree-sit/test/types"
)

// FileSnapshot is the processed result for a single file.
type FileSnapshot struct {
	Path      string              `json:"path"`
	Functions []types.FunctionDef `json:"functions"`
	Imports   []types.Import      `json:"imports"`
	Classes   []types.ClassDef    `json:"classes"`
	Routes    []types.Primitive   `json:"routes"`
	Returns   []types.ReturnDef   `json:"returns"`
}

// Worker owns a language group and all compiled rules for it.
// Rules are compiled once at construction; never recompiled per file.
type Worker struct {
	group          *scanner.LangGroup
	functionRules  []types.FunctionRule
	importRules    []types.ImportRule
	classRules     []types.ClassRule
	fieldRules     []types.FieldRule
	returnRules    []types.ReturnRule
	routeExtractor *routes.Extractor
	Results        chan FileSnapshot
}

// New constructs a Worker and compiles all rules for the given language group.
func New(group *scanner.LangGroup, cfg *types.Config, re *routes.Extractor) *Worker {
	return &Worker{
		group:          group,
		functionRules:  functions.CompileRules(cfg.FunctionRules),
		importRules:    imports.CompileRules(cfg.ImportRules),
		classRules:     classes.CompileClassRules(cfg.ClassRules),
		fieldRules:     classes.CompileFieldRules(cfg.FieldRules),
		returnRules:    returns.CompileRules(cfg.ReturnRules),
		routeExtractor: re,
		Results:        make(chan FileSnapshot, 32),
	}
}

// Run processes every file in the group one at a time.
// Designed to run in its own goroutine; closes Results and calls wg.Done() when finished.
func (w *Worker) Run(wg *sync.WaitGroup) {
	defer wg.Done()
	defer close(w.Results)

	for _, path := range w.group.Paths {
		snap, ok := w.processFile(path)
		if ok {
			w.Results <- snap
		}
	}
}

// processFile reads, processes, and immediately discards one file's content.
// Only the extracted metadata (snapshot) survives — never the raw bytes.
func (w *Worker) processFile(path string) (FileSnapshot, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return FileSnapshot{}, false
	}

	f := types.SourceFile{
		Path:     path,
		Content:  data,
		Ext:      filepath.Ext(path),
		Language: w.group.Language,
	}

	fns := functions.Extract(f, w.functionRules)
	rets := returns.Extract(f, w.returnRules, fns)
	fns = returns.Attach(fns, rets)
	imps := imports.Resolve(f, imports.Extract(f, w.importRules), fns)
	cls := classes.Extract(f, w.classRules, w.fieldRules)
	rts := w.routeExtractor.Extract(f)

	// data and f.Content go out of scope here — GC can reclaim them
	data = nil

	if len(fns) == 0 && len(imps) == 0 && len(cls) == 0 && len(rts) == 0 {
		return FileSnapshot{}, false
	}

	return FileSnapshot{
		Path:      path,
		Functions: fns,
		Imports:   imps,
		Classes:   cls,
		Routes:    rts,
		Returns:   rets,
	}, true
}
