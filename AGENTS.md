# AGENTS.md — 接手本仓库前请先读完

本文件是给 AI 协作者（Claude Code / Qoder / Cursor / Codex 等）的入口文档。
目标：读完就能动手，且不踩已知坑。**修改任何 jukebox 相关代码前，第 3、4、5 节是必读的。**

---

## 1. 这个项目是什么

Navidrome 的私有 fork，全部二开内容是**多输出端播放**：网页播放器里可以把当前歌曲路由到
不同出声设备——浏览器 / MPD（NAS 本机声卡）/ DLNA-UPnP 渲染器 / 小爱音箱原生协议。

- 上游：`https://github.com/navidrome/navidrome`，基线 commit `ee6dd1bc`。
- **本仓库的 git 历史已重置为单条初始提交，与上游没有共同祖先**，所以只能 `diff` 不能 `merge`。
  如何比对上游见 `docs/fork-and-upstream.md`。
- remote 约定：`upstream` = 上面的只读参照（**永远不要向它 push**）；`origin` = 仓库主人的 GitHub，
  由他自己添加并 push。**不要把 `origin` 指回 navidrome。**
- 默认分支是 **`master`**，不要"顺手改成 main"：`.github/workflows/pipeline.yml`（第 4、9 行）
  与 `push-translations.yml`（第 5 行）把触发分支写死成 `master`，改名后 CI 一条都不跑。
- `.gitignore` 里的模式**都是无前导斜杠的**，因此会匹配任意层级。历史上真实踩过两次：
  `artwork/` 把整个 `core/artwork/` Go 包吞掉了（81 个源文件差点没提交），
  `AGENTS.md` 也被上游那一条忽略掉了（本 fork 已把它改回跟踪，见第 8 节）。
  新增忽略规则一律写绝对形式（`/artwork/`），并跑
  `git diff --diff-filter=D --name-only <upstream-base> HEAD` 确认为空。
- 许可：GPLv3。`LICENSE` 与所有上游文件的版权头必须保留；对外分发二进制/镜像必须连带完整源码。
- 面向人的功能文档：`docs/jukebox.md`（配置/API/驱动行为/故障排查）、
  `docs/jukebox-nas-deployment.md`（NAS 部署）、`docs/xiaomi-speakers.md`（小米协议调研）。

## 2. 常用命令

Go **必须带 build tags**，否则编译失败（sqlite 需要 `sqlite_fts5`）：

```bash
# 单包测试（Ginkgo v2 + Gomega）
make test PKG=./core/jukebox        # 98 specs
make test PKG=./server/nativeapi    # 180 specs
# 等价裸命令
go test -tags=netgo,sqlite_fts5 ./core/jukebox ./server/nativeapi

go build -tags=netgo,sqlite_fts5 -o bin/navidrome .   # main 在仓库根
make lint          # golangci-lint（配置 .golangci.yml）
gofmt -l core/jukebox server/nativeapi conf            # 必须无输出
```

前端（`ui/`，React + react-admin v3 + Vite + Vitest）：

```bash
cd ui
npm run test          # Vitest：88 文件 / 774 用例（只跑单文件：npx vitest run src/audioplayer/x.test.jsx）
npm run lint          # ESLint，--max-warnings 0
npm run check-formatting   # Prettier 只检查（CI 用）；写入用 npm run prettier
npm run build         # 产物在 ui/build/（被 go:embed 打进二进制）
```

改了前端就要 `npm run build` + 重新 `go build`，否则二进制里还是旧界面。

本地跑一个实例（**同一时间只保留一台，一个 URL**）：

```bash
./bin/navidrome -c /path/to/navidrome.toml
```

用 curl 调 native API 时，鉴权头是 `X-ND-Authorization: Bearer <token>`（见
`ui/src/dataProvider/httpClient.js`），**cookie 不管用**；token 来自 `POST /auth/login`
（JSON body，响应里的 `token` 字段）。

## 3. 音量模型（最重要的不变量）

**唯一权威是 store：`state.player.volume`，含义是"感知音量"，取值 0..1。** 三层之间靠平方换算衔接：

