package openapi

import (
	"encoding/json"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/goccy/go-yaml"
)

// SpecMetadata contains top-level OpenAPI metadata used by generated drivers.
type SpecMetadata struct {
	InfoDescription    string
	ServerDescriptions []string
	Tags               []SpecTag
}

// SpecTag describes a top-level OpenAPI tag.
type SpecTag struct {
	Name         string
	Description  string
	ExternalDocs *SpecExternalDocs
}

// SpecExternalDocs describes top-level tag external documentation.
type SpecExternalDocs struct {
	Description string
	URL         string
}

// ApplySpecMetadata applies top-level metadata to a generated OpenAPI model.
func ApplySpecMetadata(spec *openapi3.T, metadata SpecMetadata) {
	if spec == nil {
		return
	}
	if metadata.InfoDescription != "" && spec.Info != nil {
		spec.Info.Description = metadata.InfoDescription
	}
	for i, description := range metadata.ServerDescriptions {
		if description != "" && len(spec.Servers) > i && spec.Servers[i] != nil {
			spec.Servers[i].Description = description
		}
	}
	if len(metadata.Tags) > 0 {
		spec.Tags = make(openapi3.Tags, 0, len(metadata.Tags))
		for _, tag := range metadata.Tags {
			if tag.Name == "" {
				continue
			}
			out := &openapi3.Tag{Name: tag.Name, Description: tag.Description}
			if tag.ExternalDocs != nil {
				out.ExternalDocs = &openapi3.ExternalDocs{
					Description: tag.ExternalDocs.Description,
					URL:         tag.ExternalDocs.URL,
				}
			}
			spec.Tags = append(spec.Tags, out)
		}
	}
}

// MarshalSpecJSON serializes an OpenAPI model as formatted JSON.
func MarshalSpecJSON(spec *openapi3.T) ([]byte, error) {
	return json.MarshalIndent(spec, "", "  ")
}

// MarshalSpecYAML serializes an OpenAPI model as YAML.
func MarshalSpecYAML(spec *openapi3.T) ([]byte, error) {
	return yaml.Marshal(spec)
}
