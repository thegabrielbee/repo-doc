# Features propostas

## Fase 1: mapeamento Java/Spring/Java EE e documentacao

| ID | Feature | Descricao | Prioridade |
| --- | --- | --- | --- |
| F01 | Descoberta da base Java | Identificar modulos, pacotes, arquivos Java, Maven/Gradle, versao alvo e estrutura do projeto. | Alta |
| F02 | Parser Java por AST | Gerar arvore sintatica dos arquivos Java, com suporte esperado de Java 8 a Java 21. ANTLR e o candidato principal citado. | Alta |
| F03 | Extracao de classes e metodos | Mapear packages, imports, classes, interfaces, enums, records, metodos, assinaturas, annotations e chamadas relevantes. | Alta |
| F04 | Deteccao de pontos de entrada por add-on | Encontrar controllers/endpoints Spring e somente entrypoints Java EE/Jakarta EE realmente invocados por HTTP/SOAP/WebSocket, timer, lifecycle, mensageria, eventos CDI, Servlets, Listeners e telas UI XHTML/JSP/HTML, incluindo chamadas client-side a HTTP/WebSocket; Filters entram como pipeline dentro dos fluxos HTTP aplicaveis. | Alta |
| F05 | Mapeamento de processo | Para cada ponto de entrada, montar o fluxo tecnico inicial: inicio, chamadas internas, decisoes, saidas e erros principais. | Alta |
| F06 | Agrupamento por produto e feature | Inferir produto, dominio e feature a partir de package, nomes, endpoints, annotations, tabelas, topicos e convencoes. | Alta |
| F07 | Mapeamento de entradas e saidas | Identificar inputs aceitos, payloads, tipos, parametros, eventos, comandos, respostas e outputs relevantes. | Alta |
| F08 | Mapeamento de dependencias internas | Relacionar classes, metodos, services, repositories, clients, DTOs, entities e componentes usados no fluxo. | Alta |
| F09 | Mapeamento de banco de dados | Detectar uso de JPA, JDBC, repositories, entities, migrations e configuracoes de datasource sem coletar variaveis de ambiente. | Alta |
| F10 | Mapeamento de S3 e recursos externos | Detectar clientes S3, nomes de buckets quando estiverem em properties/config/classes, APIs externas, filas e topicos. | Media |
| F11 | Leitura de properties e YAML | Extrair configuracoes de `application.properties`, `application.yml`, profiles e arquivos equivalentes, excluindo variaveis de ambiente. | Alta |
| F12 | Grafo de relacoes | Transformar AST e extracoes em grafo com nos e arestas: chama, depende de, le, escreve, publica, consome, related by. | Alta |
| F13 | Relacoes entre features | Identificar quando features ou servicos se conectam e explicar a relacao com `related by` e `how`. | Media |
| F14 | Geracao de documentacao | Gerar documento em estilo Confluence/Markdown contendo produto, feature, titulo, processo de negocio, tecnico e evidencias. | Alta |
| F15 | Indicador de confianca | Marcar dados como encontrados, inferidos ou pendentes para revisao humana. | Media |

## Fase 2: demonstracao e extensoes

| ID | Feature | Descricao | Prioridade |
| --- | --- | --- | --- |
| F16 | Visualizacao AST para grafo | Demonstrar a passagem de arvore sintatica para grafo de conexoes entre processos. | Media |
| F17 | Relacao entre servicos heterogeneos | Preparar extensao futura para comparar Java com outras linguagens, como Go, mas sem implementar na fase 1. | Baixa |
| F18 | Identificacao via telemetria | Avaliar uso de padroes de telemetria ou OpenTelemetry para correlacionar servicos e relacoes em cenarios futuros. | Baixa |

## Requisitos nao funcionais iniciais

- Rodar em bases grandes sem exigir execucao da aplicacao.
- Produzir documentacao incremental e revisavel.
- Preservar evidencias de origem para auditoria do documento.
- Ser extensivel para novos frameworks, tecnologias Java e, no futuro, outras linguagens.
- Evitar coleta de segredos e variaveis de ambiente.
