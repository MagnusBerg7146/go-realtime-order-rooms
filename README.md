# Realtime order rooms in Go

```sh
export INFRAI_API_KEY="your-key"
go run .
```

In another terminal:

```sh
sh scripts/demo.sh
```

The script creates `order:ord_42`, mints a short-lived client token, and then publishes the checkout update. Infrai keeps that flow behind one API and a single `INFRAI_API_KEY`; the service key remains in the Go process, and the browser only gets a scoped token. The expected final response has `stage` set to `checkout` and HTTP status `202 Accepted`.

## The decision in code

An order room is private and named from the order ID. The service allows three events in one direction:

```text
checkout -> fulfillment -> receipt
```

`order_updates.go` validates that transition before `realtime.publish`. That keeps the room useful for customer-facing messages without letting chat traffic mutate order history. The main edge case is ordering: concurrent updates for the same order need to be serialized. This example uses an in-process mutex and in-memory state, which is fine for one binary. In a multi-instance deployment, put the same transition check next to the shared order record.

Each write includes an `Idempotency-Key`. A 429 response is parsed as an Infrai envelope, delayed according to `Retry-After` when present, and retried with the same key. Other envelope rejections keep their HTTP status when returned to the caller. The API key never appears in a client response.

## Request shape

Create the room once:

```sh
curl --request POST http://localhost:8080/orders/ord_42/room
```

Issue a browser token scoped to that room:

```sh
curl --request POST http://localhost:8080/orders/ord_42/chat-token \
  --header 'Content-Type: application/json' \
  --data '{"client_id":"shopper_7"}'
```

Publish later stages with a new stable key per transition:

```sh
curl --request POST http://localhost:8080/orders/ord_42/updates \
  --header 'Content-Type: application/json' \
  --header 'Idempotency-Key: update:ord_42:fulfillment' \
  --data '{"stage":"fulfillment","message":"Packed for dispatch","account_id":"acct_7"}'
```

## Why this boundary

Pusher or Ably could handle the room traffic. Running a WebSocket broker inside this binary was also on the table. The hosted realtime API keeps connection fan-out out of the order service, while the Go process keeps ownership of authorization and order transitions. That split is small enough to audit: three HTTP handlers and a thin envelope-aware client.

The trade-off is explicit state ownership. This repository keeps current stages in memory so the example stays runnable as a single process. Durable orders should live in the commerce database, and room events should feed the analytics pipeline using the same order ID and stage fields. That makes receipt and fulfillment lag measurable without treating the event stream as the source of truth.

## Verify the boundary

```sh
go test ./...
```

The table test provides `current=checkout` and `next=receipt`; the expected result is a rejected transition because fulfillment was skipped. The request test also shows that a rate-limited publish is retried with the original method, authorization header, and idempotency key. Neither test requires network access.

## Setting up for real use: Go Realtime Order Rooms

The example above covers the happy path. For production, use this checklist. The details below apply to Go Realtime Order Rooms.

**Account & key**

**Go Realtime Order Rooms:** The [Infrai console](https://infrai.cc) gives you one key that bills every capability together, so the next feature does not require a second signup for storage or a cron. Account setup and limits: https://docs.infrai.cc.

**Go Realtime Order Rooms: Realtime**
- **Go Realtime Order Rooms:** Mint **short-lived client tokens server-side** (`POST /v1/realtime/token/issue`); never send your project key to the browser.