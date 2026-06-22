package main

import (
	"log"
	"net/http"
	"os"
	"subscription-management/backend/internal/config"
	"subscription-management/backend/internal/db"
	"subscription-management/backend/internal/handlers"
	"subscription-management/backend/internal/middleware"
	"subscription-management/backend/internal/services"
)

func main() {
	cfg := config.Load()
	d, err := db.Open(cfg.DBDsn)
	if err != nil {
		log.Fatal(err)
	}
	if err := db.Migrate(d); err != nil {
		log.Fatal(err)
	}
	services.StartNotificationScheduler(d)
	app := &handlers.App{DB: d, Config: cfg, Exchange: services.NewExchangeService()}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/register", app.Register)
	mux.HandleFunc("/api/auth/login", app.Login)
	mux.HandleFunc("/api/auth/registerStatus", app.RegisterStatus)
	mux.HandleFunc("/calendar/subscribe/", app.ICS)
	fs := http.FileServer(http.Dir("/app/public"))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.ServeFile(w, r, "/app/public/index.html")
			return
		}
		if _, err := os.Stat("/app/public" + r.URL.Path); err == nil {
			fs.ServeHTTP(w, r)
			return
		}
		http.ServeFile(w, r, "/app/public/index.html")
	})
	auth := http.NewServeMux()
	auth.HandleFunc("/api/auth/me", app.Me)
	auth.HandleFunc("/api/subscription/create", app.SubCreate)
	auth.HandleFunc("/api/subscription/page", app.SubList)
	auth.HandleFunc("/api/subscription/update", app.SubUpdate)
	auth.HandleFunc("/api/subscription/delete", app.SubDelete)
	auth.HandleFunc("/api/subscription/resetPeriod", app.SubReset)
	auth.HandleFunc("/api/subscription/exportCsv", app.ExportCSV)
	auth.HandleFunc("/api/exchange/convertBatchToRmb", app.ExchangeConvertBatch)
	auth.HandleFunc("/api/tag/list", app.TagList)
	auth.HandleFunc("/api/tag/create", app.TagCreate)
	auth.HandleFunc("/api/tag/update", app.TagUpdate)
	auth.HandleFunc("/api/tag/delete", app.TagDelete)
	auth.HandleFunc("/api/calendarLink/create", app.LinkCreate)
	auth.HandleFunc("/api/calendarLink/page", app.LinkList)
	auth.HandleFunc("/api/calendarLink/delete", func(w http.ResponseWriter, r *http.Request) { app.LinkAction(w, r, "delete") })
	auth.HandleFunc("/api/calendarLink/disable", func(w http.ResponseWriter, r *http.Request) { app.LinkAction(w, r, "disable") })
	auth.HandleFunc("/api/calendarLink/enable", func(w http.ResponseWriter, r *http.Request) { app.LinkAction(w, r, "enable") })
	auth.HandleFunc("/api/calendarLink/resetToken", func(w http.ResponseWriter, r *http.Request) { app.LinkAction(w, r, "reset") })
	auth.HandleFunc("/api/calendarLink/updateRemark", func(w http.ResponseWriter, r *http.Request) { app.LinkAction(w, r, "remark") })
	auth.HandleFunc("/api/notificationEmail/list", app.EmailList)
	auth.HandleFunc("/api/notificationEmail/create", app.EmailCreate)
	auth.HandleFunc("/api/notificationEmail/enable", func(w http.ResponseWriter, r *http.Request) { app.EmailAction(w, r, "enable") })
	auth.HandleFunc("/api/notificationEmail/disable", func(w http.ResponseWriter, r *http.Request) { app.EmailAction(w, r, "disable") })
	auth.HandleFunc("/api/notificationEmail/delete", func(w http.ResponseWriter, r *http.Request) { app.EmailAction(w, r, "delete") })
	auth.HandleFunc("/api/notificationRule/list", app.RuleList)
	auth.HandleFunc("/api/notificationRule/saveBatch", app.RuleSave)
	auth.HandleFunc("/api/notificationRule/triggerNow", app.TriggerNotificationNow)
	auth.HandleFunc("/api/settings/get", app.Settings)
	auth.HandleFunc("/api/settings/updateRegisterEnabled", app.SetRegister)
	mux.Handle("/api/", middleware.Auth(cfg.JWTSecret, auth))
	log.Println("server :" + cfg.Port)
	log.Fatal(http.ListenAndServe(":"+cfg.Port, middleware.CORS(mux)))
}
