#!/bin/bash

docker run -d --rm \
  --name twitch-chat-log-pg \
  -e POSTGRES_PASSWORD=123456 \
  -e PGDATA=/var/lib/postgresql/data/pgdata \
  -v "$(pwd)/data/postgres:/var/lib/postgresql/data" \
  -p 45433:5432 \
  postgres:17
