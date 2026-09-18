package cli

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type ingestFile struct {
	Path  string
	Title string
}

const defaultIngestExtensions = "md,markdown,txt,go,py,js,jsx,ts,tsx,java,c,h,cpp,hpp,cs,rs,rb,php,sh,ps1,sql,html,css,scss,vue,svelte"

func collectIngestFiles(ctx context.Context, options ingestOptions) ([]ingestFile, error) {
	extensions := options.Extensions
	if len(extensions) == 0 {
		extensions = strings.Split(defaultIngestExtensions, ",")
	}
	allowed := make(map[string]bool)
	for _, extension := range extensions {
		extension = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(extension)), ".")
		if extension == "" || strings.ContainsAny(extension, "./\\*?[]") {
			return nil, fmt.Errorf("invalid extension %q; use names such as md,txt,go", extension)
		}
		allowed["."+extension] = true
	}
	var files []ingestFile
	seen := make(map[string]bool)
	add := func(path, title string) error {
		key := path
		if runtime.GOOS == "windows" {
			key = strings.ToLower(key)
		}
		if seen[key] {
			return nil
		}
		if len(files) >= 256 {
			return fmt.Errorf("directory selection exceeds 256 files; narrow the paths or use --ext")
		}
		seen[key] = true
		files = append(files, ingestFile{Path: path, Title: filepath.ToSlash(title)})
		return nil
	}
	for _, input := range options.Paths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		path, err := filepath.Abs(input)
		if err != nil {
			return nil, err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("symbolic links are not supported: %s", input)
		}
		if !info.IsDir() {
			if !info.Mode().IsRegular() {
				return nil, fmt.Errorf("expected a regular file: %s", input)
			}
			if err := add(path, filepath.Base(path)); err != nil {
				return nil, err
			}
			continue
		}
		err = filepath.WalkDir(path, func(current string, entry fs.DirEntry, walkErr error) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if walkErr != nil {
				return walkErr
			}
			if current == path {
				return nil
			}
			name := entry.Name()
			if entry.Type()&os.ModeSymlink != 0 {
				return nil
			}
			if entry.IsDir() {
				if !options.Recursive || strings.HasPrefix(name, ".") || skippedIngestDirectory(name) {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.HasPrefix(name, ".") || !entry.Type().IsRegular() || !allowed[strings.ToLower(filepath.Ext(name))] {
				return nil
			}
			relative, err := filepath.Rel(path, current)
			if err != nil {
				return err
			}
			return add(current, relative)
		})
		if err != nil {
			return nil, fmt.Errorf("scan %q: %w", input, err)
		}
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no matching files; check paths, --recursive and --ext")
	}
	return files, nil
}

func skippedIngestDirectory(name string) bool {
	switch strings.ToLower(name) {
	case "node_modules", "vendor", "dist", "build", "target", "__pycache__", "venv":
		return true
	default:
		return false
	}
}
