package openapi_test

import (
	"net/http"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/require"

	"github.com/fox-gonic/openapi"
)

func TestFilterOperationsKeepsMatchingOperationsAndPrunesEmptyPaths(t *testing.T) {
	spec := &openapi3.T{Paths: openapi3.NewPaths()}
	spec.Paths.Set("/public", &openapi3.PathItem{Get: &openapi3.Operation{OperationID: "public"}})
	spec.Paths.Set("/internal", &openapi3.PathItem{Get: &openapi3.Operation{OperationID: "internal"}})

	err := openapi.ApplyFilters(spec, openapi.FilterOperations(func(op openapi.OperationContext) bool {
		return op.OperationID == "public"
	}))

	require.NoError(t, err)
	require.NotNil(t, spec.Paths.Value("/public").Get)
	require.Nil(t, spec.Paths.Value("/internal"))
}

func TestPruneUnusedComponentsRemovesFilteredOperationRefs(t *testing.T) {
	spec := &openapi3.T{
		Paths: openapi3.NewPaths(),
		Components: &openapi3.Components{
			Schemas: openapi3.Schemas{
				"Public":   openapi3.NewSchemaRef("", openapi3.NewObjectSchema()),
				"Internal": openapi3.NewSchemaRef("", openapi3.NewObjectSchema()),
			},
			Responses: openapi3.ResponseBodies{
				"PublicError":   &openapi3.ResponseRef{Value: openapi3.NewResponse().WithDescription("public")},
				"InternalError": &openapi3.ResponseRef{Value: openapi3.NewResponse().WithDescription("internal")},
			},
			SecuritySchemes: openapi3.SecuritySchemes{
				"BearerAuth": &openapi3.SecuritySchemeRef{Value: openapi3.NewSecurityScheme().WithType("http").WithScheme("bearer")},
				"Internal":   &openapi3.SecuritySchemeRef{Value: openapi3.NewSecurityScheme().WithType("apiKey").WithName("X-Internal").WithIn("header")},
			},
		},
	}
	spec.Paths.Set("/public", &openapi3.PathItem{Get: &openapi3.Operation{
		OperationID: "public",
		Responses: openapi3.NewResponses(
			openapi3.WithStatus(http.StatusOK, &openapi3.ResponseRef{Value: openapi3.NewResponse().
				WithDescription("OK").
				WithJSONSchemaRef(&openapi3.SchemaRef{Ref: "#/components/schemas/Public"})}),
		),
		Security: &openapi3.SecurityRequirements{{"BearerAuth": []string{}}},
	}})
	spec.Paths.Value("/public").Get.Responses.Set("default", &openapi3.ResponseRef{Ref: "#/components/responses/PublicError"})
	spec.Paths.Set("/internal", &openapi3.PathItem{Get: &openapi3.Operation{
		OperationID: "internal",
		Responses: openapi3.NewResponses(openapi3.WithStatus(http.StatusOK, &openapi3.ResponseRef{Value: openapi3.NewResponse().
			WithDescription("OK").
			WithJSONSchemaRef(&openapi3.SchemaRef{Ref: "#/components/schemas/Internal"})})),
		Security: &openapi3.SecurityRequirements{{"Internal": []string{}}},
	}})

	err := openapi.ApplyFilters(spec,
		openapi.FilterOperations(func(op openapi.OperationContext) bool {
			return op.OperationID == "public"
		}),
		openapi.PruneUnusedComponents(),
	)

	require.NoError(t, err)
	require.Contains(t, spec.Components.Schemas, "Public")
	require.NotContains(t, spec.Components.Schemas, "Internal")
	require.Contains(t, spec.Components.Responses, "PublicError")
	require.NotContains(t, spec.Components.Responses, "InternalError")
	require.Contains(t, spec.Components.SecuritySchemes, "BearerAuth")
	require.NotContains(t, spec.Components.SecuritySchemes, "Internal")
}

func TestPruneUnusedComponentsKeepsComponentReachedBySubpathRef(t *testing.T) {
	spec := &openapi3.T{
		Paths: openapi3.NewPaths(),
		Components: &openapi3.Components{
			Schemas: openapi3.Schemas{
				"User":   openapi3.NewSchemaRef("", openapi3.NewObjectSchema().WithProperty("name", openapi3.NewStringSchema())),
				"Unused": openapi3.NewSchemaRef("", openapi3.NewObjectSchema()),
			},
		},
	}
	spec.Paths.Set("/user-name", &openapi3.PathItem{Get: &openapi3.Operation{
		OperationID: "userName",
		Responses: openapi3.NewResponses(openapi3.WithStatus(http.StatusOK, &openapi3.ResponseRef{Value: openapi3.NewResponse().
			WithDescription("OK").
			WithJSONSchemaRef(&openapi3.SchemaRef{Ref: "#/components/schemas/User/properties/name"})})),
	}})

	err := openapi.ApplyFilters(spec, openapi.PruneUnusedComponents())

	require.NoError(t, err)
	require.Contains(t, spec.Components.Schemas, "User")
	require.NotContains(t, spec.Components.Schemas, "Unused")
}

