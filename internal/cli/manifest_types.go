package cli

import (
	"fmt"
	"go/types"
	"sort"
	"strings"

	openapi "github.com/fox-gonic/openapi"
	"golang.org/x/tools/go/packages"
)

func enrichRouteManifestTypes(workdir string, manifest *openapi.RouteManifest, includeTestFiles bool) ([]string, error) {
	resolver := &manifestTypeResolver{
		workdir:          workdir,
		includeTestFiles: includeTestFiles,
		pkgs:             map[string]*packages.Package{},
		pkgErrors:        map[string]error{},
	}
	var warnings []string
	var routeSymbols []manifestRouteSymbol
	importPaths := map[string]struct{}{}
	for i := range manifest.Routes {
		route := &manifest.Routes[i]
		handlerName := route.HandlerSymbol()
		if handlerName == "" || isRuntimeClosureHandlerSymbol(handlerName) {
			continue
		}
		cleaned := openapi.CleanHandlerName(handlerName)
		symbols := parseRuntimeHandlerSymbols(cleaned)
		if len(symbols) == 0 {
			warnings = append(warnings, fmt.Sprintf("WARN: route manifest %s %s: cannot resolve handler symbol %s", route.Method, route.Path, cleaned))
			continue
		}
		routeSymbols = append(routeSymbols, manifestRouteSymbol{
			route:       route,
			handlerName: cleaned,
			symbols:     symbols,
		})
		for _, symbol := range symbols {
			importPaths[symbol.importPath] = struct{}{}
		}
	}
	if err := resolver.loadPackages(sortedKeys(importPaths)); err != nil {
		return nil, err
	}
	for _, routeSymbol := range routeSymbols {
		sig, err := resolver.handlerSignatureForSymbols(routeSymbol.handlerName, routeSymbol.symbols)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("WARN: route manifest %s %s: %v", routeSymbol.route.Method, routeSymbol.route.Path, err))
			continue
		}
		routeSymbol.route.InputTypes = manifestInputTypes(sig)
		routeSymbol.route.ResultTypes = manifestResultTypes(sig)
	}
	return warnings, nil
}

type manifestTypeResolver struct {
	workdir          string
	includeTestFiles bool
	pkgs             map[string]*packages.Package
	pkgErrors        map[string]error
}

type manifestRouteSymbol struct {
	route       *openapi.RouteManifestRoute
	handlerName string
	symbols     []runtimeHandlerSymbol
}

func (r *manifestTypeResolver) handlerSignature(handlerName string) (*types.Signature, error) {
	handlerName = openapi.CleanHandlerName(handlerName)
	symbols := parseRuntimeHandlerSymbols(handlerName)
	if len(symbols) == 0 {
		return nil, fmt.Errorf("cannot resolve handler symbol %s", handlerName)
	}
	if err := r.loadPackages(symbolImportPaths(symbols)); err != nil {
		return nil, err
	}
	return r.handlerSignatureForSymbols(handlerName, symbols)
}

func (r *manifestTypeResolver) handlerSignatureForSymbols(handlerName string, symbols []runtimeHandlerSymbol) (*types.Signature, error) {
	var messages []string
	for _, symbol := range symbols {
		sig, err := r.handlerSignatureForSymbol(handlerName, symbol)
		if err == nil {
			return sig, nil
		}
		messages = append(messages, err.Error())
	}
	return nil, fmt.Errorf("cannot resolve handler symbol %s: %s", handlerName, strings.Join(messages, "; "))
}

func (r *manifestTypeResolver) handlerSignatureForSymbol(handlerName string, symbol runtimeHandlerSymbol) (*types.Signature, error) {
	if err := r.pkgErrors[symbol.importPath]; err != nil {
		return nil, err
	}
	pkg, ok := r.pkgs[symbol.importPath]
	if !ok || pkg == nil || pkg.Types == nil {
		return nil, fmt.Errorf("package not found: %s", symbol.importPath)
	}
	var obj types.Object
	if symbol.recvName == "" {
		obj = pkg.Types.Scope().Lookup(symbol.funcName)
	} else {
		typeObj, ok := pkg.Types.Scope().Lookup(symbol.recvName).(*types.TypeName)
		if !ok {
			return nil, fmt.Errorf("receiver type not found: %s.%s", symbol.importPath, symbol.recvName)
		}
		named, ok := types.Unalias(typeObj.Type()).(*types.Named)
		if !ok {
			return nil, fmt.Errorf("%s.%s is not a named receiver type", symbol.importPath, symbol.recvName)
		}
		if len(symbol.recvTypeArgs) > 0 {
			instantiated, err := r.instantiateNamed(pkg, named, symbol.recvTypeArgs)
			if err != nil {
				return nil, err
			}
			named = instantiated
		}
		obj, _, _ = types.LookupFieldOrMethod(types.NewPointer(named), true, pkg.Types, symbol.funcName)
	}
	fn, ok := obj.(*types.Func)
	if !ok || fn == nil {
		return nil, fmt.Errorf("handler function not found: %s", handlerName)
	}
	sig, ok := fn.Type().(*types.Signature)
	if !ok {
		return nil, fmt.Errorf("handler symbol is not a function: %s", handlerName)
	}
	if len(symbol.funcTypeArgs) > 0 {
		return r.instantiateSignature(pkg, sig, symbol.funcTypeArgs)
	}
	return sig, nil
}

