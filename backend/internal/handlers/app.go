package handlers

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"net/http"
	"strconv"
	"strings"
	"subscription-management/backend/internal/config"
	"subscription-management/backend/internal/models"
	"subscription-management/backend/internal/services"
	"time"
)

type App struct {
	DB       *sql.DB
	Config   config.Config
	Exchange *services.ExchangeService
}
type resp struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func (a *App) JSON(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json;charset=utf-8")
	json.NewEncoder(w).Encode(resp{Code: 0, Message: "ok", Data: data})
}
func Err(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json;charset=utf-8")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(resp{Code: code, Message: msg})
}
func read(r *http.Request, v any) error {
	if r.Body == nil {
		return nil
	}
	return json.NewDecoder(r.Body).Decode(v)
}
func userID(r *http.Request) int64 { v, _ := r.Context().Value("uid").(int64); return v }
func role(r *http.Request) string  { v, _ := r.Context().Value("role").(string); return v }
func (a *App) token(u models.User) (string, error) {
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"uid": u.ID, "role": u.Role, "exp": time.Now().Add(7 * 24 * time.Hour).Unix()})
	return t.SignedString([]byte(a.Config.JWTSecret))
}

func (a *App) RegisterStatus(w http.ResponseWriter, r *http.Request) {
	var v string
	a.DB.QueryRow("select setting_value from system_settings where setting_key='register_enabled'").Scan(&v)
	a.JSON(w, map[string]any{"registerEnabled": v == "true"})
}

func (a *App) Register(w http.ResponseWriter, r *http.Request) {
	var q struct{ Username, Password string }
	if read(r, &q) != nil || q.Username == "" || q.Password == "" {
		Err(w, 400, "用户名和密码必填")
		return
	}
	var cnt int
	a.DB.QueryRow("select count(*) from users where deleted=0").Scan(&cnt)
	if cnt > 0 {
		var en string
		a.DB.QueryRow("select setting_value from system_settings where setting_key='register_enabled'").Scan(&en)
		if en != "true" {
			Err(w, 403, "注册已关闭")
			return
		}
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte(q.Password), bcrypt.DefaultCost)
	role := "user"
	if cnt == 0 {
		role = "admin"
	}
	res, err := a.DB.Exec("insert into users(username,password_hash,role) values(?,?,?)", q.Username, string(hash), role)
	if err != nil {
		Err(w, 400, "用户名已存在")
		return
	}
	id, _ := res.LastInsertId()
	tok, _ := a.token(models.User{ID: id, Username: q.Username, Role: role})
	a.JSON(w, map[string]any{"token": tok, "user": models.User{ID: id, Username: q.Username, Role: role}})
}
func (a *App) Login(w http.ResponseWriter, r *http.Request) {
	var q struct{ Username, Password string }
	read(r, &q)
	var u models.User
	var h string
	if a.DB.QueryRow("select id,username,password_hash,role from users where username=? and deleted=0", q.Username).Scan(&u.ID, &u.Username, &h, &u.Role) != nil || bcrypt.CompareHashAndPassword([]byte(h), []byte(q.Password)) != nil {
		Err(w, 401, "用户名或密码错误")
		return
	}
	tok, _ := a.token(u)
	a.JSON(w, map[string]any{"token": tok, "user": u})
}
func (a *App) Me(w http.ResponseWriter, r *http.Request) {
	var username, dbRole string
	if err := a.DB.QueryRow("select username,role from users where id=? and deleted=0", userID(r)).Scan(&username, &dbRole); err != nil {
		a.JSON(w, map[string]any{"id": userID(r), "role": role(r)})
		return
	}
	a.JSON(w, map[string]any{"id": userID(r), "username": username, "role": dbRole})
}

