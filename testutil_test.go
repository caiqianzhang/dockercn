package main

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestHelperProcess 是 successCmd 的子进程替身:让测试二进制自身充当模拟命令,
// 避免依赖 /bin/true(Windows 上不存在),保证 go test 在任意平台可跑。
func TestHelperProcess(_ *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	os.Exit(0) // 直接退出,框架不再打印输出
}

// successCmd 构造一个以退出码 0 结束的模拟子进程(测试二进制自身)。
func successCmd(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=TestHelperProcess")
	cmd.Args = append(cmd.Args, "--", name)
	cmd.Args = append(cmd.Args, args...)
	cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")
	return cmd
}

// fakeExec 记录发出的命令,并让每个模拟子进程成功退出(跨平台)。
func fakeExec(calls *[]string) func(context.Context, string, ...string) *exec.Cmd {
	return func(ctx context.Context, name string, args ...string) *exec.Cmd {
		*calls = append(*calls, name+" "+strings.Join(args, " "))
		return successCmd(ctx, name, args...)
	}
}

// failingExec 对前缀匹配的命令返回一个必定启动失败的子进程,模拟 docker 报错。
func failingExec(match string) func(context.Context, string, ...string) *exec.Cmd {
	return func(ctx context.Context, name string, args ...string) *exec.Cmd {
		if strings.HasPrefix(strings.Join(args, " "), match) {
			return &exec.Cmd{
				Path: "dockercn-nonexistent-helper",
				Args: append([]string{name}, args...),
			}
		}
		return successCmd(ctx, name, args...)
	}
}
