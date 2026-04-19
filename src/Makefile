APP=virtual-printer
VERSION=1.0.0

.PHONY: all linux windows clean run

all: linux windows

linux:
	GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o bin/$(APP)-linux-amd64 .
	@echo "Gerado: bin/$(APP)-linux-amd64"

linux-arm:
	GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o bin/$(APP)-linux-arm64 .

windows:
	GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o bin/$(APP)-windows-amd64.exe .
	@echo "Gerado: bin/$(APP)-windows-amd64.exe"

run:
	go run . -port 631 -web 8080 -width 80 -name "Virtual-Thermal-Printer" -output ./jobs

run-58:
	go run . -port 631 -web 8080 -width 58 -name "Virtual-Printer-58mm" -output ./jobs

run-110:
	go run . -port 631 -web 8080 -width 110 -name "Virtual-Printer-110mm" -output ./jobs

clean:
	rm -rf bin/ jobs/
