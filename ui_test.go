package main

import (
	"strings"
	"testing"
)

func TestChooseResult(t *testing.T) {
	results := []Result{{Source: "a:1"}, {Source: "b:2"}}
	oldR, oldW := uiReader, uiWriter
	uiReader = strings.NewReader("2\n")
	var buf strings.Builder
	uiWriter = &buf
	defer func() { uiReader, uiWriter = oldR, oldW }()

	got, err := ChooseResult(results)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Source != "b:2" {
		t.Fatalf("选择结果错误: %+v", got)
	}
}

func TestChooseResultCancel(t *testing.T) {
	results := []Result{{Source: "a:1"}}
	oldR, oldW := uiReader, uiWriter
	uiReader = strings.NewReader("0\n")
	var buf strings.Builder
	uiWriter = &buf
	defer func() { uiReader, uiWriter = oldR, oldW }()

	got, err := ChooseResult(results)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("取消时应返回 nil,实际 %+v", got)
	}
}

func TestConfirm(t *testing.T) {
	oldR, oldW := uiReader, uiWriter
	defer func() { uiReader, uiWriter = oldR, oldW }()

	uiReader = strings.NewReader("y\n")
	if ok, err := Confirm("rename?"); err != nil || !ok {
		t.Fatalf("y 应返回 true,ok=%v err=%v", ok, err)
	}
	uiReader = strings.NewReader("N\n")
	if ok, err := Confirm("rename?"); err != nil || ok {
		t.Fatalf("N 应返回 false,ok=%v err=%v", ok, err)
	}
	uiReader = strings.NewReader("whatever\n")
	if ok, err := Confirm("rename?"); err != nil || ok {
		t.Fatalf("其他输入应返回 false,ok=%v err=%v", ok, err)
	}
}

// TestSharedLineReaderAcrossPrompts 模拟管道输入多行时的真实流程:
// 先候选选择,再重命名确认,第二个回答不能被第一个 Reader 提前缓冲吞掉。
func TestSharedLineReaderAcrossPrompts(t *testing.T) {
	oldR, oldW := uiReader, uiWriter
	uiReader = strings.NewReader("2\ny\n")
	var buf strings.Builder
	uiWriter = &buf
	defer func() { uiReader, uiWriter = oldR, oldW }()

	results := []Result{{Source: "a:1"}, {Source: "b:2"}}
	chosen, err := ChooseResult(results)
	if err != nil {
		t.Fatal(err)
	}
	if chosen == nil || chosen.Source != "b:2" {
		t.Fatalf("选择结果错误: %+v", chosen)
	}
	ok, err := Confirm("rename?")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("共享缓冲后,第二个输入 y 应被 Confirm 读到")
	}
}
