// Code generated from JavaStructureParser.g4 by ANTLR 4.13.1. DO NOT EDIT.

package antlrparser // JavaStructureParser
import (
	"fmt"
	"strconv"
	"sync"

	"github.com/antlr4-go/antlr/v4"
)

// Suppress unused import errors
var _ = fmt.Printf
var _ = strconv.Itoa
var _ = sync.Once{}

type JavaStructureParser struct {
	*antlr.BaseParser
}

var JavaStructureParserParserStaticData struct {
	once                   sync.Once
	serializedATN          []int32
	LiteralNames           []string
	SymbolicNames          []string
	RuleNames              []string
	PredictionContextCache *antlr.PredictionContextCache
	atn                    *antlr.ATN
	decisionToDFA          []*antlr.DFA
}

func javastructureparserParserInit() {
	staticData := &JavaStructureParserParserStaticData
	staticData.LiteralNames = []string{
		"", "'package'", "'import'", "'class'", "'interface'", "'enum'", "'record'",
		"'public'", "'protected'", "'private'", "'static'", "'final'", "'abstract'",
		"'synchronized'", "'native'", "'default'", "'strictfp'", "'void'", "'extends'",
		"'implements'", "'throws'", "'new'", "'return'", "'if'", "'for'", "'while'",
		"'switch'", "'try'", "'catch'", "'@'", "'('", "')'", "'{'", "'}'", "'['",
		"']'", "';'", "','", "'.'", "'='", "':'", "'::'", "'<'", "'>'", "'?'",
	}
	staticData.SymbolicNames = []string{
		"", "PACKAGE", "IMPORT", "CLASS", "INTERFACE", "ENUM", "RECORD", "PUBLIC",
		"PROTECTED", "PRIVATE", "STATIC", "FINAL", "ABSTRACT", "SYNCHRONIZED",
		"NATIVE", "DEFAULT", "STRICTFP", "VOID", "EXTENDS", "IMPLEMENTS", "THROWS",
		"NEW", "RETURN", "IF", "FOR", "WHILE", "SWITCH", "TRY", "CATCH", "AT",
		"LPAREN", "RPAREN", "LBRACE", "RBRACE", "LBRACK", "RBRACK", "SEMI",
		"COMMA", "DOT", "ASSIGN", "COLON", "DOUBLE_COLON", "LT", "GT", "QUESTION",
		"STRING_LITERAL", "CHAR_LITERAL", "NUMBER_LITERAL", "IDENTIFIER", "LINE_COMMENT",
		"BLOCK_COMMENT", "WS", "OTHER",
	}
	staticData.RuleNames = []string{
		"compilationUnit", "token",
	}
	staticData.PredictionContextCache = antlr.NewPredictionContextCache()
	staticData.serializedATN = []int32{
		4, 1, 52, 15, 2, 0, 7, 0, 2, 1, 7, 1, 1, 0, 5, 0, 6, 8, 0, 10, 0, 12, 0,
		9, 9, 0, 1, 0, 1, 0, 1, 1, 1, 1, 1, 1, 0, 0, 2, 0, 2, 0, 0, 13, 0, 7, 1,
		0, 0, 0, 2, 12, 1, 0, 0, 0, 4, 6, 3, 2, 1, 0, 5, 4, 1, 0, 0, 0, 6, 9, 1,
		0, 0, 0, 7, 5, 1, 0, 0, 0, 7, 8, 1, 0, 0, 0, 8, 10, 1, 0, 0, 0, 9, 7, 1,
		0, 0, 0, 10, 11, 5, 0, 0, 1, 11, 1, 1, 0, 0, 0, 12, 13, 9, 0, 0, 0, 13,
		3, 1, 0, 0, 0, 1, 7,
	}
	deserializer := antlr.NewATNDeserializer(nil)
	staticData.atn = deserializer.Deserialize(staticData.serializedATN)
	atn := staticData.atn
	staticData.decisionToDFA = make([]*antlr.DFA, len(atn.DecisionToState))
	decisionToDFA := staticData.decisionToDFA
	for index, state := range atn.DecisionToState {
		decisionToDFA[index] = antlr.NewDFA(state, index)
	}
}

// JavaStructureParserInit initializes any static state used to implement JavaStructureParser. By default the
// static state used to implement the parser is lazily initialized during the first call to
// NewJavaStructureParser(). You can call this function if you wish to initialize the static state ahead
// of time.
func JavaStructureParserInit() {
	staticData := &JavaStructureParserParserStaticData
	staticData.once.Do(javastructureparserParserInit)
}

// NewJavaStructureParser produces a new parser instance for the optional input antlr.TokenStream.
func NewJavaStructureParser(input antlr.TokenStream) *JavaStructureParser {
	JavaStructureParserInit()
	this := new(JavaStructureParser)
	this.BaseParser = antlr.NewBaseParser(input)
	staticData := &JavaStructureParserParserStaticData
	this.Interpreter = antlr.NewParserATNSimulator(this, staticData.atn, staticData.decisionToDFA, staticData.PredictionContextCache)
	this.RuleNames = staticData.RuleNames
	this.LiteralNames = staticData.LiteralNames
	this.SymbolicNames = staticData.SymbolicNames
	this.GrammarFileName = "JavaStructureParser.g4"

	return this
}

