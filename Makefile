BINARY := dockercn

# 版本:优先 git tag 描述(如 v0.1.0),无 tag 时回退到 commit;脏工作区带 -dirty。
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

# 静态链接:禁用 CGO,保证产物可运行在 Alpine / scratch 等无 glibc 的环境。
CGO_ENABLED := 0

.PHONY: all build install test vet fmt version release clean

all: build

# 本机构建(静态链接、去符号,注入版本号)
build:
	CGO_ENABLED=$(CGO_ENABLED) go build -trimpath -ldflags "$(LDFLAGS)" -o $(BINARY) .

install:
	CGO_ENABLED=$(CGO_ENABLED) go install -ldflags "$(LDFLAGS)" .

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -l .

version:
	@echo $(BINARY) $(VERSION)

# 跨平台发布:输出到 dist/(覆盖设计文档声明的 Linux/macOS/Windows)
release:
	mkdir -p dist
	CGO_ENABLED=$(CGO_ENABLED) GOOS=linux   GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/$(BINARY)-linux-amd64 .
	CGO_ENABLED=$(CGO_ENABLED) GOOS=linux   GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/$(BINARY)-linux-arm64 .
	CGO_ENABLED=$(CGO_ENABLED) GOOS=darwin  GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/$(BINARY)-darwin-amd64 .
	CGO_ENABLED=$(CGO_ENABLED) GOOS=darwin  GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/$(BINARY)-darwin-arm64 .
	CGO_ENABLED=$(CGO_ENABLED) GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/$(BINARY)-windows-amd64.exe .

clean:
	rm -rf dist $(BINARY)