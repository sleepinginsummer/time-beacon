package services

import (
	"crypto/tls"
	"database/sql"
	"fmt"
	"log"
	"net"
	"net/smtp"
	"os"
	"strings"
	"time"
)

// StartNotificationScheduler 启动固定时刻任务：凌晨维护数据，早上发送邮件。
func StartNotificationScheduler(db *sql.DB) {
	// 启动阶段不发送邮件，避免发布、重启服务时重复触发当天提醒。
	go runDailyAt(1, 0, func(now time.Time) {
		if err := RunDailyMaintenanceScan(db, now); err != nil {
			log.Printf("每日维护扫描失败: %v", err)
		}
	})
	go runDailyAt(8, 0, func(now time.Time) {
		if err := RunNotificationScan(db, now); err != nil {
			log.Printf("每日邮件通知扫描失败: %v", err)
		}
	})
}

// runDailyAt 按服务器本地时区每天固定时刻运行任务。
func runDailyAt(hour, minute int, job func(time.Time)) {
	for {
		now := time.Now()
		next := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())
		if !next.After(now) {
			next = next.AddDate(0, 0, 1)
		}
		time.Sleep(time.Until(next))
		job(time.Now())
	}
}

// RunDailyMaintenanceScan 每天凌晨维护日期类数据，不发送邮件。
func RunDailyMaintenanceScan(db *sql.DB, now time.Time) error {
	cleanupNotificationLogs(db, now)
	if err := advanceExpiredAnnualReminders(db, now, 0); err != nil {
		return err
	}
	return advanceAutoRenewSubscriptions(db, now, 0)
}

// RunNotificationScan 根据用户通知规则扫描到期订阅，使用 notification_logs 防止重复发送。
func RunNotificationScan(db *sql.DB, now time.Time) error {
	cleanupNotificationLogs(db, now)
	return runEmailNotificationScan(db, now, 0)
}

// RunStartupNotificationScan 仅用于服务重启后的启动阶段扫描。
// 启动阶段使用 startup 渠道单独记录，避免容器重启时重复发送；日常定时和手动触发仍按 email 渠道独立判断。
func RunStartupNotificationScan(db *sql.DB, now time.Time) error {
	cleanupNotificationLogs(db, now)
	return runNotificationScanWithChannel(db, now, 0, "startup", false)
}

// RunNotificationScanForUser 仅扫描指定用户，用于页面“立刻检查通知”。
func RunNotificationScanForUser(db *sql.DB, now time.Time, targetUserID int64) error {
	cleanupNotificationLogs(db, now)
	// 页面手动触发用于人工补发/测试，跳过当天重复发送检查；定时任务仍保留去重。
	return runNotificationScanWithChannel(db, now, targetUserID, "manual", true)
}

type notificationCandidate struct {
	RuleID      int64
	UserID      int64
	Days        int
	SubID       int64
	SubName     string
	RenewalDate string
	DaysLeft    int
}

type notificationGroup struct {
	UserID int64
	Items  []notificationCandidate
}

