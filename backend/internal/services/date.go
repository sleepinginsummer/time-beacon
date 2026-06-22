package services

import "time"

// CalcEndDate 按产品规则计算结束日期：自然月/年遇到目标日期不存在时，使用目标月份最后一天。
func CalcEndDate(start time.Time, typ string, value int) time.Time {
	switch typ {
	case "day":
		return start.AddDate(0, 0, value)
	case "month":
		return addMonthClamped(start, value)
	case "year":
		return addMonthClamped(start, value*12)
	default:
		return start
	}
}

func addMonthClamped(start time.Time, months int) time.Time {
	y, m, d := start.Date()
	first := time.Date(y, m, 1, 0, 0, 0, 0, start.Location()).AddDate(0, months, 0)
	lastDay := time.Date(first.Year(), first.Month()+1, 0, 0, 0, 0, 0, start.Location()).Day()
	if d > lastDay { d = lastDay }
	return time.Date(first.Year(), first.Month(), d, 0, 0, 0, 0, start.Location())
}
