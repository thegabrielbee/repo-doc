package java

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/antlr4-go/antlr/v4"
	"github.com/bee/java-process-mapper/internal/model"
	"github.com/bee/java-process-mapper/internal/parser/java/antlrparser"
)

type token struct {
	typ  int
	text string
	line int
}

func ParseFile(path, moduleID string) (model.SourceFile, []model.Type, []model.Gap, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return model.SourceFile{}, nil, nil, err
	}
	content := string(data)

	parseInput := antlr.NewInputStream(content)
	parseLexer := antlrparser.NewJavaStructureLexer(parseInput)
	parseStream := antlr.NewCommonTokenStream(parseLexer, antlr.TokenDefaultChannel)
	parseParser := antlrparser.NewJavaStructureParser(parseStream)
	parseParser.RemoveErrorListeners()
	parseParser.BuildParseTrees = false
	parseParser.CompilationUnit()

	tokens := lex(content)
	source := model.SourceFile{
		Path:     filepath.Clean(path),
		ModuleID: moduleID,
		Package:  parsePackage(tokens),
		Imports:  parseImports(tokens),
	}
	types := parseTypes(tokens, source)
	for _, typ := range types {
		source.TypeIDs = append(source.TypeIDs, typ.ID)
	}
	return source, types, nil, nil
}

func lex(content string) []token {
	input := antlr.NewInputStream(content)
	lexer := antlrparser.NewJavaStructureLexer(input)
	lexer.RemoveErrorListeners()
	var tokens []token
	for {
		tok := lexer.NextToken()
		if tok.GetTokenType() == antlr.TokenEOF {
			break
		}
		if tok.GetChannel() != antlr.TokenDefaultChannel {
			continue
		}
		tokens = append(tokens, token{typ: tok.GetTokenType(), text: tok.GetText(), line: tok.GetLine()})
	}
	return tokens
}

func parsePackage(tokens []token) string {
	for i := 0; i < len(tokens); i++ {
		if tokens[i].typ != antlrparser.JavaStructureLexerPACKAGE {
			continue
		}
		var parts []string
		for j := i + 1; j < len(tokens) && tokens[j].typ != antlrparser.JavaStructureLexerSEMI; j++ {
			if tokens[j].typ == antlrparser.JavaStructureLexerIDENTIFIER || tokens[j].typ == antlrparser.JavaStructureLexerDOT {
				parts = append(parts, tokens[j].text)
			}
		}
		return strings.Join(parts, "")
	}
	return ""
}

func parseImports(tokens []token) []string {
	var imports []string
	for i := 0; i < len(tokens); i++ {
		if tokens[i].typ != antlrparser.JavaStructureLexerIMPORT {
			continue
		}
		var parts []string
		for j := i + 1; j < len(tokens) && tokens[j].typ != antlrparser.JavaStructureLexerSEMI; j++ {
			if tokens[j].typ == antlrparser.JavaStructureLexerIDENTIFIER ||
				tokens[j].typ == antlrparser.JavaStructureLexerDOT ||
				tokens[j].typ == antlrparser.JavaStructureLexerSTATIC ||
				tokens[j].text == "*" {
				parts = append(parts, tokens[j].text)
			}
		}
		imports = append(imports, strings.Join(parts, ""))
	}
	return imports
}

func parseTypes(tokens []token, source model.SourceFile) []model.Type {
	var types []model.Type
	for i := 0; i < len(tokens); i++ {
		if !isTypeKeyword(tokens, i) {
			continue
		}
		nameIdx := nextIdentifier(tokens, i+1)
		if nameIdx < 0 {
			continue
		}
		bodyStart := findNext(tokens, nameIdx+1, antlrparser.JavaStructureLexerLBRACE)
		if bodyStart < 0 {
			continue
		}
		bodyEnd := findMatching(tokens, bodyStart, antlrparser.JavaStructureLexerLBRACE, antlrparser.JavaStructureLexerRBRACE)
		if bodyEnd < 0 {
			continue
		}
		name := tokens[nameIdx].text
		fqn := name
		if source.Package != "" {
			fqn = source.Package + "." + name
		}
		annotations := collectLeadingAnnotations(tokens, i, source.Path)
		typ := model.Type{
			ID:          stableID("type", source.Path, fqn, strconv.Itoa(tokens[i].line)),
			ModuleID:    source.ModuleID,
			FilePath:    source.Path,
			Package:     source.Package,
			Name:        name,
			FQN:         fqn,
			Kind:        typeKind(tokens, i),
			Annotations: annotations,
			Extends:     collectTypeList(tokens, nameIdx, bodyStart, antlrparser.JavaStructureLexerEXTENDS),
			Implements:  collectTypeList(tokens, nameIdx, bodyStart, antlrparser.JavaStructureLexerIMPLEMENTS),
			StartLine:   startLineWithAnnotations(tokens[i].line, annotations),
			BodyLine:    tokens[bodyStart].line,
			EndLine:     tokens[bodyEnd].line,
			Evidence:    evidence(source.Path, tokens[i].line, "type", name),
			Source:      model.SourceFound,
			Confidence:  model.ConfidenceHigh,
		}
		typ.Fields = parseFields(tokens, bodyStart, bodyEnd, typ)
		typ.Methods = parseMethods(tokens, bodyStart, bodyEnd, typ)
		types = append(types, typ)
		i = bodyStart
	}
	return types
}

