package main

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/fox-gonic/openapi/internal/cli"
)

// version is overridable at link time via -ldflags "-X main.version=...".
// When unset (the common `go install module@vX.Y.Z` path), resolveVersion
// falls back to runtime/debug.ReadBuildInfo so the binary reports the tag
// the user actually installed.
var version = ""

type exitError struct {
	code int
	err  error
}

func (e exitError) Error() string {
	if e.err == nil {
		return ""
	}
	return e.err.Error()
}

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	cmd := newRootCommand()
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		var exitErr exitError
		if errors.As(err, &exitErr) {
			if exitErr.err != nil {
				fmt.Fprintln(os.Stderr, exitErr.err)
			}
			return exitErr.code
		}
		return handleError(err)
	}
	return 0
}

func newRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "fox-openapi",
		Short:         "Generate OpenAPI specs for Fox applications",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.AddCommand(newInitCommand())
	cmd.AddCommand(newGenerateCommand())
	cmd.AddCommand(newCheckCommand())
	cmd.AddCommand(newServeCommand())
	cmd.AddCommand(newVersionCommand())
	return cmd
}

func newInitCommand() *cobra.Command {
	opts := cli.InitOptions{ConfigPath: "fox-openapi.yaml", Out: "api/openapi.yaml", Title: "Fox API", Version: "0.0.0", Workdir: "."}
	cmd := &cobra.Command{
		Use:   "init [path]",
		Short: "Create a fox-openapi.yaml config file",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 && args[0] != "" {
				info, err := os.Stat(args[0])
				if err != nil {
					return exitError{code: cli.ExitUsage, err: fmt.Errorf("stat %s: %w", args[0], err)}
				}
				if !info.IsDir() {
					return exitError{code: cli.ExitUsage, err: fmt.Errorf("%s is not a directory", args[0])}
				}
				opts.Workdir = args[0]
			}
			result, err := cli.InitConfig(opts)
			if err != nil {
				return exitError{code: cli.ExitUsage, err: err}
			}
			fmt.Printf("created %s\n", result.ConfigPath)
			fmt.Printf("  entry: %s%s\n", result.Entry, autoTag(result.AutoDiscovered))
			return nil
		},
	}
	cmd.Flags().StringVar(&opts.ConfigPath, "config", opts.ConfigPath, "config file path")
	cmd.Flags().StringVar(&opts.Entry, "entry", "", "entry function")
	cmd.Flags().StringVar(&opts.Out, "out", opts.Out, "output path")
	cmd.Flags().StringVar(&opts.Title, "title", opts.Title, "OpenAPI info title")
	cmd.Flags().StringVar(&opts.Version, "version", opts.Version, "OpenAPI info version")
	cmd.Flags().StringVar(&opts.Workdir, "workdir", opts.Workdir, "user project root")
	cmd.Flags().BoolVar(&opts.Force, "force", false, "overwrite an existing config")
	cmd.Flags().SortFlags = false
	return cmd
}

func newGenerateCommand() *cobra.Command {
	opts := newCommonOptions()
	cmd := &cobra.Command{
		Use:   "generate [path]",
		Short: "Generate an OpenAPI document",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			markOverridesFromFlags(opts, cmd.Flags())
			applyPositionalPath(opts, args)
			cfg, err := configFromOptions(opts)
			if err != nil {
				return exitError{code: cli.ExitUsage, err: err}
			}
			data, warnings, err := cli.RunPipeline(cfg)
			for _, warning := range warnings {
				fmt.Fprintln(os.Stderr, warning)
			}
			if err != nil {
				return err
			}
			out := cli.ResolveOutputPath(cfg)
			if err := cli.WriteAtomic(out, data); err != nil {
				return exitError{code: cli.ExitWriteFailed, err: fmt.Errorf("write %s: %w", out, err)}
			}
			fmt.Printf("wrote %s (%s, %d bytes)\n", out, strings.ToUpper(cfg.Format), len(data))
			fmt.Printf("  entry: %s%s\n", cfg.Entry, autoTag(cfg.EntryAutoDiscovered))
			return nil
		},
	}
	bindCommonFlags(cmd.Flags(), opts)
	return cmd
}

func newCheckCommand() *cobra.Command {
	opts := newCommonOptions()
	cmd := &cobra.Command{
		Use:   "check [path]",
		Short: "Verify the committed OpenAPI document is up to date",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			markOverridesFromFlags(opts, cmd.Flags())
			applyPositionalPath(opts, args)
			cfg, err := configFromOptions(opts)
			if err != nil {
				return exitError{code: cli.ExitUsage, err: err}
			}
			data, warnings, err := cli.RunPipeline(cfg)
			for _, warning := range warnings {
				fmt.Fprintln(os.Stderr, warning)
			}
			if err != nil {
				return err
			}
			out := cli.ResolveOutputPath(cfg)
			if err := cli.CheckDrift(out, data); err != nil {
				if errors.Is(err, cli.ErrDrift) {
					return exitError{code: cli.ExitDrift, err: fmt.Errorf("%s is out of date. Run `fox-openapi generate` to refresh", out)}
				}
				return exitError{code: cli.ExitWriteFailed, err: fmt.Errorf("check %s: %w", out, err)}
			}
			fmt.Printf("%s is up to date.\n", out)
			fmt.Printf("  entry: %s%s\n", cfg.Entry, autoTag(cfg.EntryAutoDiscovered))
			return nil
		},
	}
	bindCommonFlags(cmd.Flags(), opts)
	return cmd
}

