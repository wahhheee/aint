// Package ids 生成项目内所有实体的主键。
//
// 统一使用 UUID v7（标准库 uuid）：时间有序，可直接按主键排序与分页；以标准文本形式存储。
package ids

import "uuid"

// New 返回一个新的 UUID v7 文本。
func New() string {
	return uuid.NewV7().String()
}

// Valid 判断 s 是否为合法的 UUID 文本（用于校验路径参数，避免无意义的数据库查询）。
func Valid(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil
}
