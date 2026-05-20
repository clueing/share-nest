# ShareNest 后台升级改造方案

## 1. 目标

基于现有本地代码，对 ShareNest 进行一次后台管理系统升级，完成以下目标：

1. 增加仪表盘页。
2. 增加下载记录与下载次数统计。
3. 增加系统配置页。
4. 分享过期时间改为固定选项，并支持在配置页设置默认值。
5. 管理页顶部改为分栏导航：`仪表盘`、`文件`、`分享`、`配置`。
6. 资源列表增加搜索功能。
7. 创建分享时支持在 `文件分享` / `文本分享` 之间切换，显示不同卡片。
8. 保持现有 UI 风格一致，并完善移动端适配。

---

## 2. 当前项目现状分析

### 2.1 技术与结构

- 后端为 Go 单体服务。
- 存储使用 SQLite。
- 管理后台为服务端模板 + 原生 CSS + 原生 JavaScript。
- 当前管理后台模板集中在一个页面：
  - [internal/ui/templates/dashboard.html](/c:/Users/clue/code/go/share-nest/internal/ui/templates/dashboard.html:1)
- 当前后端路由集中在：
  - [internal/server/server.go](/c:/Users/clue/code/go/share-nest/internal/server/server.go:157)
- 数据访问在：
  - [internal/repo/sqlite.go](/c:/Users/clue/code/go/share-nest/internal/repo/sqlite.go:1)
- 配置加载在：
  - [internal/config/config.go](/c:/Users/clue/code/go/share-nest/internal/config/config.go:13)

### 2.2 当前后台能力

现有后台已具备：

- 文件上传与文本分享创建
- 分享密码
- 分享过期时间
- 下载次数限制
- 资源分页列表
- 访问日志
- 下载计数累加
- 响应式表格与移动端卡片化展示

### 2.3 当前存在的问题

1. 后台仅有单页 `/admin`，模块堆叠严重。
2. 配置仍主要依赖环境变量，无法在系统内直接修改。
3. 过期时间为 `datetime-local` 自由输入，不适合运营和管理。
4. 下载记录与下载统计没有形成独立管理视图。
5. 资源列表缺少搜索与筛选。
6. 上传文件与文本创建同时展示，移动端可读性一般。

---

## 3. 配置策略调整

这次方案的核心调整之一是：

**部分配置不再依赖环境变量，而改为系统内置默认值 + 数据库存储 + 配置页修改。**

### 3.1 配置分层原则

建议将配置分为两层：

#### A. 部署级配置

这类配置仍保留环境变量或启动参数方式，不放到后台改：

- `addr`
- `data_dir`
- `db_path`
- `admin_user`
- `admin_pass`
- `session_secret`
- `base_url`

原因：

- 涉及启动、部署、安全边界
- 改动后通常需要重启
- 不适合普通后台用户在线修改

#### B. 业务级配置

这类配置改为：

- 内置默认值
- 首次启动自动写入数据库
- 后续在系统配置页修改
- 修改后立即生效或下次请求生效

建议纳入后台配置页的项：

- 最大上传大小
- 文本预览大小限制
- 列表分页大小
- 访问日志保留条数
- 默认过期时间选项

### 3.2 默认值方案

建议内置默认值如下：

| 配置项 | 默认值 | 说明 |
|---|---:|---|
| 最大上传大小 | `64 MB` | 替代当前依赖环境变量的默认上传限制 |
| 文本预览限制 | `1 MB` | 控制文本预览和解析上限 |
| 列表分页大小 | `10` | 管理后台每页条数 |
| 访问日志保留条数 | `5000` | 控制日志表规模 |
| 默认过期时间 | `7d` | 默认选中 `7天` |

### 3.3 配置持久化方案

新增数据表：

```sql
CREATE TABLE IF NOT EXISTS system_settings (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL DEFAULT '',
  updated_at INTEGER NOT NULL DEFAULT 0
);
```

建议用键值表实现，原因：

- 改动小
- 扩展方便
- 不需要频繁迁移表结构

首次启动时：

- 读取部署级配置
- 初始化数据库
- 自动检查 `system_settings`
- 若缺少业务配置项，则写入默认值

