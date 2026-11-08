.PHONY: build check engine engine-windows install uninstall netns bench dist dist-windows
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
engine-windows:
	./scripts/build-engine-windows.sh
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
dist-windows: engine-windows
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o build/windows/fdpi.exe ./cmd/fdpi
	cp third_party/windivert/x64/WinDivert64.sys build/windows/
	cp configs/default.json build/windows/config.json
	rm -f build/fdpi-windows-amd64.zip
	cd build/windows && { command -v zip >/dev/null && zip -qr ../fdpi-windows-amd64.zip fdpi.exe winws2.exe WinDivert.dll WinDivert64.sys config.json lua ZAPRET-LICENSE engine.commit ../../README_WINDOWS.md || 7z a -tzip -r ../fdpi-windows-amd64.zip fdpi.exe winws2.exe WinDivert.dll WinDivert64.sys config.json lua ZAPRET-LICENSE engine.commit ../../README_WINDOWS.md >/dev/null; }
	@echo "Готово: build/fdpi-windows-amd64.zip"
