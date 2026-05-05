package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/mod/modfile"
)

// InitOptions configures fox-openapi.yaml initialization.
type InitOptions struct {
	Workdir    string
	ConfigPath string
	Entry      string
	Out        string
	Title      string
	Version    string
	Force      bool
}

// InitResult describes what InitConfig wrote so callers can surface it.
type InitResult struct {
	ConfigPath     string
	Entry          string
	AutoDiscovered bool
}

// InitConfig writes an initial fox-openapi.yaml.
func InitConfig(opts InitOptions) (InitResult, error) {
	if opts.Workdir == "" {
		opts.Workdir = "."
	}
	workdir, err := filepath.Abs(opts.Workdir)
	if err != nil {
		return InitResult{}, fmt.Errorf("resolve workdir: %w", err)
	}
	if opts.ConfigPath == "" {
		opts.ConfigPath = defaultConfigPath
	}
	configPath := opts.ConfigPath
	if !filepath.IsAbs(configPath) {
		configPath = filepath.Join(workdir, configPath)
	}
	if !opts.Force {
		if _, err := os.Stat(configPath); err == nil {
			return InitResult{}, fmt.Errorf("%s already exists; pass --force to overwrite", configPath)
		} else if !errors.Is(err, os.ErrNotExist) {
			return InitResult{}, fmt.Errorf("stat %s: %w", configPath, err)
		}
	}
	autoDiscovered := false
	if opts.Entry == "" {
		entry, err := DiscoverEntry(workdir, []string{"./..."})
		if err != nil {
			return InitResult{}, fmt.Errorf("entry not provided and auto-discovery failed: %w", err)
		}
		opts.Entry = entry.ImportPath + "." + entry.FuncName
		autoDiscovered = true
	}
	if opts.Out == "" {
		opts.Out = defaultOutPath
	}
	if opts.Title == "" {
		opts.Title = "Fox API"
	}
	if opts.Version == "" {
		opts.Version = "0.0.0"
	}

	entry, err := normalizeInitEntry(workdir, opts.Entry)
	if err != nil {
		return InitResult{}, err
	}
	if err := WriteAtomic(configPath, []byte(renderInitConfig(entry, opts))); err != nil {
		return InitResult{}, err
	}
	return InitResult{ConfigPath: configPath, Entry: entry, AutoDiscovered: autoDiscovered}, nil
}

func normalizeInitEntry(workdir, entry string) (string, error) {
	modulePath, err := modulePath(workdir)
	if err != nil {
		return "", err
	}
	modulePath = strings.TrimRight(modulePath, "/")
	entry = strings.TrimPrefix(entry, "./")
	if entry == modulePath || strings.HasPrefix(entry, modulePath+"/") || strings.HasPrefix(entry, modulePath+".") {
		return entry, nil
	}
	if !strings.Contains(entry, "/") && strings.Contains(entry, ".") {
		return modulePath + "/" + entry, nil
	}
	if strings.Contains(entry, "/") {
		return modulePath + "/" + entry, nil
	}
	return modulePath + "." + entry, nil
}

func modulePath(workdir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(workdir, "go.mod"))
	if err != nil {
		return "", fmt.Errorf("read go.mod: %w", err)
	}
	file, err := modfile.Parse("go.mod", data, nil)
	if err != nil {
		return "", fmt.Errorf("parse go.mod: %w", err)
	}
	if file.Module == nil || file.Module.Mod.Path == "" {
		return "", errors.New("go.mod module path is required")
	}
	return file.Module.Mod.Path, nil
}

func renderInitConfig(entry string, opts InitOptions) string {
	var b strings.Builder
	fmt.Fprintf(&b, "entry: %q\n", entry)
	fmt.Fprintf(&b, "out: %q\n", opts.Out)
	if inferFormat(opts.Out) == FormatJSON {
		b.WriteString("format: json\n")
	}
	b.WriteString("sources:\n")
	b.WriteString("  - ./...\n")
	b.WriteString("info:\n")
	fmt.Fprintf(&b, "  title: %q\n", opts.Title)
	fmt.Fprintf(&b, "  version: %q\n", opts.Version)
	return b.String()
}
