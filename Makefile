.PHONY: build check engine install uninstall netns bench dist
build:
	mkdir -p build
	go build -trimpath -o build/fdpi ./cmd/fdpi
check:
	test -z "$$(gofmt -l cmd internal)"
	go vet ./...
	go test ./...
	go test -race ./...
engine:
	./scripts/build-engine.sh
install:
	./scripts/install.sh
uninstall:
	./scripts/uninstall.sh
netns:
	./scripts/test-netns.sh
bench:
	./scripts/benchmark.sh
dist:
	mkdir -p build
	tar --exclude=./build --exclude=./.git -czf build/fdpi-0.4.0.tar.gz --transform='s,^\.,fdpi-0.4.0,' .
