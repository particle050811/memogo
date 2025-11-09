# JWT 安全机制 - 防篡改与认证对比

本文档记录 JWT 的安全原理、签名算法以及与其他认证方式的对比。

---

## 2025-10-30 JWT 防篡改原理

### Q1: JWT 是如何防篡改的？是用 RSA 加密吗？

**关键概念：JWT 不是加密，是签名！**

**JWT 的三部分结构**（用 `.` 分隔）：

```
eyJhbGc...       .  eyJ1c2Vy...    .  aePaXs5iQ...
   ↓                    ↓                  ↓
 Header            Payload             Signature
（头部）            （载荷）             （签名）
```

**1. Header 和 Payload - Base64 编码（不加密）**

```json
// Header
{"alg":"HS256","typ":"JWT"}

// Payload
{"user_id":2,"username":"logintest","exp":1761837205}
```

⚠️ **任何人都能解码！不要在 token 里放敏感信息！**

**2. Signature - 这是防篡改的关键**

我们使用的是 **HS256**（对称加密）：

```javascript
signature = HMAC-SHA256(
  base64(header) + "." + base64(payload),
  secret_key  // 服务端保密的密钥
)
```

**两种签名算法对比**：

| 算法 | 类型 | 密钥 | 适用场景 |
|------|------|------|---------|
| **HS256** | 对称加密 | 一个密钥 | 单体应用 ✅ |
| **RS256** | 非对称加密（RSA） | 公钥+私钥 | 微服务架构 |

**HMAC-SHA256 算法原理**：

```
输入: Header.Payload (明文)
密钥: memogo-default-secret...
         ↓
    HMAC-SHA256 算法
         ↓
    32字节的签名

特性:
✓ 单向：无法从签名推导出密钥
✓ 确定性：相同输入+密钥 = 相同签名
✓ 雪崩效应：输入改1位，签名完全不同
```

**防篡改验证流程**：

```
客户端请求:
Header: Authorization: Bearer eyJhbGc...

服务端验证:
┌────────────────────────────────────┐
│ 1. 分割 Token                      │
│    header, payload, signature      │
├────────────────────────────────────┤
│ 2. 重新计算签名                    │
│    new_sig = HMAC(header.payload,  │
│                   secret_key)      │
├────────────────────────────────────┤
│ 3. 对比签名                        │
│    if new_sig != signature:        │
│        return "Invalid Token" ✗    │
├────────────────────────────────────┤
│ 4. 检查过期时间                    │
│    if exp < now():                 │
│        return "Token Expired" ✗    │
├────────────────────────────────────┤
│ 5. 提取用户信息 ✓                  │
└────────────────────────────────────┘
```

**为什么无法篡改？**

| 攻击方式 | 能成功吗？ | 原因 |
|---------|-----------|------|
| 修改 user_id | ❌ | 签名不匹配 |
| 修改过期时间 | ❌ | 签名不匹配 |
| 伪造签名 | ❌ | 不知道密钥，暴力破解需要几十亿年 |
| 重放旧 token | ✅ | **需要额外防护**（黑名单） |
| 中间人攻击 | ✅ | **必须使用 HTTPS** |

**安全最佳实践**：

```go
// 1. 密钥必须保密
func GetJWTSecret() []byte {
    secret := os.Getenv("JWT_SECRET")  // ✓ 从环境变量
    if secret == "" {
        // ✗ 生产环境必须改！
        secret = "memogo-default-secret-change-in-production"
    }
    return []byte(secret)
}

// 2. Payload 不放敏感信息
// ✗ 错误：{"password": "123456"}
// ✓ 正确：{"user_id": 2, "username": "alice"}

// 3. 必须使用 HTTPS
// 防止中间人截获 token

// 4. 设置合理的过期时间
Timeout: 15 * time.Minute  // 不要设置太长
```

**类比总结**：

JWT 签名就像快递的防伪封条：
- 包裹内容（Payload）：任何人都能看
- 防伪封条（Signature）：只有邮局（服务端）能验证真伪
- 如果有人打开包裹修改内容，封条会破损（签名不匹配），邮局会拒收

**JWT 安全性依赖于：密钥保密 + HTTPS + 合理的过期时间**

---

## 2025-11-05 HS256 签名算法详解

### Q: HS256 签名算法是如何实现 JWT 签名的？

**问题代码**（`pkg/jwt/jwt.go:49-51`）：
```go
token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
return token.SignedString(GetJWTSecret())
```

**解答**：`HS256 = HMAC-SHA256`，通过**对称加密**实现防篡改签名。

#### HS256 算法说明

```
HS256 = HMAC-SHA256
```

- **HMAC**：Hash-based Message Authentication Code（基于哈希的消息认证码）
- **SHA256**：使用 SHA-256 哈希算法
- **对称加密**：签名和验证使用**相同的密钥**

#### JWT 签名的完整流程

