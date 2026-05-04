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
	MetadataHook     string            `yaml:"metadataHook"`
	EntryConfig      EntryConfig       `yaml:"entryConfig"`
	Workdir          string            `yaml:"workdir"`
	KeepDriver       bool              `yaml:"keepDriver"`
	Verbose          bool              `yaml:"verbose"`
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
	ConfigPath           string
	ConfigExplicit       bool
	Entry                string
	EntrySet             bool
	Out                  string
	OutSet               bool
	Format               string
	FormatSet            bool
	InfoTitle            string
	InfoTitleSet         bool
	InfoVersion          string
	InfoVersionSet       bool
	Servers              []string
	ServersSet           bool
	Sources              []string
	SourcesSet           bool
	IncludeTestFiles     bool
	IncludeTestFilesSet  bool
	MetadataHook         string
	MetadataHookSet      bool
	EntryConfigLoader    string
	EntryConfigLoaderSet bool
	EntryConfigPath      string
	EntryConfigPathSet   bool
	Workdir              string
	WorkdirSet           bool
	KeepDriver           bool
	KeepDriverSet        bool
	Verbose              bool
	VerboseSet           bool
}

func LoadConfig(overrides Overrides) (Config, error) {
	cfg := Config{
		ConfigPath:      defaultConfigPath,
		Out:             defaultOutPath,
		Sources:         []string{"./..."},
		Info:            InfoConfig{Title: "Fox API", Version: "0.0.0"},
		SecuritySchemes: map[string]Scheme{},
		Workdir:         ".",
	}
	if overrides.ConfigPath != "" {
		cfg.ConfigPath = overrides.ConfigPath
	}
	if overrides.WorkdirSet {
		cfg.Workdir = overrides.Workdir
	}
	if !filepath.IsAbs(cfg.ConfigPath) && cfg.Workdir != "" {
		cfg.ConfigPath = filepath.Join(cfg.Workdir, cfg.ConfigPath)
	}
	cfg.ConfigExplicit = overrides.ConfigExplicit
	if err := loadConfigFile(&cfg); err != nil {
		return Config{}, err
	}
	applyOverrides(&cfg, overrides)
	if cfg.SecuritySchemes == nil {
		cfg.SecuritySchemes = map[string]Scheme{}
	}
	abs, err := filepath.Abs(cfg.Workdir)
	if err != nil {
		return Config{}, fmt.Errorf("resolve workdir: %w", err)
	}
	cfg.Workdir = abs
	if cfg.Format == "" {
		cfg.Format = inferFormat(cfg.Out)
	}
	cfg.Format = strings.ToLower(cfg.Format)
	if cfg.Format != FormatYAML && cfg.Format != FormatJSON {
		return Config{}, fmt.Errorf("format must be yaml or json, got %q", cfg.Format)
	}
	if cfg.Entry == "" {
		return Config{}, errors.New("entry is required")
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

func loadConfigFile(cfg *Config) error {
	data, err := os.ReadFile(cfg.ConfigPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) && !cfg.ConfigExplicit {
			return nil
		}
		return fmt.Errorf("read config %s: %w", cfg.ConfigPath, err)
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return fmt.Errorf("parse config %s: %w", cfg.ConfigPath, err)
	}
	return nil
}

func applyOverrides(cfg *Config, o Overrides) {
	if o.EntrySet {
		cfg.Entry = o.Entry
	}
	if o.OutSet {
		cfg.Out = o.Out
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
		cfg.EntryConfig.Path = o.EntryConfigPath
	}
	if o.WorkdirSet {
		cfg.Workdir = o.Workdir
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
