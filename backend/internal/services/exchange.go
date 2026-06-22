package services

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// 汇率缓存按币种保存，避免列表页重复触发外部接口请求，当前缓存 1 小时。
type ExchangeService struct {
	mu     sync.Mutex
	rates  map[string]cachedRate
	client *http.Client
}

type cachedRate struct {
	Rate      float64
	ExpiredAt time.Time
}

type frankfurterResp struct {
	Rates map[string]float64 `json:"rates"`
}

func NewExchangeService() *ExchangeService {
	return &ExchangeService{rates: map[string]cachedRate{}, client: &http.Client{Timeout: 6 * time.Second}}
}

// ConvertToCNY 将常见币种金额换算成人民币。CNY/RMB 直接返回 1，外币使用 Frankfurter 最新参考汇率。
func (s *ExchangeService) ConvertToCNY(amount float64, currency string) (float64, float64, error) {
	code := NormalizeCurrency(currency)
	if code == "CNY" {
		return amount, 1, nil
	}
	rate, err := s.RateToCNY(code)
	if err != nil {
		return 0, 0, err
	}
	return amount * rate, rate, nil
}

func (s *ExchangeService) RateToCNY(currency string) (float64, error) {
	code := NormalizeCurrency(currency)
	if code == "CNY" {
		return 1, nil
	}
	now := time.Now()
	s.mu.Lock()
	if c, ok := s.rates[code]; ok && c.ExpiredAt.After(now) {
		s.mu.Unlock()
		return c.Rate, nil
	}
	s.mu.Unlock()

	url := fmt.Sprintf("https://api.frankfurter.app/latest?from=%s&to=CNY", code)
	resp, err := s.client.Get(url)
	if err != nil {
		return 0, fmt.Errorf("汇率服务请求失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("汇率服务返回异常: %s", resp.Status)
	}
	var data frankfurterResp
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return 0, fmt.Errorf("汇率响应解析失败: %w", err)
	}
	rate := data.Rates["CNY"]
	if rate <= 0 {
		return 0, fmt.Errorf("币种暂不支持: %s", code)
	}
	s.mu.Lock()
	s.rates[code] = cachedRate{Rate: rate, ExpiredAt: now.Add(1 * time.Hour)}
	s.mu.Unlock()
	return rate, nil
}

func NormalizeCurrency(currency string) string {
	code := strings.ToUpper(strings.TrimSpace(currency))
	if code == "" || code == "RMB" || code == "人民币" {
		return "CNY"
	}
	return code
}
