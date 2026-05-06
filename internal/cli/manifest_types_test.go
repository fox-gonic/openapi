package cli

import (
	"path/filepath"
	"testing"

	"go/types"

	openapi "github.com/fox-gonic/openapi"
	"golang.org/x/tools/go/packages"
)

func TestRouteManifestTypeFromTypesUsesStructuredTypeArgs(t *testing.T) {
	dir := writeUserModule(t)
	resolver := manifestTypeResolver{
		workdir: dir,
		pkgs:    map[string]*packages.Package{},
	}
	sig, err := resolver.handlerSignature("example.com/app/internal/server.GetGenericUser")
	if err != nil {
		t.Fatal(err)
	}

	results := manifestResultTypes(sig)
	if len(results) != 2 {
		t.Fatalf("results = %#v", results)
	}
	response := results[0]
	if response.Name != "GenericResponse" {
		t.Fatalf("response name = %q", response.Name)
	}
	if len(response.TypeArgs) != 1 {
		t.Fatalf("type args = %#v", response.TypeArgs)
	}
	if response.TypeArgs[0].Name != "User" || response.TypeArgs[0].PkgPath != "example.com/app/internal/server" {
		t.Fatalf("type arg = %#v", response.TypeArgs[0])
	}
}

func TestRouteManifestTypeFromTypesPopulatesString(t *testing.T) {
	field := types.NewVar(0, nil, "Items", types.NewSlice(types.Typ[types.String]))
	typ := types.NewStruct([]*types.Var{field}, []string{`json:"items"`})

	result := routeManifestTypeFromTypes(typ, map[types.Type]bool{})

	if result.String == "" {
		t.Fatalf("string is empty: %#v", result)
	}
	if len(result.Fields) != 1 {
		t.Fatalf("fields = %#v", result.Fields)
	}
	items := result.Fields[0].Type
	if items.String != "[]string" {
		t.Fatalf("slice string = %q, want []string", items.String)
	}
	if items.Elem == nil || items.Elem.String != "string" {
		t.Fatalf("elem = %#v, want string", items.Elem)
	}
}

func TestEnrichRouteManifestTypesDoesNotSkipFuncInImportPath(t *testing.T) {
	dir := writeUserModule(t)
	manifest := openapi.RouteManifest{
		Version: openapi.RouteManifestVersion,
		Routes: []openapi.RouteManifestRoute{{
			Method:  "GET",
			Path:    "/users/:id",
			Handler: "example.com/app/internal/server.GetUser",
		}},
	}

	warnings, err := enrichRouteManifestTypes(dir, &manifest, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v", warnings)
	}
	if len(manifest.Routes[0].ResultTypes) == 0 {
		t.Fatalf("route was not enriched: %#v", manifest.Routes[0])
	}
}

func TestEnrichRouteManifestTypesIncludesTestFiles(t *testing.T) {
	dir := writeUserModule(t)
	writeFile(t, filepath.Join(dir, "internal/server/server_test.go"), `package server

import "github.com/fox-gonic/fox"

func GetTestUser(ctx *fox.Context) (User, error) {
	return User{ID: "1", Name: "Ada"}, nil
}
`)
	manifest := openapi.RouteManifest{
		Version: openapi.RouteManifestVersion,
		Routes: []openapi.RouteManifestRoute{{
			Method:  "GET",
			Path:    "/test-users/:id",
			Handler: "example.com/app/internal/server.GetTestUser",
		}},
	}

	warnings, err := enrichRouteManifestTypes(dir, &manifest, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v", warnings)
	}
	if len(manifest.Routes[0].ResultTypes) == 0 || manifest.Routes[0].ResultTypes[0].Name != "User" {
		t.Fatalf("route was not enriched from test file: %#v", manifest.Routes[0])
	}
}

func TestCleanRuntimeHandlerNameOnlyTreatsFuncNumberSuffixAsClosure(t *testing.T) {
	for _, value := range []string{
		"example.com/my.func/pkg.Handler",
		"example.com/app.Function",
		"example.com/app.(*Handler).Function-fm",
	} {
		if isRuntimeClosureHandlerSymbol(value) {
			t.Fatalf("%q detected as closure", value)
		}
	}
	if !isRuntimeClosureHandlerSymbol("example.com/app.NewEngine.func1") {
		t.Fatal("closure was not detected")
	}
}

