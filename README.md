# File Service

一个轻量级的 Go 文件服务，支持：

- 文件上传与分享
- 文本内容创建与分享
- 公开分享与密码分享
- 常见格式预览
- 简约管理后台

## 当前实现范围

第一版已经包含：

- 管理员登录
- 上传文件并自动生成分享链接
- 创建文本分享并自动生成分享链接
- 分享页密码校验
- 随机生成 8 位分享密码
- 带密码 URL 直达访问
- 受保护分享的快捷直达链接
- 创建成功后自动复制分享 URL
- 文本、图片、音频、视频、PDF 预览
- 资源删除
- 本地磁盘存储
- SQLite 持久化
- 自动读取项目根目录 `.env`
- 内置 favicon
- 后台资源列表分页

## 运行前提

当前代码使用：

- Go 1.22+
- SQLite 驱动：`modernc.org/sqlite`

在本机执行：

```powershell
go mod tidy
go run ./cmd/server
```

## 环境变量

项目根目录新增了 `.env.example`，建议先复制一份：

```powershell
Copy-Item .env.example .env
```

编辑 `.env` 后重启服务即可生效。

```powershell
$env:FILESERVICE_SITE_NAME="File Service"
$env:FILESERVICE_ADDR=":8080"
$env:FILESERVICE_DATA_DIR="data"
$env:FILESERVICE_BASE_URL="http://127.0.0.1:8080"
$env:FILESERVICE_ADMIN_USER="admin"
$env:FILESERVICE_ADMIN_PASS="admin123"
$env:FILESERVICE_SESSION_SECRET="replace-with-random-secret"
$env:FILESERVICE_PAGE_SIZE="10"
```

## 默认访问地址

- 后台登录：`/login`
- 管理页：`/admin`

## 下一步建议

- 增加分享过期时间与访问次数限制
- 增加搜索和筛选
- 增加 Markdown 渲染
- 增加 Office 文档转 PDF 预览
- 增加访问日志
