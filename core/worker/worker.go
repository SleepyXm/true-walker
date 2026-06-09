package worker

import (
	"os"
	"path/filepath"
	"sync"

	"tree-sit/test/code-diagnostic/assignments"
	"tree-sit/test/code-diagnostic/classes"
	"tree-sit/test/code-diagnostic/conditionals"
	"tree-sit/test/code-diagnostic/functions"
	"tree-sit/test/code-diagnostic/imports"
	"tree-sit/test/code-diagnostic/returns"
	"tree-sit/test/code-diagnostic/routes"
	"tree-sit/test/core/scanner"
	"tree-sit/test/types"
)

// FileSnapshot is the processed result for a single file.
type FileSnapshot struct {
	Path         string                 `json:"path"`
	Functions    []types.FunctionDef    `json:"functions"`
	Imports      []types.Import         `json:"imports"`
	Classes      []types.ClassDef       `json:"classes"`
	Routes       []types.Primitive      `json:"routes"`
	Returns      []types.ReturnDef      `json:"returns"`
	Conditionals []types.ConditionalDef `json:"conditionals,omitempty"`
	Assignments  []types.AssignmentDef  `json:"assignments,omitempty"`
}

// Worker owns a language group and all compiled rules for it.
// Rules are compiled once at construction; never recompiled per file.
type Worker struct {
	group            *scanner.LangGroup
	functionRules    []types.FunctionRule
	importRules      []types.ImportRule
	classRules       []types.ClassRule
	fieldRules       []types.FieldRule
	returnRules      []types.ReturnRule
	assignmentRules  []types.AssignmentRule
	conditionalRules []types.ConditionalRule
	routeExtractor   *routes.Extractor
	Results          chan FileSnapshot
}

// New constructs a Worker with rules pre-filtered to this language group.
// Rules for other languages are discarded at compile time — never at extract time.
func New(group *scanner.LangGroup, rulesDir string, re *routes.Extractor) *Worker {
	exts := scanner.ExtensionsFor(group.Name)
	return &Worker{
		group:            group,
		functionRules:    functions.LoadRules(filepath.Join(rulesDir, "functions.yml"), exts),
		importRules:      imports.LoadRules(filepath.Join(rulesDir, "imports.yml"), exts),
		classRules:       classes.LoadClassRules(filepath.Join(rulesDir, "classes.yml"), exts),
		fieldRules:       classes.LoadFieldRules(filepath.Join(rulesDir, "classes.yml"), exts),
		returnRules:      returns.LoadRules(filepath.Join(rulesDir, "controls.yml"), exts),
		assignmentRules:  assignments.LoadRules(filepath.Join(rulesDir, "assignments.yml"), exts),
		conditionalRules: conditionals.LoadRules(filepath.Join(rulesDir, "conditionals.yml"), exts),
		routeExtractor:   re,
		Results:          make(chan FileSnapshot, 32),
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
	assignments := assignments.Extract(f, w.assignmentRules, fns)
	conditionals := conditionals.Extract(f, w.conditionalRules, fns)

	// data and f.Content go out of scope here — GC can reclaim them

	data = nil

	if len(fns) == 0 && len(imps) == 0 && len(cls) == 0 && len(rts) == 0 && len(assignments) == 0 && len(conditionals) == 0 {
		return FileSnapshot{}, false
	}

	return FileSnapshot{
		Path:         path,
		Functions:    fns,
		Imports:      imps,
		Classes:      cls,
		Routes:       rts,
		Returns:      rets,
		Assignments:  assignments,
		Conditionals: conditionals,
	}, true
}
