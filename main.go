package main

import (
	"fmt"
	"log"
	"tree-sit/test/config"
	"tree-sit/test/imports"
	"tree-sit/test/routes"
	"tree-sit/test/scanner"
)

const (
	configPath = "/Users/percedoutprince/Desktop/VSCodeProjects/Backend/Go/tree-sit/http.yml"
	targetDir  = "/Users/percedoutprince/Desktop/VSCodeProjects/Backend/Go/tree-sit/samples/cal.diy-main/apps/api/v2"
)

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
		i := imports.Extract(f, importRules)
		if len(r) == 0 {
			continue
		}
		fmt.Println("\n===", f.Path)
		if len(i) > 0 {
			fmt.Printf("  imports: %v\n", i)
		}
		for _, route := range r {
			fmt.Printf("  [%s] %s  (line %d)\n",
				route.Data["method"], route.Data["path"], route.StartLine)
		}
	}
}