服务运行时：

- 所有业务配置优先从 `system_settings` 读取
- 未命中时回落到内置默认值

### 3.4 对 `config.Load()` 的改造建议

当前 [internal/config/config.go](/c:/Users/clue/code/go/share-nest/internal/config/config.go:29) 在启动时把大量业务配置直接读进 `Config`。

建议改为：

- `Config` 只保留部署级配置和业务默认值
- 新增 `SettingsService` 或直接由 `repo` 提供设置读取能力
- `server` 在处理请求时，从 settings 中读取最新业务配置

建议结构：

```go
type Config struct {
  Addr          string
  DataDir       string
  DBPath        string
  BaseURL       string
  SiteName      string
  AdminUser     string
  AdminPass     string
  SessionSecret string
}

type RuntimeSettings struct {
  MaxUploadSize      int64
  PreviewLimit       int64
  PageSize           int
  AccessLogRetention int
  DefaultExpireOption string
}
```

这样可以避免“改配置必须重启服务”的问题在业务层扩散。

---

## 4. 后台页面重构方案

### 4.1 顶部导航重构

管理页顶部导航调整为：

- `仪表盘`
- `文件`
- `分享`
- `配置`

建议路由：

- `GET /admin` -> 跳转 `/admin/dashboard`
- `GET /admin/dashboard`
- `GET /admin/files`
- `GET /admin/shares`
- `GET /admin/settings`

### 4.2 页面职责划分

#### 仪表盘页

展示整体运营概览：

- 资源总数
- 文件数量
- 文本数量
- 有效分享数
- 已过期分享数
- 累计下载次数
- 今日下载次数
- 最近 7 天下载趋势
- 最近资源
- 最近下载活动

#### 文件页

展示与资源相关的操作：

- 创建分享区域
- 文件分享 / 文本分享切换
- 资源搜索与筛选
- 资源列表
- 批量复制、批量删除

#### 分享页

聚焦分享与下载行为：

- 分享列表
- 分享状态
- 下载次数统计
- 下载记录
- 访问记录中的下载相关事件

#### 配置页

业务配置修改入口：

- 最大上传大小
- 文本预览限制
- 列表分页大小
- 日志保留条数
- 默认过期时间选项

---

## 5. 仪表盘方案

### 5.1 展示内容

仪表盘建议拆成 3 个区块：

#### A. 核心统计卡片

- 总资源数
- 文件数
- 文本数
- 分享总数
- 有效分享数
- 累计下载数

#### B. 趋势信息

- 最近 7 天下载趋势
- 最近 7 天新增资源趋势

首版可以先做简版：

- 先用数字列表或条状统计
- 后续再加轻量 SVG 图表

#### C. 最近活动

- 最近创建资源
- 最近下载记录
- 最近访问异常记录

### 5.2 数据查询建议

新增 repo 方法：

- `GetDashboardStats(ctx)`
- `GetDownloadTrend(ctx, days int)`
- `GetRecentItems(ctx, limit int)`
- `GetRecentDownloadLogs(ctx, limit int)`

---

## 6. 文件页方案

### 6.1 创建分享区域改造

当前页面中“上传文件”和“创建文本分享”是同时显示的两张卡片。

改造为切换模式：

- `文件分享`
- `文本分享`

交互规则：

- 默认显示 `文件分享`
- 用户切换到 `文本分享` 时，显示文本创建卡片
- 仅展示当前模式对应卡片
- 切换状态前端可记忆到 `localStorage`

### 6.2 过期时间固定选项

将当前 `datetime-local` 改为固定选项：

- `7小时`
- `6小时`
- `24小时`
- `7天`
- `30天`
- `365天`
- `永不过期`

建议内部值：

- `7h`
- `6h`
- `24h`
- `7d`
- `30d`
- `365d`
- `never`

后端统一转换为：

- `expires_at = now + duration`
- `never = NULL`

### 6.3 默认选项来源

默认选中项来自系统配置：

- `default_expire_option`

配置页修改后：

- 新建文件分享和文本分享表单默认值同步变化