| 位置 | 值 | 换算 |
|---|---|---|
| Redux store / UI 滑块 / 持久化 | 0..1 感知值 | 基准 |
| `<audio>` 元素（`audioInstance.volume`） | 0..1 物理值 | `store²` |
| 远程输出设备（MPD/DLNA/小米） | 0-100 整数 | `round(store×100)` |

三条必须同时成立的规则（都在 `ui/src/audioplayer/Player.jsx`）：

1. **store → 元素**：一个 effect 把 `volume²` 写给 `audioInstance.volume`，并监听
   `volumechange` **重新断言**。原因：`navidrome-music-player` 库自己记音量、播放时从 0 淡入，
   只赋值一次会被它覆盖。
2. **store → 设备**：另一个 effect 在 `remoteActive` 时下发 `control(device,'volume',pct)`，
   200ms 防抖，并记下"用户刚刚改过音量"的时间戳。
3. **设备 → store**：`/api/jukebox/status` 轮询回来的 `volume`（0-100）经
   `adoptDeviceVolume()` 写回 store。**它必须忽略 0**：驱动在设备第一次回答音量查询前一律回报
   0，采纳 0 会让界面静音并且**被持久化下来**——这就是历史上"一刷新音量变 0%"的根因。
   另外轮询在用户拖动滑块后 3 秒内不采纳设备值，避免抢手。

配套约束（改任何一处都要检查其余几处）：

- `ui/src/store/createAdminStore.js` 把 `{queue, volume, savedPlayIndex, outputDevice}`
  白名单持久化到 localStorage 的 `state` 键；`loadState()` 时**必须把持久化的 0 量替换成
  `config.defaultUIVolume/100`**（静音只存 0，取消静音的目标值只在内存里）。
- **任何**改音量的入口都必须走 `dispatch(setVolume(...))`，包括键盘快捷键
  （`ui/src/audioplayer/keyHandlers.jsx` 的 `VOL_UP/VOL_DOWN`）。直接写
  `audioInstance.volume` 会被规则 1 立刻夺回，表现为"快捷键没作用"。
- UI 只有一处音量控件：`ui/src/audioplayer/VolumeControl.jsx`（桌面工具栏 + 移动端独立一行共用）。
  它只读/写 store，不碰元素也不碰设备——**不要把同步逻辑写进组件**。
- 移动端与桌面端**共用同一套**音量，历史上移动端被硬编码成最大音量，别退回那版。
- 远程输出时本地 `<audio>` 是**静音**的（进度条靠本地时钟），"界面有声"不等于"设备收到了"。

## 4. 仓库里存在**两套** jukebox，别搞混

| | 上游 `core/playback` | 本 fork `core/jukebox` |
|---|---|---|
| 后端 | 只有 mpv 子进程（放**服务器本地绝对路径**） | MPD / DLNA / 小米（放 **HTTP 流地址**或相对路径） |
| 对外协议 | Subsonic `jukeboxControl`（第三方 App 在用） | 自家 native JSON API，只有本 Web UI |
| 队列 | **服务端**持 `Queue`（set/add/remove/shuffle/skip） | 无状态，队列在浏览器 |
| 音量 | `SetGain(float32 0..1)` | `SetVolume(int 0-100)` |
| 配置 | `Jukebox.Devices` / `Jukebox.Default` | `Jukebox.Outputs` + 数据库持久化 + SSDP 发现 |

已知设计债（尚未修）：两者被**同一个 `Jukebox.Enabled` 点亮**。开了它，即使你只想用网页切换音箱，
后端也会：起 mpv 播放服务（无 `Devices` 时合成一个名为 `auto` 的设备）、
并通过 `server/subsonic/users.go:29` 给**所有** Subsonic 客户端广播 `jukeboxRole=true`。
于是第三方 App 的 `jukeboxControl` 会去驱动那个 mpv 设备，与网页所选输出毫无关系。
（日志证据：`Starting Jukebox service` → `1 audio devices found` → `Using audio device: auto`。）

结论与纪律：
- **不要删/改 `server/subsonic/jukebox.go` 的对外语义**——那是 Subsonic 协议契约。
- 若将来要合并，正确切法是**只合并驱动层**：把 mpv 实现成一个 `PlayerDriver`，
  `core/playback` 退化为 Subsonic 协议外壳 + 队列；协议层保持两套。
