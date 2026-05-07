package openapi

import (
	"net/http"
	"testing"
)

type manifestStatusResponse[T any] struct {
	status int
	data   T
}

type manifestTemplatePayload struct {
	ID string `json:"id"`
}

func manifestStatusResponseWithStatus[T any](status int, data T) manifestStatusResponse[T] {
	return manifestStatusResponse[T]{
		status: status,
		data:   data,
	}
}

func createManifestTemplateForRouteManifest() (manifestStatusResponse[manifestTemplatePayload], error) {
	return manifestStatusResponseWithStatus(http.StatusAccepted, manifestTemplatePayload{}), nil
}

func TestNewFromRouteManifestGeneratesPaths(t *testing.T) {
	manifest := RouteManifest{
		Version: "fox.route-manifest/v1",
		Routes: []RouteManifestRoute{{
			Method:  http.MethodGet,
			Path:    "/users/:id",
			Handler: "example.com/app/internal/handler.GetUser",
			InputTypes: []RouteManifestType{
				{
					Kind:    "struct",
					Name:    "GetUserRequest",
					PkgPath: "example.com/app/internal/handler",
					Fields: []RouteManifestField{
						{Name: "ID", Tag: `uri:"id" binding:"required"`, Type: RouteManifestType{Kind: "string", Name: "string"}},
						{Name: "Search", Tag: `query:"search"`, Type: RouteManifestType{Kind: "string", Name: "string"}},
					},
				},
			},
			ResultTypes: []RouteManifestType{
				{
					Kind:    "struct",
					Name:    "User",
					PkgPath: "example.com/app/internal/handler",
					Fields: []RouteManifestField{
						{Name: "ID", Tag: `json:"id"`, Type: RouteManifestType{Kind: "string", Name: "string"}},
						{Name: "Name", Tag: `json:"name"`, Type: RouteManifestType{Kind: "string", Name: "string"}},
					},
				},
				{Kind: "interface", Name: "error"},
			},
		}},
	}

	spec := NewFromRouteManifest(manifest, Info("Manifest API", "1.0.0")).Spec()
	path := spec.Paths.Value("/users/{id}")
	if path == nil || path.Get == nil {
		t.Fatalf("expected GET /users/{id}, got %#v", spec.Paths.Map())
	}
	if path.Get.OperationID != "example_com_app_internal_handler_GetUser" {
		t.Fatalf("operationId = %q", path.Get.OperationID)
	}
	if len(path.Get.Parameters) != 2 || path.Get.Parameters[0].Value.Name != "id" {
		t.Fatalf("parameters = %#v", path.Get.Parameters)
	}
	if path.Get.Parameters[1].Value.Name != "search" {
		t.Fatalf("parameters = %#v", path.Get.Parameters)
	}
	response := path.Get.Responses.Value("200")
	if response == nil {
		t.Fatalf("missing 200 response: %#v", path.Get.Responses.Map())
	}
	content := response.Value.Content.Get("application/json")
	if content == nil || content.Schema.Ref != "#/components/schemas/handler_User" {
		t.Fatalf("response content = %#v", response.Value.Content)
	}
	userSchema := spec.Components.Schemas["handler_User"]
	if userSchema == nil || userSchema.Value.Properties["id"] == nil {
		t.Fatalf("missing user schema: %#v", spec.Components.Schemas)
	}
	if path.Get.Responses.Value("default") == nil {
		t.Fatalf("missing default response: %#v", path.Get.Responses.Map())
	}
}

func TestNewFromRouteManifestOperationIDFallsBackToMethodPath(t *testing.T) {
	manifest := RouteManifest{
		Version: "fox.route-manifest/v1",
		Routes: []RouteManifestRoute{{
			Method: http.MethodGet,
			Path:   "/users/:id",
		}},
	}

	spec := NewFromRouteManifest(manifest, Info("Manifest API", "1.0.0")).Spec()
	path := spec.Paths.Value("/users/{id}")
	if path == nil || path.Get == nil {
		t.Fatalf("expected GET /users/{id}, got %#v", spec.Paths.Map())
	}
	if path.Get.OperationID != "GET__users__id" {
		t.Fatalf("operationId = %q", path.Get.OperationID)
	}
}