func (r *manifestTypeResolver) instantiateNamed(pkg *packages.Package, named *types.Named, argNames []string) (*types.Named, error) {
	args, err := r.resolveRuntimeTypeArgs(pkg, argNames)
	if err != nil {
		return nil, err
	}
	instantiated, err := types.Instantiate(types.NewContext(), named, args, true)
	if err != nil {
		return nil, fmt.Errorf("instantiate receiver %s[%s]: %w", named.Obj().Name(), strings.Join(argNames, ", "), err)
	}
	instNamed, ok := instantiated.(*types.Named)
	if !ok {
		return nil, fmt.Errorf("instantiated receiver is not a named type: %s", instantiated.String())
	}
	return instNamed, nil
}

func (r *manifestTypeResolver) instantiateSignature(pkg *packages.Package, sig *types.Signature, argNames []string) (*types.Signature, error) {
	args, err := r.resolveRuntimeTypeArgs(pkg, argNames)
	if err != nil {
		return nil, err
	}
	instantiated, err := types.Instantiate(types.NewContext(), sig, args, true)
	if err != nil {
		return nil, fmt.Errorf("instantiate handler signature [%s]: %w", strings.Join(argNames, ", "), err)
	}
	instSig, ok := instantiated.(*types.Signature)
	if !ok {
		return nil, fmt.Errorf("instantiated handler is not a signature: %s", instantiated.String())
	}
	return instSig, nil
}

func (r *manifestTypeResolver) resolveRuntimeTypeArgs(pkg *packages.Package, argNames []string) ([]types.Type, error) {
	args := make([]types.Type, 0, len(argNames))
	for _, argName := range argNames {
		arg, err := r.resolveRuntimeTypeArg(pkg, argName)
		if err != nil {
			return nil, err
		}
		args = append(args, arg)
	}
	return args, nil
}

func (r *manifestTypeResolver) resolveRuntimeTypeArg(pkg *packages.Package, argName string) (types.Type, error) {
	argName = strings.TrimSpace(argName)
	if strings.HasPrefix(argName, "*") {
		elem, err := r.resolveRuntimeTypeArg(pkg, strings.TrimPrefix(argName, "*"))
		if err != nil {
			return nil, err
		}
		return types.NewPointer(elem), nil
	}
	if strings.HasPrefix(argName, "[]") {
		elem, err := r.resolveRuntimeTypeArg(pkg, strings.TrimPrefix(argName, "[]"))
		if err != nil {
			return nil, err
		}
		return types.NewSlice(elem), nil
	}
	base, nestedArgs := splitRuntimeTypeName(argName)
	typ, err := r.resolveRuntimeNamedType(pkg, base)
	if err != nil {
		return nil, err
	}
	if len(nestedArgs) == 0 {
		return typ, nil
	}
	named, ok := typ.(*types.Named)
	if !ok {
		return nil, fmt.Errorf("type argument %s is not a named generic type", base)
	}
	return r.instantiateNamed(pkg, named, nestedArgs)
}

func (r *manifestTypeResolver) resolveRuntimeNamedType(pkg *packages.Package, name string) (types.Type, error) {
	if obj := types.Universe.Lookup(name); obj != nil {
		if typeObj, ok := obj.(*types.TypeName); ok {
			return typeObj.Type(), nil
		}
	}
	if obj := pkg.Types.Scope().Lookup(name); obj != nil {
		if typeObj, ok := obj.(*types.TypeName); ok {
			return types.Unalias(typeObj.Type()), nil
		}
	}
	idx := lastRuntimeSymbolDot(name)
	if idx <= 0 || idx == len(name)-1 {
		return nil, fmt.Errorf("type argument not found: %s", name)
	}
	importPath, typeName := name[:idx], name[idx+1:]
	if err := r.loadPackages([]string{importPath}); err != nil {
		return nil, err
	}
	if err := r.pkgErrors[importPath]; err != nil {
		return nil, err
	}
	target, ok := r.pkgs[importPath]
	if !ok || target == nil || target.Types == nil {
		return nil, fmt.Errorf("package not found: %s", importPath)
	}
	typeObj, ok := target.Types.Scope().Lookup(typeName).(*types.TypeName)
	if !ok {
		return nil, fmt.Errorf("type argument not found: %s", name)
	}
	return types.Unalias(typeObj.Type()), nil
}

