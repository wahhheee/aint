package main

import (
	"fmt"

	"github.com/wahhheee/yanyi/internal/buildinfo"
	"github.com/wahhheee/yanyi/internal/prompts"
)

func runVersion([]string) error {
	bi := buildinfo.Get()
	fmt.Printf("yanyi %s\n", bi.Version)
	fmt.Printf("  commit:   %s%s\n", bi.Commit, map[bool]string{true: "（含未提交改动）"}[bi.Modified])
	fmt.Printf("  built:    %s\n", bi.Date)
	fmt.Printf("  go:       %s %s\n", bi.GoVersion, bi.Platform)
	fmt.Printf("  prompts:  %s\n", prompts.Version)
	return nil
}
