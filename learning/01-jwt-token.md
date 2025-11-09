# JWT Token - 生成与验证机制

本文档记录 JWT Token 的生成、解析、验证以及时间字段的详细说明。

---

## 2025-11-05 JWT Token 生成与验证机制

### Q1: AccessToken 和 RefreshToken 有什么区别？代码里如何体现？

**问题背景**：
注册接口返回两个 Token（`biz/handler/memogo/api/memo_go_service.go:57-62`）：
```go
Data: &api.TokenPair{
    AccessToken:      accessToken,
    RefreshToken:     refreshToken,
    AccessExpiresIn:  computeExpiresIn(accessToken),
    RefreshExpiresIn: computeExpiresIn(refreshToken),
}
```

**解答**：两者的区别体现在**有效期**和**使用场景**上。

#### 1. 有效期不同（`pkg/jwt/jwt.go:75-88`）

```go
func GenerateTokenPair(userID uint, username string) (accessToken, refreshToken string, err error) {
    // 访问令牌：15分钟
    accessToken, err = GenerateToken(userID, username, 15*time.Minute)

    // 刷新令牌：7天
    refreshToken, err = GenerateToken(userID, username, 7*24*time.Hour)

    return accessToken, refreshToken, nil
}
```

| Token 类型 | 有效期 | 用途 |
|-----------|--------|------|
| **AccessToken** | 15分钟 | 访问所有受保护的 API（创建待办、查询列表等） |
| **RefreshToken** | 7天 | 仅用于刷新 AccessToken（调用 `/v1/auth/refresh`） |

#### 2. 中间件配置体现（`pkg/middleware/jwt.go:35-36`）

```go
Timeout:     15 * time.Minute,  // Access token 过期时间
MaxRefresh:  7 * 24 * time.Hour, // Refresh token 过期时间
```

#### 3. 使用场景

**AccessToken 使用流程**：
```bash
# 每次 API 调用都需要携带
curl -H "Authorization: Bearer <accessToken>" \
     http://localhost:8080/v1/todos
```

**RefreshToken 使用流程**：
```bash
# 当 AccessToken 过期（15分钟后）时使用
curl -X POST http://localhost:8080/v1/auth/refresh \
     -d '{"refresh_token": "<refreshToken>"}'

# 返回新的 AccessToken
```

#### 4. 双 Token 机制的安全优势

```
用户登录
    ↓
获得 AccessToken (15分钟) + RefreshToken (7天)
    ↓
使用 AccessToken 访问 API
    ↓
AccessToken 过期（15分钟后）
    ↓
使用 RefreshToken 刷新 → 获得新的 AccessToken
    ↓
继续使用新的 AccessToken
    ↓
RefreshToken 也过期（7天后）
    ↓
用户需要重新登录
```

| 安全特性 | AccessToken | RefreshToken |
|---------|------------|--------------|
| **使用频率** | 高（每次 API 请求） | 低（仅刷新时） |
| **被截获风险** | 高 | 低 |
| **泄露损失** | 小（15分钟后失效） | 大（需妥善保管） |
| **存储位置** | 内存 | 安全存储（HttpOnly Cookie） |

**关键**：AccessToken 频繁使用但短期有效，即使泄露影响有限；RefreshToken 使用少但长期有效，降低被截获概率。

---

### Q2: `GenerateTokenPair` 是生成随机 Token 吗？

**问题代码**（`biz/service/auth_service.go:128`）：
```go
accessToken, refreshToken, err := jwtPkg.GenerateTokenPair(user.ID, user.Username)
```

**解答**：不是随机 Token，而是基于 **JWT（JSON Web Token）标准的加密签名令牌**。

#### JWT Token 不是随机的，是可解析的

**生成过程**（`pkg/jwt/jwt.go:37-51`）：

```go
func GenerateToken(userID uint, username string, duration time.Duration) (string, error) {
    now := time.Now()

    // 1. 构建 Payload（包含用户信息）
    claims := Claims{
        UserID:   userID,        // 用户ID
        Username: username,      // 用户名
        OrigIat:  now.Unix(),    // 原始签发时间
        RegisteredClaims: jwt.RegisteredClaims{
            ExpiresAt: jwt.NewNumericDate(now.Add(duration)), // 过期时间
            IssuedAt:  jwt.NewNumericDate(now),               // 签发时间
            NotBefore: jwt.NewNumericDate(now),               // 生效时间
        },
    }

    // 2. 使用 HMAC-SHA256 算法创建 JWT
    token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

    // 3. 用密钥签名并返回完整的 JWT 字符串
    return token.SignedString(GetJWTSecret())
}
```