func (r *manifestTypeResolver) loadPackages(importPaths []string) error {
	if r.pkgs == nil {
		r.pkgs = map[string]*packages.Package{}
	}
	if r.pkgErrors == nil {
		r.pkgErrors = map[string]error{}
	}
	var missing []string
	for _, importPath := range importPaths {
		if importPath == "" {
			continue
		}
		if _, ok := r.pkgs[importPath]; ok {
			continue
		}
		if _, ok := r.pkgErrors[importPath]; ok {
			continue
		}
		missing = append(missing, importPath)
	}
	if len(missing) == 0 {
		return nil
	}
	cfg := &packages.Config{
		Dir:   r.workdir,
		Mode:  packages.NeedName | packages.NeedTypes | packages.NeedImports | packages.NeedDeps,
		Tests: r.includeTestFiles,
	}
	pkgs, err := packages.Load(cfg, missing...)
	if err != nil {
		return fmt.Errorf("load packages %s: %w", strings.Join(missing, ", "), err)
	}
	for _, pkg := range pkgs {
		if pkg == nil {
			continue
		}
		importPath := packageImportPath(pkg)
		if importPath == "" {
			continue
		}
		if len(pkg.Errors) > 0 {
			r.pkgErrors[importPath] = fmt.Errorf("load package %s: %s", importPath, pkg.Errors[0])
			continue
		}
		if pkg.Types == nil {
			r.pkgErrors[importPath] = fmt.Errorf("package not found: %s", importPath)
			continue
		}
		r.pkgs[importPath] = pkg
	}
	for _, importPath := range missing {
		if _, ok := r.pkgs[importPath]; ok {
			continue
		}
		if _, ok := r.pkgErrors[importPath]; ok {
			continue
		}
		r.pkgErrors[importPath] = fmt.Errorf("package not found: %s", importPath)
	}
	return nil
}

func packageImportPath(pkg *packages.Package) string {
	if pkg.PkgPath != "" {
		return pkg.PkgPath
	}
	if pkg.Types != nil {
		return pkg.Types.Path()
	}
	return pkg.ID
}

type runtimeHandlerSymbol struct {
	importPath   string
	recvName     string
	recvTypeArgs []string
	funcName     string
	funcTypeArgs []string
}

func parseRuntimeHandlerSymbol(handlerName string) (runtimeHandlerSymbol, bool) {
	symbols := parseRuntimeHandlerSymbols(handlerName)
	if len(symbols) == 0 {
		return runtimeHandlerSymbol{}, false
	}
	return symbols[0], true
}

func parseRuntimeHandlerSymbols(handlerName string) []runtimeHandlerSymbol {
	if idx := strings.LastIndex(handlerName, ".("); idx >= 0 {
		closeIdx := strings.Index(handlerName[idx+2:], ").")
		if closeIdx < 0 {
			return nil
		}
		recv := handlerName[idx+2 : idx+2+closeIdx]
		method := handlerName[idx+2+closeIdx+2:]
		if method == "" {
			return nil
		}
		recvName, recvTypeArgs := splitRuntimeTypeName(strings.TrimPrefix(recv, "*"))
		funcName, funcTypeArgs := splitRuntimeTypeName(strings.TrimSuffix(method, "-fm"))
		return []runtimeHandlerSymbol{{
			importPath:   handlerName[:idx],
			recvName:     recvName,
			recvTypeArgs: recvTypeArgs,
			funcName:     funcName,
			funcTypeArgs: funcTypeArgs,
		}}
	}
	idx := lastRuntimeSymbolDot(handlerName)
	if idx <= 0 || idx == len(handlerName)-1 {
		return nil
	}
	funcName, funcTypeArgs := splitRuntimeTypeName(strings.TrimSuffix(handlerName[idx+1:], "-fm"))
	var symbols []runtimeHandlerSymbol
	if recvDot := previousRuntimeSymbolDot(handlerName, idx); recvDot > previousRuntimeSymbolSlash(handlerName, idx) {
		recvName, recvTypeArgs := splitRuntimeTypeName(handlerName[recvDot+1 : idx])
		symbols = append(symbols, runtimeHandlerSymbol{
			importPath:   handlerName[:recvDot],
			recvName:     recvName,
			recvTypeArgs: recvTypeArgs,
			funcName:     funcName,
			funcTypeArgs: funcTypeArgs,
		})
	}
	symbols = append(symbols, runtimeHandlerSymbol{
		importPath:   handlerName[:idx],
		funcName:     funcName,
		funcTypeArgs: funcTypeArgs,
	})
	return symbols
}

