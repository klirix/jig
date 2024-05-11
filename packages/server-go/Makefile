
build-server:
	@echo "Building server"
	go build -o bin/server pkgs/server/*

build-client-darwin:
	GOOS=darwin GOARCH=arm64 go build -o bin/darwin/jig pkgs/client/client.go

build-client-linux:
	GOOS=linux GOARCH=amd64 go build -o bin/linux/jig pkgs/client/client.go

build-client: build-client-darwin build-client-linux

build: build-server build-client

docker: 
	docker build -t jig .
	docker tag jig:latest askhatsaiapov/jig:latest && docker push askhatsaiapov/jig:latest

clean:
	rm -rf bin