func newServeCommand() *cobra.Command {
	opts := newCommonOptions()
	serveCfg := cli.ServeConfig{Addr: "127.0.0.1:8765", UIs: []string{"swagger", "scalar", "redoc"}, Watch: true}
	var ui repeatedFlag
	cmd := &cobra.Command{
		Use:   "serve [path]",
		Short: "Serve the generated spec and offline docs UI",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			markOverridesFromFlags(opts, cmd.Flags())
			applyPositionalPath(opts, args)
			cfg, err := configFromOptions(opts)
			if err != nil {
				return exitError{code: cli.ExitUsage, err: err}
			}
			if ui.set {
				serveCfg.UIs = ui.values
			}
			if _, _, err := net.SplitHostPort(serveCfg.Addr); err != nil && strings.HasPrefix(serveCfg.Addr, ":") {
				serveCfg.Addr = "127.0.0.1" + serveCfg.Addr
			}
			if err := cli.Serve(cfg, serveCfg); err != nil {
				return exitError{code: cli.ExitUsage, err: fmt.Errorf("serve: %w", err)}
			}
			return nil
		},
	}
	bindCommonFlags(cmd.Flags(), opts)
	cmd.Flags().StringVar(&serveCfg.Addr, "addr", serveCfg.Addr, "HTTP listen address")
	cmd.Flags().Var(&ui, "ui", "UI to serve: swagger, scalar, or redoc")
	cmd.Flags().BoolVar(&serveCfg.Watch, "watch", serveCfg.Watch, "watch .go files and regenerate")
	cmd.Flags().BoolVar(&serveCfg.Open, "open", false, "open browser")
	return cmd
}

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print fox-openapi version",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(resolveVersion())
		},
	}
}

// resolveVersion picks the most authoritative version string available:
//  1. -ldflags "-X main.version=..." (release builds, CI artifacts).
//  2. Module version from runtime/debug — populated when the binary was
//     installed via `go install module@vX.Y.Z` or built from a tagged
//     module cache.
//  3. VCS revision (+dirty) recorded in build info for source builds.
//  4. "dev" for builds with no metadata at all.
func resolveVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		info = nil
	}
	return pickVersion(version, info)
}

// pickVersion is the pure core of resolveVersion, factored out so tests can
// drive it with synthetic inputs instead of mutating the package-level
// `version` symbol or relying on the test binary's build info.
func pickVersion(override string, info *debug.BuildInfo) string {
	if override != "" {
		return override
	}
	if info == nil {
		return "dev"
	}
	if v := info.Main.Version; v != "" && v != "(devel)" {
		return v
	}
	var revision, modified string
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			revision = s.Value
		case "vcs.modified":
			modified = s.Value
		}
	}
	if revision != "" {
		short := revision
		if len(short) > 12 {
			short = short[:12]
		}
		if modified == "true" {
			return short + "+dirty"
		}
		return short
	}
	return "dev"
}

type commonOptions struct {
	overrides *cli.Overrides
	sources   repeatedFlag
	servers   repeatedFlag
}

func newCommonOptions() *commonOptions {
	return &commonOptions{overrides: &cli.Overrides{}}
}

func configFromOptions(opts *commonOptions) (cli.Config, error) {
	opts.overrides.Sources = opts.sources.values
	opts.overrides.SourcesSet = opts.sources.set
	opts.overrides.Servers = opts.servers.values
	opts.overrides.ServersSet = opts.servers.set
	return cli.LoadConfig(*opts.overrides)
}

func bindCommonFlags(flags *pflag.FlagSet, opts *commonOptions) {
	o := opts.overrides
	flags.StringVar(&o.ConfigPath, "config", "fox-openapi.yaml", "config file path")
	flags.StringVar(&o.Entry, "entry", "", "entry function")
	flags.StringVar(&o.Out, "out", "api/openapi.yaml", "output path")
	flags.StringVar(&o.Format, "format", "", "yaml or json")
	flags.StringVar(&o.InfoTitle, "title", "", "OpenAPI info title")
	flags.StringVar(&o.InfoVersion, "version", "", "OpenAPI info version")
	flags.Var(&opts.servers, "server", "OpenAPI server URL")
	flags.Var(&opts.sources, "source", "source path")
	flags.BoolVar(&o.IncludeTestFiles, "include-test-files", false, "include *_test.go")
	flags.StringVar(&o.MetadataHook, "metadata-hook", "", "metadata hook")
	flags.StringVar(&o.EntryConfigLoader, "entry-config-loader", "", "entry config loader")
	flags.StringVar(&o.EntryConfigPath, "entry-config-path", "", "entry config path")
	flags.StringVar(&o.Workdir, "workdir", ".", "user project root")
	flags.BoolVar(&o.KeepDriver, "keep-driver", false, "keep generated driver")
	flags.BoolVar(&o.Verbose, "verbose", false, "verbose output")
	flags.SortFlags = false
}

