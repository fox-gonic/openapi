package cli

import (
	"fmt"
	"go/ast"
	"go/types"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

// entryMarker is the doc-comment directive that explicitly marks a function as
// the OpenAPI entry. When at least one candidate carries this marker, only
// marked candidates are considered (other matches are ignored).
const (
	entryMarker          = "fox-openapi:entry"
	entryMarkerDirective = "// " + entryMarker
)

// EntryCandidate describes a function that matches the entry signature.
// It is exported so callers can render multi-match diagnostics.
type EntryCandidate struct {
	Entry  Entry
	Marked bool
}

// Symbol returns the fully qualified runtime symbol (importPath.FuncName).
func (c EntryCandidate) Symbol() string {
	return c.Entry.ImportPath + "." + c.Entry.FuncName
}

// DiscoverEntry walks the given scope (e.g. []string{"./..."}) under workdir
// and looks for an exported function whose signature matches one of the
// supported entry shapes (see entrySignatureError).
//
// Resolution rules:
//   - If one or more candidates are marked with `// fox-openapi:entry`, only
//     marked candidates are considered.
//   - Exactly one candidate -> success.
//   - Zero candidates -> "no entry function found" error.
//   - Multiple candidates -> error listing all symbols so the user can
//     disambiguate via --entry or the marker comment.
func DiscoverEntry(workdir string, scope []string) (Entry, error) {
	patterns := scope
	if len(patterns) == 0 {
		patterns = []string{"./..."}
	}
	candidates, err := findEntryCandidates(workdir, patterns)
	if err != nil {
		return Entry{}, err
	}
	marked := make([]EntryCandidate, 0, len(candidates))
	for _, c := range candidates {
		if c.Marked {
			marked = append(marked, c)
		}
	}
	if len(marked) > 0 {
		candidates = marked
	}
	switch len(candidates) {
	case 0:
		return Entry{}, fmt.Errorf("no entry function found in %s; expected an exported func returning *fox.Engine. Set entry explicitly or annotate the function with `%s`", strings.Join(patterns, ", "), entryMarkerDirective)
	case 1:
		return candidates[0].Entry, nil
	default:
		names := make([]string, 0, len(candidates))
		for _, c := range candidates {
			names = append(names, c.Symbol())
		}
		sort.Strings(names)
		return Entry{}, fmt.Errorf("multiple entry candidates found:\n  - %s\nspecify --entry or annotate exactly one with `%s`", strings.Join(names, "\n  - "), entryMarkerDirective)
	}
}

func findEntryCandidates(workdir string, patterns []string) ([]EntryCandidate, error) {
	cfg := &packages.Config{
		Dir:  workdir,
		Mode: packages.NeedName | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedSyntax,
	}
	pkgs, err := packages.Load(cfg, patterns...)
	if err != nil {
		return nil, fmt.Errorf("load packages %v: %w", patterns, err)
	}
	if packages.PrintErrors(pkgs) > 0 {
		return nil, fmt.Errorf("load packages %v failed", patterns)
	}
	seen := make(map[string]struct{})
	var candidates []EntryCandidate
	for _, pkg := range pkgs {
		if pkg.Types == nil {
			continue
		}
		// Build map of FuncDecl by name to recover doc comments. Methods are
		// not entry candidates so we only need top-level decls.
		docs := make(map[string]*ast.FuncDecl)
		for _, file := range pkg.Syntax {
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Recv != nil {
					continue
				}
				docs[fn.Name.Name] = fn
			}
		}
		scope := pkg.Types.Scope()
		for _, name := range scope.Names() {
			obj, ok := scope.Lookup(name).(*types.Func)
			if !ok || !obj.Exported() {
				continue
			}
			entry, ok := entryFromFunc(pkg.Types.Path(), obj)
			if !ok {
				continue
			}
			key := entry.ImportPath + "." + entry.FuncName
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}
			marked := false
			if decl, ok := docs[entry.FuncName]; ok && decl.Doc != nil {
				marked = hasEntryMarker(decl.Doc)
			}
			candidates = append(candidates, EntryCandidate{Entry: entry, Marked: marked})
		}
	}
	return candidates, nil
}

// entryFromFunc inspects a *types.Func and, when its signature matches one of
// the supported entry shapes, returns the corresponding Entry. Non-fatal:
// callers iterate over many functions and only keep matches.
func entryFromFunc(importPath string, obj *types.Func) (Entry, bool) {
	sig, ok := obj.Type().(*types.Signature)
	if !ok {
		return Entry{}, false
	}
	shape, ok := matchEntrySignature(sig)
	if !ok {
		return Entry{}, false
	}
	return Entry{
		ImportPath:       importPath,
		FuncName:         obj.Name(),
		TakesContext:     shape.takesContext,
		TakesConfig:      shape.takesConfig,
		ConfigImportPath: shape.configImportPath,
		ConfigTypeName:   shape.configTypeName,
		ReturnsError:     shape.returnsError,
	}, true
}

// hasEntryMarker reports whether any line of doc carries the entry directive.
// ast.CommentGroup.Text() already strips "//", "/* */" delimiters, and the
// leading "*" prefix of block-comment lines, returning a clean newline-joined
// body — leaving us with a plain line-by-line scan.
func hasEntryMarker(doc *ast.CommentGroup) bool {
	if doc == nil {
		return false
	}
	for line := range strings.SplitSeq(doc.Text(), "\n") {
		if strings.TrimSpace(line) == entryMarker {
			return true
		}
	}
	return false
}
