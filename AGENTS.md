# AGENTS.md — 接手本仓库前请先读完

本文件是给 AI 协作者（Claude Code / Qoder / Cursor / Codex 等）的入口文档。
目标：读完就能动手，且不踩已知坑。**修改任何 jukebox 相关代码前，第 3、4、5 节是必读的。**

---

## 1. 这个项目是什么

Navidrome 的私有 fork，核心二开包含两大能力：
1. **多输出端播放（Jukebox）**：网页播放器里可以把当前歌曲路由到不同出声设备——浏览器 / MPD（NAS 本机声卡）/ DLNA-UPnP 渲染器 / 小爱音箱原生协议。
2. **歌词翻译与双语对照（Lyrics Translation）**：内置 5 大翻译引擎（Gemini、智谱 GLM-4、OpenAI/DeepSeek、百度、Google），实现按需翻译、双语同步、单行悬浮歌词与抽屉全屏歌词双层即时同步，独立管理后台，磁盘永久缓存。

- 上游：`https://github.com/navidrome/navidrome`，基线 commit `ee6dd1bc`。
- **本仓库的 git 历史已重置为单条初始提交，与上游没有共同祖先**，所以只能 `diff` 不能 `merge`。
  如何比对上游见 `docs/fork-and-upstream.md`。
- remote 约定：`upstream` = 上面的只读参照（**永远不要向它 push**）；`origin` = 仓库主人的 GitHub，
  由他自己添加并 push。**不要把 `origin` 指回 navidrome。**
- 默认分支是 **`master`**，不要"顺手改成 main"（与上游一致，便于 `diff` 对照）。
  本 fork 已**删除全部 GitHub Actions workflows 与 dependabot 配置**，没有任何线上 CI，
  第 2 节的构建/测试/lint 命令必须自己在本地跑。
- `.gitignore` 里的模式**都是无前导斜杠的**，因此会匹配任意层级。历史上真实踩过两次：
  `artwork/` 把整个 `core/artwork/` Go 包吞掉了（81 个源文件差点没提交），
  `AGENTS.md` 也被上游那一条忽略掉了（本 fork 已把它改回跟踪，见第 9 节）。
  新增忽略规则一律写绝对形式（`/artwork/`），并跑
  `git diff --diff-filter=D --name-only <upstream-base> HEAD` 确认为空。
- 许可：GPLv3。`LICENSE` 与所有上游文件的版权头必须保留；对外分发二进制/镜像必须连带完整源码。
- 面向人的功能文档：`docs/jukebox.md`（多输出端配置/API/驱动行为/故障排查）、
  `docs/jukebox-api.md`（多输出端 Jukebox API 与第三方客户端集成指南）、
  `docs/jukebox-nas-deployment.md`（NAS 部署）、`docs/xiaomi-speakers.md`（小米协议调研）、
  `docs/lyrics-translation-api.md`（歌词翻译 API 与第三方客户端集成）、
  `docs/playback-handoff-api.md`（活跃会话与多端同步接管 API 集成指南）、
  `docs/handoff-client-integration.md`（播放接管客户端集成指南）、
  `docs/risk-notes-optimization.md`（最近一轮优化的风险、验证与回滚记录，改代码前先读）。

## 2. 常用命令

Go **必须带 build tags**，否则编译失败（sqlite 需要 `sqlite_fts5`）：

```bash
# 单包测试（Ginkgo v2 + Gomega）
make test PKG=./core/jukebox        # 112 specs
make test PKG=./server/nativeapi    # 191 specs
make test PKG=./core/lyrics         # 42 specs（歌词引擎与翻译缓存）
make test PKG=./core/scrobbler      # 104 specs
# 等价裸命令
go test -tags=netgo,sqlite_fts5 ./core/jukebox ./server/nativeapi ./core/lyrics

go build -tags=netgo,sqlite_fts5 -o bin/navidrome .   # main 在仓库根
make lint          # golangci-lint（配置 .golangci.yml）
gofmt -l core server conf scripts   # 必须无输出
```

前端（`ui/`，React + react-admin v3 + Vite + Vitest）：

```bash
cd ui
npm run test          # Vitest：92 文件 / 804 用例
# 单跑：npx vitest run src/audioplayer/TranslateButton.test.jsx
#       npx vitest run src/lyricsTranslation     # 歌词翻译管理页
#       npx vitest run src/jukebox               # 输出设备控制台与向导
npm run lint          # ESLint，--max-warnings 0
npm run check-formatting   # Prettier 只检查不写入；写入用 npm run prettier
npm run build         # 产物在 ui/build/（被 go:embed 打进二进制）
```

