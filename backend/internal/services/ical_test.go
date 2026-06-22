package services

import (
	"strings"
	"testing"
	"time"
)

func TestBuildICalendar(t *testing.T) {
	d, _ := time.Parse("2006-01-02", "2026-07-07")
	ics := BuildICalendar([]CalendarEvent{{ID: 1, Name: "ChatGPT, Plus", Description: "AI\n工具", Date: d}}, []int{0, 3})
	checks := []string{
		"BEGIN:VCALENDAR",
		"UID:subscription-1@subscription-management",
		"SUMMARY:订阅续费提醒：ChatGPT\\, Plus",
		"DESCRIPTION:AI\\n工具",
		"DTSTART;VALUE=DATE:20260707",
		"DTEND;VALUE=DATE:20260708",
		"TRIGGER:-P0D",
		"TRIGGER:-P3D",
		"END:VCALENDAR",
	}
	for _, c := range checks {
		if !strings.Contains(ics, c) {
			t.Fatalf("ics missing %s in:\n%s", c, ics)
		}
	}
}