func lastRuntimeSymbolDot(handlerName string) int {
	return previousRuntimeSymbolDot(handlerName, len(handlerName))
}

func previousRuntimeSymbolDot(handlerName string, before int) int {
	return previousRuntimeSymbolByte(handlerName, before, '.')
}

func previousRuntimeSymbolSlash(handlerName string, before int) int {
	return previousRuntimeSymbolByte(handlerName, before, '/')
}

func previousRuntimeSymbolByte(handlerName string, before int, want byte) int {
	depth := 0
	for i := before - 1; i >= 0; i-- {
		switch handlerName[i] {
		case ']':
			depth++
		case '[':
			if depth > 0 {
				depth--
			}
		case want:
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func symbolImportPaths(symbols []runtimeHandlerSymbol) []string {
	result := make([]string, 0, len(symbols))
	for _, symbol := range symbols {
		result = append(result, symbol.importPath)
	}
	return result
}

func trimRuntimeTypeArgs(name string) string {
	name, _ = splitRuntimeTypeName(name)
	return name
}

func splitRuntimeTypeName(name string) (string, []string) {
	if open := strings.IndexByte(name, '['); open >= 0 {
		if matchingRuntimeBracket(name, open) == len(name)-1 {
			return name[:open], splitRuntimeTypeArgs(name[open+1 : len(name)-1])
		}
	}
	return name, nil
}

func matchingRuntimeBracket(value string, open int) int {
	depth := 0
	for i := open; i < len(value); i++ {
		switch value[i] {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func splitRuntimeTypeArgs(value string) []string {
	var args []string
	start := 0
	depth := 0
	for i := 0; i < len(value); i++ {
		switch value[i] {
		case '[':
			depth++
		case ']':
			depth--
		case ',':
			if depth == 0 {
				args = append(args, strings.TrimSpace(value[start:i]))
				start = i + 1
			}
		}
	}
	args = append(args, strings.TrimSpace(value[start:]))
	return args
}

func isRuntimeClosureHandlerSymbol(name string) bool {
	return openapi.CleanHandlerName(name) != strings.TrimSuffix(name, "-fm")
}

func sortedKeys(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func manifestInputTypes(sig *types.Signature) []openapi.RouteManifestType {
	result := make([]openapi.RouteManifestType, 0, sig.Params().Len())
	for i := 0; i < sig.Params().Len(); i++ {
		typ := sig.Params().At(i).Type()
		if isFoxContextType(typ) {
			continue
		}
		result = append(result, routeManifestTypeFromTypes(typ, map[types.Type]bool{}))
	}
	return result
}

func manifestResultTypes(sig *types.Signature) []openapi.RouteManifestType {
	result := make([]openapi.RouteManifestType, 0, sig.Results().Len())
	for i := 0; i < sig.Results().Len(); i++ {
		result = append(result, routeManifestTypeFromTypes(sig.Results().At(i).Type(), map[types.Type]bool{}))
	}
	return result
}

func routeManifestTypeFromTypes(typ types.Type, seen map[types.Type]bool) openapi.RouteManifestType {
	if typ == nil {
		return openapi.RouteManifestType{}
	}
	typ = types.Unalias(typ)
	result := openapi.RouteManifestType{
		Kind:   typeKind(typ),
		String: typ.String(),
		Name:   typeName(typ),
	}
	if pkgPath := typePkgPath(typ); pkgPath != "" {
		result.PkgPath = pkgPath
	}
	if seen[typ] || manifestOpaqueTypesType(typ) {
		return result
	}
	seen[typ] = true
	defer delete(seen, typ)
	result.TypeArgs = typeArgs(typ, seen)

	container := typ
	if _, ok := typ.(*types.Named); ok {
		container = typ.Underlying()
	}
	switch t := container.(type) {
	case *types.Pointer:
		elem := routeManifestTypeFromTypes(t.Elem(), seen)
		result.Elem = &elem
	case *types.Slice:
		elem := routeManifestTypeFromTypes(t.Elem(), seen)
		result.Elem = &elem
	case *types.Array:
		elem := routeManifestTypeFromTypes(t.Elem(), seen)
		result.Elem = &elem
	case *types.Map:
		key := routeManifestTypeFromTypes(t.Key(), seen)
		elem := routeManifestTypeFromTypes(t.Elem(), seen)
		result.Key = &key
		result.Elem = &elem
	case *types.Struct:
		result.Fields = make([]openapi.RouteManifestField, 0, t.NumFields())
		for i := 0; i < t.NumFields(); i++ {
			field := t.Field(i)
			out := openapi.RouteManifestField{
				Name:      field.Name(),
				Tag:       t.Tag(i),
				Anonymous: field.Embedded(),
				Type:      routeManifestTypeFromTypes(field.Type(), seen),
			}
			if !field.Exported() && field.Pkg() != nil {
				out.PkgPath = field.Pkg().Path()
			}
			result.Fields = append(result.Fields, out)
		}
	}
	return result
}

func typeKind(typ types.Type) string {
	typ = types.Unalias(typ)
	if _, ok := typ.(*types.Pointer); ok {
		return "ptr"
	}
	switch t := typ.(type) {
	case *types.Basic:
		return basicKind(t)
	case *types.Slice:
		return "slice"
	case *types.Array:
		return "array"
	case *types.Map:
		return "map"
	case *types.Interface:
		return "interface"
	}
	switch derefTypes(typ).Underlying().(type) {
	case *types.Struct:
		return "struct"
	case *types.Slice:
		return "slice"
	case *types.Array:
		return "array"
	case *types.Map:
		return "map"
	case *types.Basic:
		return basicKind(derefTypes(typ).Underlying().(*types.Basic))
	case *types.Interface:
		return "interface"
	}
	return "unknown"
}

func basicKind(basic *types.Basic) string {
	switch basic.Kind() {
	case types.Bool:
		return "bool"
	case types.Int:
		return "int"
	case types.Int8:
		return "int8"
	case types.Int16:
		return "int16"
	case types.Int32:
		return "int32"
	case types.Int64:
		return "int64"
	case types.Uint:
		return "uint"
	case types.Uint8:
		return "uint8"
	case types.Uint16:
		return "uint16"
	case types.Uint32:
		return "uint32"
	case types.Uint64:
		return "uint64"
	case types.Float32:
		return "float32"
	case types.Float64:
		return "float64"
	case types.String:
		return "string"
	default:
		return basic.Name()
	}
}

func typeName(typ types.Type) string {
	typ = types.Unalias(typ)
	if isErrorType(typ) {
		return "error"
	}
	switch t := derefTypes(typ).(type) {
	case *types.Basic:
		return t.Name()
	case *types.Named:
		return t.Obj().Name()
	}
	return ""
}

func typeArgs(typ types.Type, seen map[types.Type]bool) []openapi.RouteManifestType {
	named, ok := derefTypes(types.Unalias(typ)).(*types.Named)
	if !ok || named.TypeArgs() == nil || named.TypeArgs().Len() == 0 {
		return nil
	}
	args := make([]openapi.RouteManifestType, 0, named.TypeArgs().Len())
	for i := 0; i < named.TypeArgs().Len(); i++ {
		args = append(args, routeManifestTypeFromTypes(named.TypeArgs().At(i), seen))
	}
	return args
}

func typePkgPath(typ types.Type) string {
	typ = types.Unalias(typ)
	if named, ok := derefTypes(typ).(*types.Named); ok && named.Obj() != nil && named.Obj().Pkg() != nil {
		return named.Obj().Pkg().Path()
	}
	return ""
}

func derefTypes(typ types.Type) types.Type {
	for {
		ptr, ok := typ.(*types.Pointer)
		if !ok {
			return typ
		}
		typ = ptr.Elem()
	}
}

func manifestOpaqueTypesType(typ types.Type) bool {
	typ = types.Unalias(typ)
	return typePkgPath(typ) == "time"
}

func isFoxContextType(typ types.Type) bool {
	typ = types.Unalias(typ)
	ptr, ok := typ.(*types.Pointer)
	if ok {
		typ = ptr.Elem()
	}
	named, ok := typ.(*types.Named)
	if !ok || named.Obj() == nil || named.Obj().Name() != "Context" || named.Obj().Pkg() == nil {
		return false
	}
	return named.Obj().Pkg().Path() == "github.com/fox-gonic/fox"
}
