# JWT 配置与性能优化

本文档记录 JWT 密钥管理、环境变量配置、性能优化以及 Go 包初始化顺序。

---

## 2025-11-09 JWT 密钥性能优化与初始化顺序

### Q1: GetJWTSecret() 每次都读取环境变量会影响性能吗？

**原始代码**（`pkg/jwt/jwt.go:26-34`）：
```go
func GetJWTSecret() []byte {
    secret := os.Getenv("JWT_SECRET")
    if secret == "" {
        secret = "memogo-default-secret-change-in-production"
    }
    return []byte(secret)
}
```

**问题分析**：
- 每次生成或解析 JWT 都调用此函数
- 每次调用都执行 `os.Getenv()` 和字符串转换
- 高并发场景下（每秒数千请求）会产生不必要的开销

**性能对比**：
- 每次读取：数千 ns/op
- 缓存后：几 ns/op

### 用户方案：启动时初始化（最佳实践）

```go
var jwtSecret []byte

func init() {
    secret := os.Getenv("JWT_SECRET")
    if secret == "" {
        panic("JWT_SECRET environment variable is required")
    }
    jwtSecret = []byte(secret)
}

func GetJWTSecret() []byte {
    return jwtSecret
}
```

**优势**：
1. **性能最优**：启动时读取一次，后续零开销
2. **Fail-Fast**：配置错误立即暴露，不是等到第一个请求
3. **代码简洁**：清晰表达"这是启动时的配置"
4. **编译器优化**：GetJWTSecret() 会被内联，几乎无函数调用开销

**相关文件**：`pkg/jwt/jwt.go:20-28`

---

### Q2: 为什么不应该提供默认密钥？

**危险的反模式**：
```go
if secret == "" {
    secret = "memogo-default-secret-change-in-production"  // ❌ 危险！
}
```

**安全风险**：
1. **掩盖配置错误**：生产环境忘记设置环境变量，服务照样启动
2. **安全漏洞**：默认密钥暴露在代码仓库，任何人都能伪造 token
3. **违反 Fail-Fast 原则**：应该启动时立即失败

**正确做法**：
```go
if secret == "" {
    panic("JWT_SECRET environment variable is required")  // ✅ 立即失败
}
```

**核心原则**：对于安全敏感的配置（密钥、密码、证书），宁可崩溃，不要用不安全的默认值。

---

### Q3: os.Getenv() 会自动读取 .env 文件吗？

**答案**：不会！`os.Getenv()` 只读取**系统环境变量**。

**读取来源**：
```bash
# 1. Shell 环境变量
export JWT_SECRET="my-secret"
./memogo

# 2. 运行时传入
JWT_SECRET="my-secret" ./memogo

# 3. 系统级环境变量（~/.bashrc 等）
```

**如果要读取 .env 文件**：需要使用 `github.com/joho/godotenv`

---

### Q4: 为什么原来的 godotenv.Load() 位置有问题？

**问题代码**（原 `main.go`）：
```go
func main() {
    godotenv.Load()  // ← 太晚了！
    db.Init()
    // ...
}
```

**Go 初始化顺序**：
```
1. 导入的包的 init() 按依赖顺序执行
   └─ pkg/jwt/jwt.go 的 init() 在这里执行
      └─ os.Getenv("JWT_SECRET") ← 此时 .env 还没加载！
2. main() 函数开始
   └─ godotenv.Load() ← 太晚了！
```

**结果**：程序启动时 panic，即使 `.env` 文件存在。

---

### Q5: 如何保证 .env 在所有包初始化之前加载？

**解决方案**：创建 `pkg/env` 包统一管理环境变量加载

**pkg/env/env.go**：
```go
package env

import (
    "log"
    "github.com/joho/godotenv"
)

func init() {
    // 加载 .env 文件
    if err := godotenv.Load(); err != nil {
        log.Println("Warning: .env file not found, using system environment variables")
    }
}
```

