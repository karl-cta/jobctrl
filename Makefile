.PHONY: install dev build run clean

install:
	cd frontend && npm install

dev:
	@make -j2 dev-backend dev-frontend

dev-backend:
	JOB_CTRL_DB_PATH=./job-ctrl-dev.db go run .

dev-frontend:
	cd frontend && npm run dev

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

build:
	cd frontend && npm run build
	go build -ldflags="-s -w -X main.Version=$(VERSION)" -o job-ctrl .

run: build
	./job-ctrl

clean:
	rm -f job-ctrl job-ctrl-dev.db
	rm -rf frontend/dist/assets
