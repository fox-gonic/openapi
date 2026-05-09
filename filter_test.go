package openapi_test

import (
	"encoding/json"
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

func TestExcludeOperationsWithExtensionValueHandlesNonComparableValues(t *testing.T) {
	spec := &openapi3.T{Paths: openapi3.NewPaths()}
	spec.Paths.Set("/internal", &openapi3.PathItem{Get: &openapi3.Operation{
		OperationID: "internal",
		Extensions:  map[string]any{"x-filter": map[string]any{"scope": "internal"}},
	}})
	spec.Paths.Set("/public", &openapi3.PathItem{Get: &openapi3.Operation{
		OperationID: "public",
		Extensions:  map[string]any{"x-filter": map[string]any{"scope": "public"}},
	}})

	err := openapi.ApplyFilters(spec, openapi.ExcludeOperationsWithExtensionValue(
		"x-filter",
		map[string]any{"scope": "internal"},
	))

	require.NoError(t, err)
	require.Nil(t, spec.Paths.Value("/internal"))
	require.NotNil(t, spec.Paths.Value("/public"))
}

func TestExcludeOperationsWithExtensionValueHandlesNumericScalarTypes(t *testing.T) {
	spec := &openapi3.T{Paths: openapi3.NewPaths()}
	spec.Paths.Set("/created", &openapi3.PathItem{Get: &openapi3.Operation{
		OperationID: "created",
		Extensions:  map[string]any{"x-status": json.Number("201")},
	}})
	spec.Paths.Set("/accepted", &openapi3.PathItem{Get: &openapi3.Operation{
		OperationID: "accepted",
		Extensions:  map[string]any{"x-status": json.Number("202")},
	}})

	err := openapi.ApplyFilters(spec, openapi.ExcludeOperationsWithExtensionValue("x-status", int64(201)))

	require.NoError(t, err)
	require.Nil(t, spec.Paths.Value("/created"))
	require.NotNil(t, spec.Paths.Value("/accepted"))
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

func TestPruneUnusedComponentsKeepsDiscriminatorMappingRefs(t *testing.T) {
	pet := openapi3.NewObjectSchema()
	pet.Discriminator = &openapi3.Discriminator{
		PropertyName: "kind",
		Mapping: openapi3.StringMap[openapi3.MappingRef]{
			"cat": {Ref: "#/components/schemas/Cat"},
		},
	}
	spec := &openapi3.T{
		Paths: openapi3.NewPaths(),
		Components: &openapi3.Components{
			Schemas: openapi3.Schemas{
				"Pet":    openapi3.NewSchemaRef("", pet),
				"Cat":    openapi3.NewSchemaRef("", openapi3.NewObjectSchema()),
				"Unused": openapi3.NewSchemaRef("", openapi3.NewObjectSchema()),
			},
		},
	}
	spec.Paths.Set("/pets", &openapi3.PathItem{Get: &openapi3.Operation{
		OperationID: "pets",
		Responses: openapi3.NewResponses(openapi3.WithStatus(http.StatusOK, &openapi3.ResponseRef{Value: openapi3.NewResponse().
			WithDescription("OK").
			WithJSONSchemaRef(&openapi3.SchemaRef{Ref: "#/components/schemas/Pet"})})),
	}})

	err := openapi.ApplyFilters(spec, openapi.PruneUnusedComponents())

	require.NoError(t, err)
	require.Contains(t, spec.Components.Schemas, "Pet")
	require.Contains(t, spec.Components.Schemas, "Cat")
	require.NotContains(t, spec.Components.Schemas, "Unused")
}

func TestPruneUnusedComponentsKeepsOperationRefComponentRefs(t *testing.T) {
	spec := &openapi3.T{
		Paths: openapi3.NewPaths(),
		Components: &openapi3.Components{
			Links: openapi3.Links{
				"UserLookup": &openapi3.LinkRef{Value: &openapi3.Link{OperationRef: "#/components/schemas/User"}},
				"UnusedLink": &openapi3.LinkRef{Value: &openapi3.Link{OperationID: "unused"}},
			},
			Schemas: openapi3.Schemas{
				"User":   openapi3.NewSchemaRef("", openapi3.NewObjectSchema()),
				"Unused": openapi3.NewSchemaRef("", openapi3.NewObjectSchema()),
			},
		},
	}
	response := openapi3.NewResponse().WithDescription("OK")
	response.Links = openapi3.Links{"user": &openapi3.LinkRef{Ref: "#/components/links/UserLookup"}}
	spec.Paths.Set("/users", &openapi3.PathItem{Get: &openapi3.Operation{
		OperationID: "users",
		Responses:   openapi3.NewResponses(openapi3.WithStatus(http.StatusOK, &openapi3.ResponseRef{Value: response})),
	}})

	err := openapi.ApplyFilters(spec, openapi.PruneUnusedComponents())

	require.NoError(t, err)
	require.Contains(t, spec.Components.Links, "UserLookup")
	require.NotContains(t, spec.Components.Links, "UnusedLink")
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

func TestFilterOperationExpressionMatchesNumericLiteral(t *testing.T) {
	spec := &openapi3.T{Paths: openapi3.NewPaths()}
	spec.Paths.Set("/created", &openapi3.PathItem{Get: &openapi3.Operation{
		OperationID: "created",
		Extensions:  map[string]any{"x-status": json.Number("201")},
	}})
	spec.Paths.Set("/accepted", &openapi3.PathItem{Get: &openapi3.Operation{
		OperationID: "accepted",
		Extensions:  map[string]any{"x-status": json.Number("202")},
	}})

	statusFilter, err := openapi.FilterOperationExpression("x-status = 201")
	require.NoError(t, err)

	err = openapi.ApplyFilters(spec, statusFilter)

	require.NoError(t, err)
	require.NotNil(t, spec.Paths.Value("/created"))
	require.Nil(t, spec.Paths.Value("/accepted"))
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
