// Code generated from JavaStructureParser.g4 by ANTLR 4.13.1. DO NOT EDIT.

package antlrparser // JavaStructureParser
import "github.com/antlr4-go/antlr/v4"

type BaseJavaStructureParserVisitor struct {
	*antlr.BaseParseTreeVisitor
}

func (v *BaseJavaStructureParserVisitor) VisitCompilationUnit(ctx *CompilationUnitContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaStructureParserVisitor) VisitToken(ctx *TokenContext) interface{} {
	return v.VisitChildren(ctx)
}
