# Visao do produto

## Nome de trabalho

Java Process Mapper.

O nome ainda nao foi confirmado. Este documento usa esse nome apenas como
placeholder para facilitar a organizacao.

## Contexto

O audio descreve um cenario em que uma organizacao possui muitos sistemas,
servicos, processos e deploys frequentes. A dificuldade central e entender por
que as coisas acontecem, porque a documentacao do comportamento esperado nao
acompanha o codigo. Na pratica, a resposta acaba sendo "esta no codigo", mas o
codigo nao esta facil de entender por pessoas novas, times de negocio ou ate
times tecnicos que nao mantem aquele trecho todos os dias.

## Problema

Bases Java/Spring grandes tendem a espalhar um processo de negocio entre
controllers, services, repositories, clients HTTP, consumers de mensageria,
configuracoes, properties e recursos externos. Sem uma visao consolidada,
ficam dificeis perguntas como:

- qual feature este trecho implementa;
- qual produto ou dominio essa feature atende;
- onde o processo comeca;
- quais entradas sao aceitas;
- quais caminhos o processo pode seguir;
- quais classes e metodos participam;
- quais bancos de dados, buckets, filas, APIs e propriedades sao usados;
- quais outros servicos ou features sao relacionados.

## Proposta

Construir uma ferramenta que leia uma base Java, gere um mapa tecnico dos
processos e produza uma documentacao legivel para negocio e tecnologia.

A ferramenta deve extrair informacoes do codigo e de arquivos de configuracao,
montar uma representacao interna por AST e grafo, e gerar documentos em estilo
Confluence/Markdown com titulo, produto, feature, processo de negocio,
processo tecnico, dependencias e relacoes.

## Resultado esperado da fase 1

Na fase 1, a ferramenta deve conseguir apontar para uma base Java/Spring e
gerar, em minutos ou horas dependendo do tamanho do repositorio, uma primeira
versao da documentacao dos processos mapeados.

O resultado nao precisa ser perfeito na primeira execucao. Ele deve deixar
explicito:

- evidencias encontradas no codigo;
- inferencias feitas pela ferramenta;
- pontos de baixa confianca;
- lacunas que precisam de revisao humana.

## Principios

- O codigo e a fonte primaria, mas a saida precisa ser compreensivel fora do
  codigo.
- A documentacao deve falar tanto com negocio quanto com engenharia.
- Toda inferencia deve carregar evidencia: arquivo, classe, metodo, anotacao,
  property ou dependencia encontrada.
- A ferramenta deve evitar depender de execucao runtime.
- Variaveis de ambiente nao devem ser coletadas ou documentadas na fase 1.
