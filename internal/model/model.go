package model

type Confidence string

const (
	ConfidenceHigh   Confidence = "high"
	ConfidenceMedium Confidence = "medium"
	ConfidenceLow    Confidence = "low"
)

type SourceKind string

const (
	SourceFound    SourceKind = "found"
	SourceInferred SourceKind = "inferred"
	SourceMissing  SourceKind = "missing"
)

type Evidence struct {
	Path       string `json:"path,omitempty"`
	Line       int    `json:"line,omitempty"`
	Symbol     string `json:"symbol,omitempty"`
	Annotation string `json:"annotation,omitempty"`
	Property   string `json:"property,omitempty"`
	Kind       string `json:"kind"`
}

type Annotation struct {
	Name       string            `json:"name"`
	Values     map[string]string `json:"values,omitempty"`
	Raw        string            `json:"raw,omitempty"`
	Evidence   Evidence          `json:"evidence"`
	Source     SourceKind        `json:"source"`
	Confidence Confidence        `json:"confidence"`
}

type Project struct {
	Name             string           `json:"name"`
	Root             string           `json:"root"`
	JavaVersion      string           `json:"javaVersion,omitempty"`
	Modules          []Module         `json:"modules"`
	SourceFiles      []SourceFile     `json:"sourceFiles"`
	Types            []Type           `json:"types"`
	EntryPoints      []EntryPoint     `json:"entryPoints"`
	ConfigProperties []ConfigProperty `json:"configProperties"`
	Dependencies     []Dependency     `json:"dependencies"`
	Graph            Graph            `json:"graph"`
	Gaps             []Gap            `json:"gaps"`
	Summary          Summary          `json:"summary"`
}

type Summary struct {
	Modules          int `json:"modules"`
	JavaFiles        int `json:"javaFiles"`
	Types            int `json:"types"`
	Methods          int `json:"methods"`
	EntryPoints      int `json:"entryPoints"`
	ConfigProperties int `json:"configProperties"`
	Dependencies     int `json:"dependencies"`
	Gaps             int `json:"gaps"`
}

type Module struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Path            string   `json:"path"`
	BuildTool       string   `json:"buildTool,omitempty"`
	Packaging       string   `json:"packaging,omitempty"`
	JavaVersion     string   `json:"javaVersion,omitempty"`
	SourceRoots     []string `json:"sourceRoots"`
	JavaFiles       []string `json:"javaFiles"`
	ConfigFiles     []string `json:"configFiles"`
	MigrationFiles  []string `json:"migrationFiles"`
	DescriptorFiles []string `json:"descriptorFiles,omitempty"`
	UIFiles         []string `json:"uiFiles,omitempty"`
}

type SourceFile struct {
	Path      string   `json:"path"`
	ModuleID  string   `json:"moduleId,omitempty"`
	Package   string   `json:"package,omitempty"`
	Imports   []string `json:"imports,omitempty"`
	TypeIDs   []string `json:"typeIds,omitempty"`
	ParseNote string   `json:"parseNote,omitempty"`
}

type Type struct {
	ID          string       `json:"id"`
	ModuleID    string       `json:"moduleId,omitempty"`
	FilePath    string       `json:"filePath"`
	Package     string       `json:"package,omitempty"`
	Name        string       `json:"name"`
	FQN         string       `json:"fqn"`
	Kind        string       `json:"kind"`
	Annotations []Annotation `json:"annotations,omitempty"`
	Extends     []string     `json:"extends,omitempty"`
	Implements  []string     `json:"implements,omitempty"`
	Fields      []Field      `json:"fields,omitempty"`
	Methods     []Method     `json:"methods,omitempty"`
	StartLine   int          `json:"startLine,omitempty"`
	BodyLine    int          `json:"bodyLine,omitempty"`
	EndLine     int          `json:"endLine,omitempty"`
	Evidence    Evidence     `json:"evidence"`
	Source      SourceKind   `json:"source"`
	Confidence  Confidence   `json:"confidence"`
}

type Field struct {
	ID          string       `json:"id"`
	TypeID      string       `json:"typeId"`
	Name        string       `json:"name"`
	FieldType   string       `json:"fieldType,omitempty"`
	Annotations []Annotation `json:"annotations,omitempty"`
	Evidence    Evidence     `json:"evidence"`
	Source      SourceKind   `json:"source"`
	Confidence  Confidence   `json:"confidence"`
}

type Method struct {
	ID             string          `json:"id"`
	TypeID         string          `json:"typeId"`
	Name           string          `json:"name"`
	Modifiers      []string        `json:"modifiers,omitempty"`
	ReturnType     string          `json:"returnType,omitempty"`
	Parameters     []Parameter     `json:"parameters,omitempty"`
	LocalVariables []LocalVariable `json:"localVariables,omitempty"`
	Conditions     []Condition     `json:"conditions,omitempty"`
	Annotations    []Annotation    `json:"annotations,omitempty"`
	Calls          []Call          `json:"calls,omitempty"`
	StartLine      int             `json:"startLine,omitempty"`
	BodyLine       int             `json:"bodyLine,omitempty"`
	EndLine        int             `json:"endLine,omitempty"`
	Evidence       Evidence        `json:"evidence"`
	Source         SourceKind      `json:"source"`
	Confidence     Confidence      `json:"confidence"`
}