```go
// 步骤 1：构建 Header（固定格式）
header := {
    "alg": "HS256",     // 算法
    "typ": "JWT"        // 类型
}
headerBase64 := base64UrlEncode(json.Marshal(header))
// 结果：eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9

// 步骤 2：构建 Payload（用户数据）
payload := {
    "user_id": 123,
    "username": "testuser",
    "exp": 1730801700,
    "iat": 1730800800
}
payloadBase64 := base64UrlEncode(json.Marshal(payload))
// 结果：eyJ1c2VyX2lkIjoxMjMsInVzZXJuYW1lIjoidGVzdHVzZXIi...

// 步骤 3：拼接待签名字符串
message := headerBase64 + "." + payloadBase64
// 结果：eyJhbGci...JWT9.eyJ1c2Vy...MDB9

// 步骤 4：使用 HMAC-SHA256 生成签名
secret := GetJWTSecret()  // "memogo-default-secret-change-in-production"
signature := HMAC_SHA256(message, secret)
signatureBase64 := base64UrlEncode(signature)
// 结果：aePaXs5iQoxZaqYVnHYdksHUaD5Ofwxy09019m3uuaw

// 步骤 5：拼接最终 Token
jwt := headerBase64 + "." + payloadBase64 + "." + signatureBase64
```

#### HMAC-SHA256 算法原理

```go
// HMAC-SHA256 的伪代码实现
func HMAC_SHA256(message string, secret []byte) []byte {
    // 1. 密钥预处理（标准化为 64 字节）
    if len(secret) > 64 {
        secret = SHA256(secret)  // 太长就 hash
    }
    if len(secret) < 64 {
        secret = padWithZeros(secret, 64)  // 太短就填充
    }

    // 2. 创建两个密钥衍生值
    opad := XOR(secret, 0x5c5c5c5c...)  // 外部填充（0x5c 重复 64 次）
    ipad := XOR(secret, 0x36363636...)  // 内部填充（0x36 重复 64 次）

    // 3. 两次哈希（双层安全）
    innerHash := SHA256(ipad + message)        // 内层哈希
    finalHash := SHA256(opad + innerHash)      // 外层哈希

    return finalHash  // 32 字节的签名
}
```

**关键特性**：
- ✅ **单向性**：无法从签名反推密钥
- ✅ **确定性**：相同输入 + 密钥 = 相同签名
- ✅ **雪崩效应**：输入改 1 位，签名完全不同

#### 签名验证流程（`pkg/jwt/jwt.go:54-72`）

```go
func ParseToken(tokenString string) (*Claims, error) {
    // JWT 库内部验证逻辑：

    // 1. 分割 Token
    parts := strings.Split(tokenString, ".")
    // parts[0] = Header (base64)
    // parts[1] = Payload (base64)
    // parts[2] = Signature (base64)

    // 2. 重新计算签名
    message := parts[0] + "." + parts[1]
    expectedSignature := HMAC_SHA256(message, GetJWTSecret())
    actualSignature := base64Decode(parts[2])

    // 3. 对比签名
    if expectedSignature != actualSignature {
        return nil, ErrInvalidToken  // 签名不匹配，Token 被篡改！
    }

    // 4. 解析 Payload
    claims := json.Unmarshal(base64Decode(parts[1]))

    // 5. 验证时间字段
    if claims.ExpiresAt < time.Now().Unix() {
        return nil, ErrExpiredToken
    }

    return claims, nil
}
```

#### 防篡改原理

**场景：攻击者尝试修改 Token**

```
原始 Token：
  Header.Payload.Signature_A

攻击者修改 Payload：
  Header.Payload_Modified.Signature_A

服务器验证：
  重新计算 = HMAC_SHA256(Header.Payload_Modified, secret)
  计算结果 ≠ Signature_A
  → 验证失败 ❌ 拒绝请求
```

| 攻击方式 | 能成功吗？ | 原因 |
|---------|-----------|------|
| 修改 user_id | ❌ | 签名不匹配 |
| 修改过期时间 | ❌ | 签名不匹配 |
| 伪造签名 | ❌ | 不知道密钥，暴力破解需要几十亿年 |
| 重放旧 token | ✅ 需要额外防护 | 黑名单机制（Redis） |
| 中间人攻击 | ✅ 必须使用 HTTPS | 传输层加密 |

#### 签名算法对比

| 算法 | 类型 | 密钥 | 特点 | 适用场景 |
|------|------|------|------|---------|
| **HS256** | 对称 | 单个密钥（签名=验证） | 简单、快速 | 单体服务 ✅ |
| **RS256** | 非对称（RSA） | 公钥+私钥（私钥签名，公钥验证） | 安全、可分发公钥 | 微服务 |
| **ES256** | 非对称（椭圆曲线） | 椭圆曲线密钥对 | 更短的密钥、更高安全性 | 高安全场景 |

**本项目使用 HS256（对称算法）**，适合单体应用。

#### 密钥保护（关键）

```go
func GetJWTSecret() []byte {
    secret := os.Getenv("JWT_SECRET")
    if secret == "" {
        // ⚠️ 默认密钥仅用于开发环境
        secret = "memogo-default-secret-change-in-production"
    }
    return []byte(secret)
}
```

