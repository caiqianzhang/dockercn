package main

import (
	"context"
	"errors"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestMapArch(t *testing.T) {
	cases := map[string]string{
		"x86_64":      "linux/amd64",
		"amd64":       "linux/amd64",
		"aarch64":     "linux/arm64",
		"armv7l":      "linux/arm",
		"386":         "linux/386",
		"ppc64":       "linux/ppc64",
		"ppc64le":     "linux/ppc64le",
		"loongarch64": "linux/loong64",
		"mips":        "linux/mips",
		"mipsle":      "linux/mipsle",
		"mips64":      "linux/mips64",
		"mips64le":    "linux/mips64le",
		"weird":       "",
	}
	for in, want := range cases {
		if got := mapArch(in); got != want {
			t.Errorf("mapArch(%q) = %q,期望 %q", in, got, want)
		}
	}
}

func TestRenameImageSkipsSameName(t *testing.T) {
	old := execCommand
	var calls []string
	execCommand = fakeExec(&calls)
	defer func() { execCommand = old }()

	if err := renameImage(context.Background(), "node:1", "node:1"); err != nil {
		t.Fatalf("src==dst 应直接成功,实际错误: %v", err)
	}
	if len(calls) != 0 {
		t.Fatalf("src==dst 时不应执行任何 docker 命令(否则 rmi 会误删镜像),实际: %v", calls)
	}
}

func TestRenameImageTagThenRemove(t *testing.T) {
	old := execCommand
	var calls []string
	execCommand = fakeExec(&calls)
	defer func() { execCommand = old }()

	if err := renameImage(context.Background(), "mirror:1", "node:1"); err != nil {
		t.Fatalf("renameImage 错误: %v", err)
	}
	want := []string{"docker tag mirror:1 node:1", "docker rmi mirror:1"}
	if !slices.Equal(calls, want) {
		t.Fatalf("命令序列不符: %v,期望 %v", calls, want)
	}
}

func TestRenameImageRemoveFailStillOK(t *testing.T) {
	old := execCommand
	execCommand = failingExec("rmi")
	defer func() { execCommand = old }()

	// rmi 失败时已加好别名,应视为成功(仅警告),不返回错误。
	if err := renameImage(context.Background(), "mirror:1", "node:1"); err != nil {
		t.Fatalf("rmi 失败应返回 nil(仅警告),实际: %v", err)
	}
}

func TestCheckDocker(t *testing.T) {
	old := runCapture
	runCapture = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return []byte("29.6.2"), nil
	}
	defer func() { runCapture = old }()

	if err := CheckDocker(context.Background()); err != nil {
		t.Fatalf("版本可查时应返回 nil: %v", err)
	}
}

func TestCheckDockerUnavailableWithMessage(t *testing.T) {
	old := runCapture
	runCapture = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return []byte("Cannot connect to the Docker daemon"), errors.New("exit status 1")
	}
	defer func() { runCapture = old }()

	err := CheckDocker(context.Background())
	if err == nil || !strings.Contains(err.Error(), "Cannot connect to the Docker daemon") {
		t.Fatalf("期望错误包含 stderr 信息,实际: %v", err)
	}
}

func TestDetectArchFromDockerInfo(t *testing.T) {
	old := runCapture
	runCapture = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		if name == "docker" {
			return []byte("x86_64"), nil
		}
		return nil, errors.New("不应回退到 uname")
	}
	defer func() { runCapture = old }()

	arch, err := DetectArch(context.Background())
	if err != nil || arch != "linux/amd64" {
		t.Fatalf("期望 linux/amd64,实际 %q err=%v", arch, err)
	}
}

func TestDetectArchFallsBackToUname(t *testing.T) {
	old := runCapture
	runCapture = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		if name == "docker" {
			return nil, errors.New("docker info 失败")
		}
		return []byte("aarch64"), nil
	}
	defer func() { runCapture = old }()

	arch, err := DetectArch(context.Background())
	if err != nil || arch != "linux/arm64" {
		t.Fatalf("期望回退 uname 得 linux/arm64,实际 %q err=%v", arch, err)
	}
}

// TestDetectArchFallsBackToRuntime 验证 docker info 与 uname 都失败/不可用时,
// 回退到 Go 运行时已知的本机架构(Windows 无 uname 也能工作)。
func TestDetectArchFallsBackToRuntime(t *testing.T) {
	old := runCapture
	runCapture = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return nil, errors.New("所有探测失败")
	}
	defer func() { runCapture = old }()

	arch, err := DetectArch(context.Background())
	if err != nil {
		t.Fatalf("应回退 runtime.GOARCH,实际错误: %v", err)
	}
	if want := "linux/" + runtime.GOARCH; arch != want {
		t.Fatalf("期望回退到 %s,实际 %s", want, arch)
	}
}
