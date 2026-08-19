package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// execCommand 与 runCapture 可注入,便于测试模拟 docker 子进程。
var (
	execCommand = exec.CommandContext
	runCapture  = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return exec.CommandContext(ctx, name, args...).CombinedOutput()
	}
)

// runDocker 执行 docker 子进程并透传 stdio。
func runDocker(ctx context.Context, args ...string) error {
	cmd := execCommand(ctx, "docker", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// CheckDocker 校验 docker 客户端与 daemon 可用。
func CheckDocker(ctx context.Context) error {
	out, err := runCapture(ctx, "docker", "version", "--format", "{{.Server.Version}}")
	if err != nil {
		if msg := strings.TrimSpace(string(out)); msg != "" {
			return fmt.Errorf("docker 不可用: %s", msg)
		}
		return fmt.Errorf("docker 不可用,请确认已安装且 daemon 已启动")
	}
	return nil
}

// DetectArch 检测本机架构,优先 docker info,回退 uname -m。
func DetectArch(ctx context.Context) (string, error) {
	if out, err := runCapture(ctx, "docker", "info", "--format", "{{.Architecture}}"); err == nil {
		if arch := mapArch(strings.TrimSpace(string(out))); arch != "" {
			return arch, nil
		}
	}
	out, err := runCapture(ctx, "uname", "-m")
	if err != nil {
		return "", fmt.Errorf("无法检测本机架构: %w", err)
	}
	arch := mapArch(strings.TrimSpace(string(out)))
	if arch == "" {
		return "", fmt.Errorf("无法识别架构 %q", strings.TrimSpace(string(out)))
	}
	return arch, nil
}

// mapArch 把 docker info / uname 的架构名映射为平台名(linux/<arch>)。
func mapArch(raw string) string {
	switch strings.ToLower(raw) {
	case "amd64", "x86_64":
		return "linux/amd64"
	case "arm64", "aarch64":
		return "linux/arm64"
	case "arm", "armv7l", "armv7":
		return "linux/arm"
	case "ppc64le":
		return "linux/ppc64le"
	case "s390x":
		return "linux/s390x"
	case "riscv64":
		return "linux/riscv64"
	case "loong64", "loongarch64":
		return "linux/loong64"
	default:
		return ""
	}
}

// PullImage 拉取镜像;platform 非空时传给 docker。
func PullImage(ctx context.Context, image, platform string) error {
	args := []string{"pull"}
	if platform != "" {
		args = append(args, "--platform="+platform)
	}
	args = append(args, image)
	return runDocker(ctx, args...)
}

// TagImage 给镜像添加别名。
func TagImage(ctx context.Context, src, dst string) error {
	return runDocker(ctx, "tag", src, dst)
}

// RemoveTag 删除某个 tag(只删标签,不删镜像数据)。
func RemoveTag(ctx context.Context, tag string) error {
	return runDocker(ctx, "rmi", tag)
}