func parseFields(tokens []token, bodyStart, bodyEnd int, typ model.Type) []model.Field {
	var fields []model.Field
	depth := 0
	segmentStart := bodyStart + 1
	for i := bodyStart + 1; i < bodyEnd; i++ {
		switch tokens[i].typ {
		case antlrparser.JavaStructureLexerLBRACE:
			depth++
		case antlrparser.JavaStructureLexerRBRACE:
			if depth > 0 {
				depth--
			}
		case antlrparser.JavaStructureLexerSEMI:
			if depth == 0 {
				if field := parseField(tokens, segmentStart, i, typ); field.Name != "" {
					fields = append(fields, field)
				}
				segmentStart = i + 1
			}
		}
	}
	return fields
}

func parseField(tokens []token, start, end int, typ model.Type) model.Field {
	if start >= end || hasTokenBetween(tokens, start, end, antlrparser.JavaStructureLexerLPAREN) {
		return model.Field{}
	}
	assign := findNextBetween(tokens, start, end, antlrparser.JavaStructureLexerASSIGN)
	if assign > start {
		end = assign
	}
	clean := stripAnnotationsAndModifiers(tokens[start:end])
	if len(clean) < 2 {
		return model.Field{}
	}
	nameIdx := -1
	for i := len(clean) - 1; i >= 0; i-- {
		if clean[i].typ == antlrparser.JavaStructureLexerIDENTIFIER {
			nameIdx = i
			break
		}
	}
	if nameIdx <= 0 {
		return model.Field{}
	}
	name := clean[nameIdx].text
	fieldType := strings.TrimSpace(joinTokens(clean[:nameIdx]))
	if fieldType == "" || isControlName(name) {
		return model.Field{}
	}
	line := clean[nameIdx].line
	return model.Field{
		ID:          stableID("field", typ.ID, name, strconv.Itoa(line)),
		TypeID:      typ.ID,
		Name:        name,
		FieldType:   fieldType,
		Annotations: collectLeadingAnnotations(tokens, start, typ.FilePath),
		Evidence:    evidence(typ.FilePath, line, "field", name),
		Source:      model.SourceFound,
		Confidence:  model.ConfidenceHigh,
	}
}

