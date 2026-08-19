BINARY := dockercn

.PHONY: all build install test vet fmt release clean

all: build

# 本机构建(静态链接、去符号,减小体积)
build:
	go build -trimpath -ldflags "-s -w" -o $(BINARY) .

install:
	go install .

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -l .

# 跨平台发布:输出到 dist/(覆盖设计文档声明的 Linux/macOS/Windows)
release:
	mkdir -p dist
	GOOS=linux   GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o dist/$(BINARY)-linux-amd64 .
	GOOS=linux   GOARCH=arm64 go build -trimpath -ldflags "-s -w" -o dist/$(BINARY)-linux-arm64 .
	GOOS=darwin  GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o dist/$(BINARY)-darwin-amd64 .
	GOOS=darwin  GOARCH=arm64 go build -trimpath -ldflags "-s -w" -o dist/$(BINARY)-darwin-arm64 .
	GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o dist/$(BINARY)-windows-amd64.exe .

clean:
	rm -rf dist $(BINARY)