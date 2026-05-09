package openapi

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

// Filter mutates a generated OpenAPI document.
type Filter func(*openapi3.T) error

// OperationContext describes an operation while filtering.
type OperationContext struct {
	OperationID string
	Path        string
	Method      string
	PathItem    *openapi3.PathItem
	Operation   *openapi3.Operation
}

// Extension returns an operation extension value.
func (o OperationContext) Extension(name string) (any, bool) {
	if o.Operation == nil || o.Operation.Extensions == nil {
		return nil, false
	}
	value, ok := o.Operation.Extensions[name]
	return value, ok
}

// ExtensionBoolDefault returns a boolean extension value or fallback when the
// extension is absent or not a boolean.
func (o OperationContext) ExtensionBoolDefault(name string, fallback bool) bool {
	value, ok := o.Extension(name)
	if !ok {
		return fallback
	}
	boolValue, ok := value.(bool)
	if !ok {
		return fallback
	}
	return boolValue
}

// FilterOperations removes operations for which keep returns false. Paths with
// no remaining operations are removed.
func FilterOperations(keep func(OperationContext) bool) Filter {
	return func(spec *openapi3.T) error {
		if spec == nil || spec.Paths == nil || keep == nil {
			return nil
		}
		for path, item := range spec.Paths.Map() {
			if item == nil {
				continue
			}
			ops := item.Operations()
			remaining := len(ops)
			for method, operation := range ops {
				ctx := OperationContext{
					OperationID: operation.OperationID,
					Path:        path,
					Method:      method,
					PathItem:    item,
					Operation:   operation,
				}
				if !keep(ctx) {
					item.SetOperation(method, nil)
					remaining--
				}
			}
			if remaining == 0 {
				spec.Paths.Delete(path)
			}
		}
		return nil
	}
}

// ExcludeOperationsWithExtensionValue removes operations whose extension equals
// the provided scalar value. Matching extensions are also removed from retained
// operations so project-private routing metadata does not leak into derived
// documents.
func ExcludeOperationsWithExtensionValue(extension string, value any) Filter {
	return FilterOperations(func(op OperationContext) bool {
		if op.Operation == nil || op.Operation.Extensions == nil {
			return true
		}
		got, ok := op.Operation.Extensions[extension]
		if !ok {
			return true
		}
		if got == value {
			return false
		}
		delete(op.Operation.Extensions, extension)
		if len(op.Operation.Extensions) == 0 {
			op.Operation.Extensions = nil
		}
		return true
	})
}

// FilterOperationExpression builds an operation filter from a small expression
// language: "field = value", "field == value", or "field != value". Field names
// currently address operation extensions such as x-public or x-product.
func FilterOperationExpression(expression string) (Filter, error) {
	condition, err := parseOperationFilterExpressionGroup(expression)
	if err != nil {
		return nil, err
	}
	return FilterOperations(func(op OperationContext) bool {
		return condition.match(op)
	}), nil
}

// PruneUnusedComponents removes components that are no longer reachable from
// paths, operations, top-level security requirements, or other reachable
// components.
func PruneUnusedComponents() Filter {
	return func(spec *openapi3.T) error {
		if spec == nil || spec.Components == nil {
			return nil
		}
		reachable, err := reachableComponentRefs(spec)
		if err != nil {
			return err
		}
		pruneComponentMaps(spec.Components, reachable)
		return nil
	}
}

// ApplyFilters applies filters in order.
func ApplyFilters(spec *openapi3.T, filters ...Filter) error {
	for _, filter := range filters {
		if filter == nil {
			continue
		}
		if err := filter(spec); err != nil {
			return err
		}
	}
	return nil
}

type operationFilterCondition struct {
	field string
	op    string
	value any
}

type operationFilterConditionGroup []operationFilterCondition

func parseOperationFilterExpressionGroup(expression string) (operationFilterConditionGroup, error) {
	parts := strings.Split(expression, "||")
	conditions := make(operationFilterConditionGroup, 0, len(parts))
	for _, part := range parts {
		condition, err := parseOperationFilterExpression(part)
		if err != nil {
			return nil, err
		}
		conditions = append(conditions, condition)
	}
	return conditions, nil
}

func parseOperationFilterExpression(expression string) (operationFilterCondition, error) {
	expression = strings.TrimSpace(expression)
	for _, op := range []string{"!=", "==", "="} {
		left, right, ok := strings.Cut(expression, op)
		if !ok {
			continue
		}
		field := strings.TrimSpace(left)
		if field == "" {
			return operationFilterCondition{}, fmt.Errorf("filter %q: missing field", expression)
		}
		valueText := strings.TrimSpace(right)
		if valueText == "" {
			return operationFilterCondition{}, fmt.Errorf("filter %q: missing value", expression)
		}
		return operationFilterCondition{
			field: field,
			op:    op,
			value: parseFilterLiteral(valueText),
		}, nil
	}
	return operationFilterCondition{}, fmt.Errorf("filter %q: expected FIELD = VALUE or FIELD != VALUE", expression)
}

func parseFilterLiteral(value string) any {
	value = strings.TrimSpace(value)
	switch strings.ToLower(value) {
	case "true":
		return true
	case "false":
		return false
	}
	if unquoted, ok := stripQuotes(value); ok {
		return unquoted
	}
	return value
}

func stripQuotes(value string) (string, bool) {
	if len(value) < 2 {
		return "", false
	}
	first, last := value[0], value[len(value)-1]
	if first == '"' && last == '"' {
		if unquoted, err := strconv.Unquote(value); err == nil {
			return unquoted, true
		}
	}
	if first == '\'' && last == '\'' {
		return value[1 : len(value)-1], true
	}
	return "", false
}