func parseMethods(tokens []token, bodyStart, bodyEnd int, typ model.Type) []model.Method {
	var methods []model.Method
	depth := 0
	for i := bodyStart + 1; i < bodyEnd; i++ {
		switch tokens[i].typ {
		case antlrparser.JavaStructureLexerLBRACE:
			depth++
		case antlrparser.JavaStructureLexerRBRACE:
			if depth > 0 {
				depth--
			}
		case antlrparser.JavaStructureLexerLPAREN:
			if depth != 0 {
				continue
			}
			nameIdx := previousDefault(tokens, i-1)
			if nameIdx < 0 || tokens[nameIdx].typ != antlrparser.JavaStructureLexerIDENTIFIER {
				continue
			}
			if nameIdx > 0 && tokens[nameIdx-1].typ == antlrparser.JavaStructureLexerAT {
				continue
			}
			name := tokens[nameIdx].text
			if isControlName(name) {
				continue
			}
			declStart := declarationStart(tokens, nameIdx)
			if hasTopLevelAssignment(tokens, declStart, nameIdx) {
				continue
			}
			paramsEnd := findMatching(tokens, i, antlrparser.JavaStructureLexerLPAREN, antlrparser.JavaStructureLexerRPAREN)
			if paramsEnd < 0 || paramsEnd >= bodyEnd {
				continue
			}
			after := skipAfterParams(tokens, paramsEnd+1, bodyEnd)
			if after < 0 || (tokens[after].typ != antlrparser.JavaStructureLexerLBRACE && tokens[after].typ != antlrparser.JavaStructureLexerSEMI) {
				continue
			}
			annotations := collectLeadingAnnotations(tokens, nameIdx, typ.FilePath)
			method := model.Method{
				ID:          stableID("method", typ.ID, name, strconv.Itoa(tokens[nameIdx].line)),
				TypeID:      typ.ID,
				Name:        name,
				ReturnType:  returnType(tokens, declStart, nameIdx, typ.Name),
				Parameters:  parseParameters(tokens, i+1, paramsEnd),
				Annotations: annotations,
				StartLine:   startLineWithAnnotations(tokens[declStart].line, annotations),
				Evidence:    evidence(typ.FilePath, tokens[nameIdx].line, "method", name),
				Source:      model.SourceFound,
				Confidence:  model.ConfidenceHigh,
			}
			if tokens[after].typ == antlrparser.JavaStructureLexerLBRACE {
				methodEnd := findMatching(tokens, after, antlrparser.JavaStructureLexerLBRACE, antlrparser.JavaStructureLexerRBRACE)
				if methodEnd > after {
					method.BodyLine = tokens[after].line
					method.EndLine = tokens[methodEnd].line
					method.Calls = parseCalls(tokens, after+1, methodEnd, typ.FilePath)
					method.LocalVariables = parseLocalVariables(tokens, after+1, methodEnd, typ.FilePath, method.ID)
					method.Conditions = parseConditions(tokens, after+1, methodEnd, typ.FilePath, method.ID)
					i = methodEnd
				}
			} else {
				method.EndLine = tokens[after].line
			}
			methods = append(methods, method)
		}
	}
	return methods
}

func parseLocalVariables(tokens []token, start, end int, path, methodID string) []model.LocalVariable {
	seen := map[string]bool{}
	var locals []model.LocalVariable
	segmentStart := start
	for i := start; i < end; i++ {
		if tokens[i].typ != antlrparser.JavaStructureLexerSEMI {
			continue
		}
		if local := parseLocalVariable(tokens, segmentStart, i, path, methodID); local.Name != "" {
			key := fmt.Sprintf("%s:%d", local.Name, local.Evidence.Line)
			if !seen[key] {
				seen[key] = true
				locals = append(locals, local)
			}
		}
		segmentStart = i + 1
	}
	return locals
}

func parseLocalVariable(tokens []token, start, end int, path, methodID string) model.LocalVariable {
	start = localDeclarationStart(tokens, start, end)
	if start >= end {
		return model.LocalVariable{}
	}
	declEnd := end
	if assign := findStandaloneAssignBetween(tokens, start, end); assign >= 0 {
		declEnd = assign
	} else if hasTokenBetween(tokens, start, end, antlrparser.JavaStructureLexerLPAREN) {
		return model.LocalVariable{}
	}
	clean := stripAnnotationsAndModifiers(tokens[start:declEnd])
	if len(clean) < 2 || hasNonDeclarationPrefix(clean) {
		return model.LocalVariable{}
	}
	nameIdx := -1
	for i := len(clean) - 1; i >= 0; i-- {
		if clean[i].typ == antlrparser.JavaStructureLexerIDENTIFIER {
			nameIdx = i
			break
		}
	}
	if nameIdx <= 0 || isControlName(clean[nameIdx].text) {
		return model.LocalVariable{}
	}
	if nameIdx > 0 && clean[nameIdx-1].typ == antlrparser.JavaStructureLexerDOT {
		return model.LocalVariable{}
	}
	typeTokens := clean[:nameIdx]
	if !looksLikeLocalType(typeTokens) {
		return model.LocalVariable{}
	}
	name := clean[nameIdx].text
	variableType := strings.TrimSpace(joinTokens(typeTokens))
	if variableType == "" {
		return model.LocalVariable{}
	}
	line := clean[nameIdx].line
	return model.LocalVariable{
		ID:           stableID("local", methodID, name, strconv.Itoa(line)),
		MethodID:     methodID,
		Name:         name,
		VariableType: variableType,
		Evidence:     evidence(path, line, "local_variable", name),
		Source:       model.SourceFound,
		Confidence:   model.ConfidenceHigh,
	}
}

