# dockercn — 国内 Docker 镜像拉取助手(设计文档)

- 日期:2026-08-19
- 状态:已定稿(待用户审阅)
- 关联上游:https://docker.aityp.com 渡渡鸟容器同步站公共 API

## 1. 背景与目标

国内用户从 Docker Hub / gcr.io / ghcr.io / quay.io 等源拉取镜像困难。「渡渡鸟容器同步站」提供免费公共 API,查询已同步到国内华为云 SWR 的镜像地址(`swr.cn-north-4.myhuaweicloud.com/ddn-k8s/...`)。

`dockercn` 是一个本地 CLI 工具:自动查询该 API 找到国内 mirror 地址 → `docker pull` → 下载完成后询问是否将镜像重命名为用户原始指定的名字。

## 2. 已确认的设计决策

| # | 决策 | 结论 |
|---|------|------|
| 1 | 使用形态 | 独立命令 `dockercn pull <image>`,不改动用户日常 `docker` 命令 |
| 2 | 实现语言 | Go,编译为单个跨平台静态二进制(Linux/macOS/Windows) |
| 3 | 平台处理 | 自动检测本机架构,`--platform` 可覆盖 |
| 4 | 站点判定 | 不做自动站点判定;精确匹配靠镜像全名比对;仅 docker.io 精确名未命中时内部补查 `site=docker.io` 一次(防 50 条截断漏查) |
| 5 | 查不到镜像 | 提示前往 `/manage/add` 网页提交同步(需登录),本次退出;不自动回退直连 |
| 6 | 重命名 | 下载完成后询问 y/N;确认后 `docker tag` 加别名,再 `docker rmi <mirror>` 删除 mirror 标签(只删标签,数据保留一份) |
| 7 | v1 范围 | `pull` + `search` 两个子命令;无配置文件 |
| 8 | 未写 tag 的镜像 | 视为「名字精确匹配、tag 不限」:列出该名字下所有 tag 候选供用户选择;拉取前提示站点不保证 `latest` 为最新(补充决策,待用户审阅确认) |

## 3. 命令接口

### 3.1 `dockercn pull <image> [--platform=amd64] [--yes]`

流程:

1. **解析镜像名**:拆出 registry(可选)、namespace、name、tag(可选);无 registry 前缀视为 docker.io
2. **查询 API**:`GET /api/v1/image?search=<完整镜像名>`;结果按以下规则筛选
3. **匹配规则**:
   - 归一化:比较时同时接受 `docker.io/node:xx` 与 `docker.io/library/node:xx` 为同一镜像;tag 精确相等
   - 有 tag:名字 + tag 均精确命中 → 精确匹配;无 tag:名字精确命中视为「名字匹配」
   - 平台过滤:自动检测(或 `--platform`)指定后,仅保留 `platform` 字段匹配的结果
   - 精确命中且过滤后唯一 → 直接拉取;否则数字编号列出候选(镜像名 / 平台 / 大小 / 同步时间)供用户选择
4. **防漏查**:docker.io 镜像名字精确未命中时,补查一次 `search=<名>&site=docker.io`,仍无命中才进入「未找到」
5. **拉取**:`docker pull <mirror>`;若 `--platform` 指定且 mirror tag 无平台后缀,透传 `--platform` 给 docker
6. **重命名**:拉取成功后询问 `是否重命名为 <原始镜像名>? [y/N]`;确认则 `docker tag <mirror> <原始名>` 并 `docker rmi <mirror>`;拒绝则保留 mirror 标签,提示用户可用 `docker tag` 自行改名
7. **未找到**:提示「该镜像尚未同步,可前往 https://docker.aityp.com/manage/add 提交同步(需登录)」,退出码 1
8. **`--yes`**:非交互模式,跳过候选选择(仅当唯一)与重命名询问,直接下载并自动重命名

### 3.2 `dockercn search <keyword> [--site=] [--platform=]`

- 调用 `GET /api/v1/image?search=<keyword>`,`--site` / `--platform` 仅作为可选手动过滤透传
- 表格式输出 source / mirror / size / platform / createdAt,最多 50 条
- 展示 API 返回的 `aiprompt` 提示(latest 不保证最新)

## 4. 架构探测

- 优先 `docker info --format '{{.Architecture}}'`
- 失败回退 `uname -m` 映射:x86_64→amd64、aarch64→arm64、armv7l→arm、ppc64le→ppc64le、s390x→s390x、riscv64→riscv64、loongarch64→loong64
- `--platform` 指定时跳过探测

## 5. docker 子进程调用

- 前置检查:`docker version` 失败则提示「docker 未安装或 daemon 未运行」
- `docker pull`、`docker tag`、`docker rmi` 均以子进程方式调用,透传 stdout/stderr
- `docker rmi <mirror>` 只删标签不删数据(与原始名指向同一镜像 ID)

## 6. 错误处理与退出码

| 场景 | 行为 | 退出码 |
|------|------|--------|
| 成功 | 正常输出 | 0 |
| API 不可达 / 非 200 | 中文错误提示 | 1 |
| docker 不可用 | 明确提示 | 1 |
| 未找到镜像 | 提示去网页提交同步 | 1 |
| 用户取消选择 / 重命名拒绝 | 提示并结束 | 0(拒绝重命名不算失败) |
| 拉取失败 | 透传 docker 错误信息 | 1 |

## 7. 工程结构

单一 Go 模块,文件划分:

- `main.go` — CLI 入口、flag 解析、子命令分发
- `api.go` — API 客户端(请求 / 超时 / 错误处理 / 响应解析)
- `match.go` — 镜像名解析与归一化、精确匹配逻辑(纯函数,可单测)
- `docker.go` — docker 子进程封装(检查 / pull / tag / rmi)
- `ui.go` — 数字编号候选选择、y/N 询问

依赖:仅标准库(`flag`、`net/http`、`os/exec`、`encoding/json` 等),不引入第三方库。

## 8. 测试计划

### 单元测试(match.go)
- 带/不带 registry 前缀、带/不带 `library/`、带/不带 tag 的组合
- `docker.io/node:22-alpine` 与 `docker.io/library/node:22-alpine` 判定为同一镜像
- 平台过滤、名字匹配 vs 全精确匹配的区分

### 真机验证
- `dockercn search python`(表格式输出)
- `dockercn pull python:3.12-alpine`(docker.io,amd64 自动检测,直接拉取 + 重命名)
- `dockercn pull gcr.io/google-containers/coredns:1.2.6`(跨源,无站点判定)
- 预测一个未同步镜像 → 验证「仅提示退出」
- `--yes` 非交互路径
- 重命名 y/N 两条分支

## 9. 明确不在 v1 范围

- 配置文件(`~/.dockercn.yaml` 等)
- push / 多镜像批量拉取 / 其他子命令
- 自动提交同步请求(站点无公开 API,需登录)
- Windows 专用打包(发布二进制跨平台,但不在本环境验证 Windows)

## 10. 假设

- 提示语言为中文;候选选择为数字编号输入
- 拒绝重命名视为成功(退出码 0)
- 本环境未初始化 git 仓库,文档暂不提交版本管理
