package resolve

import (
	"strings"
	"unicode"

	"github.com/bee/java-process-mapper/internal/model"
)

type indexes struct {
	typesByID         map[string]*model.Type
	typeByFQN         map[string]string
	typesBySimple     map[string][]string
	sourceByPath      map[string]model.SourceFile
	implementations   map[string][]string
	beanNameToTypeIDs map[string][]string
}

type typeRef struct {
	TypeID       string
	ExternalType string
	Confidence   model.Confidence
}

func (ref typeRef) empty() bool {
	return ref.TypeID == "" && ref.ExternalType == ""
}

func Calls(project *model.Project) {
	idx := buildIndexes(project)
	for ti := range project.Types {
		typ := &project.Types[ti]
		fields := fieldTypesByName(typ, idx)
		for mi := range typ.Methods {
			method := &typ.Methods[mi]
			locals := localTypesByName(method, typ, idx)
			params := parameterTypesByName(method, typ, idx)
			for ci := range method.Calls {
				call := &method.Calls[ci]
				resolvedType, resolution, confidence := resolveReceiver(call.Receiver, call.Method, typ, fields, params, locals, idx)
				if resolvedType.empty() {
					call.Resolution = "unresolved"
					call.Confidence = model.ConfidenceLow
					continue
				}
				call.ResolvedTypeID = resolvedType.TypeID
				call.ResolvedExternalType = resolvedType.ExternalType
				call.Resolution = resolution
				call.Confidence = confidence
				if resolvedType.TypeID == "" {
					continue
				}
				if methodID := resolveMethod(resolvedType.TypeID, call.Method, idx); methodID != "" {
					call.ResolvedMethodID = methodID
				}
			}
		}
	}
}

func buildIndexes(project *model.Project) indexes {
	idx := indexes{
		typesByID:         map[string]*model.Type{},
		typeByFQN:         map[string]string{},
		typesBySimple:     map[string][]string{},
		sourceByPath:      map[string]model.SourceFile{},
		implementations:   map[string][]string{},
		beanNameToTypeIDs: map[string][]string{},
	}
	for _, source := range project.SourceFiles {
		idx.sourceByPath[source.Path] = source
	}
	for i := range project.Types {
		typ := &project.Types[i]
		idx.typesByID[typ.ID] = typ
		idx.typeByFQN[typ.FQN] = typ.ID
		idx.typesBySimple[typ.Name] = append(idx.typesBySimple[typ.Name], typ.ID)
		idx.beanNameToTypeIDs[lowerFirst(typ.Name)] = append(idx.beanNameToTypeIDs[lowerFirst(typ.Name)], typ.ID)
		for _, iface := range typ.Implements {
			idx.implementations[iface] = append(idx.implementations[iface], typ.ID)
			idx.implementations[shortName(iface)] = append(idx.implementations[shortName(iface)], typ.ID)
		}
	}
	return idx
}

func fieldTypesByName(typ *model.Type, idx indexes) map[string]typeRef {
	result := map[string]typeRef{}
	source := idx.sourceByPath[typ.FilePath]
	for _, field := range typ.Fields {
		if ref := resolveTypeName(field.FieldType, typ, source.Imports, idx); !ref.empty() {
			result[field.Name] = ref
		}
	}
	return result
}

func localTypesByName(method *model.Method, typ *model.Type, idx indexes) map[string]typeRef {
	result := map[string]typeRef{}
	source := idx.sourceByPath[typ.FilePath]
	for _, local := range method.LocalVariables {
		if local.Name == "" || local.VariableType == "" {
			continue
		}
		if ref := resolveTypeName(local.VariableType, typ, source.Imports, idx); !ref.empty() {
			result[local.Name] = ref
		}
	}
	return result
}

func parameterTypesByName(method *model.Method, typ *model.Type, idx indexes) map[string]typeRef {
	result := map[string]typeRef{}
	source := idx.sourceByPath[typ.FilePath]
	for _, param := range method.Parameters {
		if param.Name == "" || param.Type == "" {
			continue
		}
		if ref := resolveTypeName(param.Type, typ, source.Imports, idx); !ref.empty() {
			result[param.Name] = ref
		}
	}
	return result
}

func resolveReceiver(receiver string, methodName string, typ *model.Type, fields map[string]typeRef, params map[string]typeRef, locals map[string]typeRef, idx indexes) (typeRef, string, model.Confidence) {
	if receiver == "" {
		if typeHasMethod(typ, methodName) {
			return typeRef{TypeID: typ.ID, Confidence: model.ConfidenceHigh}, "same_type", model.ConfidenceHigh
		}
		return typeRef{}, "", model.ConfidenceLow
	}
	if receiver == "this" {
		return typeRef{TypeID: typ.ID, Confidence: model.ConfidenceHigh}, "same_type", model.ConfidenceHigh
	}
	if ref := locals[receiver]; !ref.empty() {
		return implementationRef(ref, methodName, idx), resolutionKind(ref, "local_variable"), ref.Confidence
	}
	if ref := params[receiver]; !ref.empty() {
		return implementationRef(ref, methodName, idx), resolutionKind(ref, "parameter"), maxConfidence(ref.Confidence, model.ConfidenceMedium)
	}
	if ref := fields[receiver]; !ref.empty() {
		return implementationRef(ref, methodName, idx), resolutionKind(ref, "field"), ref.Confidence
	}
	if typeID := unique(idx.beanNameToTypeIDs[receiver]); typeID != "" {
		return typeRef{TypeID: implementationType(typeID, methodName, idx), Confidence: model.ConfidenceMedium}, "bean_name", model.ConfidenceMedium
	}
	if isUpper(receiver) {
		if ref := resolveTypeName(receiver, typ, idx.sourceByPath[typ.FilePath].Imports, idx); !ref.empty() {
			if ref.ExternalType != "" {
				return ref, "external_import", ref.Confidence
			}
			return implementationRef(ref, methodName, idx), "type_name", maxConfidence(ref.Confidence, model.ConfidenceMedium)
		}
	}
	return typeRef{}, "", model.ConfidenceLow
}

