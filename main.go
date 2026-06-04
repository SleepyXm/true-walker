package main

import (
	"fmt"
	"log"
	"strings"
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
	//targetDir = "/Users/percedoutprince/Desktop/VSCodeProjects/Backend/Go/tree-sit/samples/full-stack-fastapi-template-master"
	targetDir = "/Users/percedoutprince/Desktop/VSCodeProjects/Webapps/Nextjs/finsec/app"
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

	files, err := scanner.ScanDir(targetDir)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("found %d files\n", len(files))

	for _, f := range files {
		r := routeExtractor.Extract(f)
		if len(r) == 0 {
			continue
		}

		fns := functions.Extract(f, functionRules)
		imps := imports.Resolve(f, imports.Extract(f, importRules), fns)

		fmt.Println("\n===", f.Path)

		// functions
		for _, fn := range fns {
			fmt.Printf("  fn %s %s  (line %d)\n", fn.Name, fmtParams(fn.Params), fn.StartLine)
		}

		// imports
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

		// routes
		for _, route := range r {
			fn := functions.Containing(fns, route.StartLine)
			if fn != "" {
				fmt.Printf("  [%s] %s  (line %d, in %s)\n",
					route.Data["method"], route.Data["path"], route.StartLine, fn)
			} else {
				fmt.Printf("  [%s] %s  (line %d)\n",
					route.Data["method"], route.Data["path"], route.StartLine)
			}
		}
	}
}
