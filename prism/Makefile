.PHONY: build test test-unit test-py test-e2e fmt vet lint tidy

build:
	go build -o prism ./cmd/prism

test: test-unit test-py

test-unit:
	go test ./...

test-py:
	cd runtime/dlt_runner && uv run --with pytest --with pyyaml --with dlt[filesystem] pytest -v

test-e2e:
	go test -tags=e2e ./...

fmt:
	gofmt -w .

vet:
	go vet ./...

tidy:
	go mod tidy
