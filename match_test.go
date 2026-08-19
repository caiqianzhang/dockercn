package main

import "testing"

func TestParseImageRef(t *testing.T) {
	cases := []struct {
		in       string
		registry string
		name     string
		tag      string
		wantErr  bool
	}{
		{"node", "docker.io", "node", "", false},
		{"node:22-alpine", "docker.io", "node", "22-alpine", false},
		{"library/node:20", "docker.io", "library/node", "20", false},
		{"docker.io/library/node:20", "docker.io", "library/node", "20", false},
		{"docker.io/node", "docker.io", "node", "", false},
		{"gcr.io/google-containers/coredns:1.2.6", "gcr.io", "google-containers/coredns", "1.2.6", false},
		{"quay.io/prometheus/node-exporter:v1.8.1", "quay.io", "prometheus/node-exporter", "v1.8.1", false},
		{"node:", "", "", "", true},
		{"", "", "", "", true},
		{"ubuntu@sha256:abc", "", "", "", true},
	}
	for _, c := range cases {
		got, err := ParseImageRef(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("ParseImageRef(%q) 期望报错,实际成功 %+v", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseImageRef(%q) 意外错误: %v", c.in, err)
			continue
		}
		if got.Registry != c.registry || got.Name != c.name || got.Tag != c.tag {
			t.Errorf("ParseImageRef(%q) = %+v,期望 registry=%q name=%q tag=%q", c.in, got, c.registry, c.name, c.tag)
		}
	}
}

func TestCanonicalName(t *testing.T) {
	cases := []struct{ registry, name, want string }{
		{"docker.io", "node", "docker.io/node"},
		{"docker.io", "library/node", "docker.io/node"},
		{"DOCKER.IO", "LIBRARY/NODE", "docker.io/node"},
		{"gcr.io", "google-containers/coredns", "gcr.io/google-containers/coredns"},
		{"index.docker.io", "node", "docker.io/node"},
	}
	for _, c := range cases {
		if got := canonicalName(c.registry, c.name); got != c.want {
			t.Errorf("canonicalName(%q, %q) = %q,期望 %q", c.registry, c.name, got, c.want)
		}
	}
}

func TestFilterCandidates(t *testing.T) {
	results := []Result{
		{Source: "docker.io/node:18-alpine", Platform: "linux/amd64"},
		{Source: "docker.io/node:22-alpine", Platform: "linux/amd64"},
		{Source: "docker.io/library/node:22-alpine", Platform: "linux/arm64"},
		{Source: "quay.io/prometheus/node-exporter:v1.8.1", Platform: "linux/amd64"},
		{Source: "docker.io/calico/node:v3.25.0", Platform: "linux/amd64"},
	}

	ref, _ := ParseImageRef("node:22-alpine")
	got := FilterCandidates(results, ref)
	if len(got) != 2 {
		t.Fatalf("期望 2 个候选,实际 %d: %+v", len(got), got)
	}
	for _, r := range got {
		if r.Source != "docker.io/node:22-alpine" && r.Source != "docker.io/library/node:22-alpine" {
			t.Errorf("意外候选: %s", r.Source)
		}
	}

	ref2, _ := ParseImageRef("node")
	got2 := FilterCandidates(results, ref2)
	if len(got2) != 3 {
		t.Fatalf("无 tag 期望 3 个候选,实际 %d: %+v", len(got2), got2)
	}

	ref3, _ := ParseImageRef("quay.io/prometheus/node-exporter:v1.8.1")
	got3 := FilterCandidates(results, ref3)
	if len(got3) != 1 || got3[0].Source != "quay.io/prometheus/node-exporter:v1.8.1" {
		t.Fatalf("跨仓库精确匹配失败: %+v", got3)
	}
}

func TestFilterPlatform(t *testing.T) {
	results := []Result{
		{Platform: "linux/amd64"},
		{Platform: "linux/arm64"},
		{Platform: "linux/amd64/v2"},
		{Platform: ""},
	}
	// 有明确匹配时,平台为空的记录不参与,避免制造歧义候选。
	if got := FilterPlatform(results, "linux/arm64"); len(got) != 1 || got[0].Platform != "linux/arm64" {
		t.Fatalf("arm64 过滤失败: %+v", got)
	}
	// linux/amd64/v2 应作为 linux/amd64 的变体被匹配。
	if got := FilterPlatform(results, "linux/amd64"); len(got) != 2 {
		t.Fatalf("amd64 应匹配 2 个(含 /v2 变体),实际 %d: %+v", len(got), got)
	}
	// 没有任何明确匹配时,平台未知的记录作为兜底候选保留。
	if got := FilterPlatform(results, "linux/ppc64le"); len(got) != 1 || got[0].Platform != "" {
		t.Fatalf("无匹配时应保留空平台兜底,实际 %+v", got)
	}
	// 目标平台为空时不过滤。
	if got := FilterPlatform(results, ""); len(got) != 4 {
		t.Fatalf("空平台应不过滤,实际 %d", len(got))
	}
}
