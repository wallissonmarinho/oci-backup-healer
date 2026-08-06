.PHONY: build test tidy run clean

build:
	go build -o bin/oci-backup-healer cmd/main.go

test:
	go test -v ./...

tidy:
	go mod tidy

clean:
	rm -rf bin/