### 6.4 资源搜索与筛选

文件页资源列表上方增加搜索区。

首版建议支持：

- 关键字搜索：名称、分享码
- 类型筛选：全部 / 文件 / 文本
- 状态筛选：全部 / 已加密 / 未加密 / 已过期 / 下载耗尽

请求参数建议：

- `q`
- `kind`
- `status`
- `page`

后端 repo 需要扩展当前 [ListItemsPage](/c:/Users/clue/code/go/share-nest/internal/repo/sqlite.go:203)。

建议新增：

- `ListItemsPageWithFilter(ctx, query ItemQuery)`

---

## 7. 分享页方案

### 7.1 分享列表

独立分享页顶部展示分享列表，字段建议为：

- 名称
- 类型
- 分享码
- 分享方式
- 过期时间
- 下载限制
- 已下载次数
- 分享状态
- 操作

其中分享方式可标记：

- `公开`
- `密码`
- `直达`

### 7.2 下载统计

基于 `shares.download_count` 直接展示：

- 单个分享下载次数
- 全站总下载次数
- 限制使用情况 `download_count / max_downloads`

当前下载累加逻辑已存在：

- [internal/server/server.go](/c:/Users/clue/code/go/share-nest/internal/server/server.go:636)
- [internal/repo/sqlite.go](/c:/Users/clue/code/go/share-nest/internal/repo/sqlite.go:459)

### 7.3 下载记录

当前 `access_logs` 已记录下载事件：

- [internal/repo/sqlite.go](/c:/Users/clue/code/go/share-nest/internal/repo/sqlite.go:478)

因此不建议重复造新表，首版直接复用 `access_logs`。

分享页的下载记录区建议仅筛选：

- `event_type = download`

展示字段：

- 时间
- 资源名称
- 分享码
- 结果
- 来源 IP
- 说明

### 7.4 建议的数据增强

为便于后续统计和定位，建议给 `access_logs` 增加：

- `item_id INTEGER NOT NULL DEFAULT 0`

这样后续可按资源聚合，不必仅依赖 `share_code` 与 `item_name`。

---

## 8. 配置页方案

### 8.1 配置页内容

配置页建议分为两个区域：

#### A. 可在线修改的业务配置

- 最大上传大小
- 文本预览大小限制
- 列表分页大小
- 访问日志保留条数
- 默认过期时间选项

#### B. 只读部署信息

仅展示，不允许修改：

- 站点名称
- 服务监听地址
- 数据目录
- 数据库路径
- Base URL

这样可以降低误操作风险，同时让管理员知道哪些配置仍由部署控制。

### 8.2 表单建议

配置项建议用更易理解的输入形式：

- 最大上传大小：数字 + 单位说明
- 文本预览限制：数字 + 单位说明
- 分页大小：数字输入
- 日志保留条数：数字输入
- 默认过期时间：下拉框或单选组

### 8.3 生效策略

建议分两类：

#### 实时生效

- 分页大小
- 默认过期时间
- 日志保留条数
- 预览限制

#### 当次请求后生效

- 最大上传大小

说明：

上传限制是在处理上传请求时读取设置即可，无需重启。

---

## 9. 数据层改造方案

### 9.1 新增表

新增：

```sql
CREATE TABLE IF NOT EXISTS system_settings (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL DEFAULT '',
  updated_at INTEGER NOT NULL DEFAULT 0
);
```

### 9.2 建议补充字段

对 `access_logs` 建议补充：

```sql
ALTER TABLE access_logs ADD COLUMN item_id INTEGER NOT NULL DEFAULT 0;
```

如果 SQLite 迁移处理较麻烦，也可以首版先不加，先按 `share_code` 查询。

### 9.3 repo 能力补充

新增 repo 方法建议：

- `InitSystemSettingsDefaults(ctx)`
- `GetSetting(ctx, key string)`
- `GetSettings(ctx, keys ...string)`
- `UpdateSettings(ctx, map[string]string)`
- `ListSharesPage(ctx, query ShareQuery)`
- `ListDownloadLogs(ctx, query DownloadLogQuery)`
- `GetDashboardStats(ctx)`

