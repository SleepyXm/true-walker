package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"tree-sit/test/classes"
	"tree-sit/test/config"
	"tree-sit/test/functions"
	"tree-sit/test/imports"
	"tree-sit/test/routes"
	"tree-sit/test/scanner"
	"tree-sit/test/types"
)

const (
	configPath = "/Users/percedoutprince/Desktop/VSCodeProjects/Backend/Go/tree-sit/http.yml"
	//targetDir  = "/Users/percedoutprince/Desktop/VSCodeProjects/Backend/Go/tree-sit/samples/cal.diy-main/apps/api/v2"
	//targetDir  = "/Users/percedoutprince/Desktop/VSCodeProjects/Backend/Go/tree-sit/samples/basic"
	//targetDir = "/Users/percedoutprince/Desktop/VSCodeProjects/Backend/Go/tree-sit/samples/redis-unstable/src"
	targetDir = "/Users/percedoutprince/Desktop/VSCodeProjects/Webapps/Nextjs/finsec/app/backend"
)

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

type FileSnapshot struct {
	Path      string              `json:"path"`
	Functions []types.FunctionDef `json:"functions"`
	Imports   []types.Import      `json:"imports"`
	Classes   []types.ClassDef    `json:"classes"`
	Routes    []types.Primitive   `json:"routes"`
}

func main() {
	cfg, err := config.Load(configPath)
	if err != nil {
		log.Printf("config error: %v, using defaults", err)
		cfg = config.Default()
	} else {
		log.Printf("config loaded: %d route rules, %d prefix rules, %d import rules",
			len(cfg.Rules), len(cfg.PrefixRules), len(cfg.ImportRules))
	}

	routeExtractor := routes.NewExtractor(cfg)
	importRules := imports.CompileRules(cfg.ImportRules)
	functionRules := functions.CompileRules(cfg.FunctionRules)
	classRules := classes.CompileClassRules(cfg.ClassRules)
	fieldRules := classes.CompileFieldRules(cfg.FieldRules)

	files, err := scanner.ScanDir(targetDir)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("found %d files\n", len(files))

	var snapshots []FileSnapshot

	for _, f := range files {
		fns := functions.Extract(f, functionRules)
		imps := imports.Resolve(f, imports.Extract(f, importRules), fns)
		cls := classes.Extract(f, classRules, fieldRules)
		rts := routeExtractor.Extract(f)

		if len(fns) == 0 && len(imps) == 0 && len(cls) == 0 && len(rts) == 0 {
			continue
		}

		// Build classOf first so the function loop can filter methods out
		classOf := make(map[int]string) // fn.StartLine → class.Name
		for _, class := range cls {
			for _, fn := range fns {
				if fn.StartLine > class.StartLine && fn.StartLine <= class.EndLine {
					classOf[fn.StartLine] = class.Name
				}
			}
		}

		fmt.Println("\n===", f.Path)

		for _, fn := range fns {
			if classOf[fn.StartLine] == "" {
				fmt.Printf("  fn %s %s  (line %d)\n", fn.Name, fmtParams(fn.Params), fn.StartLine)
			}
		}

		for _, imp := range imps {
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

		for _, route := range rts {
			fn := functions.Containing(fns, route.StartLine)
			if fn != "" {
				fmt.Printf("  [%s] %s  (line %d, in %s)\n",
					route.Data["method"], route.Data["path"], route.StartLine, fn)
			} else {
				fmt.Printf("  [%s] %s  (line %d)\n",
					route.Data["method"], route.Data["path"], route.StartLine)
			}
		}

		for _, class := range cls {
			fmt.Printf("  class %s%s  (line %d-%d)\n",
				class.Name, fmtBases(class.Bases), class.StartLine, class.EndLine)
			for _, fn := range fns {
				if classOf[fn.StartLine] == class.Name {
					fmt.Printf("    fn %s %s  (line %d)\n", fn.Name, fmtParams(fn.Params), fn.StartLine)
				}
			}
			for _, field := range class.Fields {
				fmt.Printf("    %s\n", fmtField(field))
			}
		}

		snapshots = append(snapshots, FileSnapshot{
			Path:      f.Path,
			Functions: fns,
			Imports:   imps,
			Classes:   cls,
			Routes:    rts,
		})
	}

	outFile, err := os.Create("output.json")
	if err != nil {
		log.Fatal(err)
	}
	defer outFile.Close()

	enc := json.NewEncoder(outFile)
	enc.SetIndent("", "  ")
	if err := enc.Encode(snapshots); err != nil {
		log.Fatal(err)
	}
}
