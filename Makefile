# The window is a Wails v3 app with a React frontend (frontend/). The frontend
# is built first and embedded into the Go binary.
# See packaging/*.sh and .github/workflows/release.yml.

# Linux uses GTK3 + WebKit2GTK 4.1; other platforms ignore this tag.
TAGS := gtk3

.PHONY: dev frontend bindings build test lint linux windows macos clean

# Live-reloading UI: Vite dev server + the Go app pointed at it. The Vite
# server stops when the app exits. (frontend/dist must exist to compile, so
# the frontend is built once first.)
dev: frontend
	cd frontend && { node node_modules/vite/bin/vite.js --host 127.0.0.1 --port 34115 --strictPort & VITE=$$!; \
		trap 'kill $$VITE' EXIT; sleep 2; \
		cd .. && FRONTEND_DEVSERVER_URL=http://127.0.0.1:34115 go run -tags $(TAGS) .; }

frontend:
	packaging/build-frontend.sh

# Regenerate the TypeScript bindings after changing app.go.
bindings:
	CGO_ENABLED=0 wails3 generate bindings -ts -d frontend/src/bindings

build: frontend
	go build -tags $(TAGS),production -o dist/sendgrid-template-migration .

test: frontend
	go test -race -tags $(TAGS) ./...

lint: frontend
	golangci-lint run --build-tags $(TAGS) ./...
	cd frontend && npm run lint

linux:
	packaging/build-linux.sh

windows:
	packaging/build-windows.sh

macos:
	packaging/build-macos.sh

clean:
	rm -rf dist frontend/dist rsrc_windows_*.syso
