package openapi

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
)

// commentDocs holds Go doc comments extracted from source so they can enrich
// the generated spec with summaries, descriptions and field documentation.
//
// Function lookup is keyed by fully-qualified runtime name (pkg.Func, or
// pkg.(*Type).Method, or the trailing identifier as a fallback for closures).
// Field lookup is keyed by (typeName, fieldName) — typeName uses the package
// name as it appears in source (the runtime PkgPath last segment).
type commentDocs struct {
	funcsByQualified  map[string]string
	funcsByShort      map[string]string
	statusByQualified map[string]int
	fieldsByType      map[string]map[string]string
	includeTests      bool
}

func newCommentDocs() *commentDocs {
	return &commentDocs{
		funcsByQualified:  make(map[string]string),
		funcsByShort:      make(map[string]string),
		statusByQualified: make(map[string]int),
		fieldsByType:      make(map[string]map[string]string),
	}
}

// SourceOption configures Source.
type SourceOption func(*sourceConfig)

type sourceConfig struct {
	includeTests bool
}

// IncludeTestFiles makes Source also scan _test.go files. By default test
// files are skipped so comments authored in tests do not bleed into
// production specs; this option exists for the rare case where handler
// documentation lives in test fixtures.
func IncludeTestFiles() SourceOption {
	return func(c *sourceConfig) {
		c.includeTests = true
	}
}

// Source enables Go comment extraction from the provided paths. A path ending
// in "/..." recursively walks the tree skipping vendor and dot-prefixed
// directories. Test files (_test.go) are skipped unless IncludeTestFiles is
// passed.
func Source(paths []string, opts ...SourceOption) Option {
	cfg := sourceConfig{}
	for _, opt := range opts {
		opt(&cfg)
	}
	return func(g *Generator) {
		docs := newCommentDocs()
		docs.includeTests = cfg.includeTests
		for _, path := range paths {
			if err := docs.load(path); err != nil {
				g.warnf("openapi source %q: %v", path, err)
			}
		}
		g.docs = docs
	}
}

func (d *commentDocs) load(path string) error {
	if strings.HasSuffix(path, "/...") {
		return d.loadTree(strings.TrimSuffix(path, "/..."))
	}
	return d.loadDir(path)
}

func (d *commentDocs) loadTree(root string) error {
	root = filepath.Clean(root)
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			return nil
		}
		name := entry.Name()
		if path != root && (name == "vendor" || strings.HasPrefix(name, ".")) {
			return filepath.SkipDir
		}
		return d.loadDir(path)
	})
}

func (d *commentDocs) loadDir(dir string) error {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, func(info fs.FileInfo) bool {
		name := info.Name()
		if !strings.HasSuffix(name, ".go") {
			return false
		}
		if !d.includeTests && strings.HasSuffix(name, "_test.go") {
			return false
		}
		return true
	}, parser.ParseComments)
	if err != nil {
		return err
	}

	for pkgName, pkg := range pkgs {
		for _, file := range pkg.Files {
			d.addFile(pkgName, file)
		}
	}
	return nil
}

func (d *commentDocs) addFile(pkgName string, file *ast.File) {
	imports := importAliases(file)
	for _, decl := range file.Decls {
		switch decl := decl.(type) {
		case *ast.FuncDecl:
			d.addFunc(pkgName, decl, imports)
		case *ast.GenDecl:
			d.addTypeDecl(decl)
		}
	}
}

func (d *commentDocs) addFunc(pkgName string, decl *ast.FuncDecl, imports map[string]string) {
	short := decl.Name.Name
	qualified := pkgName + "." + short
	if decl.Recv != nil && len(decl.Recv.List) > 0 {
		recv := receiverName(decl.Recv.List[0].Type)
		if recv != "" {
			qualified = pkgName + "." + recv + "." + short
		}
	}

	if decl.Doc != nil {
		text := commentText(decl.Doc)
		d.funcsByQualified[qualified] = text
		// Short name is best-effort fallback — last writer wins on collision.
		d.funcsByShort[short] = text
	}

	if status, ok := inferredReturnStatus(decl, imports); ok {
		d.statusByQualified[qualified] = status
	}
}

func importAliases(file *ast.File) map[string]string {
	imports := make(map[string]string)
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}
		name := ""
		if spec.Name != nil {
			name = spec.Name.Name
		}
		if name == "" {
			if idx := strings.LastIndex(path, "/"); idx >= 0 {
				name = path[idx+1:]
			} else {
				name = path
			}
		}
		imports[name] = path
	}
	return imports
}

func receiverName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		if id, ok := t.X.(*ast.Ident); ok {
			return "(*" + id.Name + ")"
		}
	}
	return ""
}

func (d *commentDocs) addTypeDecl(decl *ast.GenDecl) {
	for _, spec := range decl.Specs {
		typeSpec, ok := spec.(*ast.TypeSpec)
		if !ok {
			continue
		}
		structType, ok := typeSpec.Type.(*ast.StructType)
		if !ok {
			continue
		}
		for _, field := range structType.Fields.List {
			text := fieldCommentText(field)
			if text == "" {
				continue
			}
			for _, name := range field.Names {
				if d.fieldsByType[typeSpec.Name.Name] == nil {
					d.fieldsByType[typeSpec.Name.Name] = make(map[string]string)
				}
				d.fieldsByType[typeSpec.Name.Name][name.Name] = text
			}
		}
	}
}

