package types

import (
	"regexp"
	"tree-sit/test/core/syntax"

	sitter "github.com/smacker/go-tree-sitter"
)

// ──────────────────────────────────────────────────────────────────────────────
// File
// ──────────────────────────────────────────────────────────────────────────────

type SourceFile struct {
	Path     string
	Content  []byte
	Ext      string
	Language *sitter.Language
	Syntax   syntax.LangSyntax
}

// ──────────────────────────────────────────────────────────────────────────────
// Primitive / shared
// ──────────────────────────────────────────────────────────────────────────────

type Primitive struct {
	Kind        string
	File        string
	StartLine   int
	StartColumn int
	EndLine     int
	EndColumn   int
	Data        map[string]string
}

// ──────────────────────────────────────────────────────────────────────────────
// Routes
// ──────────────────────────────────────────────────────────────────────────────

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

type RoutePattern struct {
	Re          *regexp.Regexp
	Name        string
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

// ──────────────────────────────────────────────────────────────────────────────
// Imports
// ──────────────────────────────────────────────────────────────────────────────

type ImportRuleDef struct {
	Name     string `yaml:"name"`
	Pattern  string `yaml:"pattern"`
	Language string `yaml:"language"`
}

type ImportRule struct {
	Re         *regexp.Regexp
	Language   string
	PathIdx    int // "import" group — the module/package path
	NamesIdx   int // "names" group — named imports: { A, B } or "from X import A, B"
	AliasIdx   int // "alias" group — "import x as y" or "import alias path"
	ModuleIdx  int // "module" group — Python "from <module> import ..."
	ImportsIdx int // "imports" group — Python bare "import os, sys"
}

type UsageSite struct {
	Line     int
	Function string
}

type Import struct {
	Path   string
	Alias  string
	Names  []string
	Usages map[string][]UsageSite
}

// ──────────────────────────────────────────────────────────────────────────────
// Functions & parameters
// ──────────────────────────────────────────────────────────────────────────────

type FunctionRuleDef struct {
	Name     string `yaml:"name"`
	Pattern  string `yaml:"pattern"`
	Language string `yaml:"language"`
}

type FunctionRule struct {
	Re       *regexp.Regexp
	NameIdx  int
	Language string
}

type ParameterRuleDef struct {
	Name     string `yaml:"name"`
	Pattern  string `yaml:"pattern"`
	Language string `yaml:"language"`
}

type ParameterRule struct {
	Re       *regexp.Regexp
	Language string
}

type Param struct {
	Name string
	Type string
	Raw  string
}

type FunctionDef struct {
	Name        string
	StartLine   int
	EndLine     int
	Params      []Param
	RawParams   string
	Returns     []ReturnDef     `json:"returns,omitempty"`
	Assignments []AssignmentDef `json:"assignments,omitempty"`
}

// ──────────────────────────────────────────────────────────────────────────────
// Classes & fields
// ──────────────────────────────────────────────────────────────────────────────

type ClassRuleDef struct {
	Name     string `yaml:"name"`
	Pattern  string `yaml:"pattern"`
	Language string `yaml:"language"`
}

type ClassRule struct {
	Re       *regexp.Regexp
	NameIdx  int
	BasesIdx int
	Language string
}

type FieldRuleDef struct {
	Name     string `yaml:"name"`
	Pattern  string `yaml:"pattern"`
	Language string `yaml:"language"`
}

type FieldRule struct {
	Re         *regexp.Regexp
	NameIdx    int
	TypeIdx    int
	TagIdx     int
	DefaultIdx int
	Language   string
}

type ClassDef struct {
	Name      string            `json:"name"`
	Bases     []string          `json:"bases,omitempty"`
	StartLine int               `json:"startLine"`
	EndLine   int               `json:"endLine"`
	Fields    []FieldDef        `json:"fields,omitempty"`
	Data      map[string]string `json:"data,omitempty"`
}

type FieldDef struct {
	Name    string `json:"name"`
	Type    string `json:"type,omitempty"`
	Tag     string `json:"tag,omitempty"`
	Default string `json:"default,omitempty"`
}

// ──────────────────────────────────────────────────────────────────────────────
// Control flow — loops, assignments, returns
// ──────────────────────────────────────────────────────────────────────────────

type LoopRuleDef struct {
	Name     string `yaml:"name"`
	Pattern  string `yaml:"pattern"`
	Language string `yaml:"language"`
}

type LoopRule struct {
	Re       *regexp.Regexp
	Language string
}

type AssignmentDef struct {
	Var      string `json:"var"`
	Value    string `json:"value,omitempty"`
	Line     int    `json:"line"`
	Function string `json:"function,omitempty"`
}

type AssignmentRuleDef struct {
	Language string `yaml:"language"`
	Name     string `yaml:"name"`
	Pattern  string `yaml:"pattern"`
	Var      string
	Value    string
	Line     int
	Function string
}

//type AssignmentRuleDef struct {
//	Name     string `yaml:"name"`
//	Pattern  string `yaml:"pattern"`
//	Language string `yaml:"language"`
//}

type AssignmentRule struct {
	Re       *regexp.Regexp
	VarIdx   int
	ValueIdx int
	Language string
}

//type AssignmentRule struct {
//	Re     *regexp.Regexp
//	VarIdx int
//}

type ReturnRuleDef struct {
	Name     string `yaml:"name"`
	Pattern  string `yaml:"pattern"`
	Language string `yaml:"language"`
}

type ReturnRule struct {
	Re       *regexp.Regexp
	Name     string
	ValueIdx int
	Language string
}

type ReturnDef struct {
	Mechanism string `json:"mechanism"`
	Shape     string `json:"shape"`
	Value     string `json:"value,omitempty"`
	Line      int    `json:"line"`
	Function  string `json:"function,omitempty"`
}

// ──────────────────────────────────────────────────────────────────────────────
// Config — merged in-memory representation of all rule files
// ──────────────────────────────────────────────────────────────────────────────

// Config is the merged result of loading all rule YAML files.
// Use LoadConfig to populate it from a directory containing the split files.
type Config struct {
	Name string `yaml:"name"`
	// routes.yml
	RouteMethods []string        `yaml:"route_methods"`
	RouteRules   []RuleDef       `yaml:"route_rules"`
	PrefixRules  []PrefixRuleDef `yaml:"prefix_rules"`
	// imports.yml
	ImportRules []ImportRuleDef `yaml:"import_rules"`
	// functions.yml
	FunctionRules  []FunctionRuleDef  `yaml:"function_rules"`
	ParameterRules []ParameterRuleDef `yaml:"parameter_rules"`
	// types.yml
	ClassRules []ClassRuleDef `yaml:"class_rules"`
	FieldRules []FieldRuleDef `yaml:"field_rules"`
	// control_flow.yml
	LoopRules       []LoopRuleDef       `yaml:"loop_rules"`
	AssignmentRules []AssignmentRuleDef `yaml:"assignment_rules"`
	ReturnRules     []ReturnRuleDef     `yaml:"return_rules"`
}

// Per-file structs used by LoadConfig — each maps to exactly one YAML file.

type RoutesFile struct {
	Name         string          `yaml:"name"`
	RouteMethods []string        `yaml:"route_methods"`
	PrefixRules  []PrefixRuleDef `yaml:"prefix_rules"`
	RouteRules   []RuleDef       `yaml:"route_rules"`
}

type ImportsFile struct {
	ImportRules []ImportRuleDef `yaml:"import_rules"`
}

type FunctionsFile struct {
	FunctionRules  []FunctionRuleDef  `yaml:"function_rules"`
	ParameterRules []ParameterRuleDef `yaml:"parameter_rules"`
}

type TypesFile struct {
	ClassRules []ClassRuleDef `yaml:"class_rules"`
	FieldRules []FieldRuleDef `yaml:"field_rules"`
}

type ControlFlowFile struct {
	LoopRules       []LoopRuleDef       `yaml:"loop_rules"`
	AssignmentRules []AssignmentRuleDef `yaml:"assignment_rules"`
	ReturnRules     []ReturnRuleDef     `yaml:"return_rules"`
}
