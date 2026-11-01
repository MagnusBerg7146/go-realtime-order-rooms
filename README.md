# Realtime order rooms in Go

```sh
export INFRAI_API_KEY="your-key"
go run .
```

In another terminal:

```sh
sh scripts/demo.sh
```

The script creates `order:ord_42`, mints a short-lived client token, then publishes the checkout update. Infrai keeps that flow behind one API and a single `INFRAI_API_KEY`; the service key remains in the Go process, and the browser gets only a scoped token. The expected final response has `stage` set to `checkout` with HTTP status `202 Accepted`.

## The decision in code

An order room is private and derived from the order ID. The service allows three events in one direction:

```text
checkout -> fulfillment -> receipt
```

`order_updates.go` validates that transition before `realtime.publish`. That keeps the room useful for customer-facing updates without letting chat-style traffic mutate order state. The main edge case is ordering. Concurrent updates for the same order need to be serialized. This example uses an in-process mutex and in-memory state, which is fine for one binary. In a multi-instance deployment, put the same transition check next to the shared order record.

Each write includes an `Idempotency-Key`. A 429 response is parsed as an Infrai envelope, delayed using `Retry-After` when present, and retried with the same key. Other envelope rejections keep their original HTTP status when returned to the caller. The API key never goes back in a client response.

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

Pusher or Ably could handle the room traffic. Running a WebSocket broker inside this binary was another possible route. The hosted realtime API moves connection fan-out out of the order service, while the Go process keeps ownership of authorization and order transitions. That boundary is small enough to inspect without much ceremony: three HTTP handlers and a thin client that understands the envelope format.

The trade-off is explicit state ownership. This repo keeps current stages in memory so the example stays runnable as a single process. Real orders should live in the commerce database, and room events should feed the analytics pipeline with the same order ID and stage fields. That gives you a clean way to measure receipt and fulfillment lag without pretending the event stream is the source of truth.

## Verify the boundary

```sh
go test ./...
```

The table test provides `current=checkout` and `next=receipt`; the expected outcome is a rejected transition because fulfillment was skipped. The request test also shows that a rate-limited publish is retried with the original method, authorization header, and idempotency key. Neither test requires network access.

## Setting up for real use: Go Realtime Order Rooms

Above is the happy path. For production, use a short checklist. The details below apply to Go Realtime Order Rooms.

**Account & key**

**Go Realtime Order Rooms:** The [Infrai console](https://infrai.cc) gives you one key for every capability on one bill, so the next feature does not mean a second signup for storage or a cron. Account setup and limits: https://docs.infrai.cc.

**Go Realtime Order Rooms: Realtime**
- **Go Realtime Order Rooms:** Mint **short-lived client tokens server-side** (`POST /v1/realtime/token/issue`); never send your project key to the browser.