func implementationRef(ref typeRef, methodName string, idx indexes) typeRef {
	if ref.TypeID == "" {
		return ref
	}
	ref.TypeID = implementationType(ref.TypeID, methodName, idx)
	return ref
}

func resolutionKind(ref typeRef, localKind string) string {
	if ref.ExternalType != "" {
		return "external_import"
	}
	return localKind
}

func typeHasMethod(typ *model.Type, methodName string) bool {
	for _, method := range typ.Methods {
		if method.Name == methodName {
			return true
		}
	}
	return false
}

func resolveMethod(typeID string, methodName string, idx indexes) string {
	typ := idx.typesByID[typeID]
	if typ == nil {
		return ""
	}
	for _, method := range typ.Methods {
		if method.Name == methodName {
			return method.ID
		}
	}
	if implTypeID := implementationType(typeID, methodName, idx); implTypeID != typeID {
		if implMethodID := resolveMethod(implTypeID, methodName, idx); implMethodID != "" {
			return implMethodID
		}
	}
	return ""
}

func implementationType(typeID string, methodName string, idx indexes) string {
	typ := idx.typesByID[typeID]
	if typ == nil {
		return typeID
	}
	candidates := append([]string{}, idx.implementations[typ.FQN]...)
	candidates = append(candidates, idx.implementations[typ.Name]...)
	for _, candidateID := range candidates {
		candidate := idx.typesByID[candidateID]
		if candidate == nil {
			continue
		}
		for _, method := range candidate.Methods {
			if method.Name == methodName {
				return candidateID
			}
		}
	}
	return typeID
}

func resolveTypeName(name string, current *model.Type, imports []string, idx indexes) typeRef {
	name = cleanTypeName(name)
	if name == "" {
		return typeRef{}
	}
	if typeID := idx.typeByFQN[name]; typeID != "" {
		return typeRef{TypeID: typeID, Confidence: model.ConfidenceHigh}
	}
	if strings.Contains(name, ".") {
		return typeRef{ExternalType: name, Confidence: model.ConfidenceHigh}
	}
	for _, imp := range imports {
		if isWildcardImport(imp) || strings.HasPrefix(imp, "static") {
			continue
		}
		if shortName(imp) == name {
			if typeID := idx.typeByFQN[imp]; typeID != "" {
				return typeRef{TypeID: typeID, Confidence: model.ConfidenceHigh}
			}
			return typeRef{ExternalType: imp, Confidence: model.ConfidenceHigh}
		}
	}
	for _, imp := range imports {
		if isWildcardImport(imp) {
			candidate := strings.TrimSuffix(imp, ".*") + "." + name
			if typeID := idx.typeByFQN[candidate]; typeID != "" {
				return typeRef{TypeID: typeID, Confidence: model.ConfidenceHigh}
			}
		}
	}
	if current.Package != "" {
		if typeID := idx.typeByFQN[current.Package+"."+name]; typeID != "" {
			return typeRef{TypeID: typeID, Confidence: model.ConfidenceHigh}
		}
	}
	if typeID := unique(idx.typesBySimple[name]); typeID != "" {
		return typeRef{TypeID: typeID, Confidence: model.ConfidenceMedium}
	}
	var externalWildcardCandidates []string
	for _, imp := range imports {
		if isWildcardImport(imp) {
			externalWildcardCandidates = append(externalWildcardCandidates, strings.TrimSuffix(imp, ".*")+"."+name)
		}
	}
	if len(externalWildcardCandidates) == 1 {
		return typeRef{ExternalType: externalWildcardCandidates[0], Confidence: model.ConfidenceMedium}
	}
	return typeRef{}
}

func isWildcardImport(imp string) bool {
	return strings.HasSuffix(imp, ".*")
}

func maxConfidence(primary, ceiling model.Confidence) model.Confidence {
	if confidenceRank(primary) <= confidenceRank(ceiling) {
		return primary
	}
	return ceiling
}

func confidenceRank(confidence model.Confidence) int {
	switch confidence {
	case model.ConfidenceHigh:
		return 3
	case model.ConfidenceMedium:
		return 2
	default:
		return 1
	}
}

func cleanTypeName(name string) string {
	name = strings.TrimSpace(name)
	if idx := strings.Index(name, "<"); idx >= 0 {
		name = name[:idx]
	}
	name = strings.TrimSuffix(name, "[]")
	parts := strings.Fields(name)
	if len(parts) > 0 {
		name = parts[len(parts)-1]
	}
	return strings.TrimSpace(name)
}

func unique(values []string) string {
	if len(values) == 1 {
		return values[0]
	}
	return ""
}

func shortName(value string) string {
	if idx := strings.LastIndex(value, "."); idx >= 0 {
		return value[idx+1:]
	}
	return value
}

func lowerFirst(value string) string {
	if value == "" {
		return ""
	}
	runes := []rune(value)
	runes[0] = unicode.ToLower(runes[0])
	return string(runes)
}

func isUpper(value string) bool {
	if value == "" {
		return false
	}
	r, _ := utf8DecodeRuneInString(value)
	return unicode.IsUpper(r)
}

func utf8DecodeRuneInString(value string) (rune, int) {
	for _, r := range value {
		return r, len(string(r))
	}
	return 0, 0
}