Docker 打包与推送（发布镜像：`huhan333/navidrome2all:latest`，必须为 `linux/amd64`）：

```bash
# 本地起 colima (macOS 环境)
colima start
# 打包 amd64 镜像并推送到 Docker Hub
docker buildx build --platform linux/amd64 \
  --build-arg GIT_SHA=$(git rev-parse --short HEAD) \
  --build-arg GIT_TAG=v0.55.0-fork \
  --target final \
  -t huhan333/navidrome2all:latest \
  -t huhan333/navidrome2all:$(git rev-parse --short HEAD) \
  --push .
```

**推完必须验架构**（macOS 本地验不出来，NAS 上会 `exec format error`）：

```bash
docker buildx imagetools inspect huhan333/navidrome2all:latest | grep -E "Platform|Digest"
# 期望 Platform: linux/amd64（unknown/unknown 是 buildx 的 provenance attestation，正常）
```

冒烟起一次容器确认 UI 已内嵌、版本号注入正确：

```bash
docker run -d --name nd-smoke -p 14607:4533 huhan333/navidrome2all:latest
sleep 25 && curl -s -o /dev/null -w '%{http_code}\n' http://localhost:14607/ping
curl -s http://localhost:14607/app/ | grep -c jukebox
docker logs nd-smoke 2>&1 | grep -i version   # Version: 0.55.0-fork (bfdd120f)
docker rm -f nd-smoke
```

仓库历史被重置过、没有 tag，`GIT_TAG` 只能手写。推送的是**工作区当前状态**，
不限于已提交内容——发版前先看 `git status --short`。

改了前端就要 `npm run build` + 重新 `go build`，否则二进制里还是旧界面
（i18n 文案同理，是构建期打包进 bundle 的）。

i18n 除了"三份都要写"，还要查**引用了但语言包里没有**的 key——那种 key 会静默回退到
代码里的 `_:` 默认值（非默认语言下就表现为界面中英/中英混排），而 `make test-i18n`
只校验"en 的 key 在翻译包里存在"，查不出这一类：

```bash
python3 scripts/check-i18n-keys.py     # 期望：0 个 key 缺失
```

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

## 4. 歌词翻译与双语模型（第二大核心不变量）

本 fork 的第二大核心功能是**歌词多引擎翻译与双语/单行悬浮歌词实时同步**。
内置 5 大翻译引擎（Gemini、智谱 GLM-4、OpenAI/DeepSeek、百度翻译、Google 翻译），按需触发，磁盘持久化缓存。

### 核心状态与两层同步机制（改动必须同时保证）

Web 端的歌词显示在架构上分为**两层**：
1. **抽屉/全屏歌词组件**：订阅 Redux store (`state.player`)，直接响应状态变化。
2. **底栏单行悬浮歌词（`music-player-lyric`）**：由底层播放器内核（`navidrome-music-player` / `react-jinke-music-player`）驱动。它**不直接读 Redux**，而是从内部 `playerRef.current.state.audioLists[playIndex].lyric` 取歌词，由其自带的 `LyricParser` 按时间推移计算单行文本。

**切换双语时的三重动作（必须同时触发，见 `ui/src/audioplayer/TranslateButton.jsx` 与 `Player.jsx`）**：
1. **Redux 变更**：`dispatch(setBilingualActive(bool))`，并将翻译结果存入 `bilingualLyrics[trackId]`。
2. **内核内存替换**：直接将目标歌词（原版或 `inlineLrc`）写入 `playerRef.current.state.audioLists[playIndex].lyric`。
3. **唤醒解析器**：调用 `playerRef.current.initLyricParser()` 并立即以当前时间 `playerRef.current.update(currentTimeMs)` 重新计算渲染。
   *原因*：只改 Redux 不改内核 `audioLists` 和解析器，全屏抽屉虽然变了，但底栏/桌面悬浮歌词会一直停在旧歌词。

**队列同步保护（防回滚，见 `ui/src/audioplayer/Player.jsx`）**：
播放器内核在 `reduceSyncQueue` 和 `reduceCurrent` 中，会在用户调整播放队列或切歌时用传入的 queue item 覆盖 `audioLists`。
如果当前曲目处于双语激活状态（`bilingualActive == true`），**必须保留已注入的双语 `lyric`**，严禁被队列项里的原始未翻译歌词回滚覆盖。

### 三种歌词格式与分工