func localDeclarationStart(tokens []token, start, end int) int {
	for i := end - 1; i >= start; i-- {
		switch tokens[i].typ {
		case antlrparser.JavaStructureLexerLBRACE, antlrparser.JavaStructureLexerRBRACE, antlrparser.JavaStructureLexerSEMI:
			return i + 1
		}
	}
	if assign := findStandaloneAssignBetween(tokens, start, end); assign >= 0 && start < end {
		switch tokens[start].typ {
		case antlrparser.JavaStructureLexerFOR, antlrparser.JavaStructureLexerTRY:
			if paren := findLastBetween(tokens, start, assign, antlrparser.JavaStructureLexerLPAREN); paren >= 0 {
				return paren + 1
			}
		}
	}
	return start
}

func hasNonDeclarationPrefix(tokens []token) bool {
	for _, tok := range tokens {
		switch tok.typ {
		case antlrparser.JavaStructureLexerRETURN,
			antlrparser.JavaStructureLexerIF,
			antlrparser.JavaStructureLexerFOR,
			antlrparser.JavaStructureLexerWHILE,
			antlrparser.JavaStructureLexerSWITCH,
			antlrparser.JavaStructureLexerTRY,
			antlrparser.JavaStructureLexerCATCH,
			antlrparser.JavaStructureLexerNEW:
			return true
		}
	}
	return false
}

func looksLikeLocalType(tokens []token) bool {
	if len(tokens) == 0 {
		return false
	}
	switch tokens[0].typ {
	case antlrparser.JavaStructureLexerIDENTIFIER, antlrparser.JavaStructureLexerVOID:
		return true
	default:
		return false
	}
}

func parseConditions(tokens []token, start, end int, path, methodID string) []model.Condition {
	seen := map[string]bool{}
	var conditions []model.Condition
	for i := start; i < end; i++ {
		if isElseToken(tokens[i]) {
			if i+1 < end && tokens[i+1].typ == antlrparser.JavaStructureLexerIF {
				continue
			}
			bodyStart, bodyEnd := conditionBodyRange(tokens, i, end)
			if bodyEnd < bodyStart {
				continue
			}
			key := fmt.Sprintf("else:%d", tokens[i].line)
			if seen[key] {
				continue
			}
			seen[key] = true
			conditions = append(conditions, model.Condition{
				ID:         stableID("condition", methodID, "else", strconv.Itoa(tokens[i].line)),
				MethodID:   methodID,
				Kind:       "else",
				StartLine:  tokens[i].line,
				BodyLine:   tokens[bodyStart].line,
				EndLine:    tokens[bodyEnd].line,
				Evidence:   evidence(path, tokens[i].line, "condition", "else"),
				Source:     model.SourceFound,
				Confidence: model.ConfidenceHigh,
			})
			continue
		}
		kind := conditionKind(tokens[i].typ)
		if kind == "" {
			continue
		}
		open := findNextBetween(tokens, i+1, end, antlrparser.JavaStructureLexerLPAREN)
		if open < 0 {
			continue
		}
		close := findMatching(tokens, open, antlrparser.JavaStructureLexerLPAREN, antlrparser.JavaStructureLexerRPAREN)
		if close <= open || close >= end {
			continue
		}
		bodyStart, bodyEnd := conditionBodyRange(tokens, close, end)
		if bodyEnd < bodyStart {
			continue
		}
		expression := joinExpressionTokens(tokens[open+1 : close])
		key := fmt.Sprintf("%s:%d:%s", kind, tokens[i].line, expression)
		if seen[key] {
			continue
		}
		seen[key] = true
		symbol := kind
		if expression != "" {
			symbol = kind + " " + expression
		}
		conditions = append(conditions, model.Condition{
			ID:         stableID("condition", methodID, kind, strconv.Itoa(tokens[i].line), expression),
			MethodID:   methodID,
			Kind:       kind,
			Expression: expression,
			StartLine:  tokens[i].line,
			BodyLine:   tokens[bodyStart].line,
			EndLine:    tokens[bodyEnd].line,
			Evidence:   evidence(path, tokens[i].line, "condition", symbol),
			Source:     model.SourceFound,
			Confidence: model.ConfidenceHigh,
		})
	}
	return conditions
}

func conditionKind(typ int) string {
	switch typ {
	case antlrparser.JavaStructureLexerIF:
		return "if"
	case antlrparser.JavaStructureLexerSWITCH:
		return "switch"
	default:
		return ""
	}
}

func isElseToken(tok token) bool {
	return tok.typ == antlrparser.JavaStructureLexerIDENTIFIER && tok.text == "else"
}

