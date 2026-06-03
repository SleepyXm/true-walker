package config

import (
	"os"
	"tree-sit/test/types"

	"gopkg.in/yaml.v3"
)

func Load(path string) (*types.Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg types.Config
	return &cfg, yaml.Unmarshal(data, &cfg)
}

func Default() *types.Config {
	return &types.Config{
		Name:         "http_routes",
		RouteMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE"},
	}
}
