# Artist-Colony-Price-Sheet

Statically generated online price sheet
And Online Order Form

- frontend is statically generated to be hosted on GitHub Pages
- orders are submitted to an order api at `https://artist-colony.netrunner-meetup.com`

## Client Development

Install dependencies

```sh
pnpm i
# or
npx pnpm i
```

copy `.env.example` to `.env` 
set `PUBLIC_ORDER_ENDPOINT` to the order server base URL
(defaults to `https://artist-colony.netrunner-meetup.com` if empty)

start a development server:

```sh
npm run dev

# or start the server and open the app in a new browser tab
npm run dev -- --open
```

## Order server

accepts `POST /order` with a JSON order payload and stores it in Postgres
(local dev is `./data/db.sqlite3`)

answers CORS preflights for cross-origin GH Pages.

### Run locally with Docker

Run a local postgres container:

```fish
set -x POSTGRES_PASSWORD (head -c 64 /dev/urandom | base64 | tr -dc 'a-zA-Z0-9')
docker run --rm --name ac-dev-db \
  -e POSTGRES_PASSWORD=$POSTGRES_PASSWORD \
  -p 5432:5432 \
  -d postgres:17

docker exec ac-dev-db \
  psql -U postgres -c 'CREATE DATABASE "artist-colony-orders";'
```

build:

```sh
docker build -t local/artist-colony-orders:dev .
```

run:

_set uid/gid, the directory needs owner permissions_

```fish
docker run --rm \
  --network host \
  -e DB_CONNECTION_STRING='postgres://postgres:postgres@localhost:5432/artist-colony-orders?sslmode=disable' \
  -e ALLOWED_ORIGIN=http://localhost:5173 \
  local/artist-colony-orders:dev
```

- API endpoint: `http://localhost:8080/order`
- Health check: `http://localhost:8080/healthz`
- DB file: `/data/db.sqlite3` mounted in the container

set frontend `.env` to use this local server

```sh
PUBLIC_ORDER_ENDPOINT=http://localhost:8080
```

and restart `npm run dev` if it's still running


### Local DB Inspection

```sh
docker exec -it ac-dev-db psql -U postgres -d artist-colony-orders
```

```sql
SELECT * FROM orders;

SELECT o.order_id, o.name, oi.title, oi.quantity
FROM orders o
INNER JOIN order_items oi
ON oi.order_id = o.id;
```


### Manual Build and Deploy Server

```sh
docker buildx build \
    -t registry.digitalocean.com/nsgcr/artist-colony-orders-api:v0.1.0 \
    --push .
```

## Building Client

To create a production version of your app:

```sh
npm run build
```

You can preview the production build with `npm run preview`.
