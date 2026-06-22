package services

import (
	"fmt"
	"strings"
	"time"
)

// CalendarEvent 表示一个日历全天续费事件。
type CalendarEvent struct {
	ID          int64
	Name        string
	Description string
	Date        time.Time
}

// BuildICalendar 根据订阅事件和提前提醒天数生成 iCalendar 内容。
func BuildICalendar(events []CalendarEvent, daysBefore []int) string {
	var b strings.Builder
	b.WriteString("BEGIN:VCALENDAR\nVERSION:2.0\nPRODID:-//Subscription Management//CN\n")
	for _, e := range events {
		startText := e.Date.Format("20060102")
		endText := e.Date.AddDate(0, 0, 1).Format("20060102")
		fmt.Fprintf(&b, "BEGIN:VEVENT\nUID:subscription-%d@subscription-management\nSUMMARY:订阅续费提醒：%s\nDESCRIPTION:%s\nDTSTART;VALUE=DATE:%s\nDTEND;VALUE=DATE:%s\n", e.ID, EscapeICalText(e.Name), EscapeICalText(e.Description), startText, endText)
		for _, d := range daysBefore {
			fmt.Fprintf(&b, "BEGIN:VALARM\nTRIGGER:-P%dD\nACTION:DISPLAY\nDESCRIPTION:%s 续费提醒\nEND:VALARM\n", d, EscapeICalText(e.Name))
		}
		b.WriteString("END:VEVENT\n")
	}
	b.WriteString("END:VCALENDAR\n")
	return b.String()
}

// EscapeICalText 转义 iCalendar 文本字段，避免换行和逗号破坏格式。
func EscapeICalText(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, ",", "\\,")
	return s
}
