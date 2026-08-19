package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSearchImages(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/image" {
			t.Errorf("路径错误: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("search"); got != "node:22-alpine" {
			t.Errorf("search 参数错误: %q", got)
		}
		if ua := r.UserAgent(); !strings.HasPrefix(ua, "dockercn/") {
			t.Errorf("User-Agent 错误: %q", ua)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"count":1,"error":false,"aiprompt":"注意","results":[{"source":"docker.io/node:22-alpine","mirror":"swr.cn-north-4.myhuaweicloud.com/ddn-k8s/docker.io/node:22-alpine","platform":"linux/amd64","size":"157MB","createdAt":"2025-03-12T14:48:27.191+08:00"}]}`))
	}))
	defer srv.Close()

	old := apiBase
	apiBase = srv.URL + "/api/v1"
	defer func() { apiBase = old }()

	resp, err := SearchImages(context.Background(), "node:22-alpine", "", "")
	if err != nil {
		t.Fatalf("SearchImages 错误: %v", err)
	}
	if len(resp.Results) != 1 || resp.Results[0].Source != "docker.io/node:22-alpine" {
		t.Fatalf("解析结果错误: %+v", resp)
	}
	if resp.AIPrompt != "注意" {
		t.Fatalf("aiprompt 未解析: %q", resp.AIPrompt)
	}
}

func TestSearchImagesHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	old := apiBase
	apiBase = srv.URL + "/api/v1"
	defer func() { apiBase = old }()

	if _, err := SearchImages(context.Background(), "x", "", ""); err == nil {
		t.Fatal("期望非 200 时报错")
	}
}