func (a *App) SubCreate(w http.ResponseWriter, r *http.Request) {
	var q models.Subscription
	read(r, &q)
	start, err := time.Parse("2006-01-02", q.StartDate)
	if err != nil || q.Name == "" || q.PeriodValue <= 0 {
		Err(w, 400, "订阅名称、开始日期和周期必填")
		return
	}
	if q.EndDate == "" {
		q.EndDate = services.CalcEndDate(start, q.PeriodType, q.PeriodValue).Format("2006-01-02")
	}
	q.NextRenewalDate = q.EndDate
	if q.Status == "" {
		q.Status = "active"
	}
	if a.hasBuiltinTag(userID(r), q.TagIDs) {
		q.PeriodType = "year"
		q.PeriodValue = 1
		q.EndDate = calcAnnualReminderDate(q.StartDate, time.Now())
		q.NextRenewalDate = q.EndDate
		q.BudgetRmb = 0
		q.BudgetCurrency = "CNY"
		q.RenewalURL = ""
	}
	if q.BudgetCurrency == "" {
		q.BudgetCurrency = "CNY"
	}
	q.BudgetCurrency = services.NormalizeCurrency(q.BudgetCurrency)
	res, err := a.DB.Exec("insert into subscriptions(user_id,name,note,period_type,period_value,start_date,end_date,next_renewal_date,status,budget_rmb,budget_currency,renewal_url,reminder_enabled,auto_renew) values(?,?,?,?,?,?,?,?,?,?,?,?,?,?)", userID(r), q.Name, q.Note, q.PeriodType, q.PeriodValue, q.StartDate, q.EndDate, q.NextRenewalDate, q.Status, q.BudgetRmb, q.BudgetCurrency, q.RenewalURL, q.ReminderEnabled, q.AutoRenew)
	if err != nil {
		Err(w, 500, err.Error())
		return
	}
	id, _ := res.LastInsertId()
	a.saveSubscriptionTags(userID(r), id, q.TagIDs)
	a.JSON(w, map[string]any{"id": id})
}
func (a *App) SubList(w http.ResponseWriter, r *http.Request) {
	rows, err := a.DB.Query("select id,name,coalesce(note,''),period_type,period_value,date_format(start_date,'%Y-%m-%d'),date_format(end_date,'%Y-%m-%d'),date_format(next_renewal_date,'%Y-%m-%d'),status,coalesce(budget_rmb,0),coalesce(budget_currency,'CNY'),coalesce(renewal_url,''),reminder_enabled,auto_renew from subscriptions where user_id=? and deleted=0 order by next_renewal_date", userID(r))
	if err != nil {
		Err(w, 500, err.Error())
		return
	}
	defer rows.Close()
	list := []models.Subscription{}
	for rows.Next() {
		var s models.Subscription
		rows.Scan(&s.ID, &s.Name, &s.Note, &s.PeriodType, &s.PeriodValue, &s.StartDate, &s.EndDate, &s.NextRenewalDate, &s.Status, &s.BudgetRmb, &s.BudgetCurrency, &s.RenewalURL, &s.ReminderEnabled, &s.AutoRenew)
		s.Tags, s.TagIDs = a.subscriptionTags(userID(r), s.ID)
		list = append(list, s)
	}
	a.JSON(w, list)
}
func (a *App) SubUpdate(w http.ResponseWriter, r *http.Request) {
	var q models.Subscription
	read(r, &q)
	if q.ID == 0 {
		Err(w, 400, "id必填")
		return
	}
	start, _ := time.Parse("2006-01-02", q.StartDate)
	if q.EndDate == "" {
		q.EndDate = services.CalcEndDate(start, q.PeriodType, q.PeriodValue).Format("2006-01-02")
	}
	if a.hasBuiltinTag(userID(r), q.TagIDs) {
		q.PeriodType = "year"
		q.PeriodValue = 1
		q.EndDate = calcAnnualReminderDate(q.StartDate, time.Now())
		q.NextRenewalDate = q.EndDate
		q.BudgetRmb = 0
		q.BudgetCurrency = "CNY"
		q.RenewalURL = ""
	}
	if q.BudgetCurrency == "" {
		q.BudgetCurrency = "CNY"
	}
	q.BudgetCurrency = services.NormalizeCurrency(q.BudgetCurrency)
	_, err := a.DB.Exec("update subscriptions set name=?,note=?,period_type=?,period_value=?,start_date=?,end_date=?,next_renewal_date=?,status=?,budget_rmb=?,budget_currency=?,renewal_url=?,reminder_enabled=?,auto_renew=? where id=? and user_id=? and deleted=0", q.Name, q.Note, q.PeriodType, q.PeriodValue, q.StartDate, q.EndDate, q.EndDate, q.Status, q.BudgetRmb, q.BudgetCurrency, q.RenewalURL, q.ReminderEnabled, q.AutoRenew, q.ID, userID(r))
	if err != nil {
		Err(w, 500, err.Error())
		return
	}
	a.saveSubscriptionTags(userID(r), q.ID, q.TagIDs)
	a.JSON(w, true)
}
func (a *App) SubDelete(w http.ResponseWriter, r *http.Request) {
	var q struct{ ID int64 }
	read(r, &q)
	a.DB.Exec("update subscriptions set deleted=1 where id=? and user_id=?", q.ID, userID(r))
	a.JSON(w, true)
}
func (a *App) SubReset(w http.ResponseWriter, r *http.Request) {
	var q struct {
		ID                 int64
		PeriodType         string
		PeriodValue        int
		StartDate, EndDate string
	}
	read(r, &q)
	var typ string
	var val int
	a.DB.QueryRow("select period_type,period_value from subscriptions where id=? and user_id=? and deleted=0", q.ID, userID(r)).Scan(&typ, &val)
	if q.PeriodType != "" {
		typ = q.PeriodType
	}
	if q.PeriodValue > 0 {
		val = q.PeriodValue
	}
	if q.StartDate == "" {
		q.StartDate = time.Now().Format("2006-01-02")
	}
	st, _ := time.Parse("2006-01-02", q.StartDate)
	if q.EndDate == "" {
		q.EndDate = services.CalcEndDate(st, typ, val).Format("2006-01-02")
	}
	a.DB.Exec("update subscriptions set period_type=?,period_value=?,start_date=?,end_date=?,next_renewal_date=? where id=? and user_id=?", typ, val, q.StartDate, q.EndDate, q.EndDate, q.ID, userID(r))
	a.JSON(w, true)
}
func (a *App) ExportCSV(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=\"subscriptions.csv\"")
	cw := csv.NewWriter(w)
	cw.Write([]string{"订阅名称", "标签", "预算金额", "预算币种", "换算RMB", "续费链接", "备注", "周期类型", "周期值", "开始日期", "结束日期", "下次续费日期", "状态"})
	rows, _ := a.DB.Query("select id,name,coalesce(note,''),period_type,period_value,date_format(start_date,'%Y-%m-%d'),date_format(end_date,'%Y-%m-%d'),date_format(next_renewal_date,'%Y-%m-%d'),status,coalesce(budget_rmb,0),coalesce(budget_currency,'CNY'),coalesce(renewal_url,'') from subscriptions where user_id=? and deleted=0", userID(r))
	defer rows.Close()
	for rows.Next() {
		var id int64
		var name, note, typ, sd, ed, nd, st, currency, renewalURL string
		var pv int
		var budget float64
		rows.Scan(&id, &name, &note, &typ, &pv, &sd, &ed, &nd, &st, &budget, &currency, &renewalURL)
		rmb, _, err := a.exchange().ConvertToCNY(budget, currency)
		if err != nil {
			rmb = 0
		}
		tags, _ := a.subscriptionTags(userID(r), id)
		tagNames := []string{}
		for _, t := range tags {
			tagNames = append(tagNames, t.Name)
		}
		cw.Write([]string{name, strings.Join(tagNames, ","), fmt.Sprintf("%.2f", budget), services.NormalizeCurrency(currency), fmt.Sprintf("%.2f", rmb), renewalURL, note, typ, strconv.Itoa(pv), sd, ed, nd, st})
	}
	cw.Flush()
}

