# JWT 中间件 - Hertz 认证流程

本文档记录 JWT 中间件的工作原理、配置方式和执行流程。

---

## 2025-10-30 JWT 认证与 Hertz 中间件

### Q1: LoginHandler 为何一行能完成登录？它封装了什么？

**问题代码**：
```go
func Login(ctx context.Context, c *app.RequestContext) {
    middleware.JWTMiddleware.LoginHandler(ctx, c)  // ← 只有一行！
}
```

**解答**：`LoginHandler` 不是我们写的函数，是 **Hertz JWT 中间件提供的**。我们做的是配置中间件，告诉它如何处理登录。

**配置的 4 个关键回调函数**（`pkg/middleware/jwt.go`）：

| 函数 | 作用 | 类比 |
|------|------|------|
| **Authenticator** | 验证用户名密码 | 你告诉快递员："这样验证收件人" |
| **PayloadFunc** | 构建 JWT 内容 | 你告诉快递员："包裹里放这些信息" |
| **LoginResponse** | 返回成功响应 | 你告诉快递员："成功时这样回复" |
| **Unauthorized** | 返回错误响应 | 你告诉快递员："失败时这样回复" |

**LoginHandler 的执行流程**：
```
LoginHandler 内部自动执行:
┌────────────────────────────────────┐
│ 1. 调用 Authenticator (我们配置的) │
│    - 解析用户名密码                 │
│    - 验证凭证                       │
│    - 返回用户信息                   │
├────────────────────────────────────┤
│ 2. 调用 PayloadFunc (我们配置的)   │
│    - 将用户信息转为 JWT Claims     │
├────────────────────────────────────┤
│ 3. 生成 JWT Token (中间件自动)     │
│    - 使用 Key 签名                  │
│    - 设置过期时间                   │
├────────────────────────────────────┤
│ 4. 调用 LoginResponse (我们配置的) │
│    - 返回 JSON 响应                 │
└────────────────────────────────────┘
```

**核心思想**：这是**配置驱动**的设计模式，你配置规则，框架执行流程。

---

### Q2: Token 哪里来的？为何暂时使用相同的 token？

**Token 是 Hertz JWT 中间件自动生成的！**

看 `LoginResponse` 的函数签名：
```go
LoginResponse: func(ctx context.Context, c *app.RequestContext,
                     code int, token string, expire time.Time)
                              ↑
                        中间件传给我们的！
```

**标准的双令牌机制应该是**：

| Token 类型 | 有效期 | 用途 |
|-----------|--------|------|
| **access_token** | 短期（15分钟） | API 调用 |
| **refresh_token** | 长期（7天） | 刷新 access_token |

**问题**：中间件只生成一个 token（根据 `Timeout: 15 * time.Minute`），所以暂时返回了两次。

**解决方案**：

```go
// 方案 A：单令牌模式（推荐简单场景）
LoginResponse: func(..., token string, expire time.Time) {
    c.JSON(200, utils.H{
        "access_token": token,  // 只返回一个
        "expires_at": expire.Unix(),
    })
}

// 方案 B：真双令牌（手动生成 refresh_token）
LoginResponse: func(..., token string, expire time.Time) {
    claims := hertzJWT.ExtractClaims(ctx, c)
    userID := uint(claims["user_id"].(float64))

    // 使用自己的 jwt 包生成长期 token
    _, refreshToken, _ := jwt.GenerateTokenPair(userID, username)

    c.JSON(200, utils.H{
        "access_token": token,        // 15分钟
        "refresh_token": refreshToken, // 7天
    })
}
```

---

### Q3: 为何中间件不默认支持双 token？

**4 个核心原因**：

1. **JWT 标准没有定义双令牌**
   - RFC 7519 只定义了如何创建/验证 token
   - 双令牌是**安全最佳实践**，不是标准

2. **双令牌机制有多种实现方式**
   - 不同应用需求完全不同（过期时间、存储方式、撤销机制）
   - 强制一种实现会限制灵活性