// advanceExpiredAnnualReminders 将生日、纪念日这类年度提醒滚动到下一次年份。
// 只处理 next_renewal_date < 今天的记录，等于今天时不重置，保证当天提醒仍能发送。
func advanceExpiredAnnualReminders(db *sql.DB, now time.Time, targetUserID int64) error {
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	query := `select distinct s.id,date_format(s.next_renewal_date,'%Y-%m-%d')
from subscriptions s
join subscription_tags st on st.subscription_id=s.id and st.user_id=s.user_id and st.deleted=0
join tags t on t.id=st.tag_id and t.user_id=s.user_id and t.deleted=0 and t.name in ('生日','纪念日')
where s.deleted=0 and s.status='active' and s.reminder_enabled=1 and s.next_renewal_date < date(?)`
	args := []any{today.Format("2006-01-02")}
	if targetUserID > 0 {
		query += " and s.user_id=?"
		args = append(args, targetUserID)
	}
	rows, err := db.Query(query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	type renewal struct {
		id   int64
		date string
	}
	renewals := []renewal{}
	for rows.Next() {
		var r renewal
		if err := rows.Scan(&r.id, &r.date); err != nil {
			return err
		}
		renewals = append(renewals, r)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, r := range renewals {
		d, err := time.ParseInLocation("2006-01-02", r.date, now.Location())
		if err != nil {
			return err
		}
		for d.Before(today) {
			d = d.AddDate(1, 0, 0)
		}
		if _, err := db.Exec("update subscriptions set end_date=?,next_renewal_date=? where id=?", d.Format("2006-01-02"), d.Format("2006-01-02"), r.id); err != nil {
			return err
		}
	}
	return nil
}

// advanceAutoRenewSubscriptions 将开启自动续期且已经过期的普通订阅按原周期顺延。
// 只处理 next_renewal_date < 今天的记录，避免到期当天提前滚走导致当天提醒丢失。
func advanceAutoRenewSubscriptions(db *sql.DB, now time.Time, targetUserID int64) error {
	loc := now.Location()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	query := `select id,period_type,period_value,date_format(start_date,'%Y-%m-%d'),date_format(end_date,'%Y-%m-%d') from subscriptions where deleted=0 and status='active' and auto_renew=1 and next_renewal_date < date(?)`
	args := []any{today.Format("2006-01-02")}
	if targetUserID > 0 {
		query += " and user_id=?"
		args = append(args, targetUserID)
	}
	rows, err := db.Query(query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	type item struct {
		id    int64
		typ   string
		val   int
		start string
		end   string
	}
	items := []item{}
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.id, &it.typ, &it.val, &it.start, &it.end); err != nil {
			return err
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, it := range items {
		start, err := time.ParseInLocation("2006-01-02", it.start, loc)
		if err != nil {
			return err
		}
		end, err := time.ParseInLocation("2006-01-02", it.end, loc)
		if err != nil {
			return err
		}
		if it.val <= 0 {
			it.val = 1
		}
		for end.Before(today) {
			switch it.typ {
			case "year":
				start = start.AddDate(it.val, 0, 0)
				end = end.AddDate(it.val, 0, 0)
			case "month":
				start = start.AddDate(0, it.val, 0)
				end = end.AddDate(0, it.val, 0)
			default:
				start = start.AddDate(0, 0, it.val)
				end = end.AddDate(0, 0, it.val)
			}
		}
		if _, err := db.Exec("update subscriptions set start_date=?,end_date=?,next_renewal_date=? where id=?", start.Format("2006-01-02"), end.Format("2006-01-02"), end.Format("2006-01-02"), it.id); err != nil {
			return err
		}
	}
	return nil
}

func runEmailNotificationScan(db *sql.DB, now time.Time, targetUserID int64) error {
	return runNotificationScanWithChannel(db, now, targetUserID, "email", false)
}

func runNotificationScanWithChannel(db *sql.DB, now time.Time, targetUserID int64, logChannel string, skipDuplicateCheck bool) error {
	// days_before 表示提醒窗口：7 表示到期前 7 天内每天提醒，3 同理。
	// 多个窗口重叠时只选最小可匹配窗口，避免同一订阅同一天重复进入多个规则。
	query := `select r.id,r.user_id,r.days_before,s.id,s.name,date_format(s.next_renewal_date,'%Y-%m-%d')
from notification_rules r
join subscriptions s on s.user_id=r.user_id and s.deleted=0 and s.status='active' and s.reminder_enabled=1
where r.enabled=1 and r.deleted=0
  and s.next_renewal_date between date(?) and date_add(date(?), interval r.days_before day)
  and not exists (
    select 1 from notification_rules r2
    where r2.user_id=r.user_id and r2.enabled=1 and r2.deleted=0
      and r2.days_before < r.days_before
      and s.next_renewal_date between date(?) and date_add(date(?), interval r2.days_before day)
  )`
	today := now.Format("2006-01-02")
	args := []any{today, today, today, today}
	if targetUserID > 0 {
		query += " and r.user_id=?"
		args = append(args, targetUserID)
	}
	rows, err := db.Query(query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	groups := map[string]*notificationGroup{}
	for rows.Next() {
		var c notificationCandidate
		if err := rows.Scan(&c.RuleID, &c.UserID, &c.Days, &c.SubID, &c.SubName, &c.RenewalDate); err != nil {
			return err
		}
		if !skipDuplicateCheck && alreadyNotified(db, c.UserID, c.SubID, c.RuleID, now, logChannel) {
			continue
		}
		c.DaysLeft = daysUntil(now, c.RenewalDate)
		key := fmt.Sprintf("%d", c.UserID)
		if groups[key] == nil {
			groups[key] = &notificationGroup{UserID: c.UserID}
		}
		groups[key].Items = append(groups[key].Items, c)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, g := range groups {
		if len(g.Items) == 0 {
			continue
		}
		emails, err := enabledEmails(db, g.UserID)
		if err != nil {
			return err
		}
		status := "success"
		errMsg := ""
		if len(emails) == 0 {
			status = "failed"
			errMsg = "没有已启用通知邮箱"
		} else if err := sendEmail(emails, "订阅续费提醒", buildNotificationBody(*g)); err != nil {
			status = "failed"
			errMsg = err.Error()
		}
		for _, item := range g.Items {
			_, _ = db.Exec("insert ignore into notification_logs(user_id,subscription_id,notification_rule_id,notify_date,channel,status,error_message) values(?,?,?,?,?,?,?)", item.UserID, item.SubID, item.RuleID, now.Format("2006-01-02"), logChannel, status, errMsg)
		}
	}
	return nil
}

func buildNotificationBody(g notificationGroup) string {
	lines := []string{"以下订阅即将到期：", ""}
	for _, item := range g.Items {
		leftText := fmt.Sprintf("还有 %d 天到期", item.DaysLeft)
		if item.DaysLeft == 0 {
			leftText = "今天到期"
		}
		lines = append(lines, fmt.Sprintf("- %s（%s，续费日 %s）", item.SubName, leftText, item.RenewalDate))
	}
	return strings.Join(lines, "\n")
}

func daysUntil(now time.Time, renewalDate string) int {
	renewal, err := time.ParseInLocation("2006-01-02", renewalDate, now.Location())
	if err != nil {
		return 0
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	return int(renewal.Sub(today).Hours() / 24)
}

func cleanupNotificationLogs(db *sql.DB, now time.Time) {
	// 通知日志只保留最近 10 天，避免长期运行后日志表持续膨胀。
	_, err := db.Exec("delete from notification_logs where created_at < ?", now.AddDate(0, 0, -10))
	if err != nil {
		log.Printf("通知日志清理失败: %v", err)
	}
}

func alreadyNotified(db *sql.DB, userID, subID, ruleID int64, now time.Time, logChannel string) bool {
	var n int
	// 同一天同一订阅同一渠道只发一次；不按规则 ID 区分，避免 7 天和 3 天窗口重叠时重复发送。
	_ = db.QueryRow("select count(*) from notification_logs where user_id=? and subscription_id=? and notify_date=? and channel=? and status='success'", userID, subID, now.Format("2006-01-02"), logChannel).Scan(&n)
	return n > 0
}

func enabledEmails(db *sql.DB, userID int64) ([]string, error) {
	rows, err := db.Query("select email from notification_emails where user_id=? and verified=1 and enabled=1 and deleted=0", userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var email string
		rows.Scan(&email)
		out = append(out, email)
	}
	return out, rows.Err()
}

func sendEmail(to []string, subject, body string) error {
	host := os.Getenv("SMTP_HOST")
	port := os.Getenv("SMTP_PORT")
	user := os.Getenv("SMTP_USER")
	pass := os.Getenv("SMTP_PASSWORD")
	from := os.Getenv("SMTP_FROM")
	if host == "" || port == "" || from == "" {
		return fmt.Errorf("SMTP 未配置")
	}
	addr := host + ":" + port
	msg := []byte("To: " + strings.Join(to, ",") + "\r\nSubject: " + subject + "\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n" + body)
	var auth smtp.Auth
	if user != "" || pass != "" {
		auth = smtp.PlainAuth("", user, pass, host)
	}
	if port == "465" {
		return sendEmailTLS(addr, host, auth, from, to, msg)
	}
	dialer := net.Dialer{Timeout: 10 * time.Second}
	conn, err := dialer.Dial("tcp", addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	c, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	defer c.Close()
	if auth != nil {
		if err := c.Auth(auth); err != nil {
			return err
		}
	}
	return sendSMTPMessage(c, from, to, msg)
}

func sendEmailTLS(addr, host string, auth smtp.Auth, from string, to []string, msg []byte) error {
	dialer := net.Dialer{Timeout: 10 * time.Second}
	conn, err := tls.DialWithDialer(&dialer, "tcp", addr, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
	if err != nil {
		return err
	}
	defer conn.Close()
	c, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	defer c.Close()
	if auth != nil {
		if err := c.Auth(auth); err != nil {
			return err
		}
	}
	return sendSMTPMessage(c, from, to, msg)
}

func sendSMTPMessage(c *smtp.Client, from string, to []string, msg []byte) error {
	if err := c.Mail(from); err != nil {
		return err
	}
	for _, addr := range to {
		if err := c.Rcpt(addr); err != nil {
			return err
		}
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		w.Close()
		return err
	}
	return w.Close()
}
