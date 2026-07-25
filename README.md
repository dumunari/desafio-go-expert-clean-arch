# Desafio Go Expert - Clean Architecture

Sistema de pedidos em Go com Clean Architecture, expondo os mesmos casos de uso via REST, gRPC e GraphQL. Usa MySQL para persistência (com migrations automáticas) e RabbitMQ para publicar eventos de domínio.

## Pré-requisitos

- Docker e Docker Compose
- Go 1.25+ (só para rodar localmente sem Docker)

## Configuração

Copie o exemplo de variáveis de ambiente:

```bash
cp .env.example .env
```

| Variável             | Descrição                              | Exemplo                                   |
|----------------------|-----------------------------------------|--------------------------------------------|
| `DB_DRIVER`          | Driver do banco                        | `mysql`                                    |
| `DB_HOST`            | Host do MySQL                          | `localhost` (ou `mysql` no Compose)        |
| `DB_PORT`            | Porta do MySQL                         | `3306`                                     |
| `DB_USER`            | Usuário do MySQL                       | `root`                                     |
| `DB_PASSWORD`        | Senha do MySQL                         | `root`                                     |
| `DB_NAME`            | Nome do banco                          | `orders`                                   |
| `WEB_SERVER_PORT`    | Porta do servidor REST                 | `8000`                                     |
| `GRPC_SERVER_PORT`   | Porta do servidor gRPC                 | `50051`                                    |
| `GRAPHQL_SERVER_PORT`| Porta do servidor GraphQL              | `8090`                                     |
| `RABBITMQ_URL`       | URL de conexão AMQP                    | `amqp://guest:guest@localhost:5672/`       |

## Executando

Sobe MySQL, RabbitMQ e a aplicação:

```bash
docker compose up --build
```

As migrations do banco rodam automaticamente ao iniciar a aplicação.

### Rodando localmente sem Docker

1. Suba apenas a infraestrutura:
   ```bash
   docker compose up mysql rabbitmq
   ```
2. Rode a aplicação:
   ```bash
   go run cmd/ordersystem/main.go cmd/ordersystem/wire_gen.go
   ```

## Serviços e portas

| Serviço              | Porta  | Observação                                  |
|-----------------------|--------|-----------------------------------------------|
| REST                  | 8000   | requests prontos em `api/*.http`               |
| gRPC                  | 50051  | `OrderService`, com reflection habilitada      |
| GraphQL               | 8090   | Playground em `http://localhost:8090`          |
| MySQL                 | 3306   | usuário/senha `root`/`root`, banco `orders`    |
| RabbitMQ              | 5672   | AMQP                                           |
| RabbitMQ (management) | 15672  | usuário/senha `guest`/`guest`                  |

## Operações disponíveis

| Operação      | REST          | gRPC                   | GraphQL              |
|----------------|---------------|--------------------------|------------------------|
| Criar pedido   | `POST /order` | `CreateOrder`            | mutation `createOrder` |
| Listar pedidos | `GET /order`  | `ListOrders`             | query `listOrders`     |

## Testando manualmente

### REST

> Os arquivos [`api/create_order.http`](api/create_order.http) e [`api/list_orders.http`](api/list_orders.http) contêm esses mesmos exemplos, prontos pra usar. Abra-os no VS Code com a extensão [REST Client](https://marketplace.visualstudio.com/items?itemName=humao.rest-client) e clique em **"Send Request"**.

Criar pedido:

```http
POST http://localhost:8000/order HTTP/1.1
Content-Type: application/json

{
    "id": "Order 1",
    "price": 100.5,
    "tax": 0.5
}
```

Listar pedidos:

```http
GET http://localhost:8000/order HTTP/1.1
```

### GraphQL

Acesse o playground em `http://localhost:8090` e cole os exemplos abaixo.

Criar pedido:

```graphql
mutation {
  createOrder(input: { id: "Order 1", Price: 100.5, Tax: 0.5 }) {
    id
    Price
    Tax
    FinalPrice
  }
}
```

Listar pedidos:

```graphql
query {
  listOrders {
    id
    Price
    Tax
    FinalPrice
  }
}
```

### gRPC

Com a reflection habilitada, dá pra usar o [`grpcurl`](https://github.com/fullstorydev/grpcurl) direto, sem precisar do `.proto` na mão.

Criar pedido:

```bash
grpcurl -plaintext -d '{"id": "Order 1", "price": 100.5, "tax": 0.5}' localhost:50051 pb.OrderService/CreateOrder
```

Listar pedidos:

```bash
grpcurl -plaintext localhost:50051 pb.OrderService/ListOrders
```

## Geração de código

Ao alterar os arquivos abaixo, regenere o código correspondente:

| Arquivo alterado                                   | Comando                                                                 |
|------------------------------------------------------|--------------------------------------------------------------------------|
| `internal/infra/grpc/protofiles/order.proto`          | `protoc --go_out=. --go-grpc_out=. internal/infra/grpc/protofiles/order.proto` |
| `internal/infra/graph/schema.graphqls`                | `go run github.com/99designs/gqlgen generate`                            |
| `cmd/ordersystem/wire.go`                             | `wire ./cmd/ordersystem`                                                  |

## Testes automatizados

```bash
go test ./...
```
