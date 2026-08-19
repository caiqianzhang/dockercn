# dockercn — 国内 Docker 镜像拉取助手

![CI](https://github.com/caiqianzhang/dockercn/actions/workflows/ci.yml/badge.svg)

`dockercn` 是一个纯 Go 标准库编写的命令行工具:自动查询「渡渡鸟容器同步站」的公共 API,找到镜像已同步到国内华为云 SWR 的地址,执行 `docker pull`,再把镜像重命名回你原本想写的名字。解决国内直连 Docker Hub / gcr.io / quay.io 等源拉取镜像困难的问题。

> 同步站本身不隶属本项目,服务可用性以其为准。本项目只负责「查 → 拉 → 改名」。

## 功能

- **`dockercn pull <镜像名> [--platform=linux/amd64] [--yes]`**
  查询并下载国内镜像地址,完成后询问是否重命名为原始镜像名(非交互加 `--yes` 自动改名)。
- **`dockercn search <关键词> [--site=docker.io] [--platform=linux/arm64]`**
  搜索并表格化展示 source / platform / size / createdAt / mirror。
- **`dockercn version`(或 `-v` / `--version`)**
  打印版本号(构建时由 `git describe` 注入,便于报告问题版本)。

## 安装

编译为单个静态二进制,仅依赖 docker CLI:

```bash
go build -o dockercn .
# 可选:安装到 PATH
install -m 0755 dockercn /usr/local/bin/
```

## 用法示例

```bash
# 直接拉取(自动检测本机架构,唯一候选自动选中)
dockercn pull python:3.12-alpine

# 非交互:唯一候选直接下载并自动重命名
dockercn pull python:3.12-alpine --yes

# 指定平台(跨架构拉取)
dockercn pull python:3.12-alpine --platform=linux/arm64

# 未指定 tag 时列出该名字下所有 tag 候选供选择
dockercn pull node

# 搜索
dockercn search node --platform linux/arm64

# 版本
dockercn version
```

## 工作原理

1. **解析镜像名**:拆出 registry(可选)/ namespace / name / tag;无 registry 前缀视为 `docker.io`。`node:22-alpine`、`library/node:22-alpine`、`docker.io/library/node:22-alpine` 被归一化为同一镜像。
2. **查询同步站 API**:`GET https://docker.aityp.com/api/v1/image?search=<镜像名>`,返回已同步的国内 mirror 地址(`swr.cn-north-4.myhuaweicloud.com/ddn-k8s/...`)。
3. **匹配与平台过滤**:指定 tag 时要求 tag 完全一致;未指定 tag 时名字一致即可。按本机架构(`docker info` 优先,回退 `uname -m`、再回退 Go 运行时)过滤候选。
4. **拉取**:`docker pull <mirror> --platform=<arch>`。
5. **重命名**:`docker tag <mirror> <原始名>` 加别名,再 `docker rmi <mirror>` 删除 mirror 标签——两个标签指向同一镜像 ID,只删标签不删数据。

> 同步站常把同一镜像同时同步成 `docker.io/x` 与 `docker.io/library/x` 两条记录;工具会合并为同一条(优先不带 `library/` 的写法),避免「唯一候选」判定被冗余记录干扰。

## 退出码

| 码 | 含义 |
|----|------|
| 0 | 成功;或用户取消选择 / 拒绝重命名(不算失败) |
| 1 | API 不可达、docker 不可用、未找到镜像、拉取失败 |
| 2 | 参数错误 |

## 未找到镜像?

提示里会给出同步站地址,可登录后提交同步请求;本项目不做自动回退直连。

## 已知限制(v1)

- 不支持 digest 寻址(`@sha256:...`),请使用 `名字:tag`。
- 同步站不保证 `latest` 为最新,建议使用具体版本号。
- `--yes` 在合并冗余记录后仍匹配到多个候选(如同仓库多个 tag / 多个平台)时报错,需去掉 `--yes` 或使用更精确的镜像名。

## 跨平台

- 发布产物为**静态链接**二进制,可在目标平台直接运行;每次发版 CI 都会在 Linux(x86_64/arm64)、macOS、Windows 的真实环境跑一遍冒烟(`version`/`help`/真实 `search`)。
- **Windows 旧版控制台(cmd)默认 GBK 码页**,中文输出可能乱码:请使用 Windows Terminal,或先执行 `chcp 65001`。
- **Linux 精简容器(Alpine / scratch)运行静态二进制需已安装 `ca-certificates`**,否则访问 API 的 TLS 握手会失败。
- 架构自动检测为三级回退(`docker info` → `uname -m` → Go 运行时),Windows 无 `uname` 也能可靠探测。

## 开发

```bash
go test -race ./...   # 单元测试(33 项)+ 数据竞争检测
go vet ./...    # 静态检查
gofmt -l .      # 格式检查
make build      # 构建并注入版本号
make release    # 跨平台产物到 dist/
```

CI:push/PR 自动跑格式、vet、测试;打 `v*` 标签时自动 `make release` 发布 GitHub Release(5 个跨平台二进制),并在 Linux/macOS/Windows 真实平台跑冒烟验证。

## 项目结构

| 文件 | 职责 |
|------|------|
| `main.go` | CLI 入口、flag 解析、pull/search 流程编排 |
| `api.go` | 同步站 API 客户端(复用 `httpClient`,带 User-Agent) |
| `match.go` | 镜像名解析/归一化/候选与平台匹配(纯函数) |
| `docker.go` | docker 子进程封装、本机架构探测(三级回退) |
| `ui.go` | 输出收敛(uiWriter/uiErrWriter)、候选选择、y/N 询问 |
