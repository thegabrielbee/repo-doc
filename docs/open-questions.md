# Pontos em aberto

## Produto

- Qual sera o nome oficial do produto?
- A documentacao gerada deve ser somente Markdown ou deve publicar direto no
  Confluence?
- O grafo precisa ser persistido em arquivo, banco de grafos ou ambos?

## Analise

- Qual nivel de precisao e aceitavel na fase 1 para inferencia de feature?
- A ferramenta deve analisar apenas um repositorio por vez ou varios
  repositorios como um unico produto?
- Bases multi-modulo Maven/Gradle entram no MVP?
- Deve haver suporte inicial a Lombok?
- Deve haver suporte inicial a Spring Batch e mensageria, ou entram logo apos
  HTTP/controllers?

## Dados e seguranca

- Como tratar properties que apontam para segredos ou valores externos?
- Alem de variaveis de ambiente, ha outros tipos de configuracao que devem ser
  ignorados?
- A ferramenta pode armazenar trechos de codigo como evidencia ou deve guardar
  somente referencias de arquivo/linha?

## Saida

- O documento gerado deve ser um arquivo por feature, por processo ou por
  produto?
- Quais secoes sao obrigatorias para a primeira versao?
- A saida deve incluir diagramas Mermaid, JSON de grafo ou ambos?

## Proximas decisoes sugeridas

1. Confirmar nome de trabalho e formato de saida.
2. Escolher stack do parser Java: ANTLR puro ou ANTLR com camada semantica.
3. Definir MVP tecnico: HTTP controllers primeiro ou todos os entrypoints
   Spring principais.
4. Definir schema do grafo e schema do documento gerado.
5. Escolher uma base Java real para validar a primeira implementacao.
