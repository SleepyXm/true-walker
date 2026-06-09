package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"tree-sit/test/code-diagnostic/functions"
	"tree-sit/test/code-diagnostic/routes"
	"tree-sit/test/config"
	"tree-sit/test/core/scanner"
	"tree-sit/test/core/worker"
	"tree-sit/test/types"
)

const (
	// Mac
	configPath = ""
	//targetDir  = "/Users/percedoutprince/Desktop/VSCodeProjects/Backend/Go/tree-sit/samples/cal.diy-main/apps/api/v2"
	//targetDir = "/Users/percedoutprince/Desktop/VSCodeProjects/Backend/Go/tree-sit/samples/chatgptsasshole"
	rulesDir = "/Users/percedoutprince/Desktop/VSCodeProjects/Backend/Go/tree-sit/yamls/code-specific"
	//targetDir = "/Users/percedoutprince/Desktop/VSCodeProjects/Backend/Go/tree-sit/samples/redis-unstable/src"
	targetDir = "/Users/percedoutprince/Desktop/VSCodeProjects/Webapps/Nextjs/finsec/app"

	// Windows:
	//configPath = "C:/Users/perce/Desktop/Projects/Backend/Go/tree-sit/yamls/http.yml"

	//targetDir = "C:/Users/perce/Desktop/Projects/Webapps/Nextjs/finsec/app/backend"
	//targetDir = "C:/Users/perce/Desktop/Projects/Backend/Go/tree-sit/samples/kafka-trunk/core/src"
)

type DirGroup struct {
	Dir   string                `json:"dir"`
	Files []worker.FileSnapshot `json:"files"`
}

func main() {
	cfg, err := config.Load(configPath)
	if err != nil {
		log.Printf("config error: %v, using defaults", err)
		cfg = config.Default()
	}

	// Compiled once, shared read-only across all workers — safe.
	routeExtractor := routes.NewExtractor(cfg)

	// Phase 1: walk the directory — no file content read yet.
	groups, err := scanner.GroupByLanguage(targetDir)
	if err != nil {
		log.Fatal(err)
	}
	for name, g := range groups {
		fmt.Printf("language %-12s %d files\n", name, len(g.Paths))
	}

	// Phase 2: one worker goroutine per language group.
	var (
		drainWg   sync.WaitGroup
		mu        sync.Mutex
		snapshots []worker.FileSnapshot
	)

	for _, group := range groups {
		drainWg.Add(1)
		w := worker.New(group, rulesDir, routeExtractor)

		// Worker goroutine: reads and processes files one at a time.
		go func(w *worker.Worker) {
			var wg sync.WaitGroup
			wg.Add(1)
			w.Run(&wg)
			wg.Wait()
		}(w)

		// Drain goroutine: consumes results as they arrive.
		go func(w *worker.Worker) {
			defer drainWg.Done()
			for snap := range w.Results {
				printSnapshot(snap)
				mu.Lock()
				snapshots = append(snapshots, snap)
				mu.Unlock()
			}
		}(w)
	}

	drainWg.Wait()

	byDir := make(map[string][]worker.FileSnapshot)
	for _, snap := range snapshots {
		rel, _ := filepath.Rel(targetDir, filepath.Dir(snap.Path))
		byDir[rel] = append(byDir[rel], snap)
	}

	dirs := make([]string, 0, len(byDir))
	for dir := range byDir {
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)

	grouped := make([]DirGroup, 0, len(dirs))
	for _, dir := range dirs {
		grouped = append(grouped, DirGroup{Dir: dir, Files: byDir[dir]})
	}

	outFile, err := os.Create("jsons/outputchatgpt.json")
	if err != nil {
		log.Fatal(err)
	}
	defer outFile.Close()

	enc := json.NewEncoder(outFile)
	enc.SetIndent("", "  ")
	if err := enc.Encode(grouped); err != nil {
		log.Fatal(err)
	}
}

