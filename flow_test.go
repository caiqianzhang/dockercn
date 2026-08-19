package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

// flowEnv 汇集端到端测试所需的全部注入缝:API、docker 子进程、stdin、stdout/stderr。
type flowEnv struct {
	srv       *httptest.Server
	execCalls []string
	out       bytes.Buffer // uiWriter 的目标(全部正常输出)
	err       bytes.Buffer // uiErrWriter 的目标(全部错误输出)
}

// resultsJSON 把一组结果序列化为 API 响应 JSON。
func resultsJSON(count int, results []Result) string {
	b, _ := json.Marshal(SearchResponse{Count: count, Results: results})
	return string(b)
}

// setupFlow 注入全部测试缝;handler 负责返回 API 响应。
func setupFlow(t *testing.T, handler http.HandlerFunc) *flowEnv {
	t.Helper()
	e := &flowEnv{}
	e.srv = httptest.NewServer(handler)

	oldAPI := apiBase
	apiBase = e.srv.URL + "/api/v1"

	oldExec, oldCapture := execCommand, runCapture
	execCommand = fakeExec(&e.execCalls)
	// docker version(CheckDocker)与 docker info(DetectArch)都当成功;
	// 返回 x86_64 使 DetectArch 得到 linux/amd64。
	runCapture = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return []byte("x86_64"), nil
	}

	oldUIr, oldUIw, oldUIe := uiReader, uiWriter, uiErrWriter
	uiReader = strings.NewReader("")
	uiWriter = &e.out
	uiErrWriter = &e.err

	t.Cleanup(func() {
		e.srv.Close()
		apiBase = oldAPI
		execCommand, runCapture = oldExec, oldCapture
		uiReader, uiWriter, uiErrWriter = oldUIr, oldUIw, oldUIe
	})
	return e
}

// setInput 设置交互输入(可多行,如 "2\ny\n")。
func (e *flowEnv) setInput(s string) { uiReader = strings.NewReader(s) }

// amd64PairHandler 返回两个不同 tag 的 amd64 候选(同仓库不同 tag,不会被去重),
// 用于无 tag 请求触发候选选择菜单。
func amd64PairHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(resultsJSON(2, []Result{
			{Source: "docker.io/node:22-alpine", Mirror: "swr.cn-north-4.myhuaweicloud.com/ddn-k8s/docker.io/node:22-alpine", Platform: "linux/amd64", Size: "48MB"},
			{Source: "docker.io/node:22-alpine3.22", Mirror: "swr.cn-north-4.myhuaweicloud.com/ddn-k8s/docker.io/node:22-alpine3.22", Platform: "linux/amd64", Size: "49MB"},
		})))
	}
}

// dupMirrorHandler 返回同一镜像(library/ + 非 library/)的两条冗余记录,验证去重后 --yes 可用。
func dupMirrorHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(resultsJSON(2, []Result{
			{Source: "docker.io/library/node:22-alpine", Mirror: "m:lib", Platform: "linux/amd64"},
			{Source: "docker.io/node:22-alpine", Mirror: "m:direct", Platform: "linux/amd64"},
		})))
	}
}

func TestFlowPullUniqueYes(t *testing.T) {
	e := setupFlow(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(resultsJSON(1, []Result{{
			Source: "docker.io/node:22-alpine", Mirror: "swr.cn-north-4.myhuaweicloud.com/ddn-k8s/docker.io/node:22-alpine",
			Platform: "linux/amd64", Size: "48MB",
		}})))
	}))

	// --yes 放在关键词之后,顺带验证 flag 位置解析。
	code := runPull(context.Background(), []string{"node:22-alpine", "--yes"})
	if code != 0 {
		t.Fatalf("exit=%d,err=%s", code, e.err.String())
	}
	want := []string{
		"docker pull --platform=linux/amd64 swr.cn-north-4.myhuaweicloud.com/ddn-k8s/docker.io/node:22-alpine",
		"docker tag swr.cn-north-4.myhuaweicloud.com/ddn-k8s/docker.io/node:22-alpine node:22-alpine",
		"docker rmi swr.cn-north-4.myhuaweicloud.com/ddn-k8s/docker.io/node:22-alpine",
	}
	if !slices.Equal(e.execCalls, want) {
		t.Fatalf("命令序列不符:\n  got  %v\n  want %v", e.execCalls, want)
	}
}

