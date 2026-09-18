.PHONY: run fmt compose-up compose-down

run:
	go run .

fmt:
	gofmt -w .

compose-up:
	docker compose up --build

compose-down:
	docker compose down