func fieldCommentText(field *ast.Field) string {
	parts := make([]string, 0, 2)
	if field.Doc != nil {
		parts = append(parts, commentText(field.Doc))
	}
	if field.Comment != nil {
		text := commentText(field.Comment)
		if len(parts) == 0 || parts[len(parts)-1] != text {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n\n")
}

// funcDoc looks up a function's doc comment. runtimeName is what
// runtime.FuncForPC returns, e.g. "myapp/handlers.GetUser",
// "myapp/handlers.(*Server).GetUser-fm" for method values, or
// "myapp/handlers.NewServer.func1" for closures. The lookup tries the fully
// qualified name first (after stripping -fm and .funcN suffixes) and falls
// back to the trailing identifier.
func (d *commentDocs) funcDoc(runtimeName string) string {
	if d == nil || runtimeName == "" {
		return ""
	}
	qualified := normalizeRuntimeFuncName(runtimeName)
	if text, ok := d.funcsByQualified[qualified]; ok {
		return text
	}
	short := qualified
	if idx := strings.LastIndex(short, "."); idx >= 0 {
		short = short[idx+1:]
	}
	return d.funcsByShort[short]
}

func (d *commentDocs) returnStatus(runtimeName string) (int, bool) {
	if d == nil || runtimeName == "" {
		return 0, false
	}
	qualified := normalizeRuntimeFuncName(runtimeName)
	if status, ok := d.statusByQualified[qualified]; ok {
		return status, true
	}
	return 0, false
}

func (d *commentDocs) fieldDoc(typeName, fieldName string) string {
	if d == nil {
		return ""
	}
	return d.fieldsByType[typeName][fieldName]
}

// normalizeRuntimeFuncName strips method-value (-fm) and closure (.funcN)
// suffixes and reduces the leading import path to its last segment so the
// result is comparable to "pkgName.Func" or "pkgName.(*Recv).Method" indexed
// from source.
func normalizeRuntimeFuncName(name string) string {
	name = strings.TrimSuffix(name, "-fm")
	for {
		idx := strings.LastIndex(name, ".func")
		if idx < 0 {
			break
		}
		suffix := name[idx+len(".func"):]
		if suffix == "" || !allDigits(suffix) {
			break
		}
		name = name[:idx]
	}
	if slash := strings.LastIndex(name, "/"); slash >= 0 {
		name = name[slash+1:]
	}
	return name
}

func allDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func commentText(group *ast.CommentGroup) string {
	return strings.TrimSpace(group.Text())
}

func firstParagraph(text string) string {
	if idx := strings.Index(text, "\n\n"); idx >= 0 {
		return text[:idx]
	}
	return text
}

func inferredReturnStatus(decl *ast.FuncDecl, imports map[string]string) (int, bool) {
	if decl.Body == nil {
		return 0, false
	}

	status := 0
	found := false
	conflict := false
	ast.Inspect(decl.Body, func(node ast.Node) bool {
		if conflict {
			return false
		}
		if _, ok := node.(*ast.FuncLit); ok {
			return false
		}
		ret, ok := node.(*ast.ReturnStmt)
		if !ok {
			return true
		}
		next, ok := statusReturn(ret, imports)
		if !ok {
			return true
		}
		if !found {
			status = next
			found = true
			return true
		}
		if status != next {
			conflict = true
			return false
		}
		return true
	})
	return status, found && !conflict
}

func statusReturn(ret *ast.ReturnStmt, imports map[string]string) (int, bool) {
	if len(ret.Results) == 0 {
		return 0, false
	}
	if len(ret.Results) > 1 {
		last, ok := ret.Results[len(ret.Results)-1].(*ast.Ident)
		if !ok || last.Name != "nil" {
			return 0, false
		}
	}
	call, ok := ret.Results[0].(*ast.CallExpr)
	if !ok || len(call.Args) == 0 {
		return 0, false
	}
	return httpStatusValue(call.Args[0], imports)
}

func httpStatusValue(expr ast.Expr, imports map[string]string) (int, bool) {
	switch expr := expr.(type) {
	case *ast.BasicLit:
		if expr.Kind != token.INT {
			return 0, false
		}
		parsed, err := strconv.ParseInt(expr.Value, 0, 0)
		value := int(parsed)
		if err != nil || value < 200 || value >= 300 {
			return 0, false
		}
		return value, true
	case *ast.SelectorExpr:
		ident, ok := expr.X.(*ast.Ident)
		if !ok || imports[ident.Name] != "net/http" {
			return 0, false
		}
		return httpStatusConstant(expr.Sel.Name)
	}
	return 0, false
}

func httpStatusConstant(name string) (int, bool) {
	switch name {
	case "StatusOK":
		return http.StatusOK, true
	case "StatusCreated":
		return http.StatusCreated, true
	case "StatusAccepted":
		return http.StatusAccepted, true
	case "StatusNonAuthoritativeInfo":
		return http.StatusNonAuthoritativeInfo, true
	case "StatusNoContent":
		return http.StatusNoContent, true
	case "StatusResetContent":
		return http.StatusResetContent, true
	case "StatusPartialContent":
		return http.StatusPartialContent, true
	case "StatusMultiStatus":
		return http.StatusMultiStatus, true
	case "StatusAlreadyReported":
		return http.StatusAlreadyReported, true
	case "StatusIMUsed":
		return http.StatusIMUsed, true
	}
	return 0, false
}