func printSnapshot(snap worker.FileSnapshot) {
	if len(snap.Functions) == 0 && len(snap.Imports) == 0 &&
		len(snap.Classes) == 0 && len(snap.Routes) == 0 &&
		len(snap.Assignments) == 0 {
		return
	}
	fmt.Println("\n===", snap.Path)

	classOf := make(map[int]string)
	for _, class := range snap.Classes {
		for _, fn := range snap.Functions {
			if fn.StartLine > class.StartLine && fn.StartLine <= class.EndLine {
				classOf[fn.StartLine] = class.Name
			}
		}
	}

	for _, fn := range snap.Functions {
		if classOf[fn.StartLine] == "" {
			fmt.Printf("  fn %s %s  (line %d)%s\n",
				fn.Name, fmtParams(fn.Params), fn.StartLine, fmtReturns(fn.Returns))
		}
	}
	for _, imp := range snap.Imports {
		if imp.Alias != "" {
			fmt.Printf("  %s (as %s) — %s\n", imp.Path, imp.Alias, fmtSites(imp.Usages[imp.Alias]))
		} else if len(imp.Names) > 0 {
			fmt.Printf("  %s\n", imp.Path)
			for _, name := range imp.Names {
				fmt.Printf("    .%s — %s\n", name, fmtSites(imp.Usages[name]))
			}
		} else {
			if sites := imp.Usages[imp.Path]; len(sites) > 0 {
				fmt.Printf("  %s — %s\n", imp.Path, fmtSites(sites))
			} else {
				fmt.Printf("  %s\n", imp.Path)
			}
		}
	}

	for _, c := range snap.Conditionals {
		if c.Function != "" {
			fmt.Printf("  [%s] %s  (line %d, in %s)\n", c.Kind, c.Condition, c.Line, c.Function)
		} else {
			fmt.Printf("  [%s] %s  (line %d)\n", c.Kind, c.Condition, c.Line)
		}
	}

	for _, route := range snap.Routes {
		fn := functions.Containing(snap.Functions, route.StartLine)
		if fn != "" {
			fmt.Printf("  [%s] %s  (line %d, in %s)\n",
				route.Data["method"], route.Data["path"], route.StartLine, fn)
		} else {
			fmt.Printf("  [%s] %s  (line %d)\n",
				route.Data["method"], route.Data["path"], route.StartLine)
		}
	}
	for _, assignment := range snap.Assignments {
		fn := functions.Containing(snap.Functions, assignment.Line)
		if fn != "" {
			fmt.Printf("  %s = %s  (line %d, in %s)\n",
				assignment.Var, assignment.Value, assignment.Line, fn)
		} else {
			fmt.Printf("  %s = %s  (line %d)\n",
				assignment.Var, assignment.Value, assignment.Line)
		}
	}
	for _, class := range snap.Classes {
		fmt.Printf("  class %s%s  (line %d-%d)\n",
			class.Name, fmtBases(class.Bases), class.StartLine, class.EndLine)
		for _, fn := range snap.Functions {
			if classOf[fn.StartLine] == class.Name {
				fmt.Printf("    fn %s %s  (line %d)\n",
					fn.Name, fmtParams(fn.Params), fn.StartLine)
			}
		}
		for _, field := range class.Fields {
			fmt.Printf("    %s\n", fmtField(field))
		}
	}
}

// ── formatting helpers (unchanged) ───────────────────────────────────────────

func fmtSites(sites []types.UsageSite) string {
	parts := make([]string, len(sites))
	for i, s := range sites {
		if s.Function != "" {
			parts[i] = fmt.Sprintf("%d(%s)", s.Line, s.Function)
		} else {
			parts[i] = fmt.Sprintf("%d", s.Line)
		}
	}
	return "[" + strings.Join(parts, " ") + "]"
}

func fmtBases(bases []string) string {
	if len(bases) == 0 {
		return ""
	}
	return " [" + strings.Join(bases, ", ") + "]"
}

func fmtField(f types.FieldDef) string {
	s := "." + f.Name
	if f.Type != "" {
		s += ": " + f.Type
	}
	if f.Default != "" {
		s += " = " + f.Default
	}
	if f.Tag != "" {
		s += " `" + f.Tag + "`"
	}
	return s
}

func fmtParams(params []types.Param) string {
	if len(params) == 0 {
		return "()"
	}
	parts := make([]string, len(params))
	for i, p := range params {
		if p.Type != "" {
			parts[i] = p.Name + ": " + p.Type
		} else if p.Name != "" {
			parts[i] = p.Name
		} else {
			parts[i] = p.Raw
		}
	}
	return "(" + strings.Join(parts, ", ") + ")"
}

func fmtReturns(rets []types.ReturnDef) string {
	if len(rets) == 0 {
		return ""
	}
	seen := make(map[string]bool)
	var shapes []string
	for _, r := range rets {
		if !seen[r.Shape] {
			seen[r.Shape] = true
			shapes = append(shapes, r.Shape)
		}
	}
	return " → " + strings.Join(shapes, " | ")
}