**在需要环境变量的包中导入**：
```go
// pkg/jwt/jwt.go
package jwt

import (
    // ...
    _ "memogo/pkg/env"  // ← 确保 env 先初始化
)

var jwtSecret []byte

func init() {
    secret := os.Getenv("JWT_SECRET")  // ← 此时 .env 已加载
    if secret == "" {
        panic("JWT_SECRET environment variable is required")
    }
    jwtSecret = []byte(secret)
}
```

**Go 的依赖保证**：
- `pkg/jwt` 导入 `pkg/env`
- Go 编译器保证 `env.init()` 先于 `jwt.init()` 执行
- 无需手动管理复杂的初始化顺序

**修改的文件**：
- 新建：`pkg/env/env.go`
- 修改：`pkg/jwt/jwt.go`、`biz/dal/db/init.go`、`biz/dal/redis/init.go`

**初始化顺序（自动保证）**：
```
pkg/env.init()     ← 加载 .env 文件
    ↓
pkg/jwt.init()     ← 读取 JWT_SECRET
    ↓
biz/dal/db.Init()  ← 读取数据库配置（在 main() 中调用）
    ↓
main()             ← 启动服务
```

---

### Q6: 为什么用 blank import（`_`）导入 pkg/env？

**代码示例**：
```go
import (
    _ "memogo/pkg/env"  // ← 为什么用 _？
)
```

**解答**：

**Blank Import 的作用**：
- 仅执行包的 `init()` 函数
- 不使用包的任何导出内容
- 编译器不会报"导入但未使用"的错误

**`pkg/env` 的特点**：
- 只有 `init()` 函数，没有导出的函数或变量
- 作用是**副作用**（side effect）：加载环境变量
- 不需要调用任何函数

**对比**：
```go
// 正确：仅需要执行 init()
import _ "memogo/pkg/env"

// 错误：编译器报错"导入但未使用"
import "memogo/pkg/env"

// 错误：env 包没有导出任何内容可调用
import "memogo/pkg/env"
env.Load()  // ← 编译错误：undefined: env.Load
```

**Blank Import 的其他常见用途**：
```go
// 1. 注册数据库驱动（副作用：注册到 database/sql）
import _ "github.com/go-sql-driver/mysql"

// 2. 加载配置
import _ "myapp/config"

// 3. 静态资源嵌入
import _ "embed"
```

---

### Q7: 为什么不在 main 包的 init() 中加载 .env？

**可能的方案**：
```go
// main.go
package main

import "github.com/joho/godotenv"

func init() {
    godotenv.Load()
}

func main() {
    // ...
}
```

**问题**：
- `main.init()` 的执行时机不确定
- 如果 `pkg/jwt` 先初始化，还是会失败
- 依赖 Go 编译器的初始化顺序，不可靠

**Go 初始化顺序规则**：
1. 先初始化依赖的包（深度优先）
2. 再初始化当前包
3. 同一个包内：全局变量 → init() 函数

**不确定性示例**：
```
可能顺序 1：
  pkg/jwt.init() → panic (JWT_SECRET 为空)
  main.init()    → 加载 .env（太晚了）

可能顺序 2：
  main.init()    → 加载 .env
  pkg/jwt.init() → 成功（看起来正常）

→ 顺序依赖编译器实现，不可靠！
```

**正确做法**：
- 创建专门的 `pkg/env` 包
- 在需要的地方显式导入
- 利用 Go 的依赖保证机制

---

### 核心要点总结

1. **性能优化**：配置应该在启动时初始化，不要每次都读取
2. **安全优先**：敏感配置不提供默认值，使用 Fail-Fast 原则
3. **依赖管理**：用 `import` 明确声明依赖，让 Go 编译器保证初始化顺序
4. **简单优于复杂**：与其思考复杂的初始化顺序，不如在所有需要的地方导入 `pkg/env`