---

## 10. 服务层改造方案

### 10.1 路由新增

新增后台路由：

- `GET /admin/dashboard`
- `GET /admin/files`
- `GET /admin/shares`
- `GET /admin/settings`
- `POST /admin/settings`

### 10.2 handler 拆分

当前 [handleDashboard](/c:/Users/clue/code/go/share-nest/internal/server/server.go:232) 承担过多职责。

建议拆为：

- `handleAdminDashboard`
- `handleAdminFiles`
- `handleAdminShares`
- `handleAdminSettings`
- `handleUpdateSettings`

### 10.3 运行时设置读取

建议在 `Server` 中新增：

- `loadRuntimeSettings(ctx)`

用于请求级读取业务配置。

典型使用位置：

- 上传大小限制
- 文本预览限制
- 分页大小
- 默认过期选项

---

## 11. 前端与 UI 方案

### 11.1 风格原则

保持当前 UI 体系不变：

- 继续沿用现有配色与圆角
- 继续沿用 `panel`、`stat-pill`、`table-wrap`、`ghost-btn`、`primary-btn`
- 继续保持轻玻璃面板感

样式基础参考：

- [internal/ui/static/app.css](/c:/Users/clue/code/go/share-nest/internal/ui/static/app.css:177)
- [internal/ui/static/app.css](/c:/Users/clue/code/go/share-nest/internal/ui/static/app.css:187)

### 11.2 移动端方案

移动端要点：

- 顶部导航做横向滚动 tab
- 搜索区和筛选区自动折叠为单列
- 仪表盘统计卡片手机端单列展示
- 分享列表和资源列表继续沿用现有表格卡片化方案
- 创建分享卡片在手机端只显示当前模式，避免双卡堆叠

当前已有响应式基础，可在此基础上扩展：

- [internal/ui/static/app.css](/c:/Users/clue/code/go/share-nest/internal/ui/static/app.css:1339)
- [internal/ui/static/app.css](/c:/Users/clue/code/go/share-nest/internal/ui/static/app.css:1408)

### 11.3 前端交互建议

前端新增交互：

- 顶部导航激活态
- 文件分享 / 文本分享切换
- 搜索条件重置
- 配置保存提示
- 默认过期选项联动

脚本主要落点：

- [internal/ui/static/app.js](/c:/Users/clue/code/go/share-nest/internal/ui/static/app.js:1)

---

## 12. 风险与兼容性

### 12.1 配置切换风险

从“环境变量驱动”切到“数据库业务配置驱动”时，需要明确：

- 哪些配置还走环境变量
- 哪些配置以后仅由配置页修改

否则容易出现“配置源混乱”。

### 12.2 历史数据兼容

分享过期时间历史记录已存绝对时间戳，因此：

- 老数据无需迁移
- 新表单仅改变创建方式

### 12.3 访问日志扩展风险

如果对 `access_logs` 追加字段，需要确保：

- `ALTER TABLE` 可重复执行安全
- 老数据查询兼容

---

## 13. 实施顺序

建议按以下顺序实施：

1. 新增 `system_settings` 表与默认值初始化。
2. 将业务配置读取从环境变量迁移到“数据库优先、默认值兜底”。
3. 拆分后台路由与页面结构。
4. 实现顶部导航与页面骨架。
5. 实现配置页和设置保存。
6. 将过期时间改为固定选项。
7. 实现文件页资源搜索与筛选。
8. 实现分享页、下载统计、下载记录。
9. 实现仪表盘统计。
10. 最后统一做移动端样式收口与回归测试。

---

## 14. 本次方案结论

本次升级的最终方向是：

- 后台从单页堆叠改为分栏管理
- 业务配置从环境变量中抽离，改为系统默认值 + 配置页可修改
- 仅保留部署与安全相关配置继续依赖环境变量
- 将资源管理、分享管理、配置管理、统计总览彻底分层

这是当前代码结构下最稳妥、可演进、且便于后续继续扩展的方案。

后续如继续落地开发，建议优先完成：

1. `system_settings`
2. 配置页
3. 后台多页面拆分
4. 固定过期选项