func (a *App) saveSubscriptionTags(uid, sid int64, tagIDs []int64) {
	a.DB.Exec("update subscription_tags set deleted=1 where user_id=? and subscription_id=?", uid, sid)
	for _, tid := range tagIDs {
		a.DB.Exec("insert into subscription_tags(user_id,subscription_id,tag_id) values(?,?,?)", uid, sid, tid)
	}
}

func (a *App) subscriptionTags(uid, sid int64) ([]models.Tag, []int64) {
	rows, err := a.DB.Query("select t.id,t.name,coalesce(t.color,'') from subscription_tags st join tags t on t.id=st.tag_id and t.deleted=0 where st.user_id=? and st.subscription_id=? and st.deleted=0", uid, sid)
	if err != nil {
		return []models.Tag{}, []int64{}
	}
	defer rows.Close()
	tags := []models.Tag{}
	ids := []int64{}
	for rows.Next() {
		var t models.Tag
		rows.Scan(&t.ID, &t.Name, &t.Color)
		tags = append(tags, t)
		ids = append(ids, t.ID)
	}
	return tags, ids
}

func (a *App) exchange() *services.ExchangeService {
	if a.Exchange == nil {
		a.Exchange = services.NewExchangeService()
	}
	return a.Exchange
}

func (a *App) ExchangeConvertBatch(w http.ResponseWriter, r *http.Request) {
	var q []struct {
		ID       int64   `json:"id"`
		Amount   float64 `json:"amount"`
		Currency string  `json:"currency"`
	}
	if err := read(r, &q); err != nil {
		Err(w, 400, "请求参数错误")
		return
	}
	out := []map[string]any{}
	for _, item := range q {
		currency := services.NormalizeCurrency(item.Currency)
		rmb, rate, err := a.exchange().ConvertToCNY(item.Amount, currency)
		row := map[string]any{"id": item.ID, "amount": item.Amount, "currency": currency}
		if err != nil {
			row["error"] = err.Error()
		} else {
			row["rmb"] = rmb
			row["rate"] = rate
		}
		out = append(out, row)
	}
	a.JSON(w, out)
}