后端（`core/lyrics/translation.go`）生成的翻译结果包含三种 LRC 形式：

| 字段 | 格式示例 | 适用场景 |
|---|---|---|
| `inlineLrc` | `[00:12.34]Original line / 翻译行` | **桌面/移动端底栏悬浮歌词（最推荐）**。同时间戳合成一行，彻底解决播放器内核对同时间戳多行只显示首行或快速跳闪的问题 |
| `bilingualLrc` | `[00:12.34]Original line\n[00:12.34]翻译行` | 标准双语双行 LRC，适合支持同时渲染同时间戳双行的全屏滚动歌词器 |
| `combinedLrc` | `[00:12.34]Original line\n翻译行` | 首行带时间戳，次行纯文本紧随 |
| `lines` | 结构化 JSON 数组（`start/end/original/translation`） | 结构化歌词渲染或第三方移动客户端集成（毫秒精度） |

### 后端服务与缓存机制

- **配置持久化**：存储在 SQLite 的 `property` 表，key 为 `LyricsTranslationConfig`（JSON 格式）。通过 `GET/PUT /api/lyrics/translation/config` 读取与修改（管理端专有，敏感 key 在 GET 时脱敏）。
- **磁盘永久缓存**：翻译结果按 `<DataFolder>/lyrics_translations/{songId}_{lang}.json` 落地存储，命中缓存时 0ms 响应，不消耗外部 token。只有传入 `force=true` 时才会重新请求外部引擎。
- **并发防护**：Go 端使用 `golang.org/x/sync/singleflight`，同歌曲同语言的高并发翻译请求只发起一次外部 AI 调用。
- **独立管理入口**：按产品要求，歌词翻译配置在 Web 左侧侧边栏以独立的「歌词翻译」菜单展示（路由 `/lyrics-translation`），**不要合入用户偏好设置**。

## 5. 仓库里存在**两套** jukebox，别搞混

| | 上游 `core/playback` | 本 fork `core/jukebox` |
|---|---|---|
| 后端 | 只有 mpv 子进程（放**服务器本地绝对路径**） | MPD / DLNA / 小米（放 **HTTP 流地址**或相对路径） |
| 对外协议 | Subsonic `jukeboxControl`（第三方 App 在用） | 自家 native JSON API，只有本 Web UI |
| 队列 | **服务端**持 `Queue`（set/add/remove/shuffle/skip） | 无状态，队列在浏览器 |
| 音量 | `SetGain(float32 0..1)` | `SetVolume(int 0-100)` |
| 配置 | `Jukebox.Devices` / `Jukebox.Default` | `Jukebox.Outputs` + 数据库持久化 + SSDP 发现 |

**两套各有开关**（2026-09-24 拆分，之前共用 `Jukebox.Enabled` 是设计债）：

| 开关 | 点亮什么 | 代码位置 |
|---|---|---|
| `Jukebox.Enabled` | 网页设备选择器 + `/api/jukebox/*` + `jukeboxEnabled` 注入 | `server/nativeapi/jukebox.go:39`、`server/serve_index.go:65` |
| `Jukebox.SubsonicEnabled`（默认 false） | `/rest/jukeboxControl` 路由、playback server（mpv 设备表）、Subsonic `jukeboxRole` | `cmd/root.go:364`、`server/subsonic/api.go:222`、`server/subsonic/users.go:29` |

所以"开了网页切换就顺带把服务器声卡交给第三方 App"这件事已经不存在。两个细节值得记住：
`playbackserver.Run` 只建设备表（无 `Devices` 时合成名为 `auto` 的设备并打日志
`1 audio devices found` / `Using audio device: auto`），**mpv 子进程是按曲起的**
（`core/playback/device.go:292`）；`jukeboxRole` 还要再过一层 `AdminOnly`
（`users.go:30`：`!AdminOnly || user.IsAdmin`）。

结论与纪律：
- **不要删/改 `server/subsonic/jukebox.go` 的对外语义**——那是 Subsonic 协议契约。
- 若将来要合并，正确切法是**只合并驱动层**：把 mpv 实现成一个 `PlayerDriver`，
  `core/playback` 退化为 Subsonic 协议外壳 + 队列；协议层保持两套。
- 排障时先确认命令走的是哪一套：网页 → `/api/jukebox/*` → `core/jukebox`；
  第三方 App → `/rest/jukeboxControl` → `core/playback`。

## 6. 已知坑清单（逐条踩过，别再来一遍）

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

