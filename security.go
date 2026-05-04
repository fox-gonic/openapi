package openapi

import "github.com/getkin/kin-openapi/openapi3"

// SecuritySchemeConfig describes a serializable OpenAPI security scheme.
type SecuritySchemeConfig struct {
	Type             string
	Description      string
	Name             string
	In               string
	Scheme           string
	BearerFormat     string
	OpenIDConnectURL string
	Flows            *OAuthFlowsConfig
}

// OAuthFlowsConfig describes serializable OAuth2 flows.
type OAuthFlowsConfig struct {
	Implicit          *OAuthFlowConfig
	Password          *OAuthFlowConfig
	ClientCredentials *OAuthFlowConfig
	AuthorizationCode *OAuthFlowConfig
}

// OAuthFlowConfig describes a serializable OAuth2 flow.
type OAuthFlowConfig struct {
	AuthorizationURL string
	TokenURL         string
	RefreshURL       string
	Scopes           map[string]string
}

// SecurityScheme registers an OpenAPI security scheme.
func SecurityScheme(name string, scheme *openapi3.SecurityScheme) Option {
	return func(g *Generator) {
		if g.spec.Components.SecuritySchemes == nil {
			g.spec.Components.SecuritySchemes = openapi3.SecuritySchemes{}
		}
		g.spec.Components.SecuritySchemes[name] = &openapi3.SecuritySchemeRef{Value: scheme}
	}
}

// SecuritySchemeFromConfig registers a serializable OpenAPI security scheme.
func SecuritySchemeFromConfig(name string, scheme SecuritySchemeConfig) Option {
	return SecurityScheme(name, securitySchemeFromConfig(scheme))
}

// HTTPBearerSecurity creates an HTTP bearer security scheme.
func HTTPBearerSecurity(description string) *openapi3.SecurityScheme {
	return &openapi3.SecurityScheme{
		Type:         "http",
		Scheme:       "bearer",
		BearerFormat: "JWT",
		Description:  description,
	}
}

func securitySchemeFromConfig(s SecuritySchemeConfig) *openapi3.SecurityScheme {
	scheme := &openapi3.SecurityScheme{
		Type:             s.Type,
		Description:      s.Description,
		Name:             s.Name,
		In:               s.In,
		Scheme:           s.Scheme,
		BearerFormat:     s.BearerFormat,
		OpenIdConnectUrl: s.OpenIDConnectURL,
	}
	if s.Flows != nil {
		scheme.Flows = oauthFlowsFromConfig(s.Flows)
	}
	return scheme
}

func oauthFlowsFromConfig(flows *OAuthFlowsConfig) *openapi3.OAuthFlows {
	if flows == nil {
		return nil
	}
	return &openapi3.OAuthFlows{
		Implicit:          oauthFlowFromConfig(flows.Implicit),
		Password:          oauthFlowFromConfig(flows.Password),
		ClientCredentials: oauthFlowFromConfig(flows.ClientCredentials),
		AuthorizationCode: oauthFlowFromConfig(flows.AuthorizationCode),
	}
}

func oauthFlowFromConfig(flow *OAuthFlowConfig) *openapi3.OAuthFlow {
	if flow == nil {
		return nil
	}
	return &openapi3.OAuthFlow{
		AuthorizationURL: flow.AuthorizationURL,
		TokenURL:         flow.TokenURL,
		RefreshURL:       flow.RefreshURL,
		Scopes:           flow.Scopes,
	}
}
