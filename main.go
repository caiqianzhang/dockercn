package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
)

// version 由构建时 ldflags 注入(git describe),便于用户报告问题版本。
var version = "dev"

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "用法: dockercn <pull|search|version> ...  (dockercn help 查看详情)")
		os.Exit(2)
	}
	ctx := context.Background()
	switch os.Args[1] {
	case "pull":
		os.Exit(runPull(ctx, os.Args[2:]))
	case "search":
		os.Exit(runSearch(ctx, os.Args[2:]))
	case "version", "-v", "--version":
		fmt.Println(versionText())
		os.Exit(0)
	case "help", "-h", "--help":
		fmt.Println("dockercn — 国内 Docker 镜像拉取助手")
		fmt.Println("  dockercn pull <镜像名> [--platform=linux/amd64] [--yes]")
		fmt.Println("  dockercn search <关键词> [--site=docker.io] [--platform=linux/arm64]")
		fmt.Println("  dockercn version")
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "未知命令 %q\n用法: dockercn <pull|search|version> ...\n", os.Args[1])
		os.Exit(2)
	}
}

// versionText 返回版本行,供 version 子命令使用。
func versionText() string {
	return "dockercn " + version
}

func runPull(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("pull", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	platformFlag := fs.String("platform", "", "目标平台(如 linux/arm64),默认自动检测本机架构")
	yes := fs.Bool("yes", false, "非交互模式:唯一候选直接下载并自动重命名")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "用法: dockercn pull <镜像名> [--platform=linux/amd64] [--yes]")
	}
	imageName, ok := parseKeywordArgs(fs, args)
	if !ok {
		return 2
	}

	ref, err := ParseImageRef(imageName)
	if err != nil {
		fmt.Fprintf(uiErrWriter, "参数错误: %v\n", err)
		return 2
	}

	if err := CheckDocker(ctx); err != nil {
		fmt.Fprintln(uiErrWriter, err)
		return 1
	}

	// 平台:未显式指定时自动检测
	plat := *platformFlag
	if plat == "" {
		plat, err = DetectArch(ctx)
		if err != nil {
			fmt.Fprintln(uiErrWriter, err)
			return 1
		}
	}

	// 查询 API(不带 site/platform,由本地精确匹配)
	resp, err := SearchImages(ctx, ref.Raw, "", "")
	if err != nil {
		fmt.Fprintln(uiErrWriter, err)
		return 1
	}
	candidates := FilterCandidates(resp.Results, ref)

	// 防漏查:docker.io 且精确未命中时补查 site=docker.io
	if len(candidates) == 0 && ref.Registry == "docker.io" {
		if resp2, err2 := SearchImages(ctx, ref.Raw, "docker.io", ""); err2 == nil {
			candidates = FilterCandidates(resp2.Results, ref)
		} else {
			fmt.Fprintln(uiErrWriter, "补查 site=docker.io 失败:", err2)
		}
	}

	allName := candidates
	candidates = FilterPlatform(candidates, plat)
	// 合并同镜像(源仓库+tag+平台相同)的冗余 mirror 记录,否则 --yes 的「唯一」判定会因
	// 同步站同时同步 docker.io/x 与 docker.io/library/x 而永远失败。
	candidates = DedupeMirrors(candidates)
	if len(candidates) == 0 {
		fmt.Fprintln(uiErrWriter, noCandidateMessage(ref, allName, plat))
		return 1
	}

	// 选择候选
	chosen, err := chooseCandidate(ctx, candidates, *yes)
	if err != nil {
		fmt.Fprintln(uiErrWriter, err)
		return 1
	}
	if chosen == nil {
		fmt.Fprintln(uiWriter, "已取消")
		return 0
	}

	if ref.Tag == "" {
		fmt.Fprintln(uiWriter, "提示:未指定 tag,同步站不保证 latest 为最新版本,建议使用具体版本号")
	}

	// 拉取(platform 恒透传,即使自动检测也如此,确保多架构 manifest 选对变体)
	fmt.Fprintf(uiWriter, "开始拉取 %s ...\n", chosen.Mirror)
	if err := PullImage(ctx, chosen.Mirror, plat); err != nil {
		fmt.Fprintln(uiErrWriter, "拉取失败:", err)
		return 1
	}

	// 重命名
	if *yes {
		if err := renameImage(ctx, chosen.Mirror, ref.Raw); err != nil {
			fmt.Fprintln(uiErrWriter, err)
			return 1
		}
		return 0
	}
	confirmed, err := Confirm(fmt.Sprintf("是否将镜像重命名为 %s?", ref.Raw))
	if err != nil {
		fmt.Fprintln(uiErrWriter, err)
		return 1
	}
	if !confirmed {
		fmt.Fprintf(uiWriter, "已保留镜像标签 %s\n", chosen.Mirror)
		fmt.Fprintf(uiWriter, "如需改名可执行: docker tag %s %s\n", chosen.Mirror, ref.Raw)
		return 0
	}
	if err := renameImage(ctx, chosen.Mirror, ref.Raw); err != nil {
		fmt.Fprintln(uiErrWriter, err)
		return 1
	}
	return 0
}