func requestBaseURL(r *http.Request) string {
	// 日历订阅链接按当前请求动态生成，避免部署环境变更后仍复制出旧 IP 或旧域名。
	scheme := r.Header.Get("X-Forwarded-Proto")
	if scheme == "" {
		if r.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}
	host := r.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = r.Host
	}
	return scheme + "://" + host
}

func randToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return strings.TrimRight(base64.URLEncoding.EncodeToString(b), "=")
}
func (a *App) LinkCreate(w http.ResponseWriter, r *http.Request) {
	var q struct{ Remark string }
	read(r, &q)
	tok := randToken()
	res, err := a.DB.Exec("insert into calendar_subscribe_links(user_id,token,remark) values(?,?,?)", userID(r), tok, q.Remark)
	if err != nil {
		Err(w, 500, err.Error())
		return
	}
	id, _ := res.LastInsertId()
	a.JSON(w, map[string]any{"id": id})
}
func (a *App) LinkList(w http.ResponseWriter, r *http.Request) {
	rows, _ := a.DB.Query("select id,coalesce(remark,''),token,enabled,coalesce(date_format(last_access_at,'%Y-%m-%d %H:%i:%s'),''),date_format(created_at,'%Y-%m-%d %H:%i:%s') from calendar_subscribe_links where user_id=? and deleted=0 order by id desc", userID(r))
	defer rows.Close()
	list := []models.CalendarLink{}
	for rows.Next() {
		var l models.CalendarLink
		var tok, last string
		rows.Scan(&l.ID, &l.Remark, &tok, &l.Enabled, &last, &l.CreatedAt)
		l.URL = fmt.Sprintf("%s/calendar/subscribe/%s.ics", requestBaseURL(r), tok)
		if last != "" {
			l.LastAccessAt = &last
		}
		list = append(list, l)
	}
	a.JSON(w, list)
}
func (a *App) LinkAction(w http.ResponseWriter, r *http.Request, action string) {
	var q struct {
		ID     int64
		Remark string
	}
	read(r, &q)
	switch action {
	case "delete":
		a.DB.Exec("update calendar_subscribe_links set deleted=1 where id=? and user_id=?", q.ID, userID(r))
	case "disable":
		a.DB.Exec("update calendar_subscribe_links set enabled=0 where id=? and user_id=?", q.ID, userID(r))
	case "enable":
		a.DB.Exec("update calendar_subscribe_links set enabled=1 where id=? and user_id=?", q.ID, userID(r))
	case "reset":
		a.DB.Exec("update calendar_subscribe_links set token=? where id=? and user_id=?", randToken(), q.ID, userID(r))
	case "remark":
		a.DB.Exec("update calendar_subscribe_links set remark=? where id=? and user_id=?", q.Remark, q.ID, userID(r))
	}
	a.JSON(w, true)
}
func (a *App) ICS(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/calendar/subscribe/"), ".ics")
	var uid int64
	if a.DB.QueryRow("select user_id from calendar_subscribe_links where token=? and enabled=1 and deleted=0", token).Scan(&uid) != nil {
		http.NotFound(w, r)
		return
	}
	a.DB.Exec("update calendar_subscribe_links set last_access_at=now() where token=?", token)

	// 读取用户启用的通知规则并转换为日历提醒；邮箱通知仍以通知任务为准，日历提醒只作为客户端辅助。
	ruleRows, _ := a.DB.Query("select days_before from notification_rules where user_id=? and enabled=1 and deleted=0", uid)
	daysBefore := []int{}
	if ruleRows != nil {
		defer ruleRows.Close()
		for ruleRows.Next() {
			var d int
			ruleRows.Scan(&d)
			daysBefore = append(daysBefore, d)
		}
	}

	rows, _ := a.DB.Query("select id,name,coalesce(note,''),coalesce(budget_rmb,0),coalesce(budget_currency,'CNY'),coalesce(renewal_url,''),date_format(next_renewal_date,'%Y-%m-%d') from subscriptions where user_id=? and deleted=0 and status='active' and reminder_enabled=1", uid)
	defer rows.Close()
	events := []services.CalendarEvent{}
	for rows.Next() {
		var id int64
		var name, note, currency, renewalURL, dateText string
		var budget float64
		rows.Scan(&id, &name, &note, &budget, &currency, &renewalURL, &dateText)
		start, _ := time.Parse("2006-01-02", dateText)
		events = append(events, services.CalendarEvent{ID: id, Name: name, Description: fmt.Sprintf("%s\n预算：%.2f %s\n续费链接：%s", note, budget, services.NormalizeCurrency(currency), renewalURL), Date: start})
	}
	w.Header().Set("Content-Type", "text/calendar;charset=utf-8")
	fmt.Fprint(w, services.BuildICalendar(events, daysBefore))
}
func esc(s string) string { return strings.ReplaceAll(strings.ReplaceAll(s, "\n", "\\n"), ",", "\\,") }

