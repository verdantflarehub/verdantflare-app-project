# verdantflare-app-project

VerdantFlare AppCenter 项目管理应用。

## 1. 当前范围

`dashboard/` 是独立 Vue 3／Vite 页面，消费 Studio 注入的 AppSDK 1.1 和 DashboardHost；不依赖 Studio 源码或 Wails runtime。保留项目分类、固定修订、角色／音乐／视频内容与媒体组件，并迁入 AI 短剧的概览、剧本设定、素材状态／版本、镜头制作、交付与连续性锚点。本机工作副本页通过可选 `workspace.local` 使用宿主原生目录／文件选择，支持下载、显式保存、导入、原提交恢复、冲突处理和切换修订；页面不持有绝对路径或原生绑定。数字世界带入页选择可写项目，固定基准修订与用途后明确调用现有 `use_asset`，在发送前保留精确提交记录以便查询和原请求重试。当前已完成类型与纯单元检查，真实 Web／Wails 传输和写入仍待验证。

短剧入库使用本域窄 API 保存原准备信息，再以固定定位通过 `navigation.openApp` 前往 World 确认发布；返回后只信任服务端真实回执，保存原关联提交并支持丢失响应恢复。既有项目关联可以在 World 暂时离线时恢复原提交。只有重新向服务端确认原准备为失败／冲突，用户才可归档旧意图并按当前修订重新核对；未知结果保持原编号。大于 1 MiB 的既有正文不经 Dashboard 回传，浏览器始终只提交固定 ID。业务与接口依据见中央拆分方案第 7.4 节。

项目列表保留新建入口，支持原六分类、名称、入口相对路径与初始文本。新建通过本域 `create` 保存原始请求后发送，完整 UTF-8 JSON 限制 1 MiB；未知结果按原编号恢复，确认终态后才能继续新建。新建与恢复的 15 项纯行为测试及类型检查通过，真实 Web／Wails 仍待验收。

独立 Go 后端提供 Project 操作、MCP、Artifact Authority、跨 World 受控读取与持久授权协调；后台恢复原提交和终态通知。成员权限变更保存意图，未排空的授权保留使变更保持进行中，原授权者可用同一变更 ID 重试。当前只支持显式新空库初始化，旧数据接管、可信安装、Web／Wails 运行与独立升级尚待真实验收，既有业务写入未切换。

## 2. 检查与交付

使用 Node.js 22 和 pnpm 9.15.9。安装依赖时执行 `pnpm --dir dashboard install --frozen-lockfile --ignore-scripts`。

- 本机轻量检查：`pnpm --dir dashboard typecheck`。
- 无服务单元测试：`pnpm --dir dashboard test`。
- Go 无服务检查：`go test ./...`、`go vet ./...`。测试不启动数据库或本机服务。
- 网络协调集成纯编译：`go test -tags integration ./internal/coordination -run '^$'`。仅在 5090 dev 配置受控 `PROJECT_TEST_DATABASE_URL` 后执行 `go test -tags integration ./internal/coordination -run '^TestCoordinationNetwork' -count=1 -timeout=5m`，创建并清理两个独立随机库，真实 TLS 连接覆盖响应丢失、失联、重建与 fence；业务授权／CAS 为 SQL 替身，Artifact 不参与。
- Actions 编译入口：`pnpm --dir dashboard build`，生成相对路径前端资源及绑定真实 SHA-256 的索引与 Dashboard 声明。该命令不等于已执行发布。

生成声明的稳定 appId 为 `project`，最低 AppSDK 为 `1.1.0`。发行材料含 Dashboard 声明、前端摘要索引及同摘要的 Navigation 声明；声明只申请能力，部署登记、服务权限与制品校验由平台完成。生产服务启动校验制品后，只向持有服务凭据和受控主体的 Studio 返回原始字节，构建产物不进入 Git。

`release` 分支的 Actions 根据 `VERSION` 构建前后端镜像，推送前检查版本尚未存在。当前版本为 `0.1.0`，本轮未执行 Actions 或镜像推送。本机不执行容器构建、业务服务运行、数据库迁移或集成测试。

命令 `project migrate-fresh` 仅用于经授权的远程空库初始化，数据库写门默认关闭；全新空环境另行显式执行 `project activate-fresh`。普通启动校验迁移与已开放写门，不自动修改数据库。迁移账户与运行账户独立：运行账户不得是超级用户、Schema／表所有者，不得创建 Schema 对象或修改写门、迁移历史及外部 fence。运行账户需要本域数据库 CONNECT、`studio` Schema USAGE、业务／协调表适用权限，以及固定只读锁函数 `studio.project_locked_fence(text)` 的显式 EXECUTE；该函数不向运行账户授予 fence 写权限。连接、TLS、独立服务凭据及外部恢复 fence 从受控环境读取。旧迁移原文在 `migrations/legacy/`，不可修改或用新空库命令代替旧库接管；中央接管工具按共同归档逐域导入，不在应用运行时连接旧库或 World 库。

### 2.1 监听与内部发现

业务监听由 `PROJECT_LISTEN` 配置，非 loopback 必须提供 `PROJECT_TLS_CERT`／`PROJECT_TLS_KEY`。业务端口拒绝 `/mcp`；独立 MCP 监听由 `PROJECT_MCP_LISTEN` 配置，复用同一 TLS 证书，必须与业务端口不同。部署模板分别使用业务 `8443`、MCP `8444`。

`PROJECT_MCP_LISTEN`、`PROJECT_MCP_ADVERTISE_URL`、`ETCD_ENDPOINTS` 同时缺省时关闭 MCP，部分配置拒绝启动。服务独立维护 `/verdantflare/mcp/services/project`，只登记本域工具、制品版本和内部 `/mcp` 地址，不登记凭据；不同现存 endpoint 不自动覆盖。旧 publisher 退出及唯一写入者确认后才切换登记，MCP 仅允许 Studio 来源访问。完整受控配置及切换步骤遵循中央拆分方案。

已有独立库若仅应用 `0001`／`0002`，由迁移账户显式运行 `project migrate-gates` 追加 `0003_locked_fence.sql`；不修改原历史、gate 状态或外部 fence。正常运行不会自动升级。

0.1.16 启动要求生命周期扩展；已有库由迁移账户显式运行 `project migrate-lifecycle`，从受控配置设置 `PROJECT_DATABASE_URL` 与两个既有 fence。扩展使用独立记录，保留原迁移摘要及写门。迁移后须完成新版本滚动再启用删除；存在已删除记录时不得回退到不支持生命周期的旧版本。删除与恢复的业务语义以中央 Project／World 契约第 3.1 节为准。

## 3. 中央设计

- [Project／World 服务拆分与页面准备](https://github.com/verdantflarehub/verdantflare-design/blob/dev/changes/change_20261011_project_world_service_split.md)。
- [Project／World 旧数据接管与写门](https://github.com/verdantflarehub/verdantflare-design/blob/dev/changes/change_20261011_project_world_handoff.md)。
- [AppDashboard 接入契约](https://github.com/verdantflarehub/verdantflare-design/blob/dev/docs/design/contracts/app-dashboard-v1.md)。
- [Project／World 操作契约](https://github.com/verdantflarehub/verdantflare-design/blob/dev/docs/design/contracts/project-world-v1.md)。
- [GitHub Actions 指南](https://github.com/verdantflarehub/verdantflare-design/blob/dev/docs/GithubAction.md)。

产品与 API 契约以中央设计仓库为事实源，本仓库只维护实现、检查与构建入口。
