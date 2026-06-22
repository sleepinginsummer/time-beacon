# 订阅管理系统

## 技术栈

- 后端：Go + MySQL
- 前端：React + Ant Design + Tailwind CSS v4 + TanStack Router + Zustand
- PWA：vite-plugin-pwa，支持安装和离线只读

## 本地启动

### 后端

```bash
cd backend
export MYSQL_DSN='user:password@tcp(host:3306)/database?parseTime=true&loc=Local'
export JWT_SECRET='dev-secret'
export APP_BASE_URL='http://localhost:8080'
# 可选：配置后启用真实邮件发送
export SMTP_HOST=''
export SMTP_PORT=''
export SMTP_USER=''
export SMTP_PASSWORD=''
export SMTP_FROM=''
go run ./cmd/server
```

邮件到期提醒依赖 SMTP 配置；如果不配置 SMTP，系统可以保存通知规则和通知邮箱，但不会发送真实邮件。

### 前端

```bash
cd frontend
npm ci --ignore-scripts
node node_modules/vite/bin/vite.js --host 127.0.0.1
```

浏览器访问：

```text
http://127.0.0.1:5173
```

## 已实现范围

- 注册 / 登录
- 首个注册用户为管理员
- 管理员关闭注册
- 订阅 CRUD、周期计算、手动重置
- 标签管理
- 日历订阅链接管理
- `.ics` 日历订阅输出
- CSV 导出订阅数据
- 通知邮箱绑定基础能力
- 到期前 N 天通知规则配置
- PWA 构建与离线只读缓存

## 测试

```bash
cd backend && go test ./...
cd frontend && node node_modules/typescript/bin/tsc -b && node node_modules/vite/bin/vite.js build
```

PWA 安装体验和手机系统兼容性需要在目标设备自行测试。

## 一键检查

挂载盘可能不保留执行位，推荐用 `bash` 调用：

```bash
bash scripts/check.sh
```

完整后端联调需要先设置 `MYSQL_DSN`：

```bash
export MYSQL_DSN='user:password@tcp(host:3306)/database?parseTime=true&loc=Local'
bash scripts/integration.sh
```

联调脚本会验证：注册、标签、订阅绑定标签、通知规则、通知邮箱、日历链接、`.ics`、CSV 导出。
