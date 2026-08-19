package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// uiReader/uiErrWriter 可注入,便于测试。
// uiWriter 收敛全部正常输出(进度、提示、表格),uiErrWriter 收敛全部错误输出。
var (
	uiReader    io.Reader = os.Stdin
	uiWriter    io.Writer = os.Stdout
	uiErrWriter io.Writer = os.Stderr

	uiBuf      *bufio.Reader // 复用同一个缓冲 reader,避免多个 Reader 吞掉后续输入
	uiBufOwner io.Reader
)

// uiLineReader 返回基于当前 uiReader 的共享缓冲读取器;
// 多个输入提示共用它,保证管道输入(如 echo "1\ny" | dockercn pull ...)不会丢失后续行。
func uiLineReader() *bufio.Reader {
	if uiBuf == nil || uiBufOwner != uiReader {
		uiBuf = bufio.NewReader(uiReader)
		uiBufOwner = uiReader
	}
	return uiBuf
}

// ChooseResult 以数字编号方式让用户从候选中选择一个;输入 0 取消。
func ChooseResult(results []Result) (*Result, error) {
	fmt.Fprintln(uiWriter, "发现多个匹配的镜像,请选择要下载的候选:")
	for i, r := range results {
		fmt.Fprintf(uiWriter, "  [%d] %s | 平台: %s | 大小: %s | 同步时间: %s\n",
			i+1, r.Source, r.Platform, r.Size, r.CreatedAt)
	}
	fmt.Fprint(uiWriter, "输入编号(0 取消): ")

	reader := uiLineReader()
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF && line == "" {
				return nil, nil // 无输入视为取消
			}
			return nil, err
		}
		n, perr := strconv.Atoi(strings.TrimSpace(line))
		if perr != nil || n < 0 || n > len(results) {
			fmt.Fprint(uiWriter, "无效输入,请重新输入编号(0 取消): ")
			continue
		}
		if n == 0 {
			return nil, nil
		}
		return &results[n-1], nil
	}
}

// Confirm 询问 y/N,返回是否确认。
func Confirm(prompt string) (bool, error) {
	fmt.Fprintf(uiWriter, "%s [y/N]: ", prompt)
	reader := uiLineReader()
	line, err := reader.ReadString('\n')
	if err != nil {
		if err == io.EOF {
			return false, nil // 无输入视为拒绝
		}
		return false, err
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}
