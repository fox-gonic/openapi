package main

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/fox-gonic/openapi/internal/cli"
)

var version = "dev"

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
		Use:   "init",
		Short: "Create a fox-openapi.yaml config file",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := cli.InitConfig(opts); err != nil {
				return exitError{code: cli.ExitUsage, err: err}
			}
			out := opts.ConfigPath
			if !filepath.IsAbs(out) {
				out = filepath.Join(opts.Workdir, out)
			}
			fmt.Printf("created %s\n", out)
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
		Use:   "generate",
		Short: "Generate an OpenAPI document",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			markOverridesFromFlags(opts, cmd.Flags())
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
			return nil
		},
	}
	bindCommonFlags(cmd.Flags(), opts)
	return cmd
}

func newCheckCommand() *cobra.Command {
	opts := newCommonOptions()
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Verify the committed OpenAPI document is up to date",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			markOverridesFromFlags(opts, cmd.Flags())
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
			return nil
		},
	}
	bindCommonFlags(cmd.Flags(), opts)
	return cmd
}

func newServeCommand() *cobra.Command {
	opts := newCommonOptions()
	serveCfg := cli.ServeConfig{Addr: "127.0.0.1:8765", UIs: []string{"swagger"}, Watch: true}
	var ui repeatedFlag
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Serve the generated spec and offline docs UI",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			markOverridesFromFlags(opts, cmd.Flags())
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
			fmt.Println(version)
		},
	}
}

type commonOptions struct {
	overrides *cli.Overrides
	sources   repeatedFlag
	servers   repeatedFlag
}

func newCommonOptions() *commonOptions {
	return &commonOptions{overrides: &cli.Overrides{}}
}

func parseCommon(name string, args []string) (cli.Config, int) {
	opts := newCommonOptions()
	cmd := &cobra.Command{
		Use:           name,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
	bindCommonFlags(cmd.Flags(), opts)
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		return cli.Config{}, cli.ExitUsage
	}
	markOverridesFromFlags(opts, cmd.Flags())
	cfg, err := configFromOptions(opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return cli.Config{}, cli.ExitUsage
	}
	return cfg, 0
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
