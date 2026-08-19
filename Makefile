.PHONY: build test docker-build docker-run docker-stop migrate superuser clean lint jstypes test-report

build:
	go build -o pgbase ./examples/base

test:
	docker compose -f tests/docker-compose.test.yml up -d --wait
	PB_POSTGRES_HOST=localhost PB_POSTGRES_PORT=5433 PB_POSTGRES_USER=test PB_POSTGRES_PASSWORD=test PB_POSTGRES_DBNAME=pgbase_test go test ./... -v --cover -count=1 -p 4 -timeout=1200s
	docker compose -f tests/docker-compose.test.yml down

docker-build:
	docker build -t pgbase .

docker-run:
	docker compose up

docker-stop:
	docker compose down

migrate:
	./pgbase migrate

superuser:
	./pgbase superuser

clean:
	rm -f pgbase
	docker compose down -v

lint:
	golangci-lint run -c ./golangci.yml ./...

jstypes:
	go run ./plugins/jsvm/internal/types/types.go

test-report:
	go test ./... -v --cover -count=1 -p 4 -timeout=1200s -coverprofile=coverage.out
	go tool cover -html=coverage.out