func conditionBodyRange(tokens []token, after, end int) (int, int) {
	bodyStart := after + 1
	for bodyStart < end {
		if tokens[bodyStart].typ == antlrparser.JavaStructureLexerSEMI {
			bodyStart++
			continue
		}
		break
	}
	if bodyStart >= end {
		return -1, -1
	}
	if tokens[bodyStart].typ == antlrparser.JavaStructureLexerLBRACE {
		bodyEnd := findMatching(tokens, bodyStart, antlrparser.JavaStructureLexerLBRACE, antlrparser.JavaStructureLexerRBRACE)
		if bodyEnd < 0 || bodyEnd >= end {
			return -1, -1
		}
		return bodyStart, bodyEnd
	}
	for i := bodyStart; i < end; i++ {
		if tokens[i].typ == antlrparser.JavaStructureLexerSEMI {
			return bodyStart, i
		}
		if tokens[i].typ == antlrparser.JavaStructureLexerRBRACE {
			return bodyStart, i - 1
		}
	}
	return bodyStart, end - 1
}

func parseCalls(tokens []token, start, end int, path string) []model.Call {
	seen := map[string]bool{}
	var calls []model.Call
	for i := start; i+1 < end; i++ {
		if tokens[i].typ != antlrparser.JavaStructureLexerIDENTIFIER || tokens[i+1].typ != antlrparser.JavaStructureLexerLPAREN {
			continue
		}
		method := tokens[i].text
		receiver := ""
		target := method
		if i >= 2 && tokens[i-1].typ == antlrparser.JavaStructureLexerDOT && tokens[i-2].typ == antlrparser.JavaStructureLexerIDENTIFIER {
			receiver = tokens[i-2].text
			target = receiver + "." + method
		}
		key := fmt.Sprintf("%s:%d", target, tokens[i].line)
		if seen[key] {
			continue
		}
		seen[key] = true
		calls = append(calls, model.Call{
			Target:     target,
			Receiver:   receiver,
			Method:     method,
			Evidence:   evidence(path, tokens[i].line, "method_call", target),
			Source:     model.SourceFound,
			Confidence: model.ConfidenceMedium,
		})
	}
	return calls
}

func parseParameters(tokens []token, start, end int) []model.Parameter {
	var params []model.Parameter
	segmentStart := start
	depth := 0
	for i := start; i <= end; i++ {
		if i == end || (tokens[i].typ == antlrparser.JavaStructureLexerCOMMA && depth == 0) {
			if param := parseParameter(tokens[segmentStart:i]); param.Name != "" || param.Type != "" {
				params = append(params, param)
			}
			segmentStart = i + 1
			continue
		}
		switch tokens[i].typ {
		case antlrparser.JavaStructureLexerLT, antlrparser.JavaStructureLexerLPAREN, antlrparser.JavaStructureLexerLBRACK:
			depth++
		case antlrparser.JavaStructureLexerGT, antlrparser.JavaStructureLexerRPAREN, antlrparser.JavaStructureLexerRBRACK:
			if depth > 0 {
				depth--
			}
		}
	}
	return params
}

func parseParameter(tokens []token) model.Parameter {
	clean := stripAnnotationsAndModifiers(tokens)
	if len(clean) == 0 {
		return model.Parameter{}
	}
	nameIdx := -1
	for i := len(clean) - 1; i >= 0; i-- {
		if clean[i].typ == antlrparser.JavaStructureLexerIDENTIFIER {
			nameIdx = i
			break
		}
	}
	if nameIdx < 0 {
		return model.Parameter{Type: joinTokens(clean)}
	}
	return model.Parameter{
		Name: clean[nameIdx].text,
		Type: strings.TrimSpace(joinTokens(clean[:nameIdx])),
	}
}

func startLineWithAnnotations(defaultLine int, annotations []model.Annotation) int {
	start := defaultLine
	for _, annotation := range annotations {
		if annotation.Evidence.Line > 0 && (start == 0 || annotation.Evidence.Line < start) {
			start = annotation.Evidence.Line
		}
	}
	return start
}

func collectLeadingAnnotations(tokens []token, idx int, path string) []model.Annotation {
	start := idx - 1
	for start >= 0 {
		switch tokens[start].typ {
		case antlrparser.JavaStructureLexerSEMI, antlrparser.JavaStructureLexerLBRACE, antlrparser.JavaStructureLexerRBRACE:
			start++
			goto collect
		}
		start--
	}
	start = 0
collect:
	var annotations []model.Annotation
	for i := start; i < idx; i++ {
		if tokens[i].typ != antlrparser.JavaStructureLexerAT {
			continue
		}
		annotation, next := readAnnotation(tokens, i, path)
		if annotation.Name != "" {
			annotations = append(annotations, annotation)
		}
		i = next - 1
	}
	return annotations
}

