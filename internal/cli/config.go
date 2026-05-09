package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/goccy/go-yaml"
)

const (
	defaultConfigPath = "fox-openapi.yaml"
	defaultOutPath    = "api/openapi.yaml"
)

const (
	// FormatYAML emits the spec as YAML.
	FormatYAML = "yaml"
	// FormatJSON emits the spec as JSON.
	FormatJSON = "json"
)

type Config struct {
	ConfigPath       string
	ConfigExplicit   bool
	Entry            string            `yaml:"entry"`
	Out              string            `yaml:"out"`
	Format           string            `yaml:"format"`
	Sources          []string          `yaml:"sources"`
	IncludeTestFiles bool              `yaml:"includeTestFiles"`
	Info             InfoConfig        `yaml:"info"`
	Servers          []ServerConfig    `yaml:"servers"`
	Tags             []TagConfig       `yaml:"tags"`
	SecuritySchemes  map[string]Scheme `yaml:"securitySchemes"`
	Filters               []string    `yaml:"filters"`
	PruneUnusedComponents bool        `yaml:"pruneUnusedComponents"`
	MetadataHook          string      `yaml:"metadataHook"`
	EntryConfig           EntryConfig `yaml:"entryConfig"`
	RouteManifest         string      `yaml:"routeManifest"`
	Workdir               string      `yaml:"workdir"`
	KeepDriver            bool        `yaml:"keepDriver"`
	Verbose               bool        `yaml:"verbose"`
	// EntryAutoDiscovered is true when the entry was filled in by
	// DiscoverEntry rather than the config file or CLI flags. CLI commands
	// use this to surface "entry: ..." back to the user so the auto-pick is
	// never invisible.
	EntryAutoDiscovered bool `yaml:"-"`
	// EntryDiscoveryScope narrows where DiscoverEntry looks for an entry
	// function. Filled in by the CLI from the optional positional path
	// argument. It does NOT affect Sources (comment extraction), which is
	// kept module-wide so descriptions on referenced types are not lost
	// when the entry lives in a sub-tree.
	EntryDiscoveryScope []string `yaml:"-"`
	// ResolvedEntry caches the *Entry produced by DiscoverEntry so the
	// pipeline can skip a second packages.Load on the auto-discovery path.
	// nil when the entry was set explicitly and still needs ResolveEntry.
	ResolvedEntry *Entry `yaml:"-"`
}

type InfoConfig struct {
	Title       string `yaml:"title"`
	Version     string `yaml:"version"`
	Description string `yaml:"description"`
}

type ServerConfig struct {
	URL         string `yaml:"url"`
	Description string `yaml:"description"`
}

type ExternalDocsConfig struct {
	URL         string `yaml:"url"`
	Description string `yaml:"description"`
}

type TagConfig struct {
	Name         string              `yaml:"name"`
	Description  string              `yaml:"description"`
	ExternalDocs *ExternalDocsConfig `yaml:"externalDocs"`
}

type EntryConfig struct {
	Loader string `yaml:"loader"`
	Path   string `yaml:"path"`
}

type Scheme struct {
	Type             string      `yaml:"type"`
	Description      string      `yaml:"description"`
	Name             string      `yaml:"name"`
	In               string      `yaml:"in"`
	Scheme           string      `yaml:"scheme"`
	BearerFormat     string      `yaml:"bearerFormat"`
	Flows            *OAuthFlows `yaml:"flows"`
	OpenIDConnectURL string      `yaml:"openIdConnectUrl"`
}

type OAuthFlows struct {
	Implicit          *OAuthFlow `yaml:"implicit"`
	Password          *OAuthFlow `yaml:"password"`
	ClientCredentials *OAuthFlow `yaml:"clientCredentials"`
	AuthorizationCode *OAuthFlow `yaml:"authorizationCode"`
}

type OAuthFlow struct {
	AuthorizationURL string            `yaml:"authorizationUrl"`
	TokenURL         string            `yaml:"tokenUrl"`
	RefreshURL       string            `yaml:"refreshUrl"`
	Scopes           map[string]string `yaml:"scopes"`
}