func TestParseRuntimeHandlerSymbolHandlesGenericRuntimeNames(t *testing.T) {
	symbol, ok := parseRuntimeHandlerSymbol("example.com/app/internal/server.GetUser[example.com/app/internal/model.User]")
	if !ok {
		t.Fatal("symbol was not parsed")
	}
	if symbol.importPath != "example.com/app/internal/server" || symbol.funcName != "GetUser" || symbol.recvName != "" {
		t.Fatalf("symbol = %#v", symbol)
	}

	symbol, ok = parseRuntimeHandlerSymbol("example.com/app/internal/server.(*Handler[example.com/app/internal/model.User]).GetUser-fm")
	if !ok {
		t.Fatal("method symbol was not parsed")
	}
	if symbol.importPath != "example.com/app/internal/server" || symbol.recvName != "Handler" || symbol.funcName != "GetUser" {
		t.Fatalf("method symbol = %#v", symbol)
	}
}

func TestParseRuntimeHandlerSymbolsIncludesValueReceiverCandidate(t *testing.T) {
	symbols := parseRuntimeHandlerSymbols("example.com/app/internal/server.Handler.GetUser")
	if len(symbols) != 2 {
		t.Fatalf("symbols = %#v", symbols)
	}
	if symbols[0].importPath != "example.com/app/internal/server" || symbols[0].recvName != "Handler" || symbols[0].funcName != "GetUser" {
		t.Fatalf("receiver symbol = %#v", symbols[0])
	}
	if symbols[1].importPath != "example.com/app/internal/server.Handler" || symbols[1].recvName != "" || symbols[1].funcName != "GetUser" {
		t.Fatalf("function fallback = %#v", symbols[1])
	}
}

func TestHandlerSignatureResolvesAliasReceivers(t *testing.T) {
	dir := writeUserModule(t)
	resolver := manifestTypeResolver{
		workdir: dir,
		pkgs:    map[string]*packages.Package{},
	}

	sig, err := resolver.handlerSignature("example.com/app/internal/server.(*AliasHandler).AliasUser-fm")
	if err != nil {
		t.Fatal(err)
	}
	results := manifestResultTypes(sig)
	if len(results) == 0 || results[0].Name != "User" {
		t.Fatalf("results = %#v", results)
	}
}

func TestHandlerSignatureResolvesValueReceivers(t *testing.T) {
	dir := writeUserModule(t)
	resolver := manifestTypeResolver{workdir: dir}

	sig, err := resolver.handlerSignature("example.com/app/internal/server.Handler.ValueUser")
	if err != nil {
		t.Fatal(err)
	}
	results := manifestResultTypes(sig)
	if len(results) == 0 || results[0].Name != "User" {
		t.Fatalf("results = %#v", results)
	}
}

func TestHandlerSignatureInstantiatesGenericFunctions(t *testing.T) {
	dir := writeUserModule(t)
	resolver := manifestTypeResolver{workdir: dir}

	sig, err := resolver.handlerSignature("example.com/app/internal/server.GetGenericRuntimeUser[example.com/app/internal/server.User]")
	if err != nil {
		t.Fatal(err)
	}
	results := manifestResultTypes(sig)
	if len(results) == 0 {
		t.Fatalf("results = %#v", results)
	}
	if results[0].Name != "GenericResponse" || len(results[0].TypeArgs) != 1 || results[0].TypeArgs[0].Name != "User" {
		t.Fatalf("generic result = %#v", results[0])
	}
	data := results[0].Fields[0]
	if data.Type.Kind == "unknown" || data.Type.Name != "User" {
		t.Fatalf("generic field was not instantiated: %#v", data.Type)
	}
}

func TestHandlerSignatureInstantiatesGenericReceivers(t *testing.T) {
	dir := writeUserModule(t)
	resolver := manifestTypeResolver{workdir: dir}

	sig, err := resolver.handlerSignature("example.com/app/internal/server.GenericHandler[example.com/app/internal/server.User].GenericUser")
	if err != nil {
		t.Fatal(err)
	}
	results := manifestResultTypes(sig)
	if len(results) == 0 {
		t.Fatalf("results = %#v", results)
	}
	if results[0].Name != "GenericResponse" || len(results[0].TypeArgs) != 1 || results[0].TypeArgs[0].Name != "User" {
		t.Fatalf("generic receiver result = %#v", results[0])
	}
	data := results[0].Fields[0]
	if data.Type.Kind == "unknown" || data.Type.Name != "User" {
		t.Fatalf("generic receiver field was not instantiated: %#v", data.Type)
	}
}
