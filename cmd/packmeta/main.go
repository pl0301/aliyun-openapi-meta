package main

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

type indexFile struct {
	Files map[string]fileEntry `json:"files"`
	Dirs  map[string][]string  `json:"dirs"`
}

type fileEntry struct {
	Offset int64 `json:"offset"`
	Size   int64 `json:"size"`
	Raw    int64 `json:"raw"`
}

func main() {
	canonicalDir := flag.String("canonical", "canonical", "canonical metadata directory")
	outDir := flag.String("out", "packed", "output directory")
	flag.Parse()

	idx := indexFile{
		Files: map[string]fileEntry{},
		Dirs:  map[string][]string{},
	}
	dirChildren := map[string]map[string]bool{}
	addDir(dirChildren, ".")

	var blob bytes.Buffer
	err := filepath.WalkDir(*canonicalDir, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(*canonicalDir, p)
		if err != nil {
			return err
		}
		name := "canonical"
		if rel != "." {
			name = path.Join("canonical", filepath.ToSlash(rel))
		}
		if d.IsDir() {
			addDir(dirChildren, name)
			addChild(dirChildren, path.Dir(name), path.Base(name))
			return nil
		}

		raw, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		var compressed bytes.Buffer
		zw, err := gzip.NewWriterLevel(&compressed, gzip.BestCompression)
		if err != nil {
			return err
		}
		if _, err := zw.Write(raw); err != nil {
			return err
		}
		if err := zw.Close(); err != nil {
			return err
		}

		offset := int64(blob.Len())
		if _, err := blob.Write(compressed.Bytes()); err != nil {
			return err
		}
		idx.Files[name] = fileEntry{
			Offset: offset,
			Size:   int64(compressed.Len()),
			Raw:    int64(len(raw)),
		}
		addDir(dirChildren, path.Dir(name))
		addChild(dirChildren, path.Dir(name), path.Base(name))
		return nil
	})
	if err != nil {
		fatal(err)
	}

	for dir, children := range dirChildren {
		names := make([]string, 0, len(children))
		for child := range children {
			if child == "." || child == "" {
				continue
			}
			names = append(names, child)
		}
		sort.Strings(names)
		idx.Dirs[dir] = names
	}

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fatal(err)
	}
	indexBytes, err := json.Marshal(idx)
	if err != nil {
		fatal(err)
	}
	if err := os.WriteFile(filepath.Join(*outDir, "canonical.index.json"), indexBytes, 0o644); err != nil {
		fatal(err)
	}
	if err := os.WriteFile(filepath.Join(*outDir, "canonical.blob.gz"), blob.Bytes(), 0o644); err != nil {
		fatal(err)
	}
	fmt.Printf("packed %d files into %s (%s index, %s blob)\n",
		len(idx.Files), *outDir, human(len(indexBytes)), human(blob.Len()))
}

func addDir(dirs map[string]map[string]bool, dir string) {
	dir = clean(dir)
	if _, ok := dirs[dir]; !ok {
		dirs[dir] = map[string]bool{}
	}
	if dir == "." {
		return
	}
	parent := path.Dir(dir)
	addDir(dirs, parent)
	addChild(dirs, parent, path.Base(dir))
}

func addChild(dirs map[string]map[string]bool, dir, child string) {
	dir = clean(dir)
	addDir(dirs, dir)
	dirs[dir][child] = true
}

func clean(name string) string {
	name = strings.TrimPrefix(path.Clean(strings.TrimPrefix(name, "/")), "/")
	if name == "" {
		return "."
	}
	return name
}

func human(n int) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%dB", n)
	}
	div, exp := unit, 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