// renameImage 给镜像添加别名并删除原标签。
func renameImage(ctx context.Context, src, dst string) error {
	if src == dst {
		// 拉取的 mirror 名已等于目标名:无需重命名,更不能 rmi 删掉唯一标签。
		return nil
	}
	if err := TagImage(ctx, src, dst); err != nil {
		return fmt.Errorf("重命名失败: %w", err)
	}
	if err := RemoveTag(ctx, src); err != nil {
		fmt.Fprintf(uiErrWriter, "已添加别名 %s,但删除原标签失败: %v\n", dst, err)
		return nil
	}
	fmt.Fprintf(uiWriter, "已重命名为 %s\n", dst)
	return nil
}

func runSearch(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("search", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	site := fs.String("site", "", "源仓库过滤(如 docker.io / gcr.io)")
	platform := fs.String("platform", "", "平台过滤(如 linux/arm64)")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "用法: dockercn search <关键词> [--site=docker.io] [--platform=linux/arm64]")
	}
	keyword, ok := parseKeywordArgs(fs, args)
	if !ok {
		return 2
	}

	resp, err := SearchImages(ctx, keyword, *site, *platform)
	if err != nil {
		fmt.Fprintln(uiErrWriter, err)
		return 1
	}
	if resp.AIPrompt != "" {
		fmt.Fprintln(uiWriter, resp.AIPrompt)
	}
	if len(resp.Results) == 0 {
		fmt.Fprintln(uiWriter, "没有找到匹配的镜像")
		return 0
	}
	w := tabwriter.NewWriter(uiWriter, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "#\tsource\tplatform\tsize\tcreatedAt\tmirror")
	for i, r := range resp.Results {
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%s\n", i+1, r.Source, r.Platform, r.Size, r.CreatedAt, r.Mirror)
	}
	w.Flush()
	if resp.Count > len(resp.Results) {
		fmt.Fprintf(uiErrWriter, "提示:该关键词共 %d 条记录,仅显示前 %d 条(可用 --site/--platform 缩小范围)\n", resp.Count, len(resp.Results))
	}
	return 0
}

// parseKeywordArgs 摘出第一个非 flag 参数作为关键词并解析其余 flags。
// ok=false 表示参数不合法(已打印用法),调用方应返回退出码 2。
func parseKeywordArgs(fs *flag.FlagSet, args []string) (string, bool) {
	keyword, flagArgs := splitKeyword(args)
	if err := fs.Parse(flagArgs); err != nil {
		return "", false
	}
	if keyword == "" {
		fs.Usage()
		return "", false
	}
	return keyword, true
}

// chooseCandidate 从候选集中选定一个:
// 唯一 → 直接返回;多个 → 非交互(--yes)报错,交互则让用户选,输入 0 取消返回 nil。
func chooseCandidate(ctx context.Context, candidates []Result, yes bool) (*Result, error) {
	if len(candidates) == 1 {
		fmt.Fprintf(uiWriter, "找到唯一匹配:%s → %s\n", candidates[0].Source, candidates[0].Mirror)
		return &candidates[0], nil
	}
	if yes {
		return nil, fmt.Errorf("匹配到 %d 个候选,--yes 模式下无法自动选择,请去掉 --yes 或使用更精确的镜像名", len(candidates))
	}
	chosen, err := ChooseResult(candidates)
	if err != nil {
		return nil, err
	}
	if chosen == nil {
		return nil, nil // 用户取消
	}
	return chosen, nil
}

// noCandidateMessage 生成「未找到镜像」或「该平台没有版本」的提示文案。
func noCandidateMessage(ref ImageRef, allName []Result, plat string) string {
	if len(allName) > 0 {
		var b strings.Builder
		fmt.Fprintf(&b, "该镜像已同步,但没有 %s 平台的版本(已同步平台: ", plat)
		seen := map[string]bool{}
		for _, r := range allName {
			if r.Platform != "" && !seen[r.Platform] {
				seen[r.Platform] = true
				fmt.Fprintf(&b, "%s ", r.Platform)
			}
		}
		fmt.Fprintf(&b, ")")
		return b.String()
	}
	return fmt.Sprintf("未找到镜像 %s\n该镜像可能尚未同步,可前往 https://docker.aityp.com/manage/add 提交同步请求(需登录)", ref.Raw)
}

// splitKeyword 取出参数中第一个不以 "-" 开头的参数作为关键词(镜像名),
// 其余参数(flag 及值)保持原顺序返回,使 flag 可以出现在关键词前后。
func splitKeyword(args []string) (string, []string) {
	for i, a := range args {
		if !strings.HasPrefix(a, "-") {
			rest := make([]string, 0, len(args)-1)
			rest = append(rest, args[:i]...)
			rest = append(rest, args[i+1:]...)
			return a, rest
		}
	}
	return "", args
}