func readAnnotation(tokens []token, start int, path string) (model.Annotation, int) {
	i := start + 1
	var nameParts []string
	if i < len(tokens) && (tokens[i].typ == antlrparser.JavaStructureLexerIDENTIFIER || tokens[i].typ == antlrparser.JavaStructureLexerINTERFACE) {
		nameParts = append(nameParts, tokens[i].text)
		i++
		for i+1 < len(tokens) && tokens[i].typ == antlrparser.JavaStructureLexerDOT &&
			(tokens[i+1].typ == antlrparser.JavaStructureLexerIDENTIFIER || tokens[i+1].typ == antlrparser.JavaStructureLexerINTERFACE) {
			nameParts = append(nameParts, tokens[i].text, tokens[i+1].text)
			i += 2
		}
	}
	name := strings.Join(nameParts, "")
	values := map[string]string{}
	rawParts := []string{"@" + name}
	if i < len(tokens) && tokens[i].typ == antlrparser.JavaStructureLexerLPAREN {
		end := findMatching(tokens, i, antlrparser.JavaStructureLexerLPAREN, antlrparser.JavaStructureLexerRPAREN)
		if end > i {
			rawParts = append(rawParts, joinTokens(tokens[i:end+1]))
			values = annotationValues(tokens, i+1, end)
			i = end + 1
		}
	}
	return model.Annotation{
		Name:       shortName(name),
		Values:     values,
		Raw:        strings.Join(rawParts, ""),
		Evidence:   evidence(path, tokens[start].line, "annotation", shortName(name)),
		Source:     model.SourceFound,
		Confidence: model.ConfidenceHigh,
	}, i
}

func annotationValues(tokens []token, start, end int) map[string]string {
	values := map[string]string{}
	for i := start; i < end; i++ {
		if tokens[i].typ == antlrparser.JavaStructureLexerIDENTIFIER && i+2 < end && tokens[i+1].typ == antlrparser.JavaStructureLexerASSIGN {
			valueEnd := annotationValueEnd(tokens, i+2, end)
			values[tokens[i].text] = cleanLiteral(joinTokens(tokens[i+2 : valueEnd]))
			i = valueEnd - 1
			continue
		}
		if _, ok := values["value"]; !ok && (tokens[i].typ == antlrparser.JavaStructureLexerSTRING_LITERAL || tokens[i].typ == antlrparser.JavaStructureLexerIDENTIFIER) {
			values["value"] = cleanLiteral(tokens[i].text)
		}
	}
	return values
}

func annotationValueEnd(tokens []token, start, end int) int {
	depth := 0
	for i := start; i < end; i++ {
		switch tokens[i].typ {
		case antlrparser.JavaStructureLexerLPAREN, antlrparser.JavaStructureLexerLBRACE, antlrparser.JavaStructureLexerLBRACK:
			depth++
		case antlrparser.JavaStructureLexerRPAREN, antlrparser.JavaStructureLexerRBRACE, antlrparser.JavaStructureLexerRBRACK:
			if depth == 0 {
				return i
			}
			depth--
		case antlrparser.JavaStructureLexerCOMMA:
			if depth == 0 {
				return i
			}
		}
	}
	return end
}

func collectTypeList(tokens []token, from, to, keyword int) []string {
	idx := findNextBetween(tokens, from, to, keyword)
	if idx < 0 {
		return nil
	}
	var result []string
	var current []string
	for i := idx + 1; i < to; i++ {
		if tokens[i].typ == antlrparser.JavaStructureLexerIMPLEMENTS || tokens[i].typ == antlrparser.JavaStructureLexerEXTENDS {
			if len(current) > 0 {
				result = append(result, strings.Join(current, ""))
			}
			current = nil
			if tokens[i].typ != keyword {
				break
			}
			continue
		}
		if tokens[i].typ == antlrparser.JavaStructureLexerCOMMA {
			if len(current) > 0 {
				result = append(result, strings.Join(current, ""))
			}
			current = nil
			continue
		}
		if tokens[i].typ == antlrparser.JavaStructureLexerIDENTIFIER || tokens[i].typ == antlrparser.JavaStructureLexerDOT {
			current = append(current, tokens[i].text)
		}
	}
	if len(current) > 0 {
		result = append(result, strings.Join(current, ""))
	}
	return result
}