3. **Refresh Token 通常需要持久化**
   - Access Token: 短期、无状态、不需要存储 ✓
   - Refresh Token: 长期、需要可撤销、必须存储 ✗
   - 中间件无法假设你用什么数据库！

4. **框架设计哲学：提供机制，不强制策略**
   - ✅ 提供生成/验证/刷新 token 的能力
   - ❌ 不强制如何使用这些能力

**Hertz JWT 的变相双令牌方案**：

```go
Timeout:    15 * time.Minute,     // Token 15分钟后过期
MaxRefresh: 7 * 24 * time.Hour,   // 但在 7 天内可刷新
```

**工作原理**：
```
生成的 token 包含两个时间戳:
{
  "exp": 1234567890,      // 过期时间（15分钟后）
  "orig_iat": 1234567000  // 原始签发时间
}

刷新流程:
1. Token 15分钟后过期
2. 但在 7 天内，可以用这个"过期"的 token 换新 token
3. 超过 7 天，必须重新登录
```

**结论**：对于待办应用，Hertz 的方案已经够用！

---

### Q4: Hertz token 是存到内存里的吗？服务端重启会退出登录吗？

**关键结论：JWT Token 不存储在服务端！服务端重启不影响登录！**

**JWT 的无状态特性**：

```
登录流程:
┌─────────┐                    ┌─────────┐
│  客户端  │ ─(用户名密码)────→ │ 服务端   │
│         │                    │         │
│         │ ←──(JWT token)──── │ 生成token│
│ 存储到   │                    │ 不保存！ │
│ 本地     │                    └─────────┘
└─────────┘

后续请求:
┌─────────┐                    ┌─────────┐
│  客户端  │ ─(请求+token)────→ │ 服务端   │
│         │                    │         │
│ 从本地   │                    │ 1.验证签名│
│ 取token  │                    │ 2.检查过期│
│         │                    │ 3.提取信息│
│         │ ←───(响应)──────── │ 不查数据库│
└─────────┘                    └─────────┘
```

**对比不同存储方式**：

| 存储方式 | Token 存哪里？ | 服务端重启影响？ |
|---------|--------------|----------------|
| **JWT** | 客户端（浏览器/App） | ❌ 不影响 |
| **Session** | 服务端内存/Redis | ✅ 影响 |

**为什么服务端不需要存储？**

JWT token 包含了所有需要的信息：
```json
{
  "user_id": 2,
  "username": "logintest",
  "exp": 1761837205,  // 过期时间
  "iat": 1761836305   // 签发时间
}
```

服务端验证时只需要：
1. 验证签名（token 没被篡改）
2. 检查过期时间（exp < now）
3. 提取用户信息（user_id）

**不需要查数据库！不需要查 Redis！**

---

## 2025-11-09 中间件工作原理与调用链

### Q1: `[]app.HandlerFunc` 是什么类型？里面存了什么？

**类型定义**：
```go
type HandlerFunc func(c context.Context, ctx *RequestContext)
```

所以 `[]app.HandlerFunc` 就是：**一个函数切片（动态数组）**

**里面存的是**：
- **类型**：函数
- **签名**：`func(context.Context, *app.RequestContext)`
- **内容**：实际的验证逻辑代码（如 JWT 解析、日志记录等）

**具体示例**：

```go
func _deletebyscopeMw() []app.HandlerFunc {
    return []app.HandlerFunc{
        middleware.JWTMiddleware.MiddlewareFunc()  // 返回一个函数
    }
}
```

**`MiddlewareFunc()` 返回的函数内部逻辑**（简化版）：

