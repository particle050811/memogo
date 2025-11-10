# API 设计最佳实践

本笔记记录 RESTful API 设计中的常见模式、最佳实践和设计决策。

---

## 2025-11-10：API 版本控制与路径设计

### Q1: `/v1` 是 RESTful 规范的要求吗？

**答案**：❌ **不是**。`/v1` 不是 RESTful 规范强制要求的内容，而是**业界最佳实践**（Best Practice）。

#### RESTful 规范的核心原则

REST（Representational State Transfer）是 Roy Fielding 在博士论文中提出的架构风格，核心原则包括：

1. **客户端-服务器分离**
2. **无状态**（每个请求包含所有必要信息）
3. **可缓存**
4. **统一接口**（使用标准 HTTP 方法：GET/POST/PUT/DELETE/PATCH）
5. **分层系统**
6. **按需代码**（可选）

**这些原则里并没有提到版本控制**。

#### 版本控制的争议

在 REST 纯粹主义者看来，URL 中加版本号**违反了 REST 原则**：

```
❌ REST 纯粹主义观点：
   同一个资源应该只有一个 URI
   /v1/todos 和 /v2/todos 表示不同资源，违背了资源唯一性

✅ REST 纯粹主义推荐：
   使用 HTTP 头部进行版本控制
   Accept: application/vnd.myapi.v1+json
```

#### 为什么 `/v1` 仍然广泛使用？

尽管不符合纯粹的 REST 原则，但 URL 版本控制在实际工程中最流行：

| 方式 | 优点 | 缺点 |
|------|------|------|
| **URL 路径** (`/v1/todos`) | 简单直观、浏览器友好、易于测试 | 不符合 REST 纯粹主义 |
| **请求头** (`Accept: v1`) | 符合 REST 原则、资源 URL 不变 | 测试不便、浏览器不友好 |
| **查询参数** (`/todos?v=1`) | 实现简单 | 缓存不友好、语义不清晰 |

#### 版本控制的价值

**1. 向后兼容性**
- 当你需要修改 API 结构（比如修改响应格式、字段名称）时，老版本的客户端可能会出错
- 通过版本号，可以让老客户端继续使用 `/v1`，新客户端使用 `/v2`

**2. 平滑升级**
```
/v1/todos      → 返回旧格式数据
/v2/todos      → 返回新格式数据（比如增加了更多字段）
```

**3. 明确 API 契约**
- 客户端可以明确知道自己使用的是哪个版本
- 服务端可以同时维护多个版本，逐步淘汰老版本

#### 实际示例

假设将来你想改变 Todo 的响应结构：

```thrift
// v1: 简单结构
struct Todo {
  1: i64        id
  2: string     title
  3: string     content
  4: TodoStatus status
}

// v2: 增强结构（添加了标签、优先级等）
struct TodoV2 {
  1: i64        id
  2: string     title
  3: string     content
  4: TodoStatus status
  5: list<string> tags       // 新增：标签
  6: i32        priority     // 新增：优先级
  7: string     assignee     // 新增：分配人
}
```

这时你可以：
- `/v1/todos` → 继续返回旧格式（兼容老客户端）
- `/v2/todos` → 返回新格式（新功能）

#### 结论

**`/v1` 是为了未来可能的 API 升级预留的版本标识**，即使现在只有 v1 版本，这也是一个良好的设计习惯。

**这是"RESTful 风格的 API 设计最佳实践"，而不是"RESTful 规范本身"。**

**相关文件**：
- `idl/memogo.thrift:171` - API 路由定义
- `biz/router/memogo/api/memogo.go:21` - 生成的路由注册

---

### Q2: `/api` 前缀是 RESTful 规范要求的吗？为什么有的项目有，我的项目没有？

**答案**：❌ **不是**。`/api` 前缀也不是 RESTful 规范要求，同样是**可选的设计模式**。

#### 常见的 API 路由设计

```
方案1（本项目）:    /v1/auth/register
方案2（其他项目）:  /api/v1/auth/register
```

#### 为什么有的项目加 `/api` 前缀？

**1. 前后端混合部署**
```
/              → 前端页面（HTML）
/about         → 关于页面
/contact       → 联系我们
/api/v1/...    → API 接口
```
通过 `/api` 明确区分页面路由和 API 路由

**2. 网关路由分流**
```nginx
# Nginx 配置示例
location /api/ {
    proxy_pass http://backend-api:8080;  # 转发到后端 API 服务
}

location / {
    proxy_pass http://frontend-server;   # 转发到前端静态资源服务器
}
```

**3. 语义更清晰**
- 一眼就能看出这是 API 接口
- 便于日志分析、监控、限流等