- 排障时先确认命令走的是哪一套：网页 → `/api/jukebox/*` → `core/jukebox`；
  第三方 App → `/rest/jukeboxControl` → `core/playback`。

## 5. 已知坑清单（逐条踩过，别再来一遍）

**出声链路**
- `BaseUrl` 必须是**局域网可达地址**（`http://<ip>:<port>`），它决定音箱去拉哪个主机名。
  没配时下发的是 `localhost` → "选中了但没声音"，日志有
  `Jukebox stream URL points at the loopback interface`。用 localhost 打开界面**不算**配过。
- DLNA 的 `Address` 填**设备描述文档 URL**（扫描给出的那个，如 `http://ip:9999/xxxx.xml`），
  不是 `/AVTransport/control`。控制地址要从文档里解析，填错就是 HTTP 501。
- MPD 收到的是**相对音乐库根目录**的路径（数据库 `media_file.path`），所以 MPD 的
  `music_directory` 必须与 `MusicFolder` 同一份内容，且歌曲要被 MPD 自己索引过
  （`mpc update` 或 `auto_update "yes"`）；Navidrome 扫描不会同步 MPD。
  `PathFrom/PathTo` 只是相对路径的首次字符串替换，别拿它做容器路径映射。
- 小爱音箱（`type=xiaomi`）**不支持 seek**（返回 `ErrInvalidCommand` → 400，前端不转发拖动）、
  `Stop` 用 Pause 近似、**无进度回报**（`currentTime` 恒 0）；控音量必须有 `Token`（本地 miIO）。
  真机上本地 miIO 路径仍未实测，可用型号差异靠 `Model`（`l7a`/`s12`/`l05b`）与
  `TextDirective`（`siid-aiid`）覆盖。
- DLNA 设备可能**接受 `Seek` 却忽略它**（实测小爱 S12 在 `Play` 后立刻 `Seek` 返回 200 但不跳转）。
  驱动以"回报位置是否到达目标"判定并重发（`dlnaSeekAttempts`，最多 3 次）。

**后端**
- 所有设备命令经 `DeviceManager` 的一把互斥锁串行化；驱动内部不要再加自己的锁。
- 错误映射集中在 `server/nativeapi/jukebox.go` 的 `jukeboxDriverError`：
  `ErrNoRemoteOutput`→409、`ErrInvalidCommand`→400、其他驱动错误→502（body 是驱动原文，排障先看它）。
- `Jukebox.AdminOnly=true`（默认）时只有管理员能 `select/play/control`；
  `devices`/`status` 任何登录用户可读；`/outputs` CRUD 与 `/discover` **永远**是管理员
  （里面存 token/密码）。
- 服务器重启后**选中态会重置回浏览器**，前端 `jukebox.js` 靠 `ensureSelected` 补发选择，别去掉。
- 输出配置持久化在数据库（`core/jukebox/outputs_store.go`），按 `ID` 覆盖 TOML 里的同名条目；
  `ID` 必须是 1-64 位 `[a-zA-Z0-9_-]`。

**前端与验证**
- `DeviceSelector` 渲染时即使没打开菜单也会挂载多个隐藏的 `[role=menu]` popover，
  DOM 里"存在"不等于"可见"——脚本断言只看有尺寸的那个。
- `page.evaluate()` 里的 `element.click()` **不是用户手势**，移动端媒体元素拿不到播放权限；
  验证播放要用真实点击。
- UI 验收需要**登录态**：不要自己注册账号，也不要从数据库/日志/历史里翻密码。
  把实例起好、URL 交给用户，由用户完成登录。
- 只保留一台测试实例、一个 URL。多开实例会让"哪个在响"变成玄学。

## 6. 目录地图

