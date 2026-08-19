package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// apiBase 为变量以便测试时替换。
var apiBase = "https://docker.aityp.com/api/v1"

// httpClient 复用连接池,避免每次请求重建。
var httpClient = &http.Client{Timeout: 15 * time.Second}

// Result 是 API 返回的单条镜像记录。
type Result struct {
	Source    string `json:"source"`
	Mirror    string `json:"mirror"`
	Platform  string `json:"platform"`
	Size      string `json:"size"`
	CreatedAt string `json:"createdAt"`
}

// SearchResponse 是 /api/v1/image 的响应。
type SearchResponse struct {
	Count    int      `json:"count"`
	Error    bool     `json:"error"`
	AIPrompt string   `json:"aiprompt"`
	Results  []Result `json:"results"`
}

// SearchImages 调用同步站 /api/v1/image 查询镜像。
// site 与 platform 为空时不下发对应参数。
func SearchImages(ctx context.Context, query, site, platform string) (*SearchResponse, error) {
	u, err := url.Parse(apiBase + "/image")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("search", query)
	if site != "" {
		q.Set("site", site)
	}
	if platform != "" {
		q.Set("platform", platform)
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	// 标注客户端身份,便于同步站统计与问题定位。
	req.Header.Set("User-Agent", "dockercn/"+version)
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求同步站 API 失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 5<<20))
	if err != nil {
		return nil, fmt.Errorf("读取 API 响应失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("同步站 API 返回异常状态码 %d", resp.StatusCode)
	}
	var out SearchResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("解析 API 响应失败: %w", err)
	}
	if out.Error {
		return nil, fmt.Errorf("同步站 API 报告错误")
	}
	return &out, nil
}
