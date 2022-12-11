MAKEFLAGS += --silent

all: clean format test build

## help: Prints a list of available build targets.
help:
	echo "Usage: make <OPTIONS> ... <TARGETS>"
	echo ""
	echo "Available targets are:"
	echo ''
	sed -n 's/^##//p' ${PWD}/Makefile | column -t -s ':' | sed -e 's/^/ /'
	echo
	echo "Targets run by default are: `sed -n 's/^all: //p' ./Makefile | sed -e 's/ /, /g' | sed -e 's/\(.*\), /\1, and /'`"

## clean: Removes any previously created build artifacts.
clean:
	rm -f ./k6

## build: Builds a custom 'k6' with the local extension. 
build:
	go install go.k6.io/xk6/cmd/xk6@v0.7.0
	xk6 build --with github.com/radepopovic/xk6-output-nano=.

## format: Applies Go formatting to code.
format:
	go fmt ./...

## test: Executes any unit tests.
test:
	go test -cover -race ./...

.PHONY: build clean format help test