func TestFlowPullCancel(t *testing.T) {
	e := setupFlow(t, amd64PairHandler())
	e.setInput("0\n")

	code := runPull(context.Background(), []string{"node"})
	if code != 0 {
		t.Fatalf("取消应返回 0,实际 %d,err=%s", code, e.err.String())
	}
	if len(e.execCalls) != 0 {
		t.Fatalf("取消后不应执行任何 docker 命令: %v", e.execCalls)
	}
	if !strings.Contains(e.out.String(), "已取消") {
		t.Fatalf("期望输出 已取消,实际: %s", e.out.String())
	}
}

func TestFlowPullChooseAndRename(t *testing.T) {
	e := setupFlow(t, amd64PairHandler())
	e.setInput("2\ny\n")

	code := runPull(context.Background(), []string{"node"})
	if code != 0 {
		t.Fatalf("exit=%d,err=%s", code, e.err.String())
	}
	got := e.execCalls
	if len(got) != 3 || !strings.Contains(got[0], "node:22-alpine3.22") {
		t.Fatalf("应拉取第 2 个候选(22-alpine3.22),命令序列不符: %v", got)
	}
	if !strings.HasPrefix(got[1], "docker tag ") || !strings.HasSuffix(got[1], " node") {
		t.Fatalf("期望重命名命令(docker tag <mirror> node),实际: %v", got[1])
	}
}

func TestFlowPullYesMultiple(t *testing.T) {
	e := setupFlow(t, amd64PairHandler())

	code := runPull(context.Background(), []string{"node", "--yes"})
	if code != 1 {
		t.Fatalf("--yes 多候选应返回 1,实际 %d", code)
	}
	if len(e.execCalls) != 0 {
		t.Fatalf("不应执行 docker 命令: %v", e.execCalls)
	}
	if !strings.Contains(e.err.String(), "--yes") {
		t.Fatalf("期望提示 --yes,实际: %s", e.err.String())
	}
}

// TestFlowPullUniqueAfterDedup 验证同步站把同一镜像同步成 library/ 与非 library/ 两条
// 记录时,--yes 仍能认定「唯一」:去重后应拉取不带 library/ 的 mirror。
func TestFlowPullUniqueAfterDedup(t *testing.T) {
	e := setupFlow(t, dupMirrorHandler())

	code := runPull(context.Background(), []string{"node:22-alpine", "--yes"})
	if code != 0 {
		t.Fatalf("去重后 --yes 应成功,实际 exit=%d,err=%s", code, e.err.String())
	}
	want := []string{
		"docker pull --platform=linux/amd64 m:direct",
		"docker tag m:direct node:22-alpine",
		"docker rmi m:direct",
	}
	if !slices.Equal(e.execCalls, want) {
		t.Fatalf("命令序列不符:\n  got  %v\n  want %v", e.execCalls, want)
	}
}

func TestFlowPullNotFound(t *testing.T) {
	e := setupFlow(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(resultsJSON(0, nil)))
	}))

	code := runPull(context.Background(), []string{"no-such-image:9.9"})
	if code != 1 {
		t.Fatalf("未找到应返回 1,实际 %d", code)
	}
	if !strings.Contains(e.err.String(), "未找到镜像") {
		t.Fatalf("期望 未找到镜像,实际: %s", e.err.String())
	}
}

func TestFlowPullPlatformMissing(t *testing.T) {
	e := setupFlow(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(resultsJSON(1, []Result{{
			Source: "docker.io/node:22-alpine", Mirror: "swr.cn-north-4.myhuaweicloud.com/ddn-k8s/docker.io/node:22-alpine-linuxarm64",
			Platform: "linux/arm64",
		}})))
	}))

	code := runPull(context.Background(), []string{"node:22-alpine"})
	if code != 1 {
		t.Fatalf("平台缺失应返回 1,实际 %d", code)
	}
	if !strings.Contains(e.err.String(), "没有 linux/amd64 平台的版本") {
		t.Fatalf("期望平台缺失提示,实际: %s", e.err.String())
	}
}

