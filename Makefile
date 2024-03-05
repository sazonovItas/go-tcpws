.PHONY: test coverage

test:
	go test -race -v ./...

coverage:
	go test -coverprofile=c.out ./...;\
	go tool cover -func=c.out;\
	rm c.out
