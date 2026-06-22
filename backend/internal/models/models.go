package models

type User struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Role     string `json:"role"`
}
type Subscription struct {
	ID              int64   `json:"id"`
	Name            string  `json:"name"`
	Note            string  `json:"note"`
	PeriodType      string  `json:"periodType"`
	PeriodValue     int     `json:"periodValue"`
	StartDate       string  `json:"startDate"`
	EndDate         string  `json:"endDate"`
	NextRenewalDate string  `json:"nextRenewalDate"`
	Status          string  `json:"status"`
	BudgetRmb       float64 `json:"budgetRmb"`
	BudgetCurrency  string  `json:"budgetCurrency"`
	RenewalURL      string  `json:"renewalUrl"`
	ReminderEnabled bool    `json:"reminderEnabled"`
	AutoRenew       bool    `json:"autoRenew"`
	TagIDs          []int64 `json:"tagIds,omitempty"`
	Tags            []Tag   `json:"tags,omitempty"`
}
type Tag struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}
type CalendarLink struct {
	ID           int64   `json:"id"`
	Remark       string  `json:"remark"`
	URL          string  `json:"url"`
	Enabled      bool    `json:"enabled"`
	LastAccessAt *string `json:"lastAccessAt"`
	CreatedAt    string  `json:"createdAt"`
}
type NotificationEmail struct {
	ID       int64  `json:"id"`
	Email    string `json:"email"`
	Verified bool   `json:"verified"`
	Enabled  bool   `json:"enabled"`
}
type NotificationRule struct {
	ID         int64  `json:"id"`
	DaysBefore int    `json:"daysBefore"`
	Channel    string `json:"channel"`
	Enabled    bool   `json:"enabled"`
}