func TestFlowSearchRendersTable(t *testing.T) {
	e := setupFlow(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(resultsJSON(2, []Result{
			{Source: "docker.io/node:20", Mirror: "m:node-20", Platform: "linux/amd64", Size: "100MB"},
			{Source: "docker.io/node:22", Mirror: "m:node-22", Platform: "linux/amd64", Size: "120MB"},
		})))
	}))

	code := runSearch(context.Background(), []string{"node"})
	if code != 0 {
		t.Fatalf("search 应返回 0,实际 %d", code)
	}
	out := e.out.String()
	for _, want := range []string{"source", "docker.io/node:20", "docker.io/node:22", "m:node-20"} {
		if !strings.Contains(out, want) {
			t.Fatalf("search 输出缺少 %q:\n%s", want, out)
		}
	}
}

func TestFlowSearchFlagBeforeKeyword(t *testing.T) {
	_ = setupFlow(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("platform"); got != "linux/arm64" {
			t.Errorf("platform 参数错误: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(resultsJSON(1, []Result{{Source: "docker.io/node:20", Mirror: "m:node-20", Platform: "linux/arm64"}})))
	}))

	code := runSearch(context.Background(), []string{"--platform=linux/arm64", "node"})
	if code != 0 {
		t.Fatalf("search 应返回 0,实际 %d", code)
	}
}

func TestFlowSearchTruncationHint(t *testing.T) {
	e := setupFlow(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(resultsJSON(50, []Result{{Source: "docker.io/node:20", Mirror: "m:node-20", Platform: "linux/amd64"}})))
	}))

	code := runSearch(context.Background(), []string{"node"})
	if code != 0 {
		t.Fatalf("exit=%d", code)
	}
	if !strings.Contains(e.err.String(), "仅显示前") {
		t.Fatalf("期望截断提示,实际 stderr: %s", e.err.String())
	}
}

func TestFlowPullTagNotFound(t *testing.T) {
	// 真实 API 行为:带不存在 tag 的查询返回空;纯名字查询才返回该镜像的记录。
	e := setupFlow(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("search") == "node" {
			w.Write([]byte(resultsJSON(1, []Result{{Source: "docker.io/node:22-alpine", Mirror: "m:node-22", Platform: "linux/amd64"}})))
			return
		}
		w.Write([]byte(resultsJSON(0, nil)))
	}))

	code := runPull(context.Background(), []string{"node:9999", "--yes"})
	if code != 1 {
		t.Fatalf("tag 不存在应返回 1,实际 %d", code)
	}
	err := e.err.String()
	if !strings.Contains(err, "没有 tag 9999 的版本") || !strings.Contains(err, "已同步 tag") {
		t.Fatalf("期望区分 tag 未同步,实际: %s", err)
	}
	if strings.Contains(err, "未找到镜像") {
		t.Fatalf("镜像已同步,不应提示 未找到镜像: %s", err)
	}
}

func TestFlowPullExtraArgs(t *testing.T) {
	e := setupFlow(t, amd64PairHandler())

	code := runPull(context.Background(), []string{"node", "extraarg"})
	if code != 2 {
		t.Fatalf("多余位置参数应返回 2,实际 %d", code)
	}
	if !strings.Contains(e.err.String(), "多余的参数") || !strings.Contains(e.err.String(), "extraarg") {
		t.Fatalf("期望提示多余参数,实际: %s", e.err.String())
	}
	if len(e.execCalls) != 0 {
		t.Fatalf("参数错误不应执行 docker 命令: %v", e.execCalls)
	}
}

func TestVersionText(t *testing.T) {
	text := versionText()
	if !strings.HasPrefix(text, "dockercn ") || text == "dockercn " {
		t.Fatalf("versionText 格式错误: %q", text)
	}
}
