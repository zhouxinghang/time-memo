# 时光记 TimeMemo

一个仅成员可见的论坛。React + shadcn/ui + Tailwind CSS 前端、Go 后端、PostgreSQL 数据库。**生产环境只有一个 Go 应用进程**：网页构建后嵌入 Go 可执行文件，由它同时提供网页、JSON 接口与图片服务。PostgreSQL 独立运行。

## 已实现

- 用户名 / 密码登录，HttpOnly 会话，退出登录、登录过期、登录限速及同源保护。
- 文章列表与详情、全文关键字搜索、话题与标签筛选、最新 / 最多讨论排序、分页。
- Markdown 文章与评论、图片插入、发布前确认。文章草稿暂存在当前浏览会话中，按用户隔离。
- 私人收藏；所有登录成员对已发布内容具有相同的阅读和评论权限，没有文章级权限或特殊用户角色。
- **文章、评论、附件关联和图片元数据只允许追加**。没有更新或删除接口；PostgreSQL 触发器拒绝 `UPDATE`、`DELETE` 和 `TRUNCATE`，包括直接 SQL 操作。
- **只有独立 CLI 可以创建用户**，没有注册页面或注册 API。初始数据库没有预设账号。
- 未登录无法读取文章、评论、搜索结果、社区统计或图片。图片响应使用 `private, no-store`。
- JPEG / PNG / WebP 图片上传：最多 12 MB、2500 万像素；服务端在内存中解码，长边缩至最多 1920px，以质量 82 重新编码为 JPEG，透明区域铺白，校正 JPEG 拍摄方向并移除原始元数据。**原图不落盘**，只把压缩结果写入文件系统。压缩并不保证每张小图片都比原始字节更小。
- 自适应中文界面、键盘操作、发布预览、草稿恢复、错误提示及加载状态。

## Docker Compose 启动

需要 Docker 与 Compose。首次启动：

```sh
cp .env.example .env
# 将 .env 中 POSTGRES_PASSWORD 改为自己的长随机密码。
docker compose up -d --build
docker compose exec app /app/timememo-user create --username alice --name 林间
```

CLI 会隐藏密码输入并要求确认。密码须为 12–72 字节。用户名为 3–32 位小写字母、数字、`_` 或 `-`，且首位为字母或数字。

访问 [http://localhost:8080](http://localhost:8080)。应用端口默认仅绑定本机，适合在前面接 HTTPS 反向代理。Compose 为数据库和图片创建独立持久卷。

自动化创建账号可使用标准输入，避免将密码写入进程参数：

```sh
# 密码通过你的密码管理工具输出到管道，或在交互终端中输入：
docker compose exec -T app /app/timememo-user create --username bob --name 知秋 --password-stdin
```

## 本地开发

需要 Go 1.26+、Node.js 22.12+（推荐 24）、PostgreSQL 16+。

已安装 PostgreSQL 时，可以用 `bash scripts/dev-db.sh` 启动项目专用的本机开发数据库；必要时设置 `PG_BIN` 为 PostgreSQL 的 `bin` 目录。脚本仅监听 `127.0.0.1:55432`，将数据保存在 `data/postgres`，并输出应填入 `.env` 的连接串。它使用本机信任认证，专供开发；正式部署请使用上面的 Compose 或自己配置需要密码的数据库。执行 `bash scripts/dev-db.sh stop` 可停止开发数据库。

```sh
npm ci
go mod download
cp .env.example .env
# 按自己的数据库账号修改 DATABASE_URL；数据库须预先创建，启动时自动建表。
set -a
. ./.env
set +a
make build
./bin/timememo-user create --username alice --name 林间
./bin/timememo
```

Go 程序读取环境变量，**不会自动加载 `.env`**；Compose 会自动读取 `.env`。`make build` 先构建网页，再将网页嵌入两个 Go 可执行文件。前端更改后需重新构建 Go 服务才能更新嵌入页面。

前端热更新：在 Go 服务运行时另开终端执行 `npm run dev`，访问终端打印的 Vite 地址。开发代理把 `/api`、`/media` 转发至 `127.0.0.1:8080`。生产运行无需 Node.js。

### 环境变量

| 变量 | 说明 | 默认值 |
| --- | --- | --- |
| `DATABASE_URL` | PostgreSQL 连接串，必填 | 无 |
| `ADDR` | HTTP 监听地址 | `127.0.0.1:8080` |
| `UPLOAD_DIR` | 压缩图片目录 | `./data/uploads` |
| `COOKIE_SECURE` | HTTPS 下设为 `true` | `false` |
| `POSTGRES_PASSWORD` | Compose 数据库密码 | 必填 |

可在 `DATABASE_URL` 中省略密码并单独设置 `PGPASSWORD`，Compose 使用这种方式，避免密码中 URL 特殊字符的转义问题。反向代理应保留原始 Host 与 Origin；同源保护会拒绝跨站写请求。登录限速按 TCP 对端 IP 计算；反向代理后建议同时配置代理层限速。

## 验证

```sh
npm run build
go test ./...
go vet ./...

# 必须使用测试库；每次集成测试创建并清理自己的独立 schema。
export TEST_DATABASE_URL='postgres://user:password@localhost:5432/timememo_test?sslmode=disable'
make test-integration

# 浏览器端到端测试：使用隔离测试库，启动 18080 端口的单 Go 服务。
# 会通过独立 CLI 创建带随机密码的 QA 用户并留下测试文章。
# 测试库可整体重建；不可变数据不通过删除接口清理。
npx playwright install chromium
E2E_DATABASE_URL="$TEST_DATABASE_URL" npm run test:e2e
# 已装 Chrome 时可用 PLAYWRIGHT_CHANNEL=chrome，免装测试浏览器。
```

数据库集成测试验证鉴权、会话散列存储、跨站请求保护、图片压缩、发布前图片隔离、成员平等阅读、搜索筛选、幂等收藏以及数据库不可变触发器。浏览器测试在 1440px 和 375px 下验证 CLI 开户后的登录、带图文章发布、评论、收藏、退出及页面溢出。

## 目录

```text
cmd/timememo/          单体 HTTP 服务
cmd/timememo-user/     独立用户创建工具
internal/store/       PostgreSQL 结构、迁移与用户创建
internal/server/      鉴权、论坛接口、图片处理及集成测试
web/src/              React 界面、shadcn/ui 组件
web/dist/             前端构建产物（嵌入 Go）
tests/                浏览器端到端测试
```

## 部署与数据留存

- HTTPS 部署设置 `COOKIE_SECURE=true`；通过反向代理公开服务时配置请求大小上限至少 13 MB。
- 数据库和 `UPLOAD_DIR` 必须一起备份和恢复；不要将上传目录直接暴露为静态目录，否则会绕过登录校验。
- 数据库触发器保护普通 SQL 操作。持有数据库所有者 / 超级用户权限的人仍可以禁用触发器或删除整个数据库；运行账号应只授予需要的权限。它不等同于不可篡改的外部归档。
- 图片上传成功而草稿未发布时，图片仍归上传者可见并保留在磁盘，避免已发布内容发生悬空引用。本版不自动清理上传文件。
- 文件系统需为应用专用目录，容器默认以非 root 用户运行。
- 健康检查：`GET /healthz` 会检查数据库连通性。
