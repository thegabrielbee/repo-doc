# Abordagem tecnica inicial

## Pipeline proposto

1. Receber o caminho da base de codigo Java.
2. Descobrir estrutura do projeto: modulos, build tool, source sets e arquivos
   de configuracao.
3. Parsear arquivos Java e gerar AST.
4. Extrair simbolos e metadados relevantes.
5. Enriquecer a analise com convencoes Spring e arquivos de properties/YAML.
6. Montar grafo tecnico de processos, classes, metodos e dependencias.
7. Agrupar fluxos em produto, feature e processo.
8. Gerar documentacao em Markdown/Confluence-style.
9. Marcar lacunas, incertezas e evidencias.

## Analise Java

O audio menciona ANTLR como caminho para gerar a arvore sintatica da linguagem.
Isso cobre bem a leitura estrutural do codigo, mas algumas respostas exigem
mais do que sintaxe.

ANTLR deve ser suficiente para:

- packages, imports, classes, interfaces e enums;
- metodos, parametros e retornos;
- annotations;
- blocos condicionais e chamadas de metodo;
- constantes e literais;
- declaracoes de campos.

Para resolucao mais precisa de tipos e chamadas, pode ser necessario adicionar
uma camada semantica no futuro, por exemplo:

- classpath do Maven/Gradle;
- indice de simbolos proprio;
- JavaParser, Spoon, Eclipse JDT ou `javac` API como apoio.

Na fase 1, a proposta e comecar com um indice pragmatico:

- nome totalmente qualificado quando disponivel;
- imports;
- package local;
- tipos declarados no projeto;
- heuristicas com evidencia e nivel de confianca.

## Analise Spring

A ferramenta deve reconhecer padroes Spring comuns, incluindo:

- `@SpringBootApplication`;
- `@RestController`, `@Controller`;
- `@RequestMapping`, `@GetMapping`, `@PostMapping`, `@PutMapping`,
  `@PatchMapping`, `@DeleteMapping`;
- `@Service`, `@Component`, `@Repository`, `@Configuration`;
- `@Bean`;
- `@Scheduled`;
- `@EventListener`;
- `@KafkaListener`, `@RabbitListener`, `@JmsListener`, listeners similares;
- Spring Batch, quando houver jobs, steps, readers, processors e writers;
- `CommandLineRunner` e `ApplicationRunner`;
- Feign, `RestTemplate`, `WebClient` e clients equivalentes.

## Configuracoes

Arquivos esperados:

- `application.properties`;
- `application.yml` e `application.yaml`;
- profiles Spring, como `application-dev.yml`;
- `bootstrap.properties` e `bootstrap.yml`, quando existirem;
- arquivos Maven/Gradle;
- arquivos de migracao, como Flyway ou Liquibase.

Regras:

- Ler chaves e valores estaticos quando existirem no repositorio.
- Associar chaves a classes via `@Value`, `@ConfigurationProperties`,
  builders e beans de configuracao.
- Nao coletar nem resolver variaveis de ambiente.
- Marcar valores indiretos como "definido externamente" sem expor segredo.

## Grafo tecnico

Nos principais:

- Product;
- Feature;
- Process;
- Service;
- EntryPoint;
- Class;
- Method;
- DataStore;
- Table;
- Bucket;
- Queue;
- Topic;
- ExternalApi;
- ConfigProperty.

Arestas principais:

- `contains`;
- `starts_at`;
- `calls`;
- `depends_on`;
- `reads_from`;
- `writes_to`;
- `publishes_to`;
- `consumes_from`;
- `configured_by`;
- `related_by`;
- `how`.

## Modelo de evidencia

Cada item documentado deve apontar para evidencias, por exemplo:

- arquivo;
- linha ou intervalo, quando disponivel;
- classe;
- metodo;
- annotation;
- property;
- dependencia Maven/Gradle;
- migration;
- chamada de metodo;
- declaracao de client ou repository.

## Saidas

Saidas iniciais recomendadas:

- Markdown por feature/processo;
- indice geral por produto;
- grafo serializado em JSON;
- relatorio de lacunas e baixa confianca.

## Riscos tecnicos

- AST sozinha nao resolve todos os destinos de chamada.
- Reflection, proxies, Lombok e geracao de codigo podem esconder relacoes.
- Spring permite configuracoes implicitas que podem exigir heuristicas.
- Buckets, bancos e endpoints podem vir de variaveis de ambiente, que estao
  fora do escopo inicial.
- Bases multi-modulo exigem cuidado para resolver dependencias internas.