func returnType(tokens []token, start, nameIdx int, typeName string) string {
	if tokens[nameIdx].text == typeName {
		return "<constructor>"
	}
	clean := stripAnnotationsAndModifiers(tokens[start:nameIdx])
	if len(clean) == 0 {
		return ""
	}
	return joinTokens(clean)
}

func stripAnnotationsAndModifiers(tokens []token) []token {
	var clean []token
	for i := 0; i < len(tokens); i++ {
		if tokens[i].typ == antlrparser.JavaStructureLexerAT {
			if i+1 < len(tokens) {
				i++
			}
			if i+1 < len(tokens) && tokens[i+1].typ == antlrparser.JavaStructureLexerLPAREN {
				end := findMatching(tokens, i+1, antlrparser.JavaStructureLexerLPAREN, antlrparser.JavaStructureLexerRPAREN)
				if end > i {
					i = end
				}
			}
			continue
		}
		if isModifier(tokens[i].typ) {
			continue
		}
		clean = append(clean, tokens[i])
	}
	return clean
}

func declarationStart(tokens []token, nameIdx int) int {
	start := nameIdx - 1
	for start >= 0 {
		switch tokens[start].typ {
		case antlrparser.JavaStructureLexerSEMI, antlrparser.JavaStructureLexerLBRACE, antlrparser.JavaStructureLexerRBRACE:
			return start + 1
		}
		start--
	}
	return 0
}

func skipAfterParams(tokens []token, start, end int) int {
	for i := start; i < end; i++ {
		switch tokens[i].typ {
		case antlrparser.JavaStructureLexerTHROWS:
			for i+1 < end && tokens[i+1].typ != antlrparser.JavaStructureLexerLBRACE && tokens[i+1].typ != antlrparser.JavaStructureLexerSEMI {
				i++
			}
		case antlrparser.JavaStructureLexerDEFAULT:
			for i+1 < end && tokens[i+1].typ != antlrparser.JavaStructureLexerLBRACE && tokens[i+1].typ != antlrparser.JavaStructureLexerSEMI {
				i++
			}
		case antlrparser.JavaStructureLexerLBRACE, antlrparser.JavaStructureLexerSEMI:
			return i
		}
	}
	return -1
}

func isTypeKeyword(tokens []token, i int) bool {
	switch tokens[i].typ {
	case antlrparser.JavaStructureLexerCLASS, antlrparser.JavaStructureLexerENUM, antlrparser.JavaStructureLexerRECORD:
		return true
	case antlrparser.JavaStructureLexerINTERFACE:
		return true
	default:
		return false
	}
}

func typeKind(tokens []token, i int) string {
	switch tokens[i].typ {
	case antlrparser.JavaStructureLexerCLASS:
		return "class"
	case antlrparser.JavaStructureLexerINTERFACE:
		if i > 0 && tokens[i-1].typ == antlrparser.JavaStructureLexerAT {
			return "annotation"
		}
		return "interface"
	case antlrparser.JavaStructureLexerENUM:
		return "enum"
	case antlrparser.JavaStructureLexerRECORD:
		return "record"
	default:
		return "type"
	}
}

func nextIdentifier(tokens []token, start int) int {
	for i := start; i < len(tokens); i++ {
		if tokens[i].typ == antlrparser.JavaStructureLexerIDENTIFIER {
			return i
		}
		if tokens[i].typ == antlrparser.JavaStructureLexerLBRACE || tokens[i].typ == antlrparser.JavaStructureLexerSEMI {
			return -1
		}
	}
	return -1
}

func previousDefault(tokens []token, start int) int {
	for i := start; i >= 0; i-- {
		return i
	}
	return -1
}

func findNext(tokens []token, start, typ int) int {
	for i := start; i < len(tokens); i++ {
		if tokens[i].typ == typ {
			return i
		}
	}
	return -1
}

func findNextBetween(tokens []token, start, end, typ int) int {
	for i := start; i < end; i++ {
		if tokens[i].typ == typ {
			return i
		}
	}
	return -1
}

func findLastBetween(tokens []token, start, end, typ int) int {
	for i := end - 1; i >= start; i-- {
		if tokens[i].typ == typ {
			return i
		}
	}
	return -1
}

func findStandaloneAssignBetween(tokens []token, start, end int) int {
	for i := start; i < end; i++ {
		if tokens[i].typ != antlrparser.JavaStructureLexerASSIGN {
			continue
		}
		if i > start && tokens[i-1].typ == antlrparser.JavaStructureLexerASSIGN {
			continue
		}
		if i+1 < end && tokens[i+1].typ == antlrparser.JavaStructureLexerASSIGN {
			continue
		}
		return i
	}
	return -1
}