**4. GraphQL 与 REST 共存**
```
/api/rest/v1/users     → REST API
/api/graphql           → GraphQL API
```

#### 为什么本项目没有 `/api`？

**1. 纯后端 API 服务**
- MemoGo 是纯 API 服务，不提供网页，所以不需要 `/api` 来区分

**2. 简洁设计**
- 既然整个服务都是 API，加 `/api` 就显得冗余了
- 遵循"最小必要"原则

**3. Thrift IDL 定义**
```thrift
// idl/memogo.thrift:171
service AuthService {
  AuthResp Register(1: RegisterReq req) (api.post = "/v1/auth/register")
  // 路由直接定义为 /v1/...，没有 /api 前缀
}
```

#### 如何添加 `/api` 前缀？

如果将来需要，只需修改 Thrift 文件：

```thrift
service AuthService {
  // 从 /v1/auth/register 改为 /api/v1/auth/register
  AuthResp Register(1: RegisterReq req) (api.post = "/api/v1/auth/register")
  AuthResp Login(1: LoginReq req)       (api.post = "/api/v1/auth/login")
  ...
}
```

然后重新生成代码：
```bash
hz update -idl idl/memogo.thrift
```

#### 结论

- 没有 `/api` 前缀**不是问题**
- 这只是项目选择了**方案1**（简洁风格）
- 两种方案都合理，看项目需求
- 大公司案例：
  - GitHub API: `https://api.github.com/users/xxx`（使用子域名，无 `/api`）
  - Twitter API: `https://api.twitter.com/2/tweets`（使用子域名，有版本号）
  - Stripe API: `https://api.stripe.com/v1/charges`（使用子域名 + `/v1`）

**相关文件**：
- `idl/memogo.thrift` - 路由定义
- `biz/router/memogo/api/memogo.go:19-42` - 生成的路由注册

---

### Q3: 批量更新中的 `from` 字段是否多余？

**问题背景**：

批量更新状态接口定义：
```thrift
// idl/memogo.thrift:78-82
struct UpdateAllStatusReq {
  1: optional string authorization (api.header = "Authorization")
  2: TodoStatus      from_status   (api.query = "from")
  3: TodoStatus      to_status     (api.query = "to")
}
```

使用示例：
```bash
PATCH /v1/todos/status?from=0&to=1
# 含义：将所有 TODO(0) 的事项改为 DONE(1)
```

**用户质疑**：修改操作不需要看改之前是什么样，`from` 字段是否多余？而且去掉判断条件，顺序统一写入可能更快。

#### 方案对比

**方案1：保留 `from`（当前设计）**

```bash
PATCH /v1/todos/status?from=0&to=1
# SQL: UPDATE todos SET status=1 WHERE user_id=? AND status=0
```

**优点：**
- ✅ **精确控制**：只修改特定状态的事项
- ✅ **防止误操作**：如果数据库里有其他状态（比如将来扩展了 ARCHIVED=2），不会被误改
- ✅ **语义清晰**："把所有 X 状态改成 Y 状态"

**缺点：**
- ❌ 参数多一个，稍显复杂
- ❌ 需要额外的条件判断（但性能影响微乎其微）

---

**方案2：去掉 `from`**

```bash
PATCH /v1/todos/status?to=1
# SQL: UPDATE todos SET status=1 WHERE user_id=?
```

**优点：**
- ✅ 更简洁
- ✅ 顺序扫描，CPU 分支预测友好（在某些场景下可能略快）

**缺点：**
- ❌ **缺少精确控制**：会修改所有事项（包括已经是目标状态的）
- ❌ **可能产生不必要的数据库写入**：
  ```sql
  -- 会执行：UPDATE todos SET status=1 WHERE user_id=?
  -- 包括那些已经是 status=1 的记录（产生无意义的 redo log）
  ```

#### 关键对比：性能 vs 语义

**用户观点：顺序统一写入少了判断的步骤，难道不会更快？**

**技术分析：**

1. **性能差异微乎其微**
   - 在数据量不大的情况下（典型个人待办 < 1000 条），有无 `status=0` 条件判断的性能差异可忽略
   - 数据库优化器会根据索引选择最优执行计划
   - 实际瓶颈通常在网络延迟、磁盘 I/O，而不是这个简单的条件判断

2. **无意义写入的代价**
   - MySQL InnoDB 引擎即使写入相同值也会产生 redo log、binlog
   - 主从复制会传播这些无意义的更新
   - 触发器、审计日志等会被无意义地触发

