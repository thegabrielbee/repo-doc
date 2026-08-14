# Documentacao inicial

Este diretorio consolida o entendimento extraido do audio
`audio_2026-08-10_06-04-20.ogg` e do alinhamento complementar feito na
mensagem do usuario.

## Objetivo

Criar uma ferramenta para mapear bases de codigo Java, incluindo aplicacoes
Spring e Java EE/Jakarta EE legado, e gerar documentacao clara dos processos
implementados no codigo.

O foco inicial e transformar codigo fonte em entendimento navegavel:

- quais produtos e features existem;
- onde cada processo comeca;
- quais entradas, saidas e variacoes existem;
- quais classes, metodos e dependencias participam;
- quais bancos, buckets, propriedades e integracoes sao usados;
- como features e servicos se relacionam.

## Escopo inicial

- Linguagem: Java.
- Versoes alvo: Java 8 ate Java 21.
- Ecossistemas suportados por add-on: Spring/Spring Boot e Java EE/Jakarta EE
  legado.
- Analise principal: estatica, a partir da base de codigo.
- Representacao interna esperada: arvore sintatica e grafo de relacoes.
- Saida esperada: documentacao em estilo Confluence/Markdown.

## Fora do escopo por enquanto

- Analise de variaveis de ambiente.
- Suporte a Go ou outras linguagens na fase 1.
- Definicao final de UX, fluxo de perguntas do agente ou workflow interativo.
- Execucao runtime obrigatoria da aplicacao analisada.

## Arquivos

- [product-vision.md](./product-vision.md): visao do produto, problema e resultado esperado.
- [features.md](./features.md): features propostas para a ferramenta.
- [technical-approach.md](./technical-approach.md): arquitetura tecnica inicial.
- [usage-parameters.md](./usage-parameters.md): parametros publicos, valores aceitos e valores normalizados nos artefatos.
- [generated-document-template.md](./generated-document-template.md): modelo de documento gerado para cada feature/processo.
- [open-questions.md](./open-questions.md): decisoes pendentes e perguntas para proximas etapas.
- [source-audio-summary.md](./source-audio-summary.md): resumo estruturado do audio usado como fonte.
