BINARY  = proxy-server
VERSION = 1.0.0
OUTDIR  = dist

.PHONY: all clean deps darwin-arm64 darwin-amd64 linux-amd64 win-amd64

all: deps darwin-arm64 darwin-amd64 linux-amd64 win-amd64

deps:
	go mod download
	go mod verify

darwin-arm64:
	GOOS=darwin  GOARCH=arm64  go build -ldflags="-s -w" -o $(OUTDIR)/$(BINARY)-darwin-arm64  .

darwin-amd64:
	GOOS=darwin  GOARCH=amd64  go build -ldflags="-s -w" -o $(OUTDIR)/$(BINARY)-darwin-amd64  .

linux-amd64:
	GOOS=linux   GOARCH=amd64  go build -ldflags="-s -w" -o $(OUTDIR)/$(BINARY)-linux-amd64   .

win-amd64:
	GOOS=windows GOARCH=amd64  go build -ldflags="-s -w" -o $(OUTDIR)/$(BINARY)-win-amd64.exe .

clean:
	rm -rf $(OUTDIR)

dev:
	go run .