func hasTokenBetween(tokens []token, start, end, typ int) bool {
	return findNextBetween(tokens, start, end, typ) >= 0
}

func findMatching(tokens []token, start, open, close int) int {
	depth := 0
	for i := start; i < len(tokens); i++ {
		if tokens[i].typ == open {
			depth++
		}
		if tokens[i].typ == close {
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func hasTopLevelAssignment(tokens []token, start, end int) bool {
	for i := start; i < end; i++ {
		if tokens[i].typ == antlrparser.JavaStructureLexerAT {
			if i+1 < end {
				i++
			}
			if i+1 < end && tokens[i+1].typ == antlrparser.JavaStructureLexerLPAREN {
				if annotationEnd := findMatching(tokens, i+1, antlrparser.JavaStructureLexerLPAREN, antlrparser.JavaStructureLexerRPAREN); annotationEnd > i {
					i = annotationEnd
				}
			}
			continue
		}
		if tokens[i].typ == antlrparser.JavaStructureLexerASSIGN {
			return true
		}
	}
	return false
}

func joinTokens(tokens []token) string {
	var b strings.Builder
	for i, tok := range tokens {
		if i > 0 && needsSpace(tokens[i-1], tok) {
			b.WriteByte(' ')
		}
		b.WriteString(tok.text)
	}
	return strings.TrimSpace(b.String())
}

func joinExpressionTokens(tokens []token) string {
	value := joinTokens(tokens)
	replacer := strings.NewReplacer(
		" . ", ".",
		" .", ".",
		". ", ".",
		" (", "(",
		" )", ")",
		" [", "[",
		" ]", "]",
		"= =", "==",
		"! =", "!=",
		"< =", "<=",
		"> =", ">=",
		"& &", "&&",
		"| |", "||",
		"+ +", "++",
		"- -", "--",
	)
	return strings.TrimSpace(replacer.Replace(value))
}

func needsSpace(left, right token) bool {
	if left.typ == antlrparser.JavaStructureLexerDOT || right.typ == antlrparser.JavaStructureLexerDOT {
		return false
	}
	if right.typ == antlrparser.JavaStructureLexerCOMMA || right.typ == antlrparser.JavaStructureLexerSEMI || right.typ == antlrparser.JavaStructureLexerRPAREN || right.typ == antlrparser.JavaStructureLexerRBRACK {
		return false
	}
	if left.typ == antlrparser.JavaStructureLexerLPAREN || left.typ == antlrparser.JavaStructureLexerLBRACK {
		return false
	}
	return true
}

func isModifier(typ int) bool {
	switch typ {
	case antlrparser.JavaStructureLexerPUBLIC,
		antlrparser.JavaStructureLexerPROTECTED,
		antlrparser.JavaStructureLexerPRIVATE,
		antlrparser.JavaStructureLexerSTATIC,
		antlrparser.JavaStructureLexerFINAL,
		antlrparser.JavaStructureLexerABSTRACT,
		antlrparser.JavaStructureLexerSYNCHRONIZED,
		antlrparser.JavaStructureLexerNATIVE,
		antlrparser.JavaStructureLexerDEFAULT,
		antlrparser.JavaStructureLexerSTRICTFP:
		return true
	default:
		return false
	}
}

func isControlName(name string) bool {
	switch name {
	case "if", "for", "while", "switch", "catch", "try", "return", "new":
		return true
	default:
		return false
	}
}

func cleanLiteral(value string) string {
	value = strings.TrimSpace(value)
	if unquoted, err := strconv.Unquote(value); err == nil {
		return unquoted
	}
	return strings.Trim(value, "\"'")
}

func shortName(name string) string {
	if idx := strings.LastIndex(name, "."); idx >= 0 {
		return name[idx+1:]
	}
	return name
}

func stableID(parts ...string) string {
	joined := strings.Join(parts, ":")
	replacer := strings.NewReplacer("\\", "/", " ", "-", ":", "-", ".", "-", "_", "-")
	return strings.Trim(replacer.Replace(strings.ToLower(joined)), "-")
}

func evidence(path string, line int, kind, symbol string) model.Evidence {
	return model.Evidence{
		Path:   filepath.Clean(path),
		Line:   line,
		Symbol: symbol,
		Kind:   kind,
	}
}