// JavaStructureParser tokens.
const (
	JavaStructureParserEOF            = antlr.TokenEOF
	JavaStructureParserPACKAGE        = 1
	JavaStructureParserIMPORT         = 2
	JavaStructureParserCLASS          = 3
	JavaStructureParserINTERFACE      = 4
	JavaStructureParserENUM           = 5
	JavaStructureParserRECORD         = 6
	JavaStructureParserPUBLIC         = 7
	JavaStructureParserPROTECTED      = 8
	JavaStructureParserPRIVATE        = 9
	JavaStructureParserSTATIC         = 10
	JavaStructureParserFINAL          = 11
	JavaStructureParserABSTRACT       = 12
	JavaStructureParserSYNCHRONIZED   = 13
	JavaStructureParserNATIVE         = 14
	JavaStructureParserDEFAULT        = 15
	JavaStructureParserSTRICTFP       = 16
	JavaStructureParserVOID           = 17
	JavaStructureParserEXTENDS        = 18
	JavaStructureParserIMPLEMENTS     = 19
	JavaStructureParserTHROWS         = 20
	JavaStructureParserNEW            = 21
	JavaStructureParserRETURN         = 22
	JavaStructureParserIF             = 23
	JavaStructureParserFOR            = 24
	JavaStructureParserWHILE          = 25
	JavaStructureParserSWITCH         = 26
	JavaStructureParserTRY            = 27
	JavaStructureParserCATCH          = 28
	JavaStructureParserAT             = 29
	JavaStructureParserLPAREN         = 30
	JavaStructureParserRPAREN         = 31
	JavaStructureParserLBRACE         = 32
	JavaStructureParserRBRACE         = 33
	JavaStructureParserLBRACK         = 34
	JavaStructureParserRBRACK         = 35
	JavaStructureParserSEMI           = 36
	JavaStructureParserCOMMA          = 37
	JavaStructureParserDOT            = 38
	JavaStructureParserASSIGN         = 39
	JavaStructureParserCOLON          = 40
	JavaStructureParserDOUBLE_COLON   = 41
	JavaStructureParserLT             = 42
	JavaStructureParserGT             = 43
	JavaStructureParserQUESTION       = 44
	JavaStructureParserSTRING_LITERAL = 45
	JavaStructureParserCHAR_LITERAL   = 46
	JavaStructureParserNUMBER_LITERAL = 47
	JavaStructureParserIDENTIFIER     = 48
	JavaStructureParserLINE_COMMENT   = 49
	JavaStructureParserBLOCK_COMMENT  = 50
	JavaStructureParserWS             = 51
	JavaStructureParserOTHER          = 52
)

// JavaStructureParser rules.
const (
	JavaStructureParserRULE_compilationUnit = 0
	JavaStructureParserRULE_token           = 1
)

// ICompilationUnitContext is an interface to support dynamic dispatch.
type ICompilationUnitContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	EOF() antlr.TerminalNode
	AllToken() []ITokenContext
	Token(i int) ITokenContext

	// IsCompilationUnitContext differentiates from other interfaces.
	IsCompilationUnitContext()
}

type CompilationUnitContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyCompilationUnitContext() *CompilationUnitContext {
	var p = new(CompilationUnitContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = JavaStructureParserRULE_compilationUnit
	return p
}

func InitEmptyCompilationUnitContext(p *CompilationUnitContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = JavaStructureParserRULE_compilationUnit
}

func (*CompilationUnitContext) IsCompilationUnitContext() {}

func NewCompilationUnitContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *CompilationUnitContext {
	var p = new(CompilationUnitContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = JavaStructureParserRULE_compilationUnit

	return p
}

func (s *CompilationUnitContext) GetParser() antlr.Parser { return s.parser }

func (s *CompilationUnitContext) EOF() antlr.TerminalNode {
	return s.GetToken(JavaStructureParserEOF, 0)
}

func (s *CompilationUnitContext) AllToken() []ITokenContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(ITokenContext); ok {
			len++
		}
	}

	tst := make([]ITokenContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(ITokenContext); ok {
			tst[i] = t.(ITokenContext)
			i++
		}
	}

	return tst
}

func (s *CompilationUnitContext) Token(i int) ITokenContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ITokenContext); ok {
			if j == i {
				t = ctx.(antlr.RuleContext)
				break
			}
			j++
		}
	}

	if t == nil {
		return nil
	}

	return t.(ITokenContext)
}

func (s *CompilationUnitContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *CompilationUnitContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *CompilationUnitContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case JavaStructureParserVisitor:
		return t.VisitCompilationUnit(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *JavaStructureParser) CompilationUnit() (localctx ICompilationUnitContext) {
	localctx = NewCompilationUnitContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 0, JavaStructureParserRULE_compilationUnit)
	var _la int

	p.EnterOuterAlt(localctx, 1)
	p.SetState(7)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}
	_la = p.GetTokenStream().LA(1)

	for (int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&9007199254740990) != 0 {
		{
			p.SetState(4)
			p.Token()
		}

		p.SetState(9)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)
	}
	{
		p.SetState(10)
		p.Match(JavaStructureParserEOF)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// ITokenContext is an interface to support dynamic dispatch.
type ITokenContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser
	// IsTokenContext differentiates from other interfaces.
	IsTokenContext()
}

type TokenContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyTokenContext() *TokenContext {
	var p = new(TokenContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = JavaStructureParserRULE_token
	return p
}

func InitEmptyTokenContext(p *TokenContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = JavaStructureParserRULE_token
}

func (*TokenContext) IsTokenContext() {}

func NewTokenContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *TokenContext {
	var p = new(TokenContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = JavaStructureParserRULE_token

	return p
}

func (s *TokenContext) GetParser() antlr.Parser { return s.parser }
func (s *TokenContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *TokenContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *TokenContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case JavaStructureParserVisitor:
		return t.VisitToken(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *JavaStructureParser) Token() (localctx ITokenContext) {
	localctx = NewTokenContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 2, JavaStructureParserRULE_token)
	p.EnterOuterAlt(localctx, 1)
	p.SetState(12)
	p.MatchWildcard()

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}
