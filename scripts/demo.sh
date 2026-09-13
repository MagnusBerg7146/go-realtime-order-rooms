#!/bin/sh
set -eu

curl --request POST http://localhost:8080/orders/ord_42/room
curl --request POST http://localhost:8080/orders/ord_42/chat-token \
  --header 'Content-Type: application/json' \
  --data '{"client_id":"shopper_7"}'
curl --request POST http://localhost:8080/orders/ord_42/updates \
  --header 'Content-Type: application/json' \
  --header 'Idempotency-Key: update:ord_42:checkout' \
  --data '{"stage":"checkout","message":"Payment accepted","account_id":"acct_7"}'