type Overrides struct {
	ConfigPath               string
	ConfigExplicit           bool
	Entry                    string
	EntrySet                 bool
	Out                      string
	OutSet                   bool
	Format                   string
	FormatSet                bool
	InfoTitle                string
	InfoTitleSet             bool
	InfoVersion              string
	InfoVersionSet           bool
	Servers                  []string
	ServersSet               bool
	Sources                  []string
	SourcesSet               bool
	IncludeTestFiles         bool
	IncludeTestFilesSet      bool
	MetadataHook             string
	MetadataHookSet          bool
	EntryConfigLoader        string
	EntryConfigLoaderSet     bool
	EntryConfigPath          string
	EntryConfigPathSet       bool
	RouteManifest            string
	RouteManifestSet         bool
	Filters                  []string
	FiltersSet               bool
	PruneUnusedComponents    bool
	PruneUnusedComponentsSet bool
	Workdir                  string
	WorkdirSet               bool
	KeepDriver               bool
	KeepDriverSet            bool
	Verbose                  bool
	VerboseSet               bool
	// EntryDiscoveryScope is set by the CLI from the optional positional
	// path argument. It limits where DiscoverEntry searches but does not
	// affect Sources (comment extraction).
	EntryDiscoveryScope    []string
	EntryDiscoveryScopeSet bool
}

