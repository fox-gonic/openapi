package cli

import (
	"fmt"
	"go/types"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/packages"
)

type Entry struct {
	ImportPath   string
	FuncName     string
	ReturnsError bool
	TakesContext bool
	TakesConfig  bool
}

type Hook struct {
	ImportPath string
	FuncName   string
}

type ConfigLoader struct {
	ImportPath string
	FuncName   string
	Path       string
}

func ResolveEntry(workdir, value string) (Entry, error) {
	importPath, funcName, err := splitSymbol(value)
	if err != nil {
		return Entry{}, err
	}
	obj, err := loadFunc(workdir, importPath, funcName)
	if err != nil {
		return Entry{}, err
	}
	if !obj.Exported() {
		return Entry{}, fmt.Errorf("entry function %s is not exported", value)
	}
	sig := obj.Type().(*types.Signature)
	paramCount := sig.Params().Len()
	if paramCount > 2 {
		return Entry{}, entrySignatureError(value)
	}
	takesContext := false
	takesConfig := false
	if paramCount >= 1 {
		if !isContextType(sig.Params().At(0).Type()) {
			return Entry{}, entrySignatureError(value)
		}
		takesContext = true
	}
	if paramCount == 2 {
		takesConfig = true
	}
	results := sig.Results()
	if results.Len() == 1 && isFoxEngine(results.At(0).Type()) {
		return Entry{ImportPath: importPath, FuncName: funcName, TakesContext: takesContext, TakesConfig: takesConfig}, nil
	}
	if results.Len() == 2 && isFoxEngine(results.At(0).Type()) && isErrorType(results.At(1).Type()) {
		return Entry{ImportPath: importPath, FuncName: funcName, ReturnsError: true, TakesContext: takesContext, TakesConfig: takesConfig}, nil
	}
	return Entry{}, entrySignatureError(value)
}

func ResolveConfigLoader(workdir, value, path string) (ConfigLoader, error) {
	importPath, funcName, err := splitSymbol(value)
	if err != nil {
		return ConfigLoader{}, err
	}
	obj, err := loadFunc(workdir, importPath, funcName)
	if err != nil {
		return ConfigLoader{}, err
	}
	if !obj.Exported() {
		return ConfigLoader{}, fmt.Errorf("entry config loader function %s is not exported", value)
	}
	sig := obj.Type().(*types.Signature)
	if sig.Params().Len() != 1 || !isStringType(sig.Params().At(0).Type()) ||
		sig.Results().Len() != 2 || !isErrorType(sig.Results().At(1).Type()) {
		return ConfigLoader{}, fmt.Errorf("entry config loader signature mismatch for %s: expected func(string) (*Config, error)", value)
	}
	if path != "" && !filepath.IsAbs(path) {
		path = filepath.Join(workdir, path)
	}
	return ConfigLoader{ImportPath: importPath, FuncName: funcName, Path: path}, nil
}

func ResolveHook(workdir, value string) (Hook, error) {
	importPath, funcName, err := splitSymbol(value)
	if err != nil {
		return Hook{}, err
	}
	obj, err := loadFunc(workdir, importPath, funcName)
	if err != nil {
		return Hook{}, err
	}
	if !obj.Exported() {
		return Hook{}, fmt.Errorf("metadata hook function %s is not exported", value)
	}
	sig := obj.Type().(*types.Signature)
	if sig.Params().Len() != 0 || sig.Results().Len() != 1 || !isOpenAPIOptionSlice(sig.Results().At(0).Type()) {
		return Hook{}, fmt.Errorf("metadata hook signature mismatch for %s: expected func() []openapi.Option", value)
	}
	return Hook{ImportPath: importPath, FuncName: funcName}, nil
}

func splitSymbol(value string) (string, string, error) {
	idx := strings.LastIndex(value, ".")
	if idx <= 0 || idx == len(value)-1 {
		return "", "", fmt.Errorf("invalid symbol %q, expected module/path/pkg.FuncName", value)
	}
	return value[:idx], value[idx+1:], nil
}

func loadFunc(workdir, importPath, funcName string) (*types.Func, error) {
	cfg := &packages.Config{Dir: workdir, Mode: packages.NeedName | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedSyntax}
	pkgs, err := packages.Load(cfg, importPath)
	if err != nil {
		return nil, fmt.Errorf("load package %s: %w", importPath, err)
	}
	if packages.PrintErrors(pkgs) > 0 {
		return nil, fmt.Errorf("load package %s failed", importPath)
	}
	if len(pkgs) == 0 || pkgs[0].Types == nil {
		return nil, fmt.Errorf("package not found: %s", importPath)
	}
	obj := pkgs[0].Types.Scope().Lookup(funcName)
	if obj == nil {
		return nil, fmt.Errorf("function not found: %s.%s", importPath, funcName)
	}
	fn, ok := obj.(*types.Func)
	if !ok {
		return nil, fmt.Errorf("%s.%s is not a function", importPath, funcName)
	}
	return fn, nil
}

func isFoxEngine(typ types.Type) bool {
	ptr, ok := typ.(*types.Pointer)
	if !ok {
		return false
	}
	named, ok := ptr.Elem().(*types.Named)
	if !ok || named.Obj() == nil || named.Obj().Name() != "Engine" {
		return false
	}
	pkg := named.Obj().Pkg()
	return pkg != nil && (pkg.Path() == "github.com/fox-gonic/fox" || pkg.Name() == "fox")
}

func isContextType(typ types.Type) bool {
	named, ok := typ.(*types.Named)
	if !ok || named.Obj() == nil || named.Obj().Name() != "Context" {
		return false
	}
	pkg := named.Obj().Pkg()
	return pkg != nil && pkg.Path() == "context"
}

func isStringType(typ types.Type) bool {
	basic, ok := typ.(*types.Basic)
	return ok && basic.Kind() == types.String
}

func isOpenAPIOptionSlice(typ types.Type) bool {
	slice, ok := typ.(*types.Slice)
	if !ok {
		return false
	}
	named, ok := slice.Elem().(*types.Named)
	if !ok || named.Obj() == nil || named.Obj().Name() != "Option" {
		return false
	}
	pkg := named.Obj().Pkg()
	return pkg != nil && pkg.Path() == "github.com/fox-gonic/openapi"
}

func isErrorType(typ types.Type) bool {
	errObj := types.Universe.Lookup("error")
	return errObj != nil && types.Implements(typ, errObj.Type().Underlying().(*types.Interface))
}

func entrySignatureError(value string) error {
	return fmt.Errorf("entry function signature mismatch for %s: expected func() *fox.Engine, func() (*fox.Engine, error), func(context.Context) *fox.Engine, func(context.Context) (*fox.Engine, error), or func(context.Context, *Config) (*fox.Engine, error)", value)
}
