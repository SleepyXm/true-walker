package main

import (
	"fmt"
	"log"
	"strings"
	"tree-sit/test/config"
	"tree-sit/test/imports"
	"tree-sit/test/routes"
	"tree-sit/test/scanner"
)

const (
	configPath = "/Users/percedoutprince/Desktop/VSCodeProjects/Backend/Go/tree-sit/http.yml"
	targetDir  = "/Users/percedoutprince/Desktop/VSCodeProjects/Backend/Go/tree-sit/samples/cal.diy-main/apps/api/v2"
	//targetDir  = "/Users/percedoutprince/Desktop/VSCodeProjects/Backend/Go/tree-sit/samples/basic"
	//targetDir = "/Users/percedoutprince/Desktop/VSCodeProjects/Backend/Go/tree-sit/samples/full-stack-fastapi-template-master"
	//targetDir = "/Users/percedoutprince/Desktop/VSCodeProjects/Webapps/Nextjs/finsec/app"
)

func lastSegment(path string) string {
	path = strings.TrimRight(path, "/")
	if i := strings.LastIndexAny(path, "/.@-"); i >= 0 {
		return path[i+1:]
	}
	return path
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

	files, err := scanner.ScanDir(targetDir)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("found %d files\n", len(files))

	for _, f := range files {
		r := routeExtractor.Extract(f)
		imps := imports.Extract(f, importRules)
		imps = imports.Resolve(f, imps)
		if len(r) == 0 {
			continue
		}
		fmt.Println("\n===", f.Path)
		for _, imp := range imps {
			if imp.Alias != "" {
				fmt.Printf("  %s (as %s) — lines %v\n", imp.Path, imp.Alias, imp.Usages[imp.Alias])
			} else if len(imp.Names) > 0 {
				fmt.Printf("  %s\n", imp.Path)
				for _, name := range imp.Names {
					fmt.Printf("    .%s — lines %v\n", name, imp.Usages[name])
				}
			} else {
				if lines := imp.Usages[imp.Path]; len(lines) > 0 {
					fmt.Printf("  %s — lines %v\n", imp.Path, lines)
				} else {
					fmt.Printf("  %s\n", imp.Path)
				}
			}
		}
		for _, route := range r {
			fmt.Printf("  [%s] %s  (line %d)\n",
				route.Data["method"], route.Data["path"], route.StartLine)
		}
	}
}
