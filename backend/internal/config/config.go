package config

import "os"

// Config 保存服务启动所需配置，所有字段均从环境变量读取，避免敏感信息写入代码仓库。
type Config struct {
	Port      string
	DBDsn     string
	JWTSecret string
	BaseURL   string
}

func Load() Config {
	cfg := Config{
		Port:      getenv("APP_PORT", "8080"),
		DBDsn:     os.Getenv("MYSQL_DSN"),
		JWTSecret: getenv("JWT_SECRET", "dev-secret-change-me"),
		BaseURL:   getenv("APP_BASE_URL", "http://localhost:8080"),
	}
	return cfg
}

func getenv(key, fallback string) string {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	return v
}
