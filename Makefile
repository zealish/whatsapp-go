BINARY  := zealish-wa
PREFIX  ?= /usr
APPID   := com.zealish.WhatsApp
LDFLAGS := -s -w

.PHONY: all build run fmt vet lint test clean install uninstall

all: build

build:
	go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/app

run: build
	./bin/$(BINARY)

fmt:
	gofmt -w .

vet:
	go vet ./...

lint:
	golangci-lint run

test:
	go test ./...

clean:
	rm -rf bin

install: build
	install -Dm0755 bin/$(BINARY) $(DESTDIR)$(PREFIX)/bin/$(BINARY)
	install -Dm0644 assets/$(APPID).desktop $(DESTDIR)$(PREFIX)/share/applications/$(APPID).desktop
	install -Dm0644 assets/icon.svg $(DESTDIR)$(PREFIX)/share/icons/hicolor/scalable/apps/$(APPID).svg
	@command -v google-chrome >/dev/null || command -v chromium >/dev/null || { echo "error: Google Chrome or Chromium is required at runtime" >&2; exit 1; }

uninstall:
	rm -f $(DESTDIR)$(PREFIX)/bin/$(BINARY)
	rm -f $(DESTDIR)$(PREFIX)/share/applications/$(APPID).desktop
	rm -f $(DESTDIR)$(PREFIX)/share/icons/hicolor/scalable/apps/$(APPID).svg