**歌词翻译与同步**
- **底栏单行歌词必须使用 `inlineLrc`**：播放器内核解析器只支持每时间戳单行。若传入多行同时间戳（`bilingualLrc`），底栏悬浮歌词会因时间戳竞争导致跳闪或只展示第一行。
- **切换歌词必须同时刷新内核解析器**：只改 Redux 不改 `playerRef.current.state.audioLists[playIndex].lyric` 并调用 `initLyricParser()`，悬浮歌词永远不会切换。
- **独立侧边栏入口**：歌词翻译配置是全局功能，入口位于左侧抽屉（`/lyrics-translation`，管理员可见），不可合并进用户设置以免配置项再次迷失。
- **外部 API 网络环境**：国内大模型（智谱 GLM-4 等）国内直连；海外 API（Gemini/OpenAI）如遇网络受限必须在后台配置 `ProxyURL`；百度翻译必须配置 `AppID` 和 `SecretKey`。
- **无歌词歌曲**：纯音乐或元数据缺失的歌曲返回 404，前端按钮提示"该歌曲暂无可翻译歌词"，不可卡死或阻塞正常播放。

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
- **输出凭据是脱敏往返**：`GET /api/jukebox/outputs` 把 password/token/账号口令换成
  `SECRET_MASK` 返回；编辑时提交的值若等于掩码，后端理解为"沿用库里原值"而不是
  写入字面量掩码。别把掩码当真值存回去，也别在前端把它显示给用户。
- **MPD 扫描有硬边界**：只扫私网 `/24`、最多 32 个并发、每地址 400ms，整体最长约 4s
  （`discoverMPD({timeout=4})`）。这是为了不让 `/discover/mpd` 变成一个能拖住请求的
  东西，UI 必须显示 loading + 超时提示，别让用户以为卡死。
- 输出的 `ID` 留空时按名称生成：ASCII 走 `slugify`（`Living Room MPD` → `living-room-mpd`），
  纯中文名 `slugify` 拿不到 ASCII，会退化成 `<type>-<短哈希>`（`xiaomi-54hp`）——这是预期
  行为，不是 bug。生成是**保存时**才发生的，输入框只做预览，见 `docs/risk-notes-optimization.md` §10.3。

**前端与验证**
- `DeviceSelector` 渲染时即使没打开菜单也会挂载多个隐藏的 `[role=menu]` popover，
  DOM 里"存在"不等于"可见"——脚本断言只看有尺寸的那个。
- `page.evaluate()` 里的 `element.click()` **不是用户手势**，移动端媒体元素拿不到播放权限；
  验证播放要用真实点击。
- UI 验收需要**登录态**：不要自己注册账号，也不要从数据库/日志/历史里翻密码。
  把实例起好、URL 交给用户，由用户完成登录。
- 只保留一台测试实例、一个 URL。多开实例会让"哪个在响"变成玄学。

## 7. 目录地图