3. **实测才是硬道理**
   - 如果真的关心性能，应该用 benchmark 实测
   - 在典型场景下，`status=0` 条件判断的性能影响 < 1ms

#### 实际应用场景对比

```bash
场景1: "批量完成所有待办"
  方案1: PATCH ?from=0&to=1  ← 只改 TODO，语义清晰
  方案2: PATCH ?to=1         ← 改所有（包括已经是 DONE 的）

场景2: "把已完成的重新标记为待办"
  方案1: PATCH ?from=1&to=0  ← 只改 DONE
  方案2: PATCH ?to=0         ← 会把所有都改成 TODO（可能不是期望行为）

场景3: 将来扩展了更多状态
  enum TodoStatus {
    TODO = 0,
    DONE = 1,
    ARCHIVED = 2,  // 新增：已归档
    DELETED = 3    // 新增：已删除（软删除）
  }

  用户操作："一键完成今天的所有待办"

  方案1: PATCH ?from=0&to=1  ← 只改 TODO，不影响归档和已删除
  方案2: PATCH ?to=1         ← 把所有都改成 DONE，包括已归档、已删除的！💥
```

#### 结论与建议

**保留 `from` 字段**，核心理由：

1. **语义清晰** > 性能微优
   - "从 TODO 改为 DONE" vs "全部改为 DONE"
   - API 设计首先要清晰表达意图

2. **防止误操作** > 简洁性
   - 避免修改不该修改的记录
   - 特别是在状态扩展后，`from` 字段能防止严重 bug

3. **可扩展性** > 当下简便
   - 将来增加状态时不会出问题
   - 不需要破坏性变更 API

4. **性能不是瓶颈**
   - 条件判断的性能影响 < 1ms
   - 实际瓶颈在网络、数据库连接池、锁竞争等
   - 过早优化是万恶之源

#### 性能优化的正确方向

如果真的关心批量更新性能，应该考虑：

1. **批量操作优化**
   ```go
   // 差：逐条更新
   for _, id := range ids {
       db.Model(&Todo{}).Where("id = ?", id).Update("status", newStatus)
   }

   // 好：单条 SQL 批量更新
   db.Model(&Todo{}).Where("id IN ?", ids).Update("status", newStatus)
   ```

2. **事务优化**
   ```go
   // 差：多次事务
   // 好：单次事务批量提交
   ```

3. **索引优化**
   ```sql
   -- 确保 (user_id, status) 有复合索引
   CREATE INDEX idx_user_status ON todos(user_id, status);
   ```

4. **连接池配置**
   ```go
   db.SetMaxOpenConns(100)
   db.SetMaxIdleConns(10)
   ```

**相关文件**：
- `idl/memogo.thrift:78-87` - 批量更新请求定义
- `biz/handler/memogo/api/todo_manage_service.go` - 处理器实现
- `biz/service/todo_service.go` - 业务逻辑层

---

## 延伸阅读

### RESTful API 设计
- [RESTful API 设计规范](https://restfulapi.net/)
- [Microsoft REST API Guidelines](https://github.com/microsoft/api-guidelines/blob/vNext/Guidelines.md)
- [Google API 设计指南](https://cloud.google.com/apis/design)
- [Roy Fielding 的 REST 博士论文](https://www.ics.uci.edu/~fielding/pubs/dissertation/rest_arch_style.htm)

### API 版本控制
- [API Versioning Best Practices](https://www.freecodecamp.org/news/how-to-version-a-rest-api/)
- [Stripe API 版本策略](https://stripe.com/blog/api-versioning)
- [Semantic Versioning 2.0.0](https://semver.org/)

### API 设计最佳实践
- [PayPal API 设计模式](https://github.com/paypal/api-standards/blob/master/api-style-guide.md)
- [Zalando RESTful API Guidelines](https://opensource.zalando.com/restful-api-guidelines/)
- [Heroku API 设计指南](https://geemus.gitbooks.io/http-api-design/content/en/)

### 性能优化
- [Database Performance for Developers](https://use-the-index-luke.com/)
- [MySQL Query Optimization](https://dev.mysql.com/doc/refman/8.0/en/optimization.html)
- [Premature Optimization（过早优化是万恶之源）](https://wiki.c2.com/?PrematureOptimization)

---

## 总结

- **`/v1` 和 `/api` 都不是 RESTful 规范要求**，而是业界最佳实践
- **URL 版本控制**虽然不符合 REST 纯粹主义，但因实用性而广泛采用
- **API 设计应优先考虑清晰性、可维护性、可扩展性**，性能优化应基于实测数据
- **保留 `from` 字段**能提供更好的语义表达和错误防护

---

*最后更新：2025-11-10*
