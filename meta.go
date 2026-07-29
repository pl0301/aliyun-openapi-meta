package aliyunopenapimeta

import (
	"bytes"
	"compress/gzip"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"sort"
	"strings"
	"sync"
	"time"
)

//go:generate go run ./cmd/packmeta -canonical canonical -out packed

//go:embed metadatas/products.json
var embeddedMetadatas embed.FS

//go:embed packed/canonical.index.json
var canonicalIndex []byte

//go:embed packed/canonical.blob.gz
var canonicalBlob []byte

// Metadatas exposes the bundled OpenAPI metadata as one in-binary filesystem.
// metadatas/products.json is embedded directly. canonical/... is stored as a
// packed gzip blob and decompressed per file on demand.
var Metadatas = newPackedMetadataFS(embeddedMetadatas, canonicalIndex, canonicalBlob)

type packedMetadataFS struct {
	metadatas embed.FS
	indexData []byte
	blobData  []byte

	once     sync.Once
	index    packedIndex
	indexErr error
}

type packedIndex struct {
	Files map[string]packedFile `json:"files"`
	Dirs  map[string][]string   `json:"dirs"`
}

type packedFile struct {
	Offset int64 `json:"offset"`
	Size   int64 `json:"size"`
	Raw    int64 `json:"raw"`
}

func newPackedMetadataFS(metadatas embed.FS, indexData, blobData []byte) *packedMetadataFS {
	return &packedMetadataFS{
		metadatas: metadatas,
		indexData: indexData,
		blobData:  blobData,
	}
}

func (p *packedMetadataFS) Open(name string) (fs.File, error) {
	name = cleanName(name)
	if name == "." || strings.HasPrefix(name, "metadatas/") {
		return p.metadatas.Open(name)
	}
	if p.isDir(name) {
		entries, err := p.ReadDir(name)
		if err != nil {
			return nil, err
		}
		return &packedDir{name: name, entries: entries}, nil
	}
	data, err := p.ReadFile(name)
	if err != nil {
		return nil, err
	}
	return &packedFileHandle{
		Reader: bytes.NewReader(data),
		info:   packedInfo{name: path.Base(name), size: int64(len(data))},
	}, nil
}

func (p *packedMetadataFS) ReadFile(name string) ([]byte, error) {
	name = cleanName(name)
	if strings.HasPrefix(name, "metadatas/") {
		return p.metadatas.ReadFile(name)
	}
	entry, ok, err := p.lookupFile(name)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fs.ErrNotExist
	}
	if entry.Offset < 0 || entry.Size < 0 || entry.Offset+entry.Size > int64(len(p.blobData)) {
		return nil, fmt.Errorf("packed canonical entry %s has invalid range", name)
	}
	r, err := gzip.NewReader(bytes.NewReader(p.blobData[entry.Offset : entry.Offset+entry.Size]))
	if err != nil {
		return nil, fmt.Errorf("open packed canonical %s failed: %w", name, err)
	}
	defer r.Close()
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read packed canonical %s failed: %w", name, err)
	}
	return data, nil
}

func (p *packedMetadataFS) ReadDir(name string) ([]fs.DirEntry, error) {
	name = cleanName(name)
	if name == "." || name == "metadatas" || strings.HasPrefix(name, "metadatas/") {
		return p.metadatas.ReadDir(name)
	}
	idx, err := p.loadIndex()
	if err != nil {
		return nil, err
	}
	names, ok := idx.Dirs[name]
	if !ok {
		return nil, fs.ErrNotExist
	}
	entries := make([]fs.DirEntry, 0, len(names))
	for _, child := range names {
		childPath := path.Join(name, child)
		_, isFile := idx.Files[childPath]
		_, isDir := idx.Dirs[childPath]
		entries = append(entries, packedDirEntry{
			name:  child,
			isDir: isDir && !isFile,
		})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	return entries, nil
}

func (p *packedMetadataFS) Stat(name string) (fs.FileInfo, error) {
	name = cleanName(name)
	if name == "." || strings.HasPrefix(name, "metadatas") {
		return fs.Stat(p.metadatas, name)
	}
	if entry, ok, err := p.lookupFile(name); err != nil {
		return nil, err
	} else if ok {
		return packedInfo{name: path.Base(name), size: entry.Raw}, nil
	}
	if p.isDir(name) {
		return packedInfo{name: path.Base(name), dir: true}, nil
	}
	return nil, fs.ErrNotExist
}

func (p *packedMetadataFS) lookupFile(name string) (packedFile, bool, error) {
	idx, err := p.loadIndex()
	if err != nil {
		return packedFile{}, false, err
	}
	entry, ok := idx.Files[name]
	return entry, ok, nil
}

func (p *packedMetadataFS) isDir(name string) bool {
	idx, err := p.loadIndex()
	if err != nil {
		return false
	}
	_, ok := idx.Dirs[name]
	return ok
}

func (p *packedMetadataFS) loadIndex() (*packedIndex, error) {
	p.once.Do(func() {
		p.index.Files = map[string]packedFile{}
		p.index.Dirs = map[string][]string{}
		if len(p.indexData) == 0 {
			p.indexErr = fs.ErrNotExist
			return
		}
		p.indexErr = json.Unmarshal(p.indexData, &p.index)
	})
	if p.indexErr != nil {
		return nil, p.indexErr
	}
	return &p.index, nil
}

func cleanName(name string) string {
	name = strings.TrimPrefix(path.Clean(strings.TrimPrefix(name, "/")), "/")
	if name == "" {
		return "."
	}
	return name
}

type packedFileHandle struct {
	*bytes.Reader
	info fs.FileInfo
}

func (f *packedFileHandle) Stat() (fs.FileInfo, error) { return f.info, nil }
func (f *packedFileHandle) Close() error               { return nil }

type packedDir struct {
	name    string
	offset  int
	entries []fs.DirEntry
}

func (d *packedDir) Stat() (fs.FileInfo, error) {
	return packedInfo{name: path.Base(d.name), dir: true}, nil
}

func (d *packedDir) Read([]byte) (int, error) {
	return 0, &fs.PathError{Op: "read", Path: d.name, Err: errors.New("is a directory")}
}
func (d *packedDir) Close() error { return nil }

func (d *packedDir) ReadDir(n int) ([]fs.DirEntry, error) {
	if d.offset >= len(d.entries) && n > 0 {
		return nil, io.EOF
	}
	if n <= 0 || d.offset+n > len(d.entries) {
		n = len(d.entries) - d.offset
	}
	out := d.entries[d.offset : d.offset+n]
	d.offset += n
	return out, nil
}

type packedDirEntry struct {
	name  string
	isDir bool
}

func (e packedDirEntry) Name() string { return e.name }
func (e packedDirEntry) IsDir() bool  { return e.isDir }
func (e packedDirEntry) Type() fs.FileMode {
	if e.isDir {
		return fs.ModeDir
	}
	return 0
}
func (e packedDirEntry) Info() (fs.FileInfo, error) {
	return packedInfo{name: e.name, dir: e.isDir}, nil
}

type packedInfo struct {
	name string
	size int64
	dir  bool
}

func (i packedInfo) Name() string {
	if i.name == "." || i.name == "" {
		return "."
	}
	return i.name
}
func (i packedInfo) Size() int64 { return i.size }
func (i packedInfo) Mode() fs.FileMode {
	if i.dir {
		return fs.ModeDir | 0555
	}
	return 0444
}
func (i packedInfo) ModTime() time.Time { return time.Time{} }
func (i packedInfo) IsDir() bool        { return i.dir }
func (i packedInfo) Sys() interface{}   { return nil }
