.PHONY: deps build run dev test test-integration check clean

deps:
	npm ci --no-audit --no-fund
	go mod download

build:
	npm run build
	go build -trimpath -o bin/timememo ./cmd/timememo
	go build -trimpath -o bin/timememo-user ./cmd/timememo-user

run: build
	./bin/timememo

dev:
	npm run dev

test:
	go test ./...

test-integration:
	@test -n "$$TEST_DATABASE_URL" || (echo "Please set TEST_DATABASE_URL" && exit 1)
	go test -count=1 ./internal/server -run TestIntegration -v

check:
	npm run check
	go vet ./...

clean:
	rm -rf bin