```
core/jukebox/            本 fork 的播放后端（唯一真源）
  driver.go              PlayerDriver 接口 + PlaybackState
  manager.go             DeviceManager 单例：设备表 / 选中项 / 命令分发 / 串行化
  driver_mpd.go          MPD：TCP，播相对路径
  driver_dlna.go         DLNA：SOAP 控制 AVTransport，播绝对流 URL
  driver_xiaomi.go       小米：miIO UDP+AES 本地 / MIoT 云端，文本指令播放
  xiaomi_miio.go         miIO 握手与加密
  xiaomi_cloud.go        小米云登录与 MIoT 调用
  discover_dlna.go       SSDP M-SEARCH 扫描 + 描述文档解析
  discover_mpd.go        MPD 私网 /24 扫描 + 密码探测 + 连通性验证
  outputs_store.go       输出配置的 DB 读写
core/lyrics/             本 fork 的歌词翻译中枢
  translation.go         TranslationService 单例 / 磁盘缓存 / singleflight / inlineLrc 合成
  provider_gemini.go     Gemini 引擎 (REST API)
  provider_zhipu.go      智谱 GLM-4-Flash 引擎
  provider_openai.go     OpenAI / DeepSeek 兼容接口
  provider_baidu.go      百度翻译 API (MD5 签名)
  provider_google.go     Google 免费网页翻译接口
conf/configuration.go    [Jukebox] 段：Enabled（网页多输出端）/ SubsonicEnabled（上游 mpv）/ AdminOnly / Devices / Default / Outputs
server/nativeapi/
  jukebox.go             /api/jukebox/{devices,status,select,play,control} + 流 URL 生成 + 错误映射
  jukebox_outputs.go     /api/jukebox/outputs CRUD、/discover(DLNA+MPD)、/verify/mpd（admin-only）
  lyrics_translation.go  /api/lyrics/translate 与 /config /test 管理接口
server/subsonic/
  jukebox.go             上游 Subsonic jukeboxControl（另一套，勿动语义）
  stream.go + StreamAlias /rest/stream/{id}.mp3 别名端点（小爱要求带扩展名）
ui/src/audioplayer/
  Player.jsx             音量三规则、歌词内核同步、静音时钟劫持、进度校准、设备切换
  VolumeControl.jsx      唯一音量 UI（只读写 store）
  DeviceSelector.jsx     输出设备下拉
  TranslateButton.jsx    播放器工具栏翻译/双语对照按钮（带缓存快速切换与加载动画）
  jukebox.js             /api/jukebox 封装 + ensureSelected
  keyHandlers.jsx        键盘快捷键（音量必须走 store）
  PlayerToolbar.jsx      桌面/移动端两处装配
ui/src/lyricsTranslation/
  LyricsTranslation.jsx  独立歌词翻译配置管理页（路由 /lyrics-translation）
ui/src/layout/
  LyricsTranslationMenu.jsx 侧边栏菜单项
ui/src/jukebox/          管理 → 输出设备（自定义卡片页 + 三步向导弹窗，路由 /jukebox-outputs）
  JukeboxOutputs.jsx     卡片列表、header 新增按钮、移动端 FAB、选择后 dispatch(setOutputDevice)
  OutputEditorDialog.jsx 三步向导：1.类型卡片 → 2.连接(按类型裁剪) → 3.高级(可选)
  outputConstants.js     SECRET_MASK / EMPTY_OUTPUT
ui/src/audioplayer/volume.js  音量换算与限幅纯函数（测试覆盖，禁止再散落 clamp/²）
ui/src/themes/useCurrentTheme.js  主题应用：注入 player.stylesheet + 写 --nd-dock-* 变量
ui/src/themes/*.js               28 个主题；**每个都必须声明 palette.type**（缺了播放 dock 会误判深浅）
ui/src/index.css                  .responsive-fields：手机端窄屏防溢出（见第 9 节）
ui/src/reducers/playerReducer.js   outputDevice 与 bilingualActive/Lyrics 状态与迁移
ui/src/store/createAdminStore.js   持久化白名单 + 音量 0 兜底
resources/i18n/*.json    后端 i18n（zh-Hans/zh-Hant 含 jukebox 与翻译文案）
ui/src/i18n/*.json       前端 i18n
docs/                    面向人的文档（见第 1 节）
  risk-notes-optimization.md 本轮优化的风险/验证/回滚/事故记录（改代码先读）
  handoff-client-integration.md 播放接管集成指南
scripts/xiaomi_test_server.go 小米假设备（凭据走环境变量，见 scripts/.env.example）
scripts/validate-translations.sh `make test-i18n` 的实现（校验 36 份语言包与 en.json 的 key 对齐）
scripts/check-i18n-keys.py     校验"源码引用了但语言包里没有"的 key（单向校验查不出的一类）
contrib/jukebox-testing/ 假 MPD / 假 DLNA 服务器（不接真设备复现链路）
.claude/skills/ .qoder/skills/  AI 技能：build-and-test、add-jukebox-driver、jukebox-e2e、nas-jukebox-deploy、lyrics-translation、playback-handoff
```

## 8. 改动验收清单

1. `gofmt -l` 无输出；`make lint` 通过（若只改前端可跳过 Go lint）
2. `make test PKG=./core/jukebox`、`PKG=./server/nativeapi` 与 `PKG=./core/lyrics` 全绿
3. `cd ui && npm run test && npm run lint && npm run check-formatting` 全绿
4. 涉及界面：`npm run build` → **`go build`** → 重启实例 → **真实登录**后在浏览器点一遍。
   只跑 `npm run build` 不重编二进制，界面还是旧的（UI 由 `go:embed` 打进二进制）
5. 涉及界面：**必须用 390px 视口再验一次**，
   `documentElement.scrollWidth === clientWidth` 且无元素 `right > 视口宽度`；
   浏览器验证前先 `Network.setBypassServiceWorker` + `Network.setCacheDisabled`，
   否则测的是缓存里的旧 bundle
6. 涉及出声链路：确认日志里没有 loopback 告警，且 `/api/jukebox/status` 的
   `currentTime/duration/volume` 与设备实际一致
