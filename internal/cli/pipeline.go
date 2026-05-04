package cli

import "path/filepath"

func RunPipeline(cfg Config) ([]byte, []string, error) {
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
