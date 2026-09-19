# Submission checklist

## Deliverables

- [x] Go/Gin middleware, PostgreSQL schema, Docker Compose and Nginx configuration.
- [x] HTTP HIS adapter and synthetic two-hospital mock.
- [x] Staff creation, login and authenticated patient search with eight optional filters.
- [x] Positive and negative API, service, adapter and database test cases.
- [x] [Development plan](DEVELOPMENT_PLAN.md), [OpenAPI](openapi.yaml) and [editable draw.io ERD](diagrams/hospital-erd.drawio).
- [x] Final verification results recorded below.
- [x] Private GitHub repository created and uploaded.
- [x] Planning DOCX imported as native Google Docs in the ChatGPT folder.

## Reviewer demonstration

```sh
make setup
make up
make test
make smoke
```

See the README for manual create, login, token retrieval and patient searches. The mock contains synthetic records only. Hospital A and B share an identifier so the demonstration can verify hospital isolation.

## Verification evidence

Verified on 19 September 2026 after the final dependency updates (pgx 5.9.2, quic-go 0.59.1 and x/text 0.39.0):

| Command or check | Result |
| --- | --- |
| `docker compose up --build -d` | Passed; all four services started. |
| Go vet and race-enabled tests with internal-package coverage | Passed, including live PostgreSQL integration tests without skips; total internal-package statement coverage 90.8%. |
| `python3 scripts/smoke.py` | Passed; all 47 checks through Nginx. |
| Native draw.io import | Verified tables, fields and relationship connectors in the real application; saved as Agnos Hospital ER Diagram.drawio in browser storage. The repository .drawio file is the distributable artifact. |
| `govulncheck` v1.8.0 with Go 1.26 | Exit 0; zero vulnerabilities detected in called code. Two additional imported-package advisories and 24 additional module advisories concern code not called by this application. This does not mean every dependency is vulnerability-free. |
| Planning DOCX | Title sanitizer passed; eight pages rendered with bundled LibreOffice and visually reviewed. |
| Native Google Docs | MIME type and owner-only access verified; native text, tables and inline ERD read back. All eight pages of the Google Docs PDF export were visually reviewed without clipping, overflow or missing content. |
| GitHub Actions | [Verification workflow](https://github.com/SaltzSnow/agnos-hospital-middleware/actions/workflows/ci.yml) passed Go vet, race-enabled tests with PostgreSQL, stack startup and smoke checks through Nginx. |

To reproduce the coverage run after `make up`:

```sh
docker compose --profile test run --build --rm test sh -c 'go vet ./... && go test -race -count=1 -coverpkg=./internal/... -coverprofile=/tmp/coverage.out ./... && go tool cover -func=/tmp/coverage.out'
```

The test service supplies `TEST_DATABASE_URL` for the live PostgreSQL integration tests. Coverage is for `./internal/...`; it is not an application-wide or end-to-end coverage claim.

The ERD preview PNG is rasterized from the same generated SVG; it is not a native draw.io export. No real HIS integration result is claimed.

## External handoff

GitHub: [SaltzSnow/agnos-hospital-middleware](https://github.com/SaltzSnow/agnos-hospital-middleware) — private repository.

Google Docs: [Agnos Hospital Middleware Development Plan](https://docs.google.com/document/d/1_UEMZg3q8FciP0s97UB8Zc5bWVGEFgtfui-ebygGcKo) — native document in the ChatGPT folder, with owner-only access at handoff.

The implemented HTTP adapter has been prepared against the assignment response shape and a mock, not verified against a real HIS. Real credentials, hospital B's actual schema, error formats and TLS requirements remain integration prerequisites. Before sending the assignment links, grant the intended reviewers access to the private repository and Google Doc.
