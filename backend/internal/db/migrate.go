package db

import (
	"database/sql"
	"strings"
)

// Migrate 初始化开发库表结构；所有表均采用软删除字段，保持数据可恢复。
func Migrate(d *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS users (id BIGINT PRIMARY KEY AUTO_INCREMENT, username VARCHAR(64) NOT NULL, password_hash VARCHAR(255) NOT NULL, role VARCHAR(32) NOT NULL DEFAULT 'user', deleted TINYINT NOT NULL DEFAULT 0, created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP, UNIQUE KEY uk_username (username)) COMMENT='用户表'`,
		`CREATE TABLE IF NOT EXISTS system_settings (id BIGINT PRIMARY KEY AUTO_INCREMENT, setting_key VARCHAR(128) NOT NULL, setting_value VARCHAR(1024) NOT NULL, created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP, UNIQUE KEY uk_setting_key (setting_key)) COMMENT='系统配置表'`,
		`CREATE TABLE IF NOT EXISTS subscriptions (id BIGINT PRIMARY KEY AUTO_INCREMENT, user_id BIGINT NOT NULL, name VARCHAR(128) NOT NULL, note TEXT NULL, period_type VARCHAR(16) NOT NULL, period_value INT NOT NULL, start_date DATE NOT NULL, end_date DATE NOT NULL, next_renewal_date DATE NOT NULL, status VARCHAR(32) NOT NULL DEFAULT 'active', deleted TINYINT NOT NULL DEFAULT 0, created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP, KEY idx_user_renewal (user_id,next_renewal_date), KEY idx_user_deleted (user_id,deleted)) COMMENT='订阅表'`,
		`CREATE TABLE IF NOT EXISTS tags (id BIGINT PRIMARY KEY AUTO_INCREMENT, user_id BIGINT NOT NULL, name VARCHAR(64) NOT NULL, color VARCHAR(32) NULL, deleted TINYINT NOT NULL DEFAULT 0, created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP, KEY idx_user_deleted (user_id,deleted)) COMMENT='标签表'`,
		`CREATE TABLE IF NOT EXISTS subscription_tags (id BIGINT PRIMARY KEY AUTO_INCREMENT, user_id BIGINT NOT NULL, subscription_id BIGINT NOT NULL, tag_id BIGINT NOT NULL, deleted TINYINT NOT NULL DEFAULT 0, created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP, KEY idx_subscription (subscription_id,deleted), KEY idx_tag (tag_id,deleted)) COMMENT='订阅标签关系表'`,
		`CREATE TABLE IF NOT EXISTS calendar_subscribe_links (id BIGINT PRIMARY KEY AUTO_INCREMENT, user_id BIGINT NOT NULL, token VARCHAR(128) NOT NULL, remark VARCHAR(255) NULL, enabled TINYINT NOT NULL DEFAULT 1, deleted TINYINT NOT NULL DEFAULT 0, last_access_at DATETIME NULL, created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP, UNIQUE KEY uk_token (token), KEY idx_user_deleted (user_id,deleted)) COMMENT='日历订阅链接表'`,
		`CREATE TABLE IF NOT EXISTS notification_emails (id BIGINT PRIMARY KEY AUTO_INCREMENT, user_id BIGINT NOT NULL, email VARCHAR(255) NOT NULL, verified TINYINT NOT NULL DEFAULT 0, enabled TINYINT NOT NULL DEFAULT 1, verify_code VARCHAR(16) NULL, verify_expired_at DATETIME NULL, deleted TINYINT NOT NULL DEFAULT 0, created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP, KEY idx_user_deleted (user_id,deleted)) COMMENT='通知邮箱表'`,
		`CREATE TABLE IF NOT EXISTS notification_rules (id BIGINT PRIMARY KEY AUTO_INCREMENT, user_id BIGINT NOT NULL, days_before INT NOT NULL, channel VARCHAR(32) NOT NULL DEFAULT 'email', enabled TINYINT NOT NULL DEFAULT 1, deleted TINYINT NOT NULL DEFAULT 0, created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP, KEY idx_user_enabled (user_id,enabled,deleted)) COMMENT='通知规则表'`,
		`CREATE TABLE IF NOT EXISTS notification_logs (id BIGINT PRIMARY KEY AUTO_INCREMENT, user_id BIGINT NOT NULL, subscription_id BIGINT NOT NULL, notification_rule_id BIGINT NOT NULL, notify_date DATE NOT NULL, channel VARCHAR(32) NOT NULL, status VARCHAR(32) NOT NULL, error_message VARCHAR(1024) NULL, created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, UNIQUE KEY uk_notify_once (user_id,subscription_id,notification_rule_id,notify_date,channel)) COMMENT='通知发送日志表'`,
		`ALTER TABLE subscriptions ADD COLUMN budget_rmb DECIMAL(10,2) NULL COMMENT '预算金额'`,
		`ALTER TABLE subscriptions ADD COLUMN budget_currency VARCHAR(8) NOT NULL DEFAULT 'CNY' COMMENT '预算币种'`,
		`ALTER TABLE subscriptions ADD COLUMN renewal_url VARCHAR(1024) NULL COMMENT '续费链接'`,
		`ALTER TABLE subscriptions ADD COLUMN reminder_enabled TINYINT NOT NULL DEFAULT 1 COMMENT '是否启用到期提醒'`,
		`ALTER TABLE subscriptions ADD COLUMN auto_renew TINYINT NOT NULL DEFAULT 0 COMMENT '到期后是否自动顺延'`,
		`INSERT INTO system_settings(setting_key, setting_value) VALUES('register_enabled','true') ON DUPLICATE KEY UPDATE setting_key=setting_key`,
	}
	for _, s := range stmts {
		if _, err := d.Exec(s); err != nil {
			if strings.Contains(err.Error(), "Duplicate column") {
				continue
			}
			return err
		}
	}
	return nil
}