func LoadConfig(overrides Overrides) (Config, error) {
	// Resolve CWD up front: CLI flags and the default config-file lookup
	// resolve against this. Everything that comes from the YAML resolves
	// against the YAML file's directory instead, mirroring how tsconfig /
	// jest.config / pyproject treat their own relative paths.
	cwd, err := os.Getwd()
	if err != nil {
		return Config{}, fmt.Errorf("get working directory: %w", err)
	}

	cfg := Config{
		Sources:         []string{"./..."},
		Info:            InfoConfig{Title: "Fox API", Version: "0.0.0"},
		SecuritySchemes: map[string]Scheme{},
		Workdir:         cwd,
	}

	// Resolve --config relative to CWD (not workdir).
	configPath := defaultConfigPath
	if overrides.ConfigPath != "" {
		configPath = overrides.ConfigPath
	}
	if !filepath.IsAbs(configPath) {
		configPath = filepath.Join(cwd, configPath)
	}
	cfg.ConfigPath = configPath
	cfg.ConfigExplicit = overrides.ConfigExplicit

	// Read the YAML into a side struct so we can tell which fields came from
	// it — those resolve relative to the config file's directory, while CLI
	// overrides resolve relative to CWD.
	fileCfg, fileLoaded, err := readConfigFile(cfg.ConfigPath, cfg.ConfigExplicit)
	if err != nil {
		return Config{}, err
	}
	configDir := filepath.Dir(cfg.ConfigPath)

	// Merge YAML in. Path-bearing fields are resolved against configDir.
	if fileLoaded {
		mergeFromFile(&cfg, fileCfg, configDir)
	}

	// Apply CLI overrides last. Path-bearing fields resolve against CWD.
	applyOverrides(&cfg, overrides, cwd)

	// Defaults if neither YAML nor CLI provided.
	if cfg.Out == "" {
		cfg.Out = filepath.Join(cwd, defaultOutPath)
	}

	if cfg.SecuritySchemes == nil {
		cfg.SecuritySchemes = map[string]Scheme{}
	}

	// Workdir is always absolute by this point — applyOverrides resolved a
	// CLI --workdir, and the constructor seeded cwd otherwise.
	if !filepath.IsAbs(cfg.Workdir) {
		cfg.Workdir = filepath.Join(cwd, cfg.Workdir)
	}
	cfg.Workdir = filepath.Clean(cfg.Workdir)

	if cfg.Format == "" {
		cfg.Format = inferFormat(cfg.Out)
	}
	cfg.Format = strings.ToLower(cfg.Format)
	if cfg.Format != FormatYAML && cfg.Format != FormatJSON {
		return Config{}, fmt.Errorf("format must be yaml or json, got %q", cfg.Format)
	}
	if cfg.Entry == "" && cfg.RouteManifest == "" {
		// Discovery scope: explicit position arg > Sources > "./..." default.
		// Comment extraction (Sources) stays module-wide so referenced types
		// keep their field docs.
		discoveryScope := cfg.EntryDiscoveryScope
		if len(discoveryScope) == 0 {
			discoveryScope = cfg.Sources
		}
		entry, err := DiscoverEntry(cfg.Workdir, discoveryScope)
		if err != nil {
			return Config{}, err
		}
		cfg.Entry = entry.ImportPath + "." + entry.FuncName
		cfg.EntryAutoDiscovered = true
		resolved := entry
		cfg.ResolvedEntry = &resolved
	}
	if err := validateSecuritySchemes(cfg.SecuritySchemes); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func validateSecuritySchemes(schemes map[string]Scheme) error {
	for name, scheme := range schemes {
		if name == "" {
			return errors.New("security scheme name is required")
		}
		switch scheme.Type {
		case "apiKey":
			if scheme.Name == "" {
				return fmt.Errorf("security scheme %q: name is required for apiKey", name)
			}
			if scheme.In != "query" && scheme.In != "header" && scheme.In != "cookie" {
				return fmt.Errorf("security scheme %q: in must be query, header, or cookie for apiKey", name)
			}
		case "http":
			if scheme.Scheme == "" {
				return fmt.Errorf("security scheme %q: scheme is required for http", name)
			}
		case "oauth2":
			if scheme.Flows == nil || !hasOAuthFlow(scheme.Flows) {
				return fmt.Errorf("security scheme %q: at least one OAuth2 flow is required", name)
			}
		case "openIdConnect":
			if scheme.OpenIDConnectURL == "" {
				return fmt.Errorf("security scheme %q: openIdConnectUrl is required for openIdConnect", name)
			}
		default:
			return fmt.Errorf("security scheme %q: type must be apiKey, http, oauth2, or openIdConnect", name)
		}
	}
	return nil
}

func hasOAuthFlow(flows *OAuthFlows) bool {
	return flows.Implicit != nil ||
		flows.Password != nil ||
		flows.ClientCredentials != nil ||
		flows.AuthorizationCode != nil
}

func readConfigFile(path string, explicit bool) (Config, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) && !explicit {
			return Config{}, false, nil
		}
		return Config{}, false, fmt.Errorf("read config %s: %w", path, err)
	}
	var fileCfg Config
	if err := yaml.Unmarshal(data, &fileCfg); err != nil {
		return Config{}, false, fmt.Errorf("parse config %s: %w", path, err)
	}
	return fileCfg, true, nil
}

