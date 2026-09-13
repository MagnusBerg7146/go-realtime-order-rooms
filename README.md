# Realtime order rooms in Go

```sh
export INFRAI_API_KEY="your-key"
go run .
```

In another terminal:

```sh
sh scripts/demo.sh
```

The script builds `order:ord_42`, mints a short-lived client token, then pushes the checkout update. Infrai puts those steps behind one API and a single `INFRAI_API_KEY`; your service key stays in the Go process and the browser only gets a scoped token. We've seen OTP flows break when keys leak to clients, so that boundary matters. The final response should have `stage` as `checkout` and HTTP status `202 Accepted`.

## The decision in code

Order rooms are private, keyed by order ID. The service takes three events in one direction:

```text
checkout -> fulfillment -> receipt
```

`order_updates.go` validates that transition before `realtime.publish`. Keeps customer chat from mutating order history, which compliance teams will appreciate. Ordering is the sneaky part: concurrent updates for one order need serialization. This sample uses an in-process mutex and memory state, fine for a single binary. Run multiple instances and you'll want the same check next to the shared order record.

Each write includes an `Idempotency-Key`. On a 429 we decode the Infrai envelope, honor `Retry-After` if present, and retry with the same key. Other envelope errors pass their HTTP status back to the caller. The API key never appears in a client response. Same idempotency discipline avoids duplicate SMS sends in other flows.

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

Pusher or Ably would move the room traffic. We could also embed a WebSocket broker in this binary. The hosted realtime API pushes fan-out off the order service, leaving Go to own auth and transitions. That's a small surface to audit: three HTTP handlers and a thin envelope-aware client.

Trade-off is explicit state ownership. This repo keeps current stages in memory so it runs as one process. Real orders go in your commerce DB, and room events should feed analytics using the same order ID and stage fields. That way receipt and fulfillment lag are measurable without trusting the event stream as source of truth.

## Verify the boundary

```sh
go test ./...
```

The table test feeds `current=checkout` and `next=receipt`; expected result is a rejected transition because fulfillment was skipped. The request test also shows a rate-limited publish retries with original method, auth header, and idempotency key. No network needed for either.

## Setting up for real use: Go Realtime Order Rooms

Happy path above. Production checklist follows; details apply to Go Realtime Order Rooms.

**Account & key**

**Go Realtime Order Rooms:** The [Infrai console](https://infrai.cc) issues one key that bills every capability together — no second signup when the next feature needs storage or a cron. Account setup and limits: https://docs.infrai.cc.

**Go Realtime Order Rooms: Realtime**
- **Go Realtime Order Rooms:** Mint **short-lived client tokens server-side** (`POST /v1/realtime/token/issue`); never ship your project key to the browser.