func TestOperationExtensionBoolDefault(t *testing.T) {
	op := openapi.OperationContext{Operation: &openapi3.Operation{Extensions: map[string]any{"x-public": false}}}

	require.False(t, op.ExtensionBoolDefault("x-public", true))
	require.True(t, op.ExtensionBoolDefault("x-missing", true))
}

func TestFilterOperationExpressionMatchesExtensionConditions(t *testing.T) {
	spec := &openapi3.T{Paths: openapi3.NewPaths()}
	spec.Paths.Set("/public-sandbox", &openapi3.PathItem{Get: &openapi3.Operation{
		OperationID: "publicSandbox",
		Extensions:  map[string]any{"x-public": true, "x-product": "sandbox"},
	}})
	spec.Paths.Set("/internal-sandbox", &openapi3.PathItem{Get: &openapi3.Operation{
		OperationID: "internalSandbox",
		Extensions:  map[string]any{"x-public": false, "x-product": "sandbox"},
	}})
	spec.Paths.Set("/public-account", &openapi3.PathItem{Get: &openapi3.Operation{
		OperationID: "publicAccount",
		Extensions:  map[string]any{"x-public": true, "x-product": "account"},
	}})

	public, err := openapi.FilterOperationExpression("x-public != false")
	require.NoError(t, err)
	sandbox, err := openapi.FilterOperationExpression("x-product = sandbox")
	require.NoError(t, err)

	err = openapi.ApplyFilters(spec, public, sandbox)

	require.NoError(t, err)
	require.NotNil(t, spec.Paths.Value("/public-sandbox"))
	require.Nil(t, spec.Paths.Value("/internal-sandbox"))
	require.Nil(t, spec.Paths.Value("/public-account"))
}

func TestFilterOperationExpressionSupportsOrConditions(t *testing.T) {
	spec := &openapi3.T{Paths: openapi3.NewPaths()}
	spec.Paths.Set("/sandbox", &openapi3.PathItem{Get: &openapi3.Operation{
		OperationID: "sandbox",
		Extensions:  map[string]any{"x-product": "sandbox"},
	}})
	spec.Paths.Set("/account", &openapi3.PathItem{Get: &openapi3.Operation{
		OperationID: "account",
		Extensions:  map[string]any{"x-product": "account"},
	}})
	spec.Paths.Set("/admin", &openapi3.PathItem{Get: &openapi3.Operation{
		OperationID: "admin",
		Extensions:  map[string]any{"x-product": "admin"},
	}})

	productFilter, err := openapi.FilterOperationExpression("x-product = sandbox || x-product = account")
	require.NoError(t, err)

	err = openapi.ApplyFilters(spec, productFilter)

	require.NoError(t, err)
	require.NotNil(t, spec.Paths.Value("/sandbox"))
	require.NotNil(t, spec.Paths.Value("/account"))
	require.Nil(t, spec.Paths.Value("/admin"))
}

func TestFilterOperationExpressionKeepsQuotedOrLiteralTogether(t *testing.T) {
	spec := &openapi3.T{Paths: openapi3.NewPaths()}
	spec.Paths.Set("/compound", &openapi3.PathItem{Get: &openapi3.Operation{
		OperationID: "compound",
		Extensions:  map[string]any{"x-product": "A || B"},
	}})
	spec.Paths.Set("/simple", &openapi3.PathItem{Get: &openapi3.Operation{
		OperationID: "simple",
		Extensions:  map[string]any{"x-product": "A"},
	}})

	productFilter, err := openapi.FilterOperationExpression(`x-product = "A || B"`)
	require.NoError(t, err)

	err = openapi.ApplyFilters(spec, productFilter)

	require.NoError(t, err)
	require.NotNil(t, spec.Paths.Value("/compound"))
	require.Nil(t, spec.Paths.Value("/simple"))
}

func TestFilterOperationExpressionSupportsEscapedSingleQuote(t *testing.T) {
	spec := &openapi3.T{Paths: openapi3.NewPaths()}
	spec.Paths.Set("/publisher", &openapi3.PathItem{Get: &openapi3.Operation{
		OperationID: "publisher",
		Extensions:  map[string]any{"x-name": "O'Reilly"},
	}})
	spec.Paths.Set("/other", &openapi3.PathItem{Get: &openapi3.Operation{
		OperationID: "other",
		Extensions:  map[string]any{"x-name": "Other"},
	}})

	nameFilter, err := openapi.FilterOperationExpression(`x-name = 'O\'Reilly'`)
	require.NoError(t, err)

	err = openapi.ApplyFilters(spec, nameFilter)

	require.NoError(t, err)
	require.NotNil(t, spec.Paths.Value("/publisher"))
	require.Nil(t, spec.Paths.Value("/other"))
}

func TestFilterOperationExpressionRejectsUnsupportedExpression(t *testing.T) {
	_, err := openapi.FilterOperationExpression("x-public > false")

	require.Error(t, err)
}

func TestFilterOperationExpressionRejectsUnterminatedQuotedValue(t *testing.T) {
	_, err := openapi.FilterOperationExpression(`x-product = "sandbox`)

	require.Error(t, err)
}
