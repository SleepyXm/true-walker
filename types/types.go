package types

import (
	"regexp"

	sitter "github.com/smacker/go-tree-sitter"
)

type SourceFile struct {
	Path     string
	Content  []byte
	Ext      string
	Language *sitter.Language
}

type Primitive struct {
	Kind        string
	File        string
	StartLine   int
	StartColumn int
	EndLine     int
	EndColumn   int
	Data        map[string]string
}

type RoutePattern struct {
	Name        string
	Re          *regexp.Regexp
	MethodIdx   int
	PathIdx     int
	ReceiverIdx int
	Language    string
	Multi       bool
}

type PrefixRule struct {
	Re          *regexp.Regexp
	VarIdx      int
	ReceiverIdx int
	PrefixIdx   int
}

type ImportRule struct {
	Re       *regexp.Regexp
	Language string
}

type RuleDef struct {
	Name     string `yaml:"name"`
	Pattern  string `yaml:"pattern"`
	Language string `yaml:"language"`
	Multi    bool   `yaml:"multi"`
}

type PrefixRuleDef struct {
	Name    string `yaml:"name"`
	Pattern string `yaml:"pattern"`
}

type ImportRuleDef struct {
	Name     string `yaml:"name"`
	Pattern  string `yaml:"pattern"`
	Language string `yaml:"language"`
}

type Config struct {
	Name         string          `yaml:"name"`
	RouteMethods []string        `yaml:"route_methods"`
	Rules        []RuleDef       `yaml:"route_rules"`
	PrefixRules  []PrefixRuleDef `yaml:"prefix_rules"`
	ImportRules  []ImportRuleDef `yaml:"import_rules"`
}
