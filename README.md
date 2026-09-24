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

accepts `POST /order` with a JSON order payload and stores it in a db
(local dev is `./data/db.sqlite3`)

answers CORS preflights for cross-origin GH Pages.

### Run locally with Docker

build:

```sh
docker build -t local/artist-colony-orders:dev --target dev .
```

run:

_set uid/gid, the directory needs owner permissions_

```fish
mkdir -p db

docker run --rm \
  -p 8080:8080 \
  --user "$(id -u):$(id -g)" \
  -v "$PWD/db":/data \
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
sqlite3 db/db.sqlite3
```

```sql
SELECT * FROM orders;

SELECT * FROM orders
INNER JOIN order_items oi
ON o.order_id = oi.order_id;
```

## Building

To create a production version of your app:

```sh
npm run build
```

You can preview the production build with `npm run preview`.
