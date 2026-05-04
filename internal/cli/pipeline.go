package cli

import "path/filepath"

func RunPipeline(cfg Config) ([]byte, []string, error) {
	entry, err := ResolveEntry(cfg.Workdir, cfg.Entry)
	if err != nil {
		return nil, nil, err
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
	if filepath.IsAbs(cfg.Out) {
		return cfg.Out
	}
	return filepath.Join(cfg.Workdir, cfg.Out)
}
