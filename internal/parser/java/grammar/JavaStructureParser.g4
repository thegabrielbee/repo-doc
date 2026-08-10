parser grammar JavaStructureParser;

options {
    tokenVocab = JavaStructureLexer;
}

compilationUnit
    : token* EOF
    ;

token
    : .
    ;
