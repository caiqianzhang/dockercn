package main

import (
	"fmt"
	"strings"
)

// ImageRef 表示用户输入的镜像引用。
type ImageRef struct {
	Registry string // 默认 docker.io
	Name     string // namespace/name,不含 registry 与 tag
	Tag      string // 为空表示未指定 tag
	Raw      string // 原始输入
}

// ParseImageRef 解析用户输入的镜像名。
// 支持 node / node:22-alpine / library/node:20 / docker.io/library/node:20 / gcr.io/foo/bar:1.0。
// v1 不支持 digest 寻址(@sha256:...)。
func ParseImageRef(ref string) (ImageRef, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ImageRef{}, fmt.Errorf("镜像名不能为空")
	}
	if strings.Contains(ref, "@") {
		return ImageRef{}, fmt.Errorf("不支持 digest 寻址(@sha256:...),请直接使用 名字:tag")
	}
	raw := ref

	name := ref
	tag := ""
	if i := strings.LastIndex(ref, ":"); i >= 0 && !strings.Contains(ref[i+1:], "/") {
		name = ref[:i]
		tag = ref[i+1:]
		if tag == "" {
			return ImageRef{}, fmt.Errorf("镜像 tag 不能为空: %q", ref)
		}
	}

	registry := "docker.io"
	if parts := strings.Split(name, "/"); len(parts) > 1 {
		first := strings.ToLower(parts[0])
		isDockerAlias := first == "docker.io" || first == "index.docker.io" || first == "registry-1.docker.io"
		if isDockerAlias || strings.ContainsAny(first, ".:") || first == "localhost" {
			if !isDockerAlias {
				registry = first
			}
			name = strings.Join(parts[1:], "/")
		}
	}

	return ImageRef{
		Registry: strings.ToLower(registry),
		Name:     strings.ToLower(name),
		Tag:      tag,
		Raw:      raw,
	}, nil
}

// canonicalName 返回用于比对的规范名:小写、docker.io 别名统一、docker.io 的 library/ 前缀去除。
func canonicalName(registry, name string) string {
	reg := strings.ToLower(strings.TrimSpace(registry))
	if reg == "index.docker.io" || reg == "registry-1.docker.io" {
		reg = "docker.io"
	}
	n := strings.ToLower(strings.Trim(name, "/"))
	if reg == "docker.io" {
		n = strings.TrimPrefix(n, "library/")
	}
	return reg + "/" + n
}

// sourceTag 提取 source 里的 tag。
func sourceTag(source string) string {
	ref, err := ParseImageRef(source)
	if err != nil {
		return ""
	}
	return ref.Tag
}

// nameMatches 判断结果 source 与请求的镜像名是否一致(忽略 tag)。
func nameMatches(result Result, ref ImageRef) bool {
	src, err := ParseImageRef(result.Source)
	if err != nil {
		return false
	}
	return canonicalName(src.Registry, src.Name) == canonicalName(ref.Registry, ref.Name)
}

// FilterCandidates 返回与请求匹配的候选:
// 指定了 tag 时要求 tag 完全一致;未指定 tag 时名字一致即可。
func FilterCandidates(results []Result, ref ImageRef) []Result {
	var out []Result
	for _, r := range results {
		if !nameMatches(r, ref) {
			continue
		}
		if ref.Tag != "" && sourceTag(r.Source) != ref.Tag {
			continue
		}
		out = append(out, r)
	}
	return out
}

// platformMatches 判断记录平台是否匹配目标平台:
// 完全相等,或一方是另一方带变体后缀(如 /v2)的前缀。
func platformMatches(record, target string) bool {
	if record == target {
		return true
	}
	if strings.HasPrefix(record, target+"/") {
		return true // record 是 target 的变体,如 linux/amd64/v2 匹配 linux/amd64
	}
	if strings.HasPrefix(target, record+"/") {
		return true // target 是 record 的变体
	}
	return false
}

// FilterPlatform 只保留与目标平台匹配的结果;platform 为空时不过滤。
// 与目标平台不匹配的记录被丢弃;platform 字段为空的记录视为「平台未知」,
// 仅在没有任何明确匹配时作为兜底候选保留。
func FilterPlatform(results []Result, platform string) []Result {
	if platform == "" {
		return results
	}
	var matched []Result
	unknown := make([]Result, 0, 4)
	for _, r := range results {
		if r.Platform == "" {
			unknown = append(unknown, r)
			continue
		}
		if platformMatches(r.Platform, platform) {
			matched = append(matched, r)
		}
	}
	if len(matched) == 0 {
		matched = append(matched, unknown...)
	}
	return matched
}
