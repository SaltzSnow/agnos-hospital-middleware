.PHONY: setup up test smoke down
setup:
	sh scripts/bootstrap.sh
up: setup
	docker compose up --build -d
test: setup
	docker compose --profile test run --build --rm test
smoke:
	python3 scripts/smoke.py
down:
	docker compose down
