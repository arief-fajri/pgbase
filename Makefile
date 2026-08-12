.PHONY: build test docker-build docker-run migrate superuser clean

build:
	go build -o pgbase .

test:
	docker-compose -f tests/docker-compose.test.yml up -d
	sleep 3
	PGTEST_HOST=localhost PGTEST_PORT=5433 go test ./... -v --cover
	docker-compose -f tests/docker-compose.test.yml down

docker-build:
	docker build -t pgbase .

docker-run:
	docker-compose up

docker-stop:
	docker-compose down

migrate:
	./pgbase migrate

superuser:
	./pgbase superuser

clean:
	rm -f pgbase
	docker-compose down -v

lint:
	golangci-lint run -c ./golangci.yml ./...

jstypes:
	go run ./plugins/jsvm/internal/types/types.go

test-report:
	go test ./... -v --cover -coverprofile=coverage.out
	go tool cover -html=coverage.out
