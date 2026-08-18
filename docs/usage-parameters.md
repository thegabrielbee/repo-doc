# Parametros e valores disponiveis

Esta referencia descreve os parametros publicos do mapper e os valores
normalizados que aparecem nos artefatos gerados.

## CLI

A CLI publica e implementada em Python e delega a execucao pesada para o core
Go interno. Ela pode ser chamada pelo script instalado `java-process-mapper`,
por `python -m java_process_mapper` ou pelo shim de compatibilidade
`go run ./cmd/java-process-mapper`.

### `serve`

Inicia o servidor MCP via stdio.

```bash
java-process-mapper serve
python -m java_process_mapper serve
```

### `scan`

Executa o mapeamento local e imprime um resumo JSON.

```bash
java-process-mapper scan --root <repo> --out <dir> --addons spring
java-process-mapper scan --root <repo> --addons javaee --java-version 8
java-process-mapper scan --root <repo> --addons spring,javaee
python -m java_process_mapper scan --root <repo> --addons javaee --java-version 8
```

| Parametro | Obrigatorio | Padrao | Valores aceitos | Descricao |
| --- | --- | --- | --- | --- |
| `--root` | sim | n/a | caminho de diretorio | Raiz da base Java a analisar. |
| `--out` | nao | `<root>/out/java-process-mapper` | caminho de diretorio | Diretorio onde `graph.json`, `findings.json`, `traces.json` e docs serao escritos. |
| `--addons` | nao | `spring` | `spring`, `javaee`, ou lista separada por virgula | Add-ons de framework/tecnologia a executar. |
| `--java-version` | nao | inferido ou `unknown` | `8`, `11`, `17`, `21`, `1.8`, ou outro numero de versao Java | Override da versao fonte Java para todos os modulos. |
| `--include-tests` | nao | `false` | `true`, `false` | Inclui `src/test` e pastas de teste na analise. |

## MCP

### `start_mapping`

Inicia um job de mapeamento.

| Campo | Obrigatorio | Padrao | Valores aceitos | Descricao |
| --- | --- | --- | --- | --- |
| `rootPath` | sim | n/a | caminho de diretorio | Raiz da base Java a analisar. |
| `outputDir` | nao | `<root>/out/java-process-mapper` | caminho de diretorio | Diretorio dos artefatos. |
| `addons` | nao | `["spring"]` | `["spring"]`, `["javaee"]`, `["spring","javaee"]` | Add-ons de analise. |
| `javaVersion` | nao | inferido ou `unknown` | string numerica, como `"8"` ou `"17"` | Override da versao fonte Java. |
| `includeTests` | nao | `false` | `true`, `false` | Inclui fontes de teste. |

Exemplo:

```json
{
  "rootPath": "D:/repos/legacy",
  "outputDir": "D:/repos/legacy/out/java-process-mapper",
  "addons": ["javaee"],
  "javaVersion": "8",
  "includeTests": false
}
```

### Outras tools MCP

| Tool | Parametros | Descricao |
| --- | --- | --- |
| `get_mapping_status` | `jobId` | Retorna status, fase, contadores, erro e outputDir. |
| `get_mapping_result` | `jobId` | Retorna artefatos e resumo quando o job esta completo. |
| `get_next_mapping_item` | `jobId`, `includeMechanicalMarkdown` | Retorna o proximo entrypoint pendente e, opcionalmente, o Markdown mecanico. |
| `mark_mapping_item_mapped` | `jobId`, `entryPointId`, `markdown`, `title`, `notes`, `finalDocPath` | Salva a documentacao final de um item e marca como mapeado. |

## Add-ons

| Valor | Detecta |
| --- | --- |
| `spring` | Spring MVC/REST, schedulers, listeners, runners, batch, repositories, entities, clients HTTP, filas/topicos e configuracoes Spring. |
| `javaee` | Java EE/Jakarta EE legado: JAX-RS, JAX-WS, JSF/CDI, EJB, JPA, JMS/MDB, Servlets, Filters como pipeline HTTP, JAAS, descritores XML, EAR/WAR, XHTML/JSP/HTML e chamadas client-side HTTP/WebSocket. |

O default continua sendo `spring`. Para bases legadas Java EE, informe
`--addons javaee`. Para sistemas mistos, use `--addons spring,javaee`.

## Versao Java

A versao Java e normalizada para numero simples quando possivel.

| Entrada | Valor registrado |
| --- | --- |
| `8`, `1.8`, `JavaVersion.VERSION_1_8` | `8` |
| `11`, `JavaVersion.VERSION_11` | `11` |
| `17` | `17` |
| `21` | `21` |
| modulos com versoes diferentes | `mixed` no projeto |
| versao nao encontrada | `unknown` |

Ordem de decisao:

1. `--java-version` ou `javaVersion` explicito.
2. Maven/Gradle do modulo.
3. Maven/Gradle ancestral.
4. `unknown`.

## Metadados de modulo

| Campo | Valores comuns | Origem |
| --- | --- | --- |
| `buildTool` | `maven`, `gradle` | `pom.xml`, `build.gradle`, `build.gradle.kts`. |
| `packaging` | `pom`, `jar`, `war`, `ear` | Maven `<packaging>` ou plugins Gradle `war`/`ear`. |
| `javaVersion` | `8`, `11`, `17`, `21`, `unknown` | Override ou inferencia de build. |
| `descriptorFiles` | paths XML/WSDL | `web.xml`, `ejb-jar.xml`, `application.xml`, `persistence.xml`, `beans.xml`, `webservices.xml`, arquivos JBoss e `.wsdl`. |
| `uiFiles` | paths XHTML/JSP/HTML | Arquivos `.xhtml`, `.jsp`, `.html` e `.htm`. |