```go
func(c context.Context, ctx *app.RequestContext) {
    // 1. 从请求头获取 token
    tokenString := ctx.GetHeader("Authorization")  // "Bearer eyJhbGc..."

    // 2. 去掉 "Bearer " 前缀
    tokenString = strings.TrimPrefix(tokenString, "Bearer ")

    // 3. 解析并验证 token
    token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
        return jwtSecret, nil  // 用密钥验证签名
    })

    if err != nil {
        // 验证失败
        ctx.JSON(401, utils.H{
            "status": 401,
            "msg":    "invalid token",
        })
        ctx.Abort()  // 中止请求，不执行后续处理器
        return
    }

    // 4. 提取用户信息
    claims := token.Claims.(jwt.MapClaims)
    userID := uint(claims["user_id"].(float64))
    username := claims["username"].(string)

    // 5. 存入上下文，供后续处理器使用
    ctx.Set("user_id", &JWTClaims{
        UserID:   userID,
        Username: username,
    })

    // 6. 继续执行下一个处理器
    ctx.Next()
}
```

---

### Q2: 中间件函数对应哪个接口？命名规则是什么？

**实际映射关系**（`biz/router/memogo/api/memogo.go:22`）：

```go
_v1.DELETE("/todos", append(_deletebyscopeMw(), api.DeleteByScope)...)
```

**`_deletebyscopeMw()` 对应的接口是：`DELETE /v1/todos`**

**命名规则**（hz 工具自动生成）：

| Thrift 方法名 | 中间件函数名 | HTTP 路由 |
|------------|-------------|----------|
| `DeleteByScope` | `_deletebyscopeMw()` | `DELETE /v1/todos` |
| `ListTodos` | `_listtodosMw()` | `GET /v1/todos` |
| `CreateTodo` | `_createtodoMw()` | `POST /v1/todos` |

**命名转换规则**：
- 将 Thrift 方法名转为**小写+下划线**
- 前缀加 `_`
- 后缀加 `Mw`（Middleware 的缩写）

例如：`DeleteByScope` → `_delete_by_scope` → `_deletebyscopeMw`

---

### Q3: 中间件是如何起到 JWT 验证作用的？

**这是一个中间件链机制**：

```go
// 在 memogo.go:22
_v1.DELETE("/todos", append(_deletebyscopeMw(), api.DeleteByScope)...)
```

**执行顺序**：
```
请求到达
  ↓
[JWT 中间件]（来自 _deletebyscopeMw()）
  ├─ 验证 Authorization header
  ├─ 解析 JWT token
  ├─ 验证签名和过期时间
  ├─ 提取 user_id 存入上下文
  ├─ 如果验证失败 → 返回 401，不执行后续处理器
  └─ 如果验证成功 → ctx.Next() 继续执行
  ↓
[业务处理器] api.DeleteByScope
  └─ 从上下文获取 user_id
  └─ 调用 service 层删除数据
  └─ 返回响应
```

**关键点**：
- 中间件通过 `ctx.Abort()` 中断请求
- 中间件通过 `ctx.Set()` 传递数据给后续处理器
- 中间件通过 `ctx.Next()` 继续执行链

---

### Q4: 为什么会有多个中间件？是重复验证权限吗？

**不是重复验证，而是每个中间件负责不同的职责！**

```go
func _deletebyscopeMw() []app.HandlerFunc {
    return []app.HandlerFunc{
        middleware.JWTMiddleware.MiddlewareFunc(),  // 1. 身份认证
        middleware.RBACMiddleware(),                // 2. 权限控制
        middleware.LoggerMiddleware(),              // 3. 日志记录
        middleware.RateLimitMiddleware(),           // 4. 限流保护
    }
}
```

**每个中间件的作用**：

### 1️⃣ **JWT 中间件** - 身份认证
- **职责**：验证你是谁（Authentication）
- **做什么**：
  - 解析 `Authorization: Bearer <token>`
  - 验证 token 是否有效、是否过期
  - 提取 `user_id`、`username` 存入上下文
- **失败时**：返回 401 Unauthorized

