# Arquitetura do Desafio Go Expert - Clean Architecture

Explicação técnica de como o projeto funciona e o papel de cada pasta/arquivo.

## Visão geral do fluxo

```
REST :8000 / gRPC :50051 / GraphQL :8090
                    │
                    ▼
   internal/usecase (CreateOrderUseCase, ListOrdersUseCase)
                    │
                    ▼
 entity.OrderRepositoryInterface (Save, FindAll)
                    │
                    ▼
   infra/database.OrderRepository ──► MySQL

   CreateOrderUseCase, depois de salvar:
                    │
                    ▼
   events.EventDispatcher.Dispatch(OrderCreated)
                    │
                    ▼
   event/handler.OrderCreatedHandler ──► RabbitMQ (exchange amq.direct)
```

Três protocolos (REST, gRPC, GraphQL) são camadas finas de adaptação que
convertem o formato de cada um (JSON, protobuf, GraphQL model) para o mesmo
par de casos de uso. Nenhum dos três conhece o outro, e nenhum conhece MySQL
ou RabbitMQ diretamente: só a interface `entity.OrderRepositoryInterface` e
`events.EventDispatcherInterface`.

## `internal/entity`: núcleo, sem infraestrutura

**`order.go`**: `Order{ID, Price, Tax, FinalPrice}`. `NewOrder` já valida
(`IsValid`: id não vazio, price/tax > 0). `CalculateFinalPrice` soma
`Price+Tax` e revalida; é chamado pelo use case antes de persistir.

**`interface.go`**: `OrderRepositoryInterface{Save, FindAll}`. É o único
contrato que a camada de banco precisa cumprir; qualquer implementação
(MySQL, SQLite em teste, outra coisa) entra aqui.

## `internal/usecase`

**`create_order.go`**: `CreateOrderUseCase.Execute(dto)` monta `Order`,
chama `CalculateFinalPrice`, salva via `Save` no repositório, seta o
payload no evento `OrderCreated` e despacha via `EventDispatcher`, tudo
isso antes de devolver o DTO de saída. Ou seja, a publicação no RabbitMQ é
parte do caso de uso, não da camada HTTP/gRPC/GraphQL.

**`list_orders.go`**: `ListOrdersUseCase.Execute()` chama `FindAll` no
repositório e mapeia `[]entity.Order` para `[]OrderOutputDTO`. Não dispara
evento (é só leitura).

## `internal/infra/database`

**`order_repository.go`**: implementação MySQL de
`OrderRepositoryInterface` usando `database/sql` puro (sem ORM).

- `Save`: `INSERT INTO orders (...)`.
- `FindAll`: `SELECT id, price, tax, final_price FROM orders`, itera com
  `rows.Next()`/`Scan` e retorna `[]entity.Order`.

**`migrations/`**: `000001_create_orders_table.up/down.sql` +
`migrations.go` com `//go:embed *.sql` num `embed.FS`. Isso embute os
`.sql` no binário final (importante porque a imagem de produção é
`FROM scratch`, sem filesystem externo pra ler migrations de disco).
`cmd/ordersystem/main.go` usa esse `FS` via `iofs.New` +
`golang-migrate/migrate/v4` pra rodar `m.Up()` automaticamente no startup,
ignorando `migrate.ErrNoChange` (banco já migrado).

## `internal/event` e `pkg/events`: publish/subscribe interno

**`pkg/events/interface.go`** define três interfaces: `EventInterface`
(nome, payload, timestamp), `EventHandlerInterface`
(`Handle(event, *sync.WaitGroup)`) e `EventDispatcherInterface`
(`Register/Dispatch/Remove/Has/Clear`).

**`pkg/events/event_dispatcher.go`**: `Dispatch` sobe uma goroutine por
handler registrado pro evento e usa `sync.WaitGroup` pra esperar todos
terminarem antes de retornar. `Register` recusa handler duplicado
(`ErrHandlerAlreadyRegistered`, comparando ponteiros).

**`internal/event/order_created.go`**: implementação concreta do evento
`OrderCreated` (nome fixo, payload setável).

**`internal/event/handler/order_created_handler.go`**: único handler
registrado (em `main.go`, para o nome `"OrderCreated"`). Serializa o
payload em JSON e publica no RabbitMQ (`amq.direct`, sem routing key,
`mandatory=false`).

## `internal/infra/web`

**`webserver/webserver.go`**: wrapper fino sobre `chi.Router`. `AddHandler`
registra `POST` e `AddGetHandler` registra `GET`, direto no router do chi,
sem mapa manual de rotas. O middleware `Logger` é plugado já na criação
(`NewWebServer`), não no `Start`.

**`order_handler.go`**: `WebOrderHandler{EventDispatcher, OrderRepository,
OrderCreatedEvent}`.

- `Create`: decodifica `OrderInputDTO` do body, chama
  `usecase.NewCreateOrderUseCase(...).Execute`. Em erro, checa se a
  mensagem contém `"Duplicate entry"` (erro do MySQL pra PK duplicada) e
  responde `409` com JSON `{"error": "..."}`; qualquer outro erro vira
  `500` no mesmo formato.
