package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	openapi "github.com/fox-gonic/openapi"
)

func RunPipeline(cfg Config) ([]byte, []string, error) {
	if cfg.RouteManifest != "" {
		return runManifestPipeline(cfg)
	}
	var entry Entry
	if cfg.ResolvedEntry != nil {
		// DiscoverEntry already loaded packages and validated the signature
		// during LoadConfig; reuse that result instead of reloading.
		entry = *cfg.ResolvedEntry
	} else {
		resolved, err := ResolveEntry(cfg.Workdir, cfg.Entry)
		if err != nil {
			return nil, nil, err
		}
		entry = resolved
	}
	var loader *ConfigLoader
	if entry.TakesConfig && cfg.EntryConfig.Loader != "" {
		resolved, err := ResolveConfigLoader(cfg.Workdir, cfg.EntryConfig.Loader, cfg.EntryConfig.Path)
		if err != nil {
			return nil, nil, err
		}
		loader = &resolved
	} else if entry.TakesConfig && cfg.EntryConfig.Path != "" {
		resolved, err := ResolveConfigLoaderFromEntry(cfg.Workdir, entry, cfg.EntryConfig.Path)
		if err != nil {
			return nil, nil, err
		}
		loader = &resolved
	}
	var hook *Hook
	if cfg.MetadataHook != "" {
		resolved, err := ResolveHook(cfg.Workdir, cfg.MetadataHook)
		if err != nil {
			return nil, nil, err
		}
		hook = &resolved
	}
	driverDir, err := WriteDriver(cfg, entry, hook, loader)
	if err != nil {
		return nil, nil, err
	}
	defer CleanupDriver(driverDir, cfg.KeepDriver)
	data, stderr, err := RunDriver(driverDir)
	if err != nil {
		return nil, warningLines(stderr), err
	}
	return data, warningLines(stderr), nil
}

func runManifestPipeline(cfg Config) ([]byte, []string, error) {
	data, err := os.ReadFile(cfg.RouteManifest)
	if err != nil {
		return nil, nil, fmt.Errorf("read route manifest %s: %w", cfg.RouteManifest, err)
	}
	var manifest openapi.RouteManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, nil, fmt.Errorf("parse route manifest %s: %w", cfg.RouteManifest, err)
	}
	if manifest.Version != openapi.RouteManifestVersion {
		return nil, nil, fmt.Errorf("unsupported route manifest version %q, want %q", manifest.Version, openapi.RouteManifestVersion)
	}
	warnings, err := enrichRouteManifestTypes(cfg.Workdir, &manifest, cfg.IncludeTestFiles)
	if err != nil {
		return nil, nil, err
	}
	opts, err := manifestOptions(cfg)
	if err != nil {
		return nil, nil, err
	}
	g := openapi.NewFromRouteManifest(manifest, opts...)
	spec := g.Spec()
	openapi.ApplySpecMetadata(spec, specMetadata(cfg))
	var out []byte
	if cfg.Format == FormatJSON {
		out, err = g.JSON()
	} else {
		out, err = g.YAML()
	}
	if err != nil {
		return nil, nil, fmt.Errorf("generate spec: %w", err)
	}
	return out, append(warnings, g.Warnings()...), nil
}

func manifestOptions(cfg Config) ([]openapi.Option, error) {
	opts := []openapi.Option{
		openapi.Info(defaultString(cfg.Info.Title, "Fox API"), defaultString(cfg.Info.Version, "0.0.0")),
	}
	for _, server := range cfg.Servers {
		if server.URL != "" {
			opts = append(opts, openapi.Server(server.URL))
		}
	}
	sources, err := absoluteSources(cfg.Workdir, cfg.Sources)
	if err != nil {
		return nil, err
	}
	if len(sources) > 0 {
		sourceOpts := []openapi.SourceOption{}
		if cfg.IncludeTestFiles {
			sourceOpts = append(sourceOpts, openapi.IncludeTestFiles())
		}
		opts = append(opts, openapi.Source(sources, sourceOpts...))
	}
	for _, name := range sortedSchemeNames(cfg.SecuritySchemes) {
		opts = append(opts, openapi.SecuritySchemeFromConfig(name, securitySchemeConfig(cfg.SecuritySchemes[name])))
	}
	filters, err := filtersFromConfig(cfg)
	if err != nil {
		return nil, err
	}
	if len(filters) > 0 {
		opts = append(opts, openapi.WithFilters(filters...))
	}
	return opts, nil
}

func filtersFromConfig(cfg Config) ([]openapi.Filter, error) {
	var filters []openapi.Filter
	for _, expression := range cfg.Filters {
		filter, err := openapi.FilterOperationExpression(expression)
		if err != nil {
			return nil, err
		}
		filters = append(filters, filter)
	}
	if cfg.PruneUnusedComponents {
		filters = append(filters, openapi.PruneUnusedComponents())
	}
	return filters, nil
}

func specMetadata(cfg Config) openapi.SpecMetadata {
	serverDescriptions := make([]string, len(cfg.Servers))
	for i, server := range cfg.Servers {
		serverDescriptions[i] = server.Description
	}
	tags := make([]openapi.SpecTag, 0, len(cfg.Tags))
	for _, tag := range cfg.Tags {
		out := openapi.SpecTag{Name: tag.Name, Description: tag.Description}
		if tag.ExternalDocs != nil {
			out.ExternalDocs = &openapi.SpecExternalDocs{
				Description: tag.ExternalDocs.Description,
				URL:         tag.ExternalDocs.URL,
			}
		}
		tags = append(tags, out)
	}
	return openapi.SpecMetadata{
		InfoDescription:    cfg.Info.Description,
		ServerDescriptions: serverDescriptions,
		Tags:               tags,
	}
}

func securitySchemeConfig(s Scheme) openapi.SecuritySchemeConfig {
	return openapi.SecuritySchemeConfig{
		Type:             s.Type,
		Description:      s.Description,
		Name:             s.Name,
		In:               s.In,
		Scheme:           s.Scheme,
		BearerFormat:     s.BearerFormat,
		OpenIDConnectURL: s.OpenIDConnectURL,
		Flows:            oauthFlowsConfig(s.Flows),
	}
}

func oauthFlowsConfig(flows *OAuthFlows) *openapi.OAuthFlowsConfig {
	if flows == nil {
		return nil
	}
	return &openapi.OAuthFlowsConfig{
		Implicit:          oauthFlowConfig(flows.Implicit),
		Password:          oauthFlowConfig(flows.Password),
		ClientCredentials: oauthFlowConfig(flows.ClientCredentials),
		AuthorizationCode: oauthFlowConfig(flows.AuthorizationCode),
	}
}

func oauthFlowConfig(flow *OAuthFlow) *openapi.OAuthFlowConfig {
	if flow == nil {
		return nil
	}
	return &openapi.OAuthFlowConfig{
		AuthorizationURL: flow.AuthorizationURL,
		TokenURL:         flow.TokenURL,
		RefreshURL:       flow.RefreshURL,
		Scopes:           flow.Scopes,
	}
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func ResolveOutputPath(cfg Config) string {
	// cfg.Out is fully resolved during LoadConfig — CLI flags are joined
	// against CWD and YAML values are joined against the config file's
	// directory. Callers can rely on the result being an absolute, ready
	// to write path.
	if filepath.IsAbs(cfg.Out) {
		return cfg.Out
	}
	abs, err := filepath.Abs(cfg.Out)
	if err != nil {
		return cfg.Out
	}
	return abs
}
