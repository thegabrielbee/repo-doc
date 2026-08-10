# Template de documentacao gerada

Este template representa o documento que a ferramenta deve gerar para cada
feature ou processo identificado.

## Titulo

`<Titulo claro do processo>`

## Produto

- Nome: `<produto ou dominio>`
- Confianca: `<alta | media | baixa>`
- Evidencias: `<packages, endpoints, modulos, tabelas ou configuracoes>`

## Feature

- Nome: `<nome da feature>`
- Descricao curta: `<o que essa feature faz>`
- Status da inferencia: `<encontrado | inferido | pendente>`

## Resumo para negocio

Descrever em linguagem de negocio:

- qual problema o processo resolve;
- quando ele e acionado;
- quem ou o que inicia o fluxo;
- quais entradas sao aceitas;
- quais resultados sao produzidos;
- quais excecoes ou caminhos alternativos relevantes existem.

## Passo a passo de negocio

| Ordem | Passo | Entrada | Saida | Observacoes |
| --- | --- | --- | --- | --- |
| 1 | `<passo de negocio>` | `<input>` | `<output>` | `<observacao>` |

## Mapeamento tecnico

### Entrada do processo

- Tipo: `<HTTP | evento | scheduler | batch | runner | metodo interno>`
- Classe: `<classe>`
- Metodo: `<metodo>`
- Annotation: `<annotation Spring ou Java>`
- Endpoint/topico/job: `<identificador>`
- Evidencia: `<arquivo:linha>`

### Fluxo tecnico

| Ordem | Componente | Classe | Metodo | Responsabilidade |
| --- | --- | --- | --- | --- |
| 1 | `<controller/service/repository/client>` | `<classe>` | `<metodo>` | `<papel no fluxo>` |

### Entradas aceitas

| Nome | Tipo | Origem | Obrigatorio | Evidencia |
| --- | --- | --- | --- | --- |
| `<campo>` | `<tipo Java>` | `<path/query/body/event>` | `<sim/nao>` | `<arquivo:linha>` |

### Saidas produzidas

| Nome | Tipo | Destino | Evidencia |
| --- | --- | --- | --- |
| `<saida>` | `<tipo>` | `<HTTP response/evento/banco/S3>` | `<arquivo:linha>` |

## Classes e metodos

| Classe | Metodo | Tipo | Papel | Evidencia |
| --- | --- | --- | --- | --- |
| `<classe>` | `<metodo>` | `<controller/service/repository/client>` | `<descricao>` | `<arquivo:linha>` |

## Dependencias

### Banco de dados

| Recurso | Tipo | Uso | Evidencia |
| --- | --- | --- | --- |
| `<datasource/tabela/entity/repository>` | `<JPA/JDBC/etc>` | `<leitura/escrita>` | `<arquivo:linha>` |

### S3 ou storage

| Bucket/recurso | Operacao | Origem do valor | Evidencia |
| --- | --- | --- | --- |
| `<bucket>` | `<read/write/delete>` | `<property/config/classe>` | `<arquivo:linha>` |

### APIs externas

| Servico | Client | Operacao | Evidencia |
| --- | --- | --- | --- |
| `<servico>` | `<Feign/WebClient/RestTemplate/etc>` | `<acao>` | `<arquivo:linha>` |

### Filas, topicos e eventos

| Recurso | Direcao | Operacao | Evidencia |
| --- | --- | --- | --- |
| `<fila/topico/evento>` | `<consume/publish>` | `<acao>` | `<arquivo:linha>` |

## Configuracoes

| Chave | Valor encontrado | Origem | Observacao |
| --- | --- | --- | --- |
| `<property>` | `<valor ou definido externamente>` | `<arquivo>` | `<sem variavel de ambiente>` |

## Relacoes com outras features

| Feature relacionada | Related by | How | Evidencia |
| --- | --- | --- | --- |
| `<feature>` | `<chamada/evento/tabela/recurso>` | `<explicacao da relacao>` | `<arquivo:linha>` |

## Lacunas e incertezas

- `<informacao nao encontrada ou inferencia de baixa confianca>`

## Evidencias principais

- `<arquivo:linha> - <motivo>`
