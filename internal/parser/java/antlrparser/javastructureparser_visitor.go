// Code generated from JavaStructureParser.g4 by ANTLR 4.13.1. DO NOT EDIT.

package antlrparser // JavaStructureParser
import "github.com/antlr4-go/antlr/v4"

// A complete Visitor for a parse tree produced by JavaStructureParser.
type JavaStructureParserVisitor interface {
	antlr.ParseTreeVisitor

	// Visit a parse tree produced by JavaStructureParser#compilationUnit.
	VisitCompilationUnit(ctx *CompilationUnitContext) interface{}

	// Visit a parse tree produced by JavaStructureParser#token.
	VisitToken(ctx *TokenContext) interface{}
}
