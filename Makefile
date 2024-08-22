
build-server:
	@echo "Building server"
	go build -o bin/server pkgs/server/*

build-client:
	go build -o bin/jig pkgs/client/*

build: build-server build-client

docker: 
	docker build -t jig .
	docker tag jig:latest askhatsaiapov/jig:latest && docker push askhatsaiapov/jig:latest

clean:
	rm -rf bin