7. 涉及歌词翻译：验证底栏悬浮歌词与抽屉歌词能同步切换为双语；验证多次切换时使用缓存无多余网络请求；验证空歌词歌曲优雅提示
8. 新增输出类型：按 `.claude/skills/add-jukebox-driver/SKILL.md` 的清单补齐
   接口/工厂/配置字段/向导表单（步骤 2 连接 + 步骤 3 高级）/
   i18n（**zh-Hans、zh-Hant、en 三处都要**，并跑覆盖校验）/
   自动发现（可选，但强烈建议）/文档/测试
9. 新增翻译引擎：按 `.claude/skills/lyrics-translation/SKILL.md` 的清单补齐
   Provider/配置字段/表单/i18n/单测
10. 提交前：`git status --short` 只应剩预期文件；改过 `.gitignore` 时另跑
   `git diff --diff-filter=D --name-only ee6dd1bc HEAD`，输出必须为空
   （非空说明有上游源文件被忽略规则吃掉了，见第 1 节）。

## 9. 手机端适配（自研页面必读）

上游 react-admin 页面在 390px 下是正常的，**溢出几乎都出自本 fork 的自研页面**。
根因不是缺媒体查询，而是 `Layout` 是 `min-width: fit-content`：任何后代的
min-content 超过视口，整页就被撑宽，右侧按钮/文字落到屏幕外（看起来像"被隐藏"）。

九条硬规则：

1. **自研表单页的容器加 `className="responsive-fields"`**。`ui/src/index.css` 里已备好
   `width:0 + min-width:100%` 的输入控件规则——给控件加 `min-width: 0` **没用**，
   它不影响 min-content 贡献值。
2. **穿透 MUI 内部类名的规则只能写进 `ui/src/index.css`**，不能写进 `makeStyles` 的
   `'& .MuiSelect-select'`——后者在本项目构建下不生成规则（只有伪类生效）。
3. **不要按固定高度算列表高度**（条目会因标题换行变高），用 `min(按条数算的值, 60vh)`
   并保留滚动；折叠区加 `unmountOnExit`，否则隐藏内容仍参与宽度计算。
4. **表格**：`overflow-x: auto` **单独不够**——容器是普通块时 min-content 仍等于表格
   宽度，必须配 `contain: inline-size`（`.responsive-fields` 里已配好）。
5. **MUI Popover / Menu 类弹层**：窄屏宽度上限用 `calc(100vw - 32px)`（2 × Popover 的
   `marginThreshold=16`），**不要用 `- 16px`**。Popover 超宽时不会缩窄内容，只会把面板
   左推到 `left=16`，右缘 = 16 + 面板宽，容易出屏（`NowPlayingPanel` 就踩过）。
6. **验收必须复现用户的真实数据状态**：空表格/空列表/未登录态会隐藏一整条代码路径。
   用户说"还有问题"时，直接去他的实例上量，别在自建等价环境里反复验证。

7. **下拉/选项的文案要短**。Select 的选项宽度会变成控件的 min-content，
   `Google Gemini (支持 gemini-flash-latest / gemini-flash-lite-latest)` 这种文案
   会把整页撑到 500+px。详细信息放说明卡片或 helper 文本里。
8. **验收不能只看"没有溢出元素"**——那可能意味着控件被压到只剩几个字符。
   还要断言关键控件宽度足够（如 Select > 200px）且文案未被截断
   （`el.scrollWidth <= el.clientWidth`）。

验收：`document.documentElement.scrollWidth` 必须等于 `clientWidth`，
且弹窗内没有 `right > 视口宽度` 的元素。完整排查过程见
`docs/risk-notes-optimization.md` §11。

## 10. 约定

- 提交信息用英文，遵循上游风格（`fix(scanner): ...`、`feat: ...`）。
- 代码注释只写"为什么"，不写"这行做什么"；文档与注释默认中文（面向本项目使用者）。
- 不新增未使用的 prop / export / 兼容垫片；删代码要连带测试断言一起删。
- 改 `.gitignore`：新增项一律写成带前导斜杠的形式（`/artwork/`）。
  不带斜杠的模式匹配任意层级，`AGENTS.md` 与 `core/artwork/` 都曾被这种规则吞掉。
  本 fork 特意把 `AGENTS.md` 从忽略列表里移除并保持跟踪。
- git 身份、`origin`、push 由仓库主人负责；AI 不改 `git config`、不改分支名、不 push。
- 不动 `db/migrations/*`、`server/subsonic/*` 的对外行为，除非任务明确要求。