## Entry points

| `framework` | `kind` | Exemplos de origem |
| --- | --- | --- |
| `spring` | `http` | `@RequestMapping`, `@GetMapping`, `@PostMapping`, etc. |
| `spring` | `scheduler` | `@Scheduled`. |
| `spring` | `message_listener` | `@KafkaListener`, `@RabbitListener`, `@JmsListener`. |
| `spring` | `event_listener` | `@EventListener`. |
| `spring` | `runner` | `CommandLineRunner`, `ApplicationRunner`. |
| `spring` | `batch` | `Job`/`Step` expostos por `@Bean`. |
| `javaee` | `http` | JAX-RS `@Path` + `@GET/@POST/@PUT/@DELETE/@PATCH/@HEAD/@OPTIONS`. |
| `javaee` | `soap` | `@WebService`, `@WebMethod`. |
| `javaee` | `scheduler` | EJB `@Schedule`, `@Schedules`, `@Timeout`. |
| `javaee` | `startup` | `@Startup` sem metodo `@PostConstruct`, ou metodo `@PostConstruct` em tipo gerenciado pelo container. |
| `javaee` | `message_listener` | `@MessageDriven` e `onMessage`. |
| `javaee` | `servlet` | `@WebServlet` ou `web.xml`. |
| `javaee` | `listener` | `@WebListener`. |
| `javaee` | `websocket` | `@ServerEndpoint` com `@OnOpen`, `@OnMessage`, `@OnClose` ou `@OnError`. |
| `javaee` | `ui_page` | Arquivos `.xhtml`, `.jsp`, `.html` e `.htm`; quando possivel, EL como `#{bean.metodo}` ligado a `@Named/@ManagedBean`, ou chamada client-side `XMLHttpRequest`/`fetch`/`WebSocket` em script local referenciado pela pagina. |
| `javaee` | `event_listener` | Parametros `@Observes` ou `@ObservesAsync` para eventos de aplicacao. Eventos SPI de extensao CDI, como `BeforeBeanDiscovery`, `AfterBeanDiscovery` e `ProcessAnnotatedType`, nao viram processo. |

Nao sao tratados como entrypoints primarios:

- `@Asynchronous`: indica execucao assincrona de metodo EJB, mas nao inicia fluxo
  por si so.
- `@Path` sem `@GET/@POST/@PUT/@DELETE/@PATCH/@HEAD/@OPTIONS`: pode ser
  sub-recurso ou metadado de roteamento, mas nao uma operacao HTTP completa.
- `@Named`, `@ManagedBean`, scopes CDI/JSF, `@Stateless`, `@Stateful`,
  `@Singleton`, `@Entity`, `@Table`, `@PersistenceContext`, `@Resource`:
  descrevem componentes, modelo ou injecao; viram contexto/dependencia quando
  aplicavel.
- `<session><ejb-name>` em `ejb-jar.xml`: declara bean EJB, mas nao prova um
  gatilho externo ou de container para um processo.
- Eventos SPI de extensao CDI (`BeforeBeanDiscovery`, `AfterBeanDiscovery`,
  `ProcessAnnotatedType`, etc.): sao hooks de boot/extensibilidade do container,
  nao entrypoints de processo de aplicacao.
- `@WebFilter` e filtros em `web.xml`: nao aparecem na lista principal de
  entrypoints. Eles entram como `http_filter` dentro dos fluxos HTTP/Servlet
  cujo path casa com o `urlPatterns`/`url-pattern`.

## Dependencias

Valores de `dependency.kind` que podem aparecer:

| Valor | Significado |
| --- | --- |
| `database_repository` | Repository Spring ou equivalente. |
| `database_access` | Mapper/DAO. |
| `database_client` | JPA, JDBC, `EntityManager` ou client de banco. |
| `database_migration` | Arquivo Flyway/Liquibase. |
| `persistence_unit` | Unidade em `persistence.xml`. |
| `table` | Entidade/tabela JPA. |
| `repository_call` | Chamada inferida de persistencia, como `find`, `save`, `delete`, `persist`, `merge`. |
| `external_api` | Client HTTP/SOAP/Feign/REST. |
| `external_dependency` | Import externo resolvido no fluxo. |
| `queue` | JMS/SQS/Rabbit ou recurso de fila. |
| `topic` | Kafka/JMS topic. |
| `message_publish` | Publicacao/envio de mensagem inferido por chamada. |
| `bucket` | Bucket/storage inferido em chamada. |
| `s3` | Client S3. |
| `mail_server` | JavaMail/SMTP. |
| `ftp_endpoint` | FTP/SFTP. |
| `cache` | Redis/Jedis. |
| `auth_provider` | JAAS/LoginModule. |
| `http_filter` | Filter Servlet/Java EE aplicavel a um fluxo HTTP/Servlet por padrao de URL. |
| `ui_api_call` | Chamada HTTP iniciada por UI client-side, como `XMLHttpRequest` ou `fetch`. |
| `ui_websocket` | Conexao WebSocket iniciada por UI client-side. |
| `config_property` | Propriedade de configuracao ligada ao codigo. |

## Confianca e fonte

| Campo | Valores |
| --- | --- |
| `confidence` | `high`, `medium`, `low` |
| `source` | `found`, `inferred`, `missing` |

Valores sensiveis continuam redigidos. Placeholders como `${DB_URL}` aparecem
como `defined_externally`.