**相关文件**：
- `pkg/env/env.go` - 环境变量加载
- `pkg/jwt/jwt.go:10-11, 23-28` - JWT 密钥初始化
- `biz/dal/db/init.go:14-15` - 数据库配置加载
- `biz/dal/redis/init.go:12-13` - Redis 配置加载

---

## Go 包初始化顺序深入

### 完整的初始化流程

```
程序启动
    ↓
1. 初始化 main 包依赖的所有包（递归，深度优先）
    ├─ pkg/env/env.go
    │   └─ init() 执行：加载 .env 文件
    ├─ pkg/jwt/jwt.go
    │   ├─ 导入 pkg/env（已初始化，跳过）
    │   └─ init() 执行：读取 JWT_SECRET
    ├─ pkg/hash/hash.go
    │   └─ init() 执行（如果有）
    └─ ...（其他包）
    ↓
2. 初始化 main 包
    ├─ 初始化全局变量
    └─ 执行 main.init()（如果有）
    ↓
3. 执行 main() 函数
    ├─ db.Init()
    ├─ redis.Init()
    └─ h.Spin()
```

### 依赖顺序示例

```go
// pkg/env/env.go
package env
func init() { /* 1. 最先执行 */ }

// pkg/jwt/jwt.go
package jwt
import _ "memogo/pkg/env"  // 声明依赖
func init() { /* 2. env 之后执行 */ }

// biz/dal/db/init.go
package db
import _ "memogo/pkg/env"  // 声明依赖
func Init() { /* 3. 在 main() 中手动调用 */ }

// main.go
package main
import (
    "memogo/pkg/jwt"      // 触发 jwt 初始化
    "memogo/biz/dal/db"   // 触发 db 包加载
)
func main() {
    db.Init()  // 4. 显式调用
}
```

### 关键规则

1. **包级 init() 自动执行**，无需手动调用
2. **依赖的包先初始化**（深度优先）
3. **同一个包只初始化一次**（即使被多次导入）
4. **Blank import 仅执行 init()**，不引入符号
5. **循环依赖会编译错误**

---

## 环境变量最佳实践

### 开发环境

```bash
# .env 文件（不提交到 Git）
JWT_SECRET=dev-secret-12345
DB_DSN=root:password@tcp(127.0.0.1:3306)/memogo?charset=utf8mb4&parseTime=True&loc=Local
REDIS_ADDR=localhost:6379
```

### 生产环境

```bash
# 使用系统环境变量（不使用 .env 文件）
export JWT_SECRET="$(openssl rand -base64 32)"
export DB_DSN="user:pass@tcp(prod-db:3306)/memogo?..."
export REDIS_ADDR="prod-redis:6379"
```

### Docker 环境

```dockerfile
# Dockerfile
ENV JWT_SECRET=${JWT_SECRET}
ENV DB_DSN=${DB_DSN}
```

```yaml
# docker-compose.yml
services:
  memogo:
    environment:
      - JWT_SECRET=${JWT_SECRET}
      - DB_DSN=${DB_DSN}
    env_file:
      - .env.production  # 或使用 env_file
```

### Kubernetes 环境

```yaml
# secret.yaml
apiVersion: v1
kind: Secret
metadata:
  name: memogo-secrets
type: Opaque
data:
  jwt-secret: <base64-encoded>

---
# deployment.yaml
env:
  - name: JWT_SECRET
    valueFrom:
      secretKeyRef:
        name: memogo-secrets
        key: jwt-secret
```

---

## 延伸阅读

- Go 初始化顺序官方文档：https://go.dev/ref/spec#Package_initialization
- 12-Factor App 配置管理：https://12factor.net/config
- godotenv 库文档：https://github.com/joho/godotenv
- Go blank import 详解：https://go.dev/doc/effective_go#blank_import

---

*最后更新：2025-11-09（完整的配置管理与性能优化指南）*
