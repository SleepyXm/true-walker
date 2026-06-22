package core

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"tree-sit/test/code-diagnostic/routes"
	"tree-sit/test/core/config"
	"tree-sit/test/core/scanner"
	"tree-sit/test/core/worker"
)

const (
	defaultRulesDir  = "yamls/code-specific"
	defaultOutputDir = "jsons"
	defaultOutput    = "output"
)

type ScanCodebaseOptions struct {
	ProjectPath string
	RulesDir    string
	ConfigPath  string
	OutputDir   string
	OutputLabel string
}

type DirGroup struct {
	Dir   string                `json:"dir"`
	Files []worker.FileSnapshot `json:"files"`
}

func ScanCodebase(opts ScanCodebaseOptions) (string, error) {
	opts.ProjectPath = strings.TrimSpace(opts.ProjectPath)
	opts.RulesDir = strings.TrimSpace(opts.RulesDir)
	opts.ConfigPath = strings.TrimSpace(opts.ConfigPath)
	opts.OutputDir = strings.TrimSpace(opts.OutputDir)
	opts.OutputLabel = strings.TrimSpace(opts.OutputLabel)

	if opts.ProjectPath == "" {
		return "", fmt.Errorf("Project Path is required")
	}

	if opts.RulesDir == "" {
		opts.RulesDir = defaultRulesDir
	}

	if opts.OutputDir == "" {
		opts.OutputDir = defaultOutputDir
	}

	if opts.OutputLabel == "" {
		opts.OutputLabel = defaultOutput
	}

	projectInfo, err := os.Stat(opts.ProjectPath)
	if err != nil {
		return "", fmt.Errorf("Project Path is invalid: %w", err)
	}

	if !projectInfo.IsDir() {
		return "", fmt.Errorf("Project Path must be a directory")
	}

	rulesInfo, err := os.Stat(opts.RulesDir)
	if err != nil {
		return "", fmt.Errorf("Rules Dir is invalid: %w", err)
	}

	if !rulesInfo.IsDir() {
		return "", fmt.Errorf("Rules Dir must be a directory")
	}

	cfg := config.Default()

	if opts.ConfigPath != "" {
		loaded, err := config.Load(opts.ConfigPath)
		if err != nil {
			log.Printf("config error: %v, using defaults", err)
		} else {
			cfg = loaded
		}
	}

	routeExtractor := routes.NewExtractor(cfg)

	groups, err := scanner.GroupByLanguage(opts.ProjectPath)
	if err != nil {
		return "", err
	}

	var (
		drainWg   sync.WaitGroup
		mu        sync.Mutex
		snapshots []worker.FileSnapshot
	)

	for _, group := range groups {
		drainWg.Add(1)

		w := worker.New(group, opts.RulesDir, routeExtractor)

		go func(w *worker.Worker) {
			var wg sync.WaitGroup
			wg.Add(1)
			w.Run(&wg)
			wg.Wait()
		}(w)

		go func(w *worker.Worker) {
			defer drainWg.Done()

			for snap := range w.Results {
				mu.Lock()
				snapshots = append(snapshots, snap)
				mu.Unlock()
			}
		}(w)
	}

	drainWg.Wait()

	grouped, err := groupSnapshotsByDir(opts.ProjectPath, snapshots)
	if err != nil {
		return "", err
	}

	outputPath := buildScanOutputPath(opts.OutputDir, opts.OutputLabel)

	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return "", err
	}

	outFile, err := os.Create(outputPath)
	if err != nil {
		return "", err
	}
	defer outFile.Close()

	enc := json.NewEncoder(outFile)
	enc.SetIndent("", "  ")

	if err := enc.Encode(grouped); err != nil {
		return "", err
	}

	return outputPath, nil
}

func buildScanOutputPath(outputDir string, outputLabel string) string {
	outputLabel = strings.TrimSpace(outputLabel)
	outputLabel = strings.TrimSuffix(outputLabel, ".json")

	outputLabel = strings.ReplaceAll(outputLabel, "/", "-")
	outputLabel = strings.ReplaceAll(outputLabel, "\\", "-")

	if outputLabel == "" {
		outputLabel = defaultOutput
	}

	return filepath.Join(outputDir, outputLabel+".json")
}

func groupSnapshotsByDir(targetDir string, snapshots []worker.FileSnapshot) ([]DirGroup, error) {
	byDir := make(map[string][]worker.FileSnapshot)

	for _, snap := range snapshots {
		rel, err := filepath.Rel(targetDir, filepath.Dir(snap.Path))
		if err != nil {
			return nil, err
		}

		byDir[rel] = append(byDir[rel], snap)
	}

	dirs := make([]string, 0, len(byDir))
	for dir := range byDir {
		dirs = append(dirs, dir)
	}

	sort.Strings(dirs)

	grouped := make([]DirGroup, 0, len(dirs))
	for _, dir := range dirs {
		grouped = append(grouped, DirGroup{
			Dir:   dir,
			Files: byDir[dir],
		})
	}

	return grouped, nil
}