// mergeFromFile copies YAML-sourced fields onto cfg, resolving any path-bearing
// values relative to the config file's directory.
func mergeFromFile(cfg *Config, fileCfg Config, configDir string) {
	if fileCfg.Entry != "" {
		cfg.Entry = fileCfg.Entry
	}
	if fileCfg.Out != "" {
		cfg.Out = resolveRelative(configDir, fileCfg.Out)
	}
	if fileCfg.Format != "" {
		cfg.Format = fileCfg.Format
	}
	if len(fileCfg.Sources) > 0 {
		cfg.Sources = append([]string(nil), fileCfg.Sources...)
	}
	cfg.IncludeTestFiles = fileCfg.IncludeTestFiles
	if fileCfg.Info.Title != "" {
		cfg.Info.Title = fileCfg.Info.Title
	}
	if fileCfg.Info.Version != "" {
		cfg.Info.Version = fileCfg.Info.Version
	}
	if fileCfg.Info.Description != "" {
		cfg.Info.Description = fileCfg.Info.Description
	}
	if len(fileCfg.Servers) > 0 {
		cfg.Servers = fileCfg.Servers
	}
	if len(fileCfg.Tags) > 0 {
		cfg.Tags = fileCfg.Tags
	}
	if len(fileCfg.SecuritySchemes) > 0 {
		cfg.SecuritySchemes = fileCfg.SecuritySchemes
	}
	if len(fileCfg.Filters) > 0 {
		cfg.Filters = append([]string(nil), fileCfg.Filters...)
	}
	cfg.PruneUnusedComponents = fileCfg.PruneUnusedComponents
	if fileCfg.MetadataHook != "" {
		cfg.MetadataHook = fileCfg.MetadataHook
	}
	if fileCfg.EntryConfig.Loader != "" {
		cfg.EntryConfig.Loader = fileCfg.EntryConfig.Loader
	}
	if fileCfg.EntryConfig.Path != "" {
		cfg.EntryConfig.Path = resolveRelative(configDir, fileCfg.EntryConfig.Path)
	}
	if fileCfg.RouteManifest != "" {
		cfg.RouteManifest = resolveRelative(configDir, fileCfg.RouteManifest)
	}
	if fileCfg.Workdir != "" {
		cfg.Workdir = resolveRelative(configDir, fileCfg.Workdir)
	}
	cfg.KeepDriver = fileCfg.KeepDriver
	cfg.Verbose = fileCfg.Verbose
}

// resolveRelative returns abs unchanged if it is already absolute,
// otherwise resolves it against base.
func resolveRelative(base, p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(base, p)
}

// applyOverrides merges CLI flag values on top of cfg. Path-bearing flags
// resolve relative to cwd (the directory the user actually invoked the CLI
// from), not the config file's directory or workdir.
func applyOverrides(cfg *Config, o Overrides, cwd string) {
	if o.EntrySet {
		cfg.Entry = o.Entry
	}
	if o.OutSet {
		cfg.Out = resolveRelative(cwd, o.Out)
	}
	if o.FormatSet {
		cfg.Format = o.Format
	}
	if o.InfoTitleSet {
		cfg.Info.Title = o.InfoTitle
	}
	if o.InfoVersionSet {
		cfg.Info.Version = o.InfoVersion
	}
	if o.ServersSet {
		cfg.Servers = make([]ServerConfig, 0, len(o.Servers))
		for _, url := range o.Servers {
			if url != "" {
				cfg.Servers = append(cfg.Servers, ServerConfig{URL: url})
			}
		}
	}
	if o.SourcesSet {
		cfg.Sources = append([]string(nil), o.Sources...)
	}
	if o.IncludeTestFilesSet {
		cfg.IncludeTestFiles = o.IncludeTestFiles
	}
	if o.MetadataHookSet {
		cfg.MetadataHook = o.MetadataHook
	}
	if o.EntryConfigLoaderSet {
		cfg.EntryConfig.Loader = o.EntryConfigLoader
	}
	if o.EntryConfigPathSet {
		cfg.EntryConfig.Path = resolveRelative(cwd, o.EntryConfigPath)
	}
	if o.RouteManifestSet {
		cfg.RouteManifest = resolveRelative(cwd, o.RouteManifest)
	}
	if o.FiltersSet {
		cfg.Filters = append([]string(nil), o.Filters...)
	}
	if o.PruneUnusedComponentsSet {
		cfg.PruneUnusedComponents = o.PruneUnusedComponents
	}
	if o.WorkdirSet {
		cfg.Workdir = resolveRelative(cwd, o.Workdir)
	}
	if o.EntryDiscoveryScopeSet {
		cfg.EntryDiscoveryScope = append([]string(nil), o.EntryDiscoveryScope...)
	}
	if o.KeepDriverSet {
		cfg.KeepDriver = o.KeepDriver
	}
	if o.VerboseSet {
		cfg.Verbose = o.Verbose
	}
}

func inferFormat(out string) string {
	if strings.EqualFold(filepath.Ext(out), ".json") {
		return FormatJSON
	}
	return FormatYAML
}
