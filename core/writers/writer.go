package writer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"tree-sit/test/core/worker"
)

// Manifest is the entry point for all partitioned output.
// Load this first, then fetch individual partition files as needed.
type Manifest struct {
	Generated  time.Time       `json:"generated"`
	Target     string          `json:"target"`
	Partitions []PartitionMeta `json:"partitions"`
}

type PartitionMeta struct {
	ID        string `json:"id"`        // language group name, e.g. "go"
	File      string `json:"file"`      // relative path to partition file
	FileCount int    `json:"fileCount"` // number of source files in this partition
	Functions int    `json:"functions"`
	Imports   int    `json:"imports"`
	Classes   int    `json:"classes"`
	Routes    int    `json:"routes"`
}

type partition struct {
	meta      PartitionMeta
	snapshots []worker.FileSnapshot
}

// Writer accumulates snapshots from workers and flushes them to partitioned files.
// Each language group gets its own output file. A manifest ties them together.
type Writer struct {
	outDir string
	target string
	mu     sync.Mutex
	parts  map[string]*partition
}

func New(outDir, target string) *Writer {
	return &Writer{
		outDir: outDir,
		target: target,
		parts:  make(map[string]*partition),
	}
}

// Add records a snapshot under the given language group.
// Safe to call concurrently from multiple workers.
func (w *Writer) Add(language string, snap worker.FileSnapshot) {
	w.mu.Lock()
	defer w.mu.Unlock()

	p, ok := w.parts[language]
	if !ok {
		p = &partition{
			meta: PartitionMeta{
				ID:   language,
				File: language + ".json",
			},
		}
		w.parts[language] = p
	}

	p.snapshots = append(p.snapshots, snap)
	p.meta.FileCount++
	p.meta.Functions += len(snap.Functions)
	p.meta.Imports += len(snap.Imports)
	p.meta.Classes += len(snap.Classes)
	p.meta.Routes += len(snap.Routes)
}

// Flush writes all partition files and the manifest to outDir.
// Call once after all workers have finished.
func (w *Writer) Flush() error {
	if err := os.MkdirAll(w.outDir, 0755); err != nil {
		return fmt.Errorf("creating output dir: %w", err)
	}

	manifest := Manifest{
		Generated: time.Now().UTC(),
		Target:    w.target,
	}

	for _, p := range w.parts {
		if err := writeJSON(filepath.Join(w.outDir, p.meta.File), p.snapshots); err != nil {
			return fmt.Errorf("writing partition %s: %w", p.meta.ID, err)
		}
		manifest.Partitions = append(manifest.Partitions, p.meta)
	}

	if err := writeJSON(filepath.Join(w.outDir, "manifest.json"), manifest); err != nil {
		return fmt.Errorf("writing manifest: %w", err)
	}

	return nil
}

func writeJSON(path string, v any) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
