package config

import (
	"fmt"
	"os"
	"path/filepath"

	"tree-sit/test/types"

	"gopkg.in/yaml.v3"
)

// Load reads the 5 rule YAML files from dir and merges them into a single Config.
// dir should contain: routes.yml, imports.yml, functions.yml, types.yml, control_flow.yml
func Load(dir string) (*types.Config, error) {
	routes, err := decodeFile[types.RoutesFile](filepath.Join(dir, "/Users/percedoutprince/Desktop/VSCodeProjects/Backend/Go/tree-sit/yamls/code-specific/routes.yml"))
	if err != nil {
		return nil, err
	}
	imports, err := decodeFile[types.ImportsFile](filepath.Join(dir, "/Users/percedoutprince/Desktop/VSCodeProjects/Backend/Go/tree-sit/yamls/code-specific/imports.yml"))
	if err != nil {
		return nil, err
	}
	functions, err := decodeFile[types.FunctionsFile](filepath.Join(dir, "/Users/percedoutprince/Desktop/VSCodeProjects/Backend/Go/tree-sit/yamls/code-specific/functions.yml"))
	if err != nil {
		return nil, err
	}
	typesFile, err := decodeFile[types.TypesFile](filepath.Join(dir, "/Users/percedoutprince/Desktop/VSCodeProjects/Backend/Go/tree-sit/yamls/code-specific/classes.yml"))
	if err != nil {
		return nil, err
	}
	controlFlow, err := decodeFile[types.ControlFlowFile](filepath.Join(dir, "/Users/percedoutprince/Desktop/VSCodeProjects/Backend/Go/tree-sit/yamls/code-specific/controls.yml"))
	if err != nil {
		return nil, err
	}

	return &types.Config{
		Name:            routes.Name,
		RouteMethods:    routes.RouteMethods,
		RouteRules:      routes.RouteRules,
		PrefixRules:     routes.PrefixRules,
		ImportRules:     imports.ImportRules,
		FunctionRules:   functions.FunctionRules,
		ParameterRules:  functions.ParameterRules,
		ClassRules:      typesFile.ClassRules,
		FieldRules:      typesFile.FieldRules,
		LoopRules:       controlFlow.LoopRules,
		AssignmentRules: controlFlow.AssignmentRules,
		ReturnRules:     controlFlow.ReturnRules,
	}, nil
}

// Default returns an empty Config. Used as a fallback when Load fails so the
// program can still run — it just won't match any rules.
func Default() *types.Config {
	return &types.Config{}
}

func decodeFile[T any](path string) (T, error) {
	var out T
	data, err := os.ReadFile(path)
	if err != nil {
		return out, fmt.Errorf("reading %s: %w", path, err)
	}
	if err := yaml.Unmarshal(data, &out); err != nil {
		return out, fmt.Errorf("parsing %s: %w", path, err)
	}
	return out, nil
}