#### JWT Token 的结构（三部分用 `.` 分隔）

```
eyJhbGc...JWT9  .  eyJleHAi...MDB9  .  aePaXs5i...uuaw
     ↓                   ↓                   ↓
   Header            Payload            Signature
  (算法信息)         (用户数据)          (防篡改签名)
```

**解码后的内容**：

```json
// Header
{"alg":"HS256","typ":"JWT"}

// Payload（包含用户信息）
{
  "user_id": 2,
  "username": "logintest",
  "exp": 1761837205,      // 过期时间
  "iat": 1761836305,      // 签发时间
  "orig_iat": 1761836305  // 原始签发时间
}

// Signature
// HMAC_SHA256(Header.Payload, secret)
```

#### 随机 Token vs JWT Token

| 特性 | 随机 Token（如 UUID） | JWT Token |
|------|---------------------|-----------|
| **生成方式** | 纯随机字符串 | 包含用户信息 + 加密签名 |
| **可解析性** | 不可解析 | 可直接解析出用户信息 |
| **验证方式** | 必须查数据库/Redis | 验证签名即可（无需查库） |
| **存储需求** | 服务端必须存储 | 服务端无需存储（无状态） |
| **包含信息** | 无意义的随机值 | userID、username、过期时间等 |

**JWT 的优势**：
- ✅ **自包含**：Token 本身包含用户信息，服务器无需查数据库
- ✅ **无状态**：不需要在服务端存储 session
- ✅ **防篡改**：任何修改 Payload 都会导致签名验证失败

---

### Q3: JWT RegisteredClaims 的三个时间字段分别是什么意思？

**问题代码**（`pkg/jwt/jwt.go:43-47`）：
```go
RegisteredClaims: jwt.RegisteredClaims{
    ExpiresAt: jwt.NewNumericDate(now.Add(duration)),
    IssuedAt:  jwt.NewNumericDate(now),
    NotBefore: jwt.NewNumericDate(now),
}
```

**解答**：这三个字段定义了 Token 的**生命周期**。

#### 1. ExpiresAt（过期时间）

**含义**：Token 的**失效截止时间**

```go
ExpiresAt: jwt.NewNumericDate(now.Add(duration))
// AccessToken: now + 15分钟
// RefreshToken: now + 7天
```

**验证逻辑**：
```
当前时间：2025-11-05 10:00:00
ExpiresAt：2025-11-05 10:15:00  （15分钟后）

10:14:59 → Token 有效 ✓
10:15:01 → Token 过期 ✗（返回 ErrTokenExpired）
```

#### 2. IssuedAt（签发时间）

**含义**：Token 的**创建时间**

```go
IssuedAt: jwt.NewNumericDate(now)
```

**用途**：
- 记录 Token 何时生成
- 用于审计日志："用户在 10:00 登录并获得 Token"
- 可实现安全策略："拒绝超过 30 天的 Token（即使未过期）"

#### 3. NotBefore（生效时间）

**含义**：Token 的**最早可用时间**

```go
NotBefore: jwt.NewNumericDate(now)  // 立即生效
```

**验证逻辑**：
```
如果 NotBefore = now（当前代码）：
  → Token 立即生效 ✓

如果 NotBefore = now + 1小时（延迟生效）：
  → 1小时内使用会返回 ErrTokenNotValidYet ✗
```

**使用场景**（设置未来时间）：
- 预约系统："这个优惠券明天才能用"
- 定时任务："这个任务令牌晚上 8 点才生效"
- 防止时钟偏差：设置稍微未来的时间

#### 时间关系图

```
时间线：
  ├─────────┼──────────────────────┼─────────→
  NotBefore  IssuedAt              ExpiresAt
  (生效时间) (签发时间)            (过期时间)

本项目中：
  ├─────────────────────────────────┼─────────→
  now (立即生效)                    now + 15分钟/7天
  NotBefore = IssuedAt              ExpiresAt
```

#### 验证实现（`pkg/jwt/jwt.go:54-72`）