**安全最佳实践**：
1. ✅ **生产环境必须修改密钥**（至少 32 字节随机字符串）
2. ✅ **密钥不能泄露**（不写入代码，使用环境变量）
3. ✅ **Payload 不放敏感信息**（任何人都能 Base64 解码）
4. ✅ **必须使用 HTTPS**（防止中间人截获 Token）
5. ✅ **设置合理的过期时间**（15 分钟，不要设置太长）

#### 图解签名过程

```
┌─────────────────────────────────────────────┐
│              生成 JWT Token                  │
└─────────────────────────────────────────────┘
                     │
        ┌────────────┼────────────┐
        ▼            ▼            ▼
    Header       Payload       Secret
   (算法信息)    (用户数据)    (密钥)
        │            │            │
        ▼            ▼            │
  Base64Encode  Base64Encode     │
        │            │            │
        └────────┬───┘            │
                 ▼                │
          "Header.Payload"        │
                 │                │
                 └────────┬───────┘
                          ▼
                    HMAC-SHA256
                          │
                          ▼
                   Base64Encode
                          │
                          ▼
                      Signature
                          │
                          ▼
         "Header.Payload.Signature"
```

**类比总结**：

JWT 签名就像快递的防伪封条：
- **包裹内容（Payload）**：任何人都能看（Base64 解码）
- **防伪封条（Signature）**：只有邮局（服务端）能验证真伪
- **如果有人修改内容**：封条会破损（签名不匹配），邮局拒收

**JWT 安全性依赖于：密钥保密 + HTTPS + 合理的过期时间 + 不在 Payload 存敏感信息**

---

## 2025-10-31 JWT vs Cookie 认证选择

### Q: 为什么项目里使用 JWT（签名，Authorization 头携带），而很多网站用 Cookie 区分用户？

**结论**：本项目是面向多端的无状态 REST API，更适合使用 JWT；传统主要面向浏览器的站点更适合 Cookie + 服务端会话。

**为什么本项目用 JWT**
- 无状态、易水平扩展：不依赖服务端会话存储或粘滞会话。
- 多端/跨域友好：移动端、Postman/Apifox、SPA 都可直接用 `Authorization: Bearer <token>`。
- 降低 CSRF 风险：令牌不随浏览器自动携带，默认不受第三方站点跨站请求影响（仍需防 XSS）。
- 微服务友好：下游服务可独立校验签名，无需回源查会话。

**为什么很多网站用 Cookie**
- 浏览器原生支持：自动携带，配合 `HttpOnly/SameSite/Secure` 易控管。
- 撤销/风控强：服务端集中失效会话即可（踢人、权限变更即时生效）。
- SEO/SSR/后台系统：以浏览器为主的产品形态更贴合会话模型。

**对比要点**
- 存储位置：JWT 在客户端（Header/Storage）；Cookie 会话在服务端（内存/Redis）+ 客户端保存会话 ID。
- 安全关注：JWT 关注泄露与刷新机制；Cookie 关注 CSRF（配合 SameSite/CSRF Token）。
- 撤销与权限变更：JWT 需黑名单或短期+刷新；Cookie 服务器集中失效即可。
- 跨域：JWT（Header）更直接；Cookie 需处理 `CORS` 与 `SameSite=None; Secure`。

**你提到"用 username 区分用户"**
- 建议使用不可变的唯一标识 `user_id` 作为主身份声明；`username` 仅用于展示或冗余。
- 切记不要把敏感信息放入 JWT；全程使用 HTTPS 传输。

**代码/请求示例**
```bash
# 使用 JWT（推荐本项目）
curl -H "Authorization: Bearer <ACCESS_TOKEN>" \
     "http://localhost:8080/v1/todos?page=1&page_size=10"

# 使用 Cookie 会话（典型网站）
curl -H "Cookie: sid=abcdef123456; Path=/; HttpOnly; Secure" \
     "http://example.com/dashboard"
```

**实践建议**
- 当前项目继续用 JWT：统一从 `Authorization` 头读取并在中间件校验；令牌建议短期+刷新，配合黑名单（如 Redis）。
- 主要面向浏览器时，可采用"JWT 装进 HttpOnly Cookie"的混合方案，降低 XSS 窃取风险，同时保持无状态验证。
- 如需切换到 Cookie 会话，我可以协助改造：新增会话存储、CSRF 防护、SameSite 策略与登录/登出流程。

---

## 延伸阅读

- OWASP JWT 安全最佳实践: https://cheatsheetseries.owasp.org/cheatsheets/JSON_Web_Token_for_Java_Cheat_Sheet.html
- HMAC-SHA256 算法详解: https://en.wikipedia.org/wiki/HMAC
- JWT vs Session 深度对比：https://auth0.com/blog/session-vs-token-based-authentication/
- OWASP CSRF 防护：https://owasp.org/www-community/attacks/csrf
- CORS 与 Cookie 的 SameSite：https://web.dev/articles/samesite-cookies-explained

---

*最后更新：2025-11-09*
