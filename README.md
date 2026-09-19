# Agnos hospital middleware

Go/Gin API with PostgreSQL persistence, an HTTP hospital-system adapter, Nginx and Docker Compose. Staff can search patients only within their registered hospital. The demo uses synthetic patients in hospitals A and B, including overlapping identifiers to exercise isolation.

The implementation follows Approach C: identifier searches call the configured HIS over HTTP and persist validated results; other searches query local PostgreSQL records. The mock demonstrates the assumed response shape. Real HIS credentials and operational contracts have not been verified.

## Run locally

Requires Docker with Compose, Make, OpenSSL and Python 3 for the smoke script.

```sh
make setup
make up
make test
make smoke
```

`make setup` creates a private, ignored `.env` with random database, JWT and registration credentials. Existing configuration is preserved. `make up` also runs setup. Nginx is exposed at `http://localhost:8080` on loopback only; PostgreSQL, API and mock HIS have no host ports. Change `HTTP_PORT` in `.env` if needed. Initial migration and synthetic seed run when the database volume is first created.

`make test` runs Go vet and race-enabled tests in the test container, including PostgreSQL integration tests. `make smoke` exercises the live stack through Nginx. `make down` stops services and preserves data. Do not remove the database volume unless you intend to erase local records.

## Try the API

Staff creation requires the deployment's `X-Registration-Key`, separate from a staff bearer token. Run from the repository root after startup. These examples use a disposable demonstration password; tokens and keys are read into variables rather than printed.

```sh
set -a
. ./.env
set +a
BASE_URL="http://localhost:${HTTP_PORT:-8080}"

curl --fail-with-body -sS "$BASE_URL/staff/create" \
  -H 'Content-Type: application/json' \
  -H "X-Registration-Key: $REGISTRATION_KEY" \
  -d '{"username":"reviewer","password":"DemoPass123!","hospital":"A"}'

TOKEN=$(curl --fail-with-body -sS "$BASE_URL/staff/login" \
  -H 'Content-Type: application/json' \
  -d '{"username":"reviewer","password":"DemoPass123!","hospital":"A"}' \
  | python3 -c 'import json,sys; print(json.load(sys.stdin)["access_token"])')

curl --fail-with-body -sS "$BASE_URL/patient/search" \
  -H 'Content-Type: application/json' -H "Authorization: Bearer $TOKEN" \
  -d '{"national_id":"0000000000001"}'

curl --fail-with-body -sS "$BASE_URL/patient/search" \
  -H 'Content-Type: application/json' -H "Authorization: Bearer $TOKEN" \
  -d '{"first_name":"Somchai","page":1,"page_size":20}'
unset TOKEN REGISTRATION_KEY JWT_SECRET POSTGRES_PASSWORD
```

Creating the same username in hospital A again returns 409; login still works with the original credentials. The same username can exist in hospital B. Never commit `.env` or use demonstration passwords in a real deployment.

## API behavior

| Endpoint | Authentication | Success |
| --- | --- | --- |
| `POST /staff/create` | Provisioning key | 201 with `id`, `username` |
| `POST /staff/login` | Username, password, hospital body | 200 with bearer token and 3600-second lifetime |
| `POST /patient/search` | Staff bearer token | 200 with `data`, `total`, `page`, `page_size`, `source` |
| `GET /healthz` | None | Process liveness only |
| `GET /readyz` | None | Database readiness; 200 ready or 503 unavailable |

Eight optional search fields are `national_id`, `passport_id`, `first_name`, `middle_name`, `last_name`, `date_of_birth`, `phone_number`, and `email`. Filters combine with AND. Names match case-insensitive literal substrings in the corresponding Thai or English field. Other filters match exactly after trimming. Dates must be real `YYYY-MM-DD` dates. Empty, null, unknown or duplicate fields are rejected. Hospital selection in search is rejected; it comes from the authenticated staff record.

`{}` lists locally stored patients in that hospital, with default page 1 and page size 20 (maximum 100). It does not list all patients in a real HIS. An identifier always triggers HIS lookup; national ID takes priority when both identifiers are supplied. The returned patient must match that identifier. Remaining filters apply only to the fetched record. A HIS 404 gives an empty success; failures return 502/503/504 without stale fallback. `source` identifies `local` or `his`; `total` counts matches before pagination.

## Review the delivery

- [Development plan](docs/DEVELOPMENT_PLAN.md) — structure, decisions, flow, schema and test strategy.
- [OpenAPI contract](docs/openapi.yaml) — exact request and response schema, authentication and errors (JSON syntax, valid YAML 1.2).
- [Editable draw.io ERD](docs/diagrams/hospital-erd.drawio) and [SVG preview](docs/diagrams/hospital-erd.svg).
- [Google Docs development plan](https://docs.google.com/document/d/1_UEMZg3q8FciP0s97UB8Zc5bWVGEFgtfui-ebygGcKo) and [downloadable DOCX](output/agnos-development-plan.docx).
- [Submission checklist](docs/SUBMISSION.md) — verification evidence and delivery links.

## Configuration and limits

Compose configures `DATABASE_URL`, `JWT_SECRET`, `REGISTRATION_KEY`, `PORT`, `HIS_A_BASE_URL`, `HIS_B_BASE_URL`, and `HIS_TIMEOUT` (3 seconds). Real integration requires confirming each hospital's endpoint, authentication, response and error formats, and TLS requirements. The adapter currently expects a single patient JSON object from `GET /patient/search/{id}`, allows no redirects, and limits responses to 1 MiB.

Passwords use bcrypt. SQL is parameterized; patient queries and HN uniqueness are hospital-scoped. JWT requests re-read staff membership. The provisioning key can create staff in either configured hospital. Nginx limits `/staff/create` and `/staff/login` to 10 requests per second per client IP, with burst 20; excess requests return 429. There is no user administration UI, synchronization worker, refresh token, public TLS setup or production deployment in this assignment.