func (g operationFilterConditionGroup) match(op OperationContext) bool {
	for _, condition := range g {
		if condition.match(op) {
			return true
		}
	}
	return false
}

func (c operationFilterCondition) match(op OperationContext) bool {
	got, ok := op.Extension(c.field)
	equal := ok && scalarEqual(got, c.value)
	switch c.op {
	case "=", "==":
		return equal
	case "!=":
		return !equal
	default:
		return false
	}
}

func scalarEqual(left, right any) bool {
	if reflect.TypeOf(left) == reflect.TypeOf(right) {
		return reflect.DeepEqual(left, right)
	}
	return fmt.Sprint(left) == fmt.Sprint(right)
}

func reachableComponentRefs(spec *openapi3.T) (map[string]struct{}, error) {
	reachable := map[string]struct{}{}
	queue := []string{}
	addRef := func(ref string) {
		if _, ok := reachable[ref]; ok {
			return
		}
		reachable[ref] = struct{}{}
		queue = append(queue, ref)
	}

	root, err := jsonObject(spec)
	if err != nil {
		return nil, fmt.Errorf("inspect OpenAPI document refs: %w", err)
	}
	collectRefsOutsideComponents(root, addRef)
	collectSecurityRequirementRefs(spec.Security, addRef)
	if spec.Paths != nil {
		for _, item := range spec.Paths.Map() {
			if item == nil {
				continue
			}
			for _, operation := range item.Operations() {
				if operation != nil && operation.Security != nil {
					collectSecurityRequirementRefs(*operation.Security, addRef)
				}
			}
		}
	}

	for len(queue) > 0 {
		ref := queue[0]
		queue = queue[1:]
		component, ok := componentValue(spec.Components, ref)
		if !ok {
			continue
		}
		value, err := jsonObject(component)
		if err != nil {
			return nil, fmt.Errorf("inspect component %s refs: %w", ref, err)
		}
		collectRefs(value, addRef)
	}
	return reachable, nil
}

func jsonObject(value any) (any, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var out any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func collectRefsOutsideComponents(value any, addRef func(string)) {
	object, ok := value.(map[string]any)
	if !ok {
		collectRefs(value, addRef)
		return
	}
	for key, child := range object {
		if key == "components" {
			continue
		}
		collectRefs(child, addRef)
	}
}

func collectRefs(value any, addRef func(string)) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if key == "$ref" {
				if ref, ok := child.(string); ok && strings.HasPrefix(ref, "#/components/") {
					addRef(ref)
				}
			}
			collectRefs(child, addRef)
		}
	case []any:
		for _, child := range typed {
			collectRefs(child, addRef)
		}
	}
}

func collectSecurityRequirementRefs(requirements openapi3.SecurityRequirements, addRef func(string)) {
	for _, requirement := range requirements {
		for name := range requirement {
			addRef("#/components/securitySchemes/" + escapeJSONPointer(name))
		}
	}
}

func componentValue(components *openapi3.Components, ref string) (any, bool) {
	group, name, ok := splitComponentRef(ref)
	if !ok {
		return nil, false
	}
	switch group {
	case "schemas":
		value, ok := components.Schemas[name]
		return value, ok
	case "responses":
		value, ok := components.Responses[name]
		return value, ok
	case "parameters":
		value, ok := components.Parameters[name]
		return value, ok
	case "requestBodies":
		value, ok := components.RequestBodies[name]
		return value, ok
	case "headers":
		value, ok := components.Headers[name]
		return value, ok
	case "securitySchemes":
		value, ok := components.SecuritySchemes[name]
		return value, ok
	case "examples":
		value, ok := components.Examples[name]
		return value, ok
	case "links":
		value, ok := components.Links[name]
		return value, ok
	case "callbacks":
		value, ok := components.Callbacks[name]
		return value, ok
	default:
		return nil, false
	}
}

func pruneComponentMaps(components *openapi3.Components, reachable map[string]struct{}) {
	pruneByGroup(components.Schemas, "schemas", reachable)
	pruneByGroup(components.Responses, "responses", reachable)
	pruneByGroup(components.Parameters, "parameters", reachable)
	pruneByGroup(components.RequestBodies, "requestBodies", reachable)
	pruneByGroup(components.Headers, "headers", reachable)
	pruneByGroup(components.SecuritySchemes, "securitySchemes", reachable)
	pruneByGroup(components.Examples, "examples", reachable)
	pruneByGroup(components.Links, "links", reachable)
	pruneByGroup(components.Callbacks, "callbacks", reachable)
}

func pruneByGroup[V any](m map[string]V, group string, reachable map[string]struct{}) {
	prefix := "#/components/" + group + "/"
	for name := range m {
		if _, ok := reachable[prefix+escapeJSONPointer(name)]; !ok {
			delete(m, name)
		}
	}
}

func splitComponentRef(ref string) (string, string, bool) {
	parts := strings.Split(ref, "/")
	if len(parts) != 4 || parts[0] != "#" || parts[1] != "components" {
		return "", "", false
	}
	return unescapeJSONPointer(parts[2]), unescapeJSONPointer(parts[3]), true
}

func escapeJSONPointer(value string) string {
	value = strings.ReplaceAll(value, "~", "~0")
	return strings.ReplaceAll(value, "/", "~1")
}

func unescapeJSONPointer(value string) string {
	value = strings.ReplaceAll(value, "~1", "/")
	return strings.ReplaceAll(value, "~0", "~")
}
