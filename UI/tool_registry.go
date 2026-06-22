package ui

import (
	"fmt"
	"os/exec"
	"strings"
	"tree-sit/test/core"
)

// ToolStatus represents the validity state of a registered tool.
type ToolStatus int

const (
	StatusUnknown ToolStatus = iota
	StatusValid
	StatusInvalid
)

func (s ToolStatus) String() string {
	switch s {
	case StatusValid:
		return "valid"
	case StatusInvalid:
		return "invalid"
	default:
		return "unknown"
	}
}

// FieldKind distinguishes how a Field should be collected from the user.
type FieldKind int

const (
	FieldText FieldKind = iota
	FieldMultiSelect
	FieldKeyValueList
)

// Field describes a single piece of input a tool needs before it can run.
type Field struct {
	Key         string // map key passed into Run
	Label       string // shown to the user during step-by-step prompt
	Placeholder string
	Kind        FieldKind
	// Options lists the choices for FieldMultiSelect fields.
	Options []string
	// AutoFill, if non-nil, pulls this field's value from the active Project
	// instead of prompting the user. Returns ("", false) if not available.
	AutoFill func(p *Project) (string, bool)
}

// Tool wraps a core function as a runnable, validatable registry entry.
type Tool struct {
	Name        string
	Category    string
	Description string

	// RequiresProject means this tool cannot run until a project is active
	// (i.e. a repo has been cloned in this session).
	RequiresProject bool

	// Fields lists the inputs needed, in prompt order. Fields with a
	// successful AutoFill are skipped during step-by-step collection.
	Fields []Field

	// Validate reports whether the tool's prerequisites are met (e.g. binaries on PATH).
	Validate func() (ToolStatus, string)

	// Run executes the tool given fully-collected args.
	Run func(args map[string]string) (string, error)
}

// Project holds the active project context once a repo has been cloned,
// so downstream tools can auto-fill instead of re-asking.
type Project struct {
	Path    string
	Name    string
	RepoURL string
}

// binAvailable checks whether a binary exists on PATH.
func binAvailable(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func fromProjectPath(p *Project) (string, bool) {
	if p == nil || p.Path == "" {
		return "", false
	}
	return p.Path, true
}

func fromProjectName(p *Project) (string, bool) {
	if p == nil || p.Name == "" {
		return "", false
	}
	return p.Name, true
}

func fromProjectRepoURL(p *Project) (string, bool) {
	if p == nil || p.RepoURL == "" {
		return "", false
	}
	return p.RepoURL, true
}

// Registry returns the full set of tools wired to core package functions.
func Registry() []Tool {
	return []Tool{
		{
			Name:            "Scan Codebase",
			Category:        "Project Management",
			Description:     "Scans a repo path and writes the results to a labelled JSON file.",
			RequiresProject: false,
			Fields: []Field{
				{
					Key:         "Project Path",
					Label:       "Project Path",
					Placeholder: "/path/to/repo-or-folder-to-scan",
				},
				{
					Key:         "Output Label",
					Label:       "Output Label",
					Placeholder: "myapp-output",
				},
			},
			Validate: func() (ToolStatus, string) {
				return StatusValid, "ready"
			},
			Run: func(args map[string]string) (string, error) {
				projectPath := strings.TrimSpace(args["Project Path"])
				outputLabel := strings.TrimSpace(args["Output Label"])

				if projectPath == "" {
					return "", fmt.Errorf("Project Path is required")
				}

				outputPath, err := core.ScanCodebase(core.ScanCodebaseOptions{
					ProjectPath: projectPath,
					OutputLabel: outputLabel,
				})
				if err != nil {
					return "", err
				}

				return fmt.Sprintf("Scan Codebase completed → %s", outputPath), nil
			},
		},
	}
}

func splitCSV(s string) []string {
	var out []string
	cur := ""
	for _, r := range s {
		if r == ',' {
			if cur != "" {
				out = append(out, cur)
				cur = ""
			}
			continue
		}
		cur += string(r)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}