func markOverridesFromFlags(opts *commonOptions, flags *pflag.FlagSet) {
	flags.Visit(func(f *pflag.Flag) { markOverride(opts.overrides, f.Name) })
}

// applyPositionalPath maps the optional positional [path] argument onto
// EntryDiscoveryScope. The position argument narrows where the CLI looks
// for the entry function — it does NOT narrow Sources (comment extraction),
// because handler/field docs commonly live in sub-packages outside the
// directory that contains the entry. Letting Sources default to the whole
// module ensures descriptions on referenced types make it into the spec.
//
//   - "./internal/aone"      → EntryDiscoveryScope=["./internal/aone"]
//   - "internal/aone"        → EntryDiscoveryScope=["./internal/aone/..."]
//   - "github.com/x/y"       → EntryDiscoveryScope=["github.com/x/y"]
func applyPositionalPath(opts *commonOptions, args []string) {
	if len(args) == 0 {
		return
	}
	path := args[0]
	if path == "" {
		return
	}
	opts.overrides.EntryDiscoveryScope = []string{normalizeSourcePattern(path)}
	opts.overrides.EntryDiscoveryScopeSet = true
}

// normalizeSourcePattern adapts user-friendly input into a pattern that
// go/packages interprets as a local directory rather than a stdlib path.
// Bare relative directories ("internal/aone") gain a "./" prefix so they are
// not mistaken for std packages, and a "/..." suffix so the whole subtree is
// scanned (matching the user's intuition that the positional argument names
// a region, not a single package — that's what comment extraction needs to
// pick up handler/field docs from sub-packages too).
//
// The user can still pin to a single package by passing the explicit form
// "./internal/aone" (no recursion) — when the input already starts with "./"
// or "../" we trust it verbatim.
func normalizeSourcePattern(p string) string {
	if p == "" {
		return p
	}
	// Already explicit: ./foo, ../bar, /abs/path, .
	if p == "." || strings.HasPrefix(p, "./") || strings.HasPrefix(p, "../") || filepath.IsAbs(p) {
		return p
	}
	// Existing local directory — prefix with "./" and recurse, so descriptions
	// from sub-packages are picked up by Source().
	if info, err := os.Stat(p); err == nil && info.IsDir() {
		return "./" + p + "/..."
	}
	// Recursive pattern starting with a bare segment: "foo/..." → "./foo/..."
	if head, ok := strings.CutSuffix(p, "/..."); ok {
		if head != "" && !strings.Contains(head, ".") {
			if info, err := os.Stat(head); err == nil && info.IsDir() {
				return "./" + p
			}
		}
	}
	// Otherwise assume a Go import path / module pattern.
	return p
}

func markOverride(o *cli.Overrides, name string) {
	switch name {
	case "config":
		o.ConfigExplicit = true
	case "entry":
		o.EntrySet = true
	case "out":
		o.OutSet = true
	case "format":
		o.FormatSet = true
	case "title":
		o.InfoTitleSet = true
	case "version":
		o.InfoVersionSet = true
	case "include-test-files":
		o.IncludeTestFilesSet = true
	case "metadata-hook":
		o.MetadataHookSet = true
	case "entry-config-loader":
		o.EntryConfigLoaderSet = true
	case "entry-config-path":
		o.EntryConfigPathSet = true
	case "workdir":
		o.WorkdirSet = true
	case "keep-driver":
		o.KeepDriverSet = true
	case "verbose":
		o.VerboseSet = true
	}
}

func handleError(err error) int {
	var driverErr *cli.DriverError
	if errors.As(err, &driverErr) {
		fmt.Fprintln(os.Stderr, driverErr.Error())
		return driverErr.ExitCode
	}
	fmt.Fprintln(os.Stderr, err)
	return cli.ExitUsage
}

// autoTag annotates entry output with " (auto-discovered)" when the entry
// was filled in by DiscoverEntry instead of explicit config/flag input.
func autoTag(autoDiscovered bool) string {
	if autoDiscovered {
		return " (auto-discovered)"
	}
	return ""
}

type repeatedFlag struct {
	values []string
	set    bool
}

func (f *repeatedFlag) String() string { return strings.Join(f.values, ",") }

func (f *repeatedFlag) Set(value string) error {
	f.set = true
	f.values = append(f.values, value)
	return nil
}

func (f *repeatedFlag) Type() string { return "stringArray" }
