BINARY := bin/quicky-control

.PHONY: build start stop restart status logs run test clean install-daemon install-plasmoid install-kde

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

install-daemon: build
	mkdir -p ~/.local/bin ~/.config/systemd/user
	install -m 0755 $(BINARY) ~/.local/bin/quicky-control
	install -m 0644 systemd/quicky-daemon.service ~/.config/systemd/user/quicky-daemon.service
	systemctl --user daemon-reload
	systemctl --user enable quicky-daemon.service
	systemctl --user restart quicky-daemon.service

install-plasmoid:
	@if kpackagetool6 --type Plasma/Applet --show me.muriloscolari.quicky >/dev/null 2>&1; then \
		kpackagetool6 --type Plasma/Applet --upgrade plasmoid; \
	else \
		kpackagetool6 --type Plasma/Applet --install plasmoid; \
	fi

install-kde: install-daemon install-plasmoid