func TestNewFromRouteManifestUnwrapsStatusResponseBody(t *testing.T) {
	manifest := RouteManifest{
		Version: "fox.route-manifest/v1",
		Routes: []RouteManifestRoute{{
			Method:  http.MethodPost,
			Path:    "/sandbox/templates",
			Handler: "github.com/fox-gonic/openapi.createManifestTemplateForRouteManifest",
			ResultTypes: []RouteManifestType{
				{
					Kind:    "struct",
					Name:    "StatusResponse",
					PkgPath: "github.com/aonesuite/infra/internal/products/sandbox/handler",
					TypeArgs: []RouteManifestType{{
						Kind:    "struct",
						Name:    "TemplateResponse",
						PkgPath: "github.com/aonesuite/infra/internal/products/sandbox/handler",
					}},
					Fields: []RouteManifestField{
						{Name: "status", PkgPath: "github.com/aonesuite/infra/internal/products/sandbox/handler", Type: RouteManifestType{Kind: "int", Name: "int"}},
						{Name: "data", PkgPath: "github.com/aonesuite/infra/internal/products/sandbox/handler", Type: RouteManifestType{
							Kind:    "struct",
							Name:    "TemplateResponse",
							PkgPath: "github.com/aonesuite/infra/internal/products/sandbox/handler",
							Fields: []RouteManifestField{
								{Name: "ID", Tag: `json:"id"`, Type: RouteManifestType{Kind: "string", Name: "string"}},
							},
						}},
					},
				},
			},
		}},
	}

	spec := NewFromRouteManifest(manifest, Info("Manifest API", "1.0.0"), Source([]string{"./..."}, IncludeTestFiles())).Spec()
	response := spec.Paths.Value("/sandbox/templates").Post.Responses.Value("202")
	if response == nil {
		t.Fatalf("missing 202 response")
	}
	content := response.Value.Content.Get("application/json")
	if content == nil {
		t.Fatalf("missing response content")
	}
	const wantRef = "#/components/schemas/handler_TemplateResponse"
	if content.Schema.Ref != wantRef {
		t.Fatalf("response schema ref = %q, want %q", content.Schema.Ref, wantRef)
	}
	if spec.Components.Schemas["handler_StatusResponse_TemplateResponse"] != nil {
		t.Fatalf("generated status wrapper schema: %#v", spec.Components.Schemas)
	}
	if spec.Components.Schemas["handler_StatusResponse_github_com_aonesuite_infra_internal_products_sandbox_handler_TemplateResponse"] != nil {
		t.Fatalf("generated long generic schema name: %#v", spec.Components.Schemas)
	}
}

func TestNewFromRouteManifestUsesByteSchema(t *testing.T) {
	manifest := RouteManifest{
		Version: "fox.route-manifest/v1",
		Routes: []RouteManifestRoute{{
			Method:  http.MethodGet,
			Path:    "/files/:id/content",
			Handler: "example.com/app/internal/handler.GetFileContent",
			ResultTypes: []RouteManifestType{{
				Kind: "slice",
				Elem: &RouteManifestType{Kind: "uint8", Name: "uint8"},
			}},
		}},
	}

	spec := NewFromRouteManifest(manifest, Info("Manifest API", "1.0.0")).Spec()
	response := spec.Paths.Value("/files/{id}/content").Get.Responses.Value("200")
	if response == nil {
		t.Fatalf("missing 200 response")
	}
	content := response.Value.Content.Get("application/json")
	if content == nil || content.Schema == nil || content.Schema.Value == nil {
		t.Fatalf("missing response schema: %#v", response.Value.Content)
	}
	schema := content.Schema.Value
	if schema.Type == nil || !schema.Type.Is("string") || schema.Format != "byte" {
		t.Fatalf("schema = %#v, want string byte schema", schema)
	}
}

func TestManifestSchemaNameUsesStructuredTypeArgs(t *testing.T) {
	name := manifestSchemaName(RouteManifestType{
		Kind:    "struct",
		Name:    "StatusResponse",
		PkgPath: "github.com/aonesuite/infra/internal/products/sandbox/handler",
		TypeArgs: []RouteManifestType{{
			Kind:    "struct",
			Name:    "TemplateResponse",
			PkgPath: "github.com/aonesuite/infra/internal/products/sandbox/handler",
		}},
	})

	if name != "handler_StatusResponse_TemplateResponse" {
		t.Fatalf("schema name = %q", name)
	}
}

func TestManifestSchemaNameKeepsLegacyGenericStringCompatibility(t *testing.T) {
	name := manifestSchemaName(RouteManifestType{
		Kind:    "struct",
		Name:    "StatusResponse[github.com/aonesuite/infra/internal/products/sandbox/handler.TemplateResponse]",
		PkgPath: "github.com/aonesuite/infra/internal/products/sandbox/handler",
	})

	if name != "handler_StatusResponse_TemplateResponse" {
		t.Fatalf("schema name = %q", name)
	}
}

func TestManifestTypeKeyDistinguishesAnonymousStructs(t *testing.T) {
	first := RouteManifestType{
		Kind: "struct",
		Fields: []RouteManifestField{{
			Name: "ID",
			Tag:  `json:"id"`,
			Type: RouteManifestType{Kind: "string", Name: "string"},
		}},
	}
	second := RouteManifestType{
		Kind: "struct",
		Fields: []RouteManifestField{{
			Name: "Name",
			Tag:  `json:"name"`,
			Type: RouteManifestType{Kind: "string", Name: "string"},
		}},
	}

	if manifestTypeKey(first) == "" {
		t.Fatal("first key is empty")
	}
	if manifestTypeKey(first) == manifestTypeKey(second) {
		t.Fatalf("anonymous struct keys collided: %q", manifestTypeKey(first))
	}
}
