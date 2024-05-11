
build-server:
	@echo "Building server"
	go build -o bin/server pkgs/server/*

build-client:
	@echo "Building client"
	go build -o bin/server pkgs/client/client.go

build: build-server build-client

docker-image:
	docker build -t jig .

docker-push:
	docker tag jig:latest askhatsaiapov/jig:latest && docker push askhatsaiapov/jig:latest

docker-deploy: docker-image docker-push

clean:
	rm -rf bin