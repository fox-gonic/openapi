package openapi

import (
	"reflect"
	"strings"
)

// RouteManifestVersion is the supported Fox route manifest format version.
const RouteManifestVersion = "fox.route-manifest/v1"

// RouteManifest is the route registry exchange format written by Fox.
type RouteManifest struct {
	Version string               `json:"version"`
	Routes  []RouteManifestRoute `json:"routes"`
}

// RouteManifestRoute describes one registered Fox route.
type RouteManifestRoute struct {
	Method      string              `json:"method"`
	Path        string              `json:"path"`
	Handler     string              `json:"handler,omitempty"`
	HandlerName string              `json:"handlerName,omitempty"`
	HandlerType string              `json:"handlerType,omitempty"`
	Inputs      []string            `json:"inputs,omitempty"`
	Results     []string            `json:"results,omitempty"`
	InputTypes  []RouteManifestType `json:"inputTypes,omitempty"`
	ResultTypes []RouteManifestType `json:"resultTypes,omitempty"`
}

func (route RouteManifestRoute) HandlerSymbol() string {
	if route.Handler != "" {
		return route.Handler
	}
	return route.HandlerName
}

// RouteManifestType is a serializable subset of Go reflect.Type.
type RouteManifestType struct {
	Kind     string               `json:"kind"`
	String   string               `json:"string,omitempty"`
	Name     string               `json:"name,omitempty"`
	PkgPath  string               `json:"pkgPath,omitempty"`
	TypeArgs []RouteManifestType  `json:"typeArgs,omitempty"`
	Key      *RouteManifestType   `json:"key,omitempty"`
	Elem     *RouteManifestType   `json:"elem,omitempty"`
	Fields   []RouteManifestField `json:"fields,omitempty"`
}

// RouteManifestField is a serializable subset of Go reflect.StructField.
type RouteManifestField struct {
	Name      string            `json:"name"`
	PkgPath   string            `json:"pkgPath,omitempty"`
	Tag       string            `json:"tag,omitempty"`
	Anonymous bool              `json:"anonymous,omitempty"`
	Type      RouteManifestType `json:"type"`
}

func manifestRouteReturnsError(route RouteManifestRoute) bool {
	for _, result := range route.ResultTypes {
		if manifestIsErrorType(result) {
			return true
		}
	}
	for _, result := range route.Results {
		if result == "error" {
			return true
		}
	}
	return false
}

func manifestSuccessBody(route RouteManifestRoute) (RouteManifestType, bool) {
	if len(route.ResultTypes) == 0 {
		return RouteManifestType{}, false
	}
	first := derefManifestType(route.ResultTypes[0])
	if manifestIsErrorType(first) {
		return RouteManifestType{}, false
	}
	return route.ResultTypes[0], true
}

func manifestRequestBody(route RouteManifestRoute) (RouteManifestType, bool) {
	for _, input := range route.InputTypes {
		if manifestIsFoxContext(input) {
			continue
		}
		return input, true
	}
	return RouteManifestType{}, false
}

func manifestStatusWrapperBodyType(typ RouteManifestType) (RouteManifestType, bool) {
	typ = derefManifestType(typ)
	if typ.Kind != "struct" {
		return RouteManifestType{}, false
	}
	for _, field := range typ.Fields {
		if strings.EqualFold(field.Name, "data") {
			return field.Type, true
		}
	}

	var body *RouteManifestType
	for _, field := range typ.Fields {
		if manifestIsStatusField(field) {
			continue
		}
		if body != nil {
			return RouteManifestType{}, false
		}
		body = &field.Type
	}
	if body == nil {
		return RouteManifestType{}, false
	}
	return *body, true
}

func manifestIsErrorType(typ RouteManifestType) bool {
	typ = derefManifestType(typ)
	return typ.Name == "error" || typ.String == "error"
}

func manifestIsFoxContext(typ RouteManifestType) bool {
	typ = derefManifestType(typ)
	return typ.Name == "Context" && typ.PkgPath == "github.com/fox-gonic/fox"
}

func manifestIsTimeType(typ RouteManifestType) bool {
	typ = derefManifestType(typ)
	return (typ.Name == "Time" && typ.PkgPath == "time") || typ.String == "time.Time"
}

func manifestIsStatusField(field RouteManifestField) bool {
	if strings.EqualFold(field.Name, "status") {
		return true
	}
	switch derefManifestType(field.Type).Kind {
	case "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64":
		return true
	}
	return false
}

func derefManifestType(typ RouteManifestType) RouteManifestType {
	for typ.Kind == "ptr" || typ.Kind == "pointer" {
		if typ.Elem == nil {
			return typ
		}
		typ = *typ.Elem
	}
	return typ
}

func manifestTagName(tag, key string) string {
	value := reflect.StructTag(tag).Get(key)
	if value == "" || value == "-" {
		return ""
	}
	name := splitManifestTag(value)
	if name == "-" {
		return ""
	}
	return name
}

func manifestHasBinding(tag, rule string) bool {
	value := reflect.StructTag(tag).Get("binding")
	for value != "" {
		var part string
		part, value, _ = strings.Cut(value, ",")
		name, _, _ := strings.Cut(part, "=")
		if name == rule {
			return true
		}
	}
	return false
}

func splitManifestTag(value string) string {
	name, _, _ := strings.Cut(value, ",")
	return name
}