```
core/jukebox/            本 fork 的播放后端（唯一真源）
  driver.go              PlayerDriver 接口 + PlaybackState
  manager.go             DeviceManager 单例：设备表 / 选中项 / 命令分发 / 串行化
  driver_mpd.go          MPD：TCP，播相对路径
  driver_dlna.go         DLNA：SOAP 控制 AVTransport，播绝对流 URL
  driver_xiaomi.go       小米：miIO UDP+AES 本地 / MIoT 云端，文本指令播放
  miio_client.go         miIO 握手与加密
  xiaomi_cloud.go        小米云登录与 MIoT 调用
  discover_dlna.go       SSDP M-SEARCH 扫描 + 描述文档解析
  outputs_store.go       输出配置的 DB 读写
conf/configuration.go    [Jukebox] 段：Enabled / AdminOnly / Devices / Default / Outputs
server/nativeapi/
  jukebox.go             /api/jukebox/{devices,status,select,play,control} + 流 URL 生成 + 错误映射
  jukebox_outputs.go     /api/jukebox/outputs CRUD 与 /discover（admin-only）
server/subsonic/
  jukebox.go             上游 Subsonic jukeboxControl（另一套，勿动语义）
  stream.go + StreamAlias /rest/stream/{id}.mp3 别名端点（小爱要求带扩展名）
ui/src/audioplayer/
  Player.jsx             音量三规则、静音时钟劫持、进度校准、设备切换
  VolumeControl.jsx      唯一音量 UI（只读写 store）
  DeviceSelector.jsx     输出设备下拉
  jukebox.js             /api/jukebox 封装 + ensureSelected
  keyHandlers.jsx        键盘快捷键（音量必须走 store）
  PlayerToolbar.jsx      桌面/移动端两处装配
ui/src/jukebox/          管理 → 输出设备（list/create/edit，**没有 show**）
ui/src/reducers/playerReducer.js   outputDevice 状态与迁移
ui/src/store/createAdminStore.js   持久化白名单 + 音量 0 兜底
resources/i18n/*.json    后端 i18n（zh-Hans/zh-Hant 含 jukebox 文案）
ui/src/i18n/*.json       前端 i18n
docs/                    面向人的文档（见第 1 节）
contrib/jukebox-testing/ 假 MPD / 假 DLNA 服务器（不接真设备复现链路）
.claude/skills/ .qoder/skills/  AI 技能：build-and-test、add-jukebox-driver、jukebox-e2e、nas-jukebox-deploy
```

## 7. 改动验收清单

1. `gofmt -l` 无输出；`make lint` 通过（若只改前端可跳过 Go lint）
2. `make test PKG=./core/jukebox` 与 `PKG=./server/nativeapi` 全绿
3. `cd ui && npm run test && npm run lint && npm run check-formatting`
4. 涉及界面：`npm run build` → `go build` → 重启实例 → **真实登录**后在浏览器（含移动视口）点一遍
5. 涉及出声链路：确认日志里没有 loopback 告警，且 `/api/jukebox/status` 的
   `currentTime/duration/volume` 与设备实际一致
6. 新增输出类型：按 `.claude/skills/add-jukebox-driver/SKILL.md` 的清单补齐
   接口/工厂/配置字段/表单/i18n（**zh-Hans、zh-Hant、en 三处都要**）/文档/测试
7. 提交前：`git status --short` 只应剩预期文件；改过 `.gitignore` 时另跑
   `git diff --diff-filter=D --name-only ee6dd1bc HEAD`，输出必须为空
   （非空说明有上游源文件被忽略规则吃掉了，见第 1 节）。

## 8. 约定

- 提交信息用英文，遵循上游风格（`fix(scanner): ...`、`feat: ...`）。
- 代码注释只写"为什么"，不写"这行做什么"；文档与注释默认中文（面向本项目使用者）。
- 不新增未使用的 prop / export / 兼容垫片；删代码要连带测试断言一起删。
- 改 `.gitignore`：新增项一律写成带前导斜杠的形式（`/artwork/`）。
  不带斜杠的模式匹配任意层级，`AGENTS.md` 与 `core/artwork/` 都曾被这种规则吞掉。
  本 fork 特意把 `AGENTS.md` 从忽略列表里移除并保持跟踪。
- git 身份、`origin`、push 由仓库主人负责；AI 不改 `git config`、不改分支名、不 push。
- 不动 `db/migrations/*`、`server/subsonic/*` 的对外行为，除非任务明确要求。