- `List`: monta `usecase.NewListOrdersUseCase(h.OrderRepository)` na hora
  (não precisa de estado extra) e serializa `[]OrderOutputDTO` como JSON.

## `internal/infra/grpc`

**`protofiles/order.proto`**: define `CreateOrderRequest/Response` e, para
a listagem, `Blank` (mensagem vazia, já que a chamada não recebe
argumentos) e `OrderList{repeated CreateOrderResponse orders}`. Serviço
`OrderService` com `CreateOrder` e `ListOrders`. Gerado com
`protoc --go_out=. --go-grpc_out=.` (protoc-gen-go v1.36.6,
protoc-gen-go-grpc v1.5.1) para `pb/order.pb.go` e `pb/order_grpc.pb.go`.

**`service/order_service.go`**: `OrderService{CreateOrderUseCase,
ListOrdersUseCase}` implementa os dois métodos convertendo DTOs para as
mensagens protobuf (`float64` para `float32`). `main.go` registra
`reflection.Register(grpcServer)`, o que permite inspecionar e chamar os
métodos com `grpcurl` sem precisar do `.proto` na mão.

## `internal/infra/graph`

**`schema.graphqls`**: `type Query { listOrders: [Order!]! }` e
`type Mutation { createOrder(input: OrderInput): Order }`.

**`gqlgen.yml`**: configura o gqlgen em layout single-file pro código
gerado (`generated.go`) e follow-schema pros resolvers
(`schema.resolvers.go`). Regenerado com
`go run github.com/99designs/gqlgen generate`.

**`resolver.go`**: arquivo que o gqlgen não regenera automaticamente, onde
entra a injeção de dependência: `Resolver{CreateOrderUseCase,
ListOrdersUseCase}`.

**`schema.resolvers.go`**: `CreateOrder` e `ListOrders` só convertem entre
`model.Order`/`model.OrderInput` (tipos gerados pelo gqlgen a partir do
schema) e os DTOs do usecase.

## `cmd/ordersystem`: composição e injeção de dependência

**`wire.go`** (`//go:build wireinject`, nunca compilado direto) declara os
providers via [`google/wire`](https://github.com/google/wire):
`setOrderRepositoryDependency` (liga `OrderRepositoryInterface` à
`*database.OrderRepository`), `setOrderCreatedEvent`, e as funções
injetoras `NewCreateOrderUseCase`, `NewWebOrderHandler` e
`NewListOrdersUseCase`. Cada uma recebe `*sql.DB` (e o que mais precisar) e
devolve o tipo pronto, já com as dependências resolvidas.

**`wire_gen.go`**: gerado por `wire` (rodado dentro de
`cmd/ordersystem/`), contém o código real, sem reflection em runtime, tudo
construído explicitamente. É o que de fato compila e é usado pelo
`Dockerfile`.

**`main.go`**: orquestra tudo nessa ordem: carrega config (`viper`, via
`.env`), abre a conexão MySQL, roda `runMigrations` (bloqueia até
terminar, antes de qualquer servidor subir), conecta no RabbitMQ, registra
`OrderCreatedHandler` no `EventDispatcher`, monta os três casos de uso via
wire, e sobe os três servidores em paralelo: REST (`go webserver.Start()`),
gRPC (`go grpcServer.Serve(lis)`) e GraphQL (bloqueante,
`http.ListenAndServe` na goroutine principal), cada um na sua porta.

## Infra Docker

**`Dockerfile`**: build multi-stage. Compila com `CGO_ENABLED=0
GOOS=linux` em `golang:1.25-alpine` (binário estático, sem libc) e roda em
`FROM scratch`, então a imagem final tem só o binário e o `.env`, sem
shell nem SO.

**`docker-compose.yaml`**: sobe `mysql`, `rabbitmq` e `app`. O `mysql` tem
`healthcheck` (`mysqladmin ping`) e o `app` só sobe quando
`depends_on.mysql.condition: service_healthy`, o que evita a race
condition clássica de a aplicação tentar migrar ou conectar antes do MySQL
aceitar conexões.

## Testes

- `internal/entity/order_test.go`: regras de negócio puras, id/price/tax
  inválidos, cálculo do `FinalPrice`.
- `internal/infra/database/order_repository_test.go`: usa
  `github.com/mattn/go-sqlite3` em memória (`:memory:`) com o mesmo schema
  da tabela `orders`, via `testify/suite`, pra validar `Save` sem precisar
  de um MySQL real rodando.
- `pkg/events/event_dispatcher_test.go`: cobre `Register` (com detecção de
  handler duplicado), `Has`, `Remove`, `Clear` e `Dispatch` com
  `testify/mock`, garantindo que todos os handlers registrados pro evento
  são chamados exatamente uma vez.

Nenhum teste depende de MySQL, RabbitMQ ou dos servidores REST/gRPC/GraphQL
de verdade. Cada camada é testada isolada; a integração completa é
validada manualmente subindo `docker compose up --build`.