### 2️⃣ **权限中间件** - 授权检查
- **职责**：验证你能做什么（Authorization）
- **做什么**：
  - 检查用户角色（admin、user、guest）
  - 检查是否有权限执行此操作
  - 例如：只有 admin 能删除所有人的待办
- **失败时**：返回 403 Forbidden

### 3️⃣ **日志中间件** - 记录请求
- **职责**：审计追踪
- **做什么**：
  - 记录谁（user_id）
  - 在什么时候（timestamp）
  - 做了什么（DELETE /v1/todos?scope=all）
  - 结果如何（成功/失败）

### 4️⃣ **限流中间件** - 防止滥用
- **职责**：保护服务
- **做什么**：
  - 限制每个用户每分钟只能调用 100 次
  - 防止恶意攻击或误操作
- **失败时**：返回 429 Too Many Requests

**实际执行流程示例**：

假设用户发起：`DELETE /v1/todos?scope=all`

```
请求到达
  ↓
[JWT 中间件]
  → 验证 token ✓
  → 提取 user_id=123
  ↓
[权限中间件]
  → 检查 user_id=123 的角色
  → 角色是 "user"，不是 "admin" ✗
  → 返回 403: "只有管理员能删除所有待办"
  ↓
❌ 请求被拦截，不会执行到业务处理器
```

如果用户是 admin：

```
请求到达
  ↓
[JWT 中间件] ✓ user_id=1, role=admin
  ↓
[权限中间件] ✓ admin 有权限
  ↓
[日志中间件] ✓ 记录：admin 在 2025-11-09 删除所有待办
  ↓
[限流中间件] ✓ 今天调用次数 < 100
  ↓
[业务处理器] 执行删除逻辑
  ↓
返回 200 成功
```

**类比生活场景**：

想象进入公司机房：

1. **门禁卡（JWT）** - 验证你是员工
2. **权限系统（RBAC）** - 验证你有机房权限
3. **登记簿（Logger）** - 记录你几点进入
4. **限制规则（Rate Limit）** - 防止一个人频繁进出

每一层都有独立的作用，缺一不可！

---

### Q5: 中间件切片的内存结构是什么样的？

**可视化存储结构**：

```go
[]app.HandlerFunc{
    // 索引 0：JWT 验证函数
    0: func(c context.Context, ctx *app.RequestContext) {
        // 验证 token...
        ctx.Next()  // 执行索引1的函数
    },

    // 索引 1：日志记录函数
    1: func(c context.Context, ctx *app.RequestContext) {
        start := time.Now()
        // 记录请求开始...
        ctx.Next()  // 执行索引2的函数
        // 记录请求结束，耗时...
    },

    // 索引 2：限流检查函数
    2: func(c context.Context, ctx *app.RequestContext) {
        if rateLimitExceeded() {
            ctx.JSON(429, "too many requests")
            ctx.Abort()
            return
        }
        ctx.Next()  // 执行业务处理器
    }
}
```

**内存中的实际存储**：

```
切片内存结构：
┌─────────────────────────────────────┐
│ []app.HandlerFunc                   │
├─────────────────────────────────────┤
│ [0] → 函数地址: 0x12ab340           │  ← JWT 验证函数的内存地址
│ [1] → 函数地址: 0x12ab440           │  ← 日志函数的内存地址
│ [2] → 函数地址: 0x12ab540           │  ← 限流函数的内存地址
└─────────────────────────────────────┘
```

**总结**：`[]app.HandlerFunc` 就像一个**任务清单**，里面每一项都是一个**要执行的函数**！

---

## 延伸阅读

- Hertz JWT 中间件文档: https://www.cloudwego.io/zh/docs/hertz/tutorials/basic-feature/middleware/jwt/
- JWT 官方规范 RFC 7519: https://datatracker.ietf.org/doc/html/rfc7519
- Go 中间件设计模式: https://www.alexedwards.net/blog/making-and-using-middleware

---

*最后更新：2025-11-09（新增中间件工作原理与调用链）*