```go
func ParseToken(tokenString string) (*Claims, error) {
    token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
        return GetJWTSecret(), nil
    })

    // JWT 库自动验证三个时间字段：
    // 1. 检查 NotBefore：当前时间 >= NotBefore
    // 2. 检查 ExpiresAt：当前时间 < ExpiresAt
    // 3. 检查 IssuedAt：当前时间 >= IssuedAt（可选）

    if err != nil {
        if errors.Is(err, jwt.ErrTokenExpired) {
            return nil, ErrExpiredToken  // ExpiresAt 检查失败
        }
        // 注意：jwt.ErrTokenNotValidYet 也会进入这里
        return nil, ErrInvalidToken
    }

    // token.Valid 已经包含了所有时间字段的验证
    if claims, ok := token.Claims.(*Claims); ok && token.Valid {
        return claims, nil
    }

    return nil, ErrInvalidToken
}
```

**关键**：`token.Valid` 字段由 JWT 库自动设置，已包含对三个时间字段的验证，无需手动检查。

---

### Q4: NotBefore 是必须设置的吗？

**解答**：**不是必须的**，可以省略。

#### JWT 标准中的定义

根据 [RFC 7519](https://datatracker.ietf.org/doc/html/rfc7519#section-4.1)，所有时间字段都是**可选的**：

| 字段 | 简称 | 是否必填 | 说明 |
|------|-----|----------|------|
| exp | ExpiresAt | 可选 | 过期时间 |
| **nbf** | **NotBefore** | **可选** | 生效时间 |
| iat | IssuedAt | 可选 | 签发时间 |

#### 不设置时的行为

```go
// 方式 1：当前的代码（设置为 now）
claims := Claims{
    RegisteredClaims: jwt.RegisteredClaims{
        ExpiresAt: jwt.NewNumericDate(now.Add(duration)),
        IssuedAt:  jwt.NewNumericDate(now),
        NotBefore: jwt.NewNumericDate(now),  // 可以省略
    },
}

// 方式 2：不设置 NotBefore（完全可行）
claims := Claims{
    RegisteredClaims: jwt.RegisteredClaims{
        ExpiresAt: jwt.NewNumericDate(now.Add(duration)),
        IssuedAt:  jwt.NewNumericDate(now),
        // NotBefore 为 nil，验证时会跳过检查
    },
}
```

#### JWT 库的验证逻辑

```go
// JWT 库内部实现（简化版）
func (c RegisteredClaims) Valid() error {
    now := time.Now()

    // 如果 NotBefore 为 nil，跳过检查
    if c.NotBefore != nil && now.Before(c.NotBefore.Time) {
        return ErrTokenNotValidYet
    }

    // 如果 ExpiresAt 不为 nil，才检查过期
    if c.ExpiresAt != nil && now.After(c.ExpiresAt.Time) {
        return ErrTokenExpired
    }

    return nil
}
```

**结论**：不设置 `NotBefore` = Token 立即生效（没有限制）

#### 本项目可以简化

由于 `NotBefore` 设置为 `now`（立即生效），与不设置效果相同：

```go
// 简化前（当前代码）
RegisteredClaims: jwt.RegisteredClaims{
    ExpiresAt: jwt.NewNumericDate(now.Add(duration)),
    IssuedAt:  jwt.NewNumericDate(now),
    NotBefore: jwt.NewNumericDate(now),  // ← 可以删除
}

// 简化后（效果相同）
RegisteredClaims: jwt.RegisteredClaims{
    ExpiresAt: jwt.NewNumericDate(now.Add(duration)),
    IssuedAt:  jwt.NewNumericDate(now),
    // NotBefore 省略，Token 立即生效
}
```

#### 什么时候需要设置 NotBefore？

只有需要**延迟生效**时才设置：

```go
// 场景 1：明天才能用的优惠券
NotBefore: jwt.NewNumericDate(now.Add(24 * time.Hour))

// 场景 2：只在晚上 8-10 点有效的 Token
start := time.Date(2025, 11, 5, 20, 0, 0, 0, time.Local)
NotBefore: jwt.NewNumericDate(start)
ExpiresAt: jwt.NewNumericDate(start.Add(2 * time.Hour))
```

**建议**：
- ✅ 如果不需要延迟生效，可以删除 `NotBefore` 这行代码
- ✅ 保留也可以，代码更明确（"立即生效"的显式声明）
- ✅ 只保留 `ExpiresAt` 和 `IssuedAt` 即可满足大部分场景

---

## 延伸阅读

- JWT 官方规范 RFC 7519: https://datatracker.ietf.org/doc/html/rfc7519
- JWT.io 在线调试工具: https://jwt.io/
- Go JWT 库文档: https://pkg.go.dev/github.com/golang-jwt/jwt/v5

---

*最后更新：2025-11-09*
