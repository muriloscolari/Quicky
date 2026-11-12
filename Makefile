BINARY := bin/quicky-control

.PHONY: build start stop restart status logs run test clean

build:
	mkdir -p bin
	go build -o $(BINARY) ./cli

start: build
	./$(BINARY) start

stop: build
	./$(BINARY) stop

restart: build
	./$(BINARY) restart

status: build
	./$(BINARY) status

logs: build
	./$(BINARY) logs -f

run:
	go run ./cli serve

test:
	go test ./...
	go vet ./...

clean:
	rm -rf bin