func isBuiltinTagName(name string) bool { return name == "生日" || name == "纪念日" }

func calcAnnualReminderDate(start string, now time.Time) string {
	d, err := time.Parse("2006-01-02", start)
	if err != nil {
		return start
	}
	next := time.Date(now.Year(), d.Month(), d.Day(), 0, 0, 0, 0, now.Location())
	if next.Before(time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())) {
		next = next.AddDate(1, 0, 0)
	}
	return next.Format("2006-01-02")
}

func (a *App) hasBuiltinTag(uid int64, tagIDs []int64) bool {
	for _, tid := range tagIDs {
		var name string
		_ = a.DB.QueryRow("select name from tags where id=? and user_id=? and deleted=0", tid, uid).Scan(&name)
		if isBuiltinTagName(name) {
			return true
		}
	}
	return false
}

func (a *App) ensureBuiltinTags(uid int64) {
	defaults := []models.Tag{{Name: "生日", Color: "#ff6b9a"}, {Name: "纪念日", Color: "#ffb020"}}
	for _, t := range defaults {
		var n int
		_ = a.DB.QueryRow("select count(*) from tags where user_id=? and name=? and deleted=0", uid, t.Name).Scan(&n)
		if n == 0 {
			_, _ = a.DB.Exec("insert into tags(user_id,name,color) values(?,?,?)", uid, t.Name, t.Color)
		}
	}
}

func (a *App) TagList(w http.ResponseWriter, r *http.Request) {
	a.ensureBuiltinTags(userID(r))
	rows, _ := a.DB.Query("select id,name,coalesce(color,'') from tags where user_id=? and deleted=0 order by field(name,'生日','纪念日') desc,id desc", userID(r))
	defer rows.Close()
	out := []models.Tag{}
	for rows.Next() {
		var t models.Tag
		rows.Scan(&t.ID, &t.Name, &t.Color)
		out = append(out, t)
	}
	a.JSON(w, out)
}
func (a *App) TagCreate(w http.ResponseWriter, r *http.Request) {
	var q models.Tag
	read(r, &q)
	if q.Name == "" {
		Err(w, 400, "标签名称必填")
		return
	}
	if isBuiltinTagName(q.Name) {
		Err(w, 400, "生日和纪念日是内置标签，不能重复创建")
		return
	}
	a.DB.Exec("insert into tags(user_id,name,color) values(?,?,?)", userID(r), q.Name, q.Color)
	a.JSON(w, true)
}
func (a *App) TagUpdate(w http.ResponseWriter, r *http.Request) {
	var q models.Tag
	read(r, &q)
	var oldName string
	_ = a.DB.QueryRow("select name from tags where id=? and user_id=? and deleted=0", q.ID, userID(r)).Scan(&oldName)
	if isBuiltinTagName(oldName) {
		a.DB.Exec("update tags set color=? where id=? and user_id=? and deleted=0", q.Color, q.ID, userID(r))
		a.JSON(w, true)
		return
	}
	if q.Name == "" {
		Err(w, 400, "标签名称必填")
		return
	}
	if isBuiltinTagName(q.Name) {
		Err(w, 400, "生日和纪念日是内置标签，不能改成该名称")
		return
	}
	a.DB.Exec("update tags set name=?,color=? where id=? and user_id=? and deleted=0", q.Name, q.Color, q.ID, userID(r))
	a.JSON(w, true)
}
func (a *App) TagDelete(w http.ResponseWriter, r *http.Request) {
	var q struct{ ID int64 }
	read(r, &q)
	var name string
	_ = a.DB.QueryRow("select name from tags where id=? and user_id=? and deleted=0", q.ID, userID(r)).Scan(&name)
	if isBuiltinTagName(name) {
		Err(w, 400, "生日和纪念日是内置标签，不能删除")
		return
	}
	a.DB.Exec("update tags set deleted=1 where id=? and user_id=?", q.ID, userID(r))
	a.JSON(w, true)
}

