package cli

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/goccy/go-yaml"

	"github.com/fox-gonic/openapi/internal/cli/ui"
)

type ServeConfig struct {
	Addr  string
	UIs   []string
	Watch bool
	Open  bool
	// Stdout is where startup banners are written. Defaults to os.Stdout.
	Stdout io.Writer
}

type servedSpec struct {
	mu   sync.RWMutex
	yaml []byte
	json []byte
	err  string
}

func Serve(cfg Config, serveCfg ServeConfig) error {
	stdout := serveCfg.Stdout
	if stdout == nil {
		stdout = os.Stdout
	}
	state := &servedSpec{}
	if err := refreshSpec(cfg, state); err != nil {
		return err
	}
	if serveCfg.Watch {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go watchAndRefresh(ctx, cfg, state)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/openapi.yaml", state.handleYAML)
	mux.HandleFunc("/openapi.json", state.handleJSON)
	uis := normalizeUIs(serveCfg.UIs)
	uiRoutes := make([]uiRoute, 0, len(uis))
	for _, name := range uis {
		switch name {
		case "swagger", "docs":
			mux.HandleFunc("/docs", ui.Handler("swagger", "/openapi.yaml"))
			uiRoutes = append(uiRoutes, uiRoute{path: "/docs", label: "swagger"})
		case "scalar":
			mux.HandleFunc("/scalar", ui.Handler("scalar", "/openapi.yaml"))
			uiRoutes = append(uiRoutes, uiRoute{path: "/scalar"})
		case "redoc":
			mux.HandleFunc("/redoc", ui.Handler("redoc", "/openapi.yaml"))
			uiRoutes = append(uiRoutes, uiRoute{path: "/redoc"})
		}
	}
	mux.Handle("/assets/", ui.AssetsHandler())

	base := "http://" + serveCfg.Addr
	autoTag := ""
	if cfg.EntryAutoDiscovered {
		autoTag = " (auto-discovered)"
	}
	fmt.Fprintf(stdout, "fox-openapi serve listening on %s\n", base)
	fmt.Fprintf(stdout, "  entry:  %s%s\n", cfg.Entry, autoTag)
	fmt.Fprintf(stdout, "  spec:   %s/openapi.yaml | %s/openapi.json\n", base, base)
	for i, r := range uiRoutes {
		prefix := "          "
		if i == 0 {
			prefix = "  ui:     "
		}
		fmt.Fprintf(stdout, "%s%s\n", prefix, r.render(base))
	}
	if serveCfg.Watch {
		fmt.Fprintln(stdout, "  watch:  enabled (regenerates on .go changes)")
	}
	fmt.Fprintln(stdout, "  press Ctrl+C to stop")

	if serveCfg.Open {
		go func() {
			time.Sleep(300 * time.Millisecond)
			_ = openBrowser(base + "/docs")
		}()
	}
	return http.ListenAndServe(serveCfg.Addr, mux)
}

// uiRoute is a banner entry: a relative path plus an optional label
// like "swagger" that disambiguates the URL from neighbouring entries.
// Keeping path and label separate avoids parsing the rendered string back
// out when prefixing with the listen base.
type uiRoute struct {
	path  string
	label string
}

func (r uiRoute) render(base string) string {
	if r.label == "" {
		return base + r.path
	}
	return base + r.path + " (" + r.label + ")"
}

func refreshSpec(cfg Config, state *servedSpec) error {
	cfg.Format = FormatYAML
	yamlBytes, _, err := RunPipeline(cfg)
	if err != nil {
		state.setError(err.Error())
		return err
	}
	jsonBytes, err := yaml.YAMLToJSON(yamlBytes)
	if err != nil {
		state.setError(err.Error())
		return fmt.Errorf("convert spec to json: %w", err)
	}
	state.set(yamlBytes, jsonBytes)
	return nil
}

func (s *servedSpec) set(yamlBytes, jsonBytes []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.yaml = append([]byte(nil), yamlBytes...)
	s.json = append([]byte(nil), jsonBytes...)
	s.err = ""
}

func (s *servedSpec) setError(value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.err = value
}

func (s *servedSpec) handleYAML(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	_, _ = w.Write(s.yaml)
}

func (s *servedSpec) handleJSON(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write(s.json)
}

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}

func normalizeUIs(values []string) []string {
	if len(values) == 0 {
		return []string{"swagger", "scalar", "redoc"}
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "docs" {
			value = "swagger"
		}
		if value != "" {
			if _, ok := seen[value]; ok {
				continue
			}
			seen[value] = struct{}{}
			out = append(out, value)
		}
	}
	return out
}