type Parameter struct {
	Name        string       `json:"name,omitempty"`
	Type        string       `json:"type,omitempty"`
	Annotations []Annotation `json:"annotations,omitempty"`
}

type LocalVariable struct {
	ID           string     `json:"id"`
	MethodID     string     `json:"methodId"`
	Name         string     `json:"name"`
	VariableType string     `json:"variableType,omitempty"`
	Evidence     Evidence   `json:"evidence"`
	Source       SourceKind `json:"source"`
	Confidence   Confidence `json:"confidence"`
}

type Condition struct {
	ID         string     `json:"id"`
	MethodID   string     `json:"methodId"`
	Kind       string     `json:"kind"`
	Expression string     `json:"expression,omitempty"`
	StartLine  int        `json:"startLine,omitempty"`
	BodyLine   int        `json:"bodyLine,omitempty"`
	EndLine    int        `json:"endLine,omitempty"`
	Evidence   Evidence   `json:"evidence"`
	Source     SourceKind `json:"source"`
	Confidence Confidence `json:"confidence"`
}

type Call struct {
	Target               string     `json:"target"`
	Receiver             string     `json:"receiver,omitempty"`
	Method               string     `json:"method,omitempty"`
	ResolvedTypeID       string     `json:"resolvedTypeId,omitempty"`
	ResolvedExternalType string     `json:"resolvedExternalType,omitempty"`
	ResolvedMethodID     string     `json:"resolvedMethodId,omitempty"`
	Resolution           string     `json:"resolution,omitempty"`
	Evidence             Evidence   `json:"evidence"`
	Source               SourceKind `json:"source"`
	Confidence           Confidence `json:"confidence"`
}

type ConfigProperty struct {
	Key               string     `json:"key"`
	Value             string     `json:"value,omitempty"`
	DefinedExternally bool       `json:"definedExternally"`
	Redacted          bool       `json:"redacted,omitempty"`
	SourceFile        string     `json:"sourceFile"`
	Evidence          Evidence   `json:"evidence"`
	Source            SourceKind `json:"source"`
	Confidence        Confidence `json:"confidence"`
}

type EntryPoint struct {
	ID         string     `json:"id"`
	Kind       string     `json:"kind"`
	Framework  string     `json:"framework,omitempty"`
	Name       string     `json:"name"`
	Product    string     `json:"product,omitempty"`
	Feature    string     `json:"feature,omitempty"`
	Path       string     `json:"path,omitempty"`
	HTTPMethod string     `json:"httpMethod,omitempty"`
	Resource   string     `json:"resource,omitempty"`
	ClassID    string     `json:"classId,omitempty"`
	MethodID   string     `json:"methodId,omitempty"`
	Evidence   Evidence   `json:"evidence"`
	Source     SourceKind `json:"source"`
	Confidence Confidence `json:"confidence"`
}

type Dependency struct {
	ID         string     `json:"id"`
	Kind       string     `json:"kind"`
	Name       string     `json:"name"`
	Detail     string     `json:"detail,omitempty"`
	ClassID    string     `json:"classId,omitempty"`
	MethodID   string     `json:"methodId,omitempty"`
	Evidence   Evidence   `json:"evidence"`
	Source     SourceKind `json:"source"`
	Confidence Confidence `json:"confidence"`
}

type Graph struct {
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
}

type GraphNode struct {
	ID         string         `json:"id"`
	Kind       string         `json:"kind"`
	Name       string         `json:"name"`
	Properties map[string]any `json:"properties,omitempty"`
	Evidence   []Evidence     `json:"evidence,omitempty"`
	Source     SourceKind     `json:"source"`
	Confidence Confidence     `json:"confidence"`
}

type GraphEdge struct {
	ID         string     `json:"id"`
	From       string     `json:"from"`
	To         string     `json:"to"`
	Kind       string     `json:"kind"`
	How        string     `json:"how,omitempty"`
	Evidence   []Evidence `json:"evidence,omitempty"`
	Source     SourceKind `json:"source"`
	Confidence Confidence `json:"confidence"`
}

type Gap struct {
	ID         string     `json:"id"`
	Message    string     `json:"message"`
	Severity   string     `json:"severity"`
	Evidence   Evidence   `json:"evidence,omitempty"`
	Source     SourceKind `json:"source"`
	Confidence Confidence `json:"confidence"`
}

func (p *Project) RefreshSummary() {
	methods := 0
	for _, typ := range p.Types {
		methods += len(typ.Methods)
	}
	p.Summary = Summary{
		Modules:          len(p.Modules),
		JavaFiles:        len(p.SourceFiles),
		Types:            len(p.Types),
		Methods:          methods,
		EntryPoints:      len(p.EntryPoints),
		ConfigProperties: len(p.ConfigProperties),
		Dependencies:     len(p.Dependencies),
		Gaps:             len(p.Gaps),
	}
}
