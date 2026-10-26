# XYZ 教学服务平台

FireCloud 2.0。专为学校教室设计的单文件教学平台：一个 exe、内嵌 SQLite，旧笔记本就能跑，经 Cloudflare Tunnel 对外访问。

> 当前进度：**第1步完成（登录与组织模型）**。第0步的整理重构 + 账号、班级、学生、学科、任课。

## 已有功能（从 FireCloud 继承）

| 功能 | 入口 |
|---|---|
| 文件云盘：浏览、上传、拖拽文件夹、删除、二维码分享 | `/` |
| H5 课件托管：文件夹内有 `index.html` 自动当网站打开（`?manage=1` 强制显示管理界面） | `/课件/` |
| 视频播放（可拖动进度）、视频书签、图片灯箱与标注 | `/` |
| 题库：人教A版目录树、题型/难度筛选、公式渲染 | `/tiku` |
| 组卷工作台：5 套模板，导出 Word（原生公式）/ PDF / LaTeX | `/tiku` |
| 备课系统：素材树、模板画布、涂鸦、演示 | `/lesson` |
| Markdown 杂志式阅读器 | `/reader` |

## 第1步：登录与组织模型

**首次使用**：在服务器本机打开 `http://localhost/setup`，填学校名称、创建管理员账号（只能在本机做，防止别人抢先）。

**谁需要登录**

| 访问方式 | 是否要登录 |
|---|---|
| 服务器本机 `localhost` | 不用，视为管理员（录题脚本照常用）。可在 `xyz.json` 设 `"trust_localhost": false` 关闭 |
| 局域网其他电脑、手机 | 要登录 |
| 经 Cloudflare 隧道（外网） | 要登录（隧道请求带 Cf-Connecting-Ip，不会被当成本机） |
| 学生扫分享二维码 | 不用，链接带签名，只能打开那一个文件，改一个字就失效 |

**管理后台** `/admin`（各页面右下角头像也能进）

- 我的班级：我当班主任/任课的班级
- 班级：新建、设班主任、毕业归档
- 学生名单：从 Excel 复制「学号、姓名、性别」直接粘贴导入，列顺序随意、表头自动跳过；学号相同自动更新；可转班
- 任课安排：班级 × 学科表格，下拉选老师，自动保存
- 教师账号（管理员）：建账号（自动生成 6 位初始密码）、改角色、停用（立即下线）、重置密码
- 学科（管理员）：默认 14 科，可增删改排序
- 我的账号：改密码

**权限**：管理员管全部；教师可用全部教学功能，只能改自己当班主任的班级名单；学生账号已预留（练习/PK 上线后开放）。

**安全**：密码 PBKDF2-SHA256 加盐存储；登录会话 30 天（`session_days`），HttpOnly Cookie，https 下自动 Secure；同一 IP 15 分钟内错 8 次锁定；停用/重置密码会踢掉该账号所有登录；永远保留至少一个管理员。

**新接口**统一为 `/api/v1/...`（auth、users、school、subjects、classes、students、teaching、me/classes），旧接口路径不变。

**现在可以接 Cloudflare 隧道了**：建议 `xyz.json` 里把 `public_url` 设为 `https://你的域名`，二维码就会用外网地址。

## 第0步改了什么

1. **配置文件**：端口、数据目录、对外域名写在 exe 旁边的 `xyz.json`（首次运行自动生成，记事本可改），不再需要改代码重编译。模板见 `xyz.example.json`。
2. **一个数据库**：题库、组卷、素材标签、视频书签、备课方案、教材目录树全部存进 `<数据目录>\.xyz\xyz.db`。每份组卷/每个方案一行，保存只改一行，不再整文件重写，多人同时用不丢数据。
3. **自动导入旧数据**：首次启动自动导入 FireCloud 的 `.fire_tiku.db`、`.fire_papers.json`、`.fire_tags.json`、`.fire_markers.json`、`.fire_lessons\`、`.fire_curriculum.json`。**原文件不动**，随时能回到旧版。
4. **自动备份**：每天凌晨 2 点后（关机则开机补做）快照到 `.xyz\backups\`，保留 30 天。手动备份：`XYZ.exe -backup`。
5. **安全修补**：路径校验改为严格判断（修复 `D:\Fire2` 前缀绕过）；网页不能读写 `.xyz` 系统目录；上传先写临时文件再替换，中断不留半截文件；单个请求出错不会拖垮服务。
6. **去掉写死的域名**：首页 logo 原来指向 `home.150700.xyz`，改为 `/`；二维码分享在设置了 `public_url` 时用它，否则自动按访问地址生成（隧道下自动用 https）。
7. **日志**：`.xyz\xyz.log` 记录启动、导入、备份、每个接口请求。
8. **跨平台**：Windows 带托盘（菜单与原来一样）；Linux 无托盘，可用 systemd 常驻。

## 目录结构

```
cmd/xyz/            程序入口：读配置 → 开数据库 → 导入旧数据 → 备份计划 → 启动服务
internal/config     xyz.json 配置
internal/store      SQLite 打开、按版本升级表结构、导入 FireCloud 旧数据
internal/backup     每日快照与清理
internal/safepath   路径安全
internal/httpx      公共 HTTP 工具（JSON、日志、防崩溃）
internal/server     组装各模块（新模块在这里注册一行）
internal/auth       登录、会话、角色、权限中间件、分享签名、账号管理
internal/org        学科、班级、学生、任课
internal/files      云盘、H5 课件、文件直链、Markdown、分享、书签、标签
internal/tiku       题库、组卷、教材目录树、Word 导出
internal/lesson     备课方案
internal/web        内嵌前端页面（static/*.html）
internal/tray       Windows 托盘与开机自启
tools/              题库批量录入脚本（seed.py / seed.ps1 / import_md.py）
```

## 编译与运行

```bat
build.bat            :: 运行测试并生成 XYZ.exe
```

双击 `XYZ.exe`，浏览器自动打开。默认端口 80、数据目录 `D:\Fire`（与 FireCloud 相同，直接接着用）。

**注意**：XYZ 和 FireCloud 都默认占用 80 端口，不要同时运行。切到 XYZ 后，旧版里再做的修改不会自动同步过来。

Linux：`./build.sh`，然后 `./xyz -config /etc/xyz/xyz.json`。

## 接口

与 FireCloud 完全一致（`/api/tiku/*`、`/api/lesson/*`、`/api/list` 等），`tools/` 下的录入脚本无需修改。`import_md.py --fix-imported` 已改为读写新数据库。

## 路线图

- [x] 第0步 整理：模块化、配置文件、数据入库、自动备份、安全修补
- [x] 第1步 登录与权限 + 学校/教师/班级/学生/学科模型
- [ ] 第2步 题库多学科 + 大屏轮播 + 通知 + 挂载画板（DRAW）与座次表（ZCANPAI）
- [ ] 第3步 单人练习、双人PK、错题本、成绩统计
- [ ] 第4步 作业、早读跟读、班会、升旗、表扬
- [ ] 第5步 多教师试用、商用准备