func (a *App) EmailList(w http.ResponseWriter, r *http.Request) {
	rows, _ := a.DB.Query("select id,email,verified,enabled from notification_emails where user_id=? and deleted=0", userID(r))
	defer rows.Close()
	out := []models.NotificationEmail{}
	for rows.Next() {
		var e models.NotificationEmail
		rows.Scan(&e.ID, &e.Email, &e.Verified, &e.Enabled)
		out = append(out, e)
	}
	a.JSON(w, out)
}
func (a *App) EmailCreate(w http.ResponseWriter, r *http.Request) {
	var q struct{ Email string }
	read(r, &q)
	a.DB.Exec("insert into notification_emails(user_id,email,verified) values(?,?,1)", userID(r), q.Email)
	a.JSON(w, true)
}

func (a *App) EmailAction(w http.ResponseWriter, r *http.Request, action string) {
	var q struct{ ID int64 }
	read(r, &q)
	switch action {
	case "enable":
		a.DB.Exec("update notification_emails set enabled=1 where id=? and user_id=?", q.ID, userID(r))
	case "disable":
		a.DB.Exec("update notification_emails set enabled=0 where id=? and user_id=?", q.ID, userID(r))
	case "delete":
		a.DB.Exec("update notification_emails set deleted=1 where id=? and user_id=?", q.ID, userID(r))
	}
	a.JSON(w, true)
}
func (a *App) RuleList(w http.ResponseWriter, r *http.Request) {
	rows, _ := a.DB.Query("select id,days_before,channel,enabled from notification_rules where user_id=? and deleted=0", userID(r))
	defer rows.Close()
	out := []models.NotificationRule{}
	for rows.Next() {
		var x models.NotificationRule
		rows.Scan(&x.ID, &x.DaysBefore, &x.Channel, &x.Enabled)
		out = append(out, x)
	}
	a.JSON(w, out)
}
func (a *App) TriggerNotificationNow(w http.ResponseWriter, r *http.Request) {
	if err := services.RunNotificationScanForUser(a.DB, time.Now(), userID(r)); err != nil {
		Err(w, 500, err.Error())
		return
	}
	a.JSON(w, true)
}

func (a *App) RuleSave(w http.ResponseWriter, r *http.Request) {
	var q struct{ Days []int }
	read(r, &q)
	a.DB.Exec("update notification_rules set deleted=1 where user_id=?", userID(r))
	for _, d := range q.Days {
		a.DB.Exec("insert into notification_rules(user_id,days_before,channel,enabled) values(?,?, 'email',1)", userID(r), d)
	}
	a.JSON(w, true)
}
func (a *App) currentRole(uid int64) string {
	var dbRole string
	if err := a.DB.QueryRow("select role from users where id=? and deleted=0", uid).Scan(&dbRole); err != nil {
		return ""
	}
	return dbRole
}

func (a *App) Settings(w http.ResponseWriter, r *http.Request) {
	var v string
	a.DB.QueryRow("select setting_value from system_settings where setting_key='register_enabled'").Scan(&v)
	a.JSON(w, map[string]any{"registerEnabled": v == "true", "role": a.currentRole(userID(r))})
}
func (a *App) SetRegister(w http.ResponseWriter, r *http.Request) {
	if a.currentRole(userID(r)) != "admin" {
		Err(w, 403, "仅管理员可操作")
		return
	}
	var q struct{ Enabled bool }
	read(r, &q)
	v := "false"
	if q.Enabled {
		v = "true"
	}
	a.DB.Exec("update system_settings set setting_value=? where setting_key='register_enabled'", v)
	a.JSON(w, true)
}
