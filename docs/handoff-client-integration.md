# 多端同步与接管（Playback Handoff）— 第三方客户端接入任务书

> **发给谁**：另一个音乐 App 项目的 AI 协作者 / 开发者。本文自包含，不需要访问本仓库源码。
> **怎么用**：整份贴给对方 AI，让它按第 10 节的任务清单逐条实现，并用第 11 节的脚本自测。
> **可信度**：每一条都对照本仓库真实代码核对，并在本地实例上用 curl 实测通过
> （验证时间 2026-09-27，`navidrome2all` fork，单实例 `http://localhost:4599`）。
> 与 `docs/playback-handoff-api.md` 冲突的地方，**以本文第 9 节为准**。

---

## 0. 要实现什么（验收目标）

做一套「Spotify Connect / Apple Handoff 式」的跨设备接力：

1. **被看见**：App 播放时，服务端能记录这台设备在播什么、第几毫秒、什么状态，网页端"正在播放"里能看到我。
2. **看见别人**：App 能拉到其他设备的实时播放会话列表（歌曲、进度、状态、输出设备、音量）。
3. **接管**：点别人的会话 → 本地从 `positionMs` 毫秒处起播，原设备被服务端广播停播。
4. **被接管**：我在播时别人接管 → 我收到 SSE 事件，毫秒级暂停 + 提示"播放已被 X 接管"。
5. **全量继承**：接管时继承输出设备、音量、进度、播放/暂停意图、循环模式、双语歌词 6 个维度。

第 3、4 条**必须**同时实现，否则会"两台设备同时出声"。

---

## 1. 服务端前提（先跟用户确认）

| 项 | 说明 | 默认值 |
|---|---|---|
| 服务端版本 | `navidrome2all` fork；**原版 Navidrome 没有这些端点** | — |
| `DevActivityPanel` | SSE 端点 `/api/events` 只在它为 `true` 时才挂载 | `true` |
| `Jukebox.Enabled` | 才能列出/切换**远程输出设备**（MPD/DLNA/小米）。只用本机播放可忽略 | `false` |
| 网络 | 客户端必须能访问服务端；`BaseUrl` 决定流地址主机名 | — |

> 接管会顺带暂停正在发声的远程音箱，这一步需要 Jukebox，但**端点本身不依赖它**
（`Jukebox.Enabled=false` 时只是跳过那一步，接管照常 200）。
`Jukebox.AdminOnly=true` 且当前用户非管理员时同样跳过——暂停音箱是"别两边同时响"
的善后动作，不该因此拒绝一次合法接管。

---

## 2. 鉴权（三套并存，最容易踩坑）

### 2.1 Native API（`/api/*`）—— **只认 `X-ND-Authorization`**

```http
X-ND-Authorization: Bearer <JWT>
```

**实测**：带标准 `X-ND-Authorization: Bearer <JWT>` → **401**（服务端不读这个头）。
后门：任意 `/api/*` 都支持 query `?jwt=<JWT>`（如 `/api/playback/sessions?jwt=xxx`，实测 200），
适合不方便设 header 的调用方。

### 2.2 Subsonic API（`/rest/*`）—— 三选一

```http
# A（推荐）：登录响应直接给了 token+salt，不要自己算 md5
GET /rest/reportPlayback?u=<username>&t=<subsonicToken>&s=<subsonicSalt>&f=json&v=1.8.0&c=<AppId>&...

# B：明文或十六进制密码
...&p=<password>      或      ...&p=enc:<hex(password)>

# C：JWT（需带 subsonic audience）
...&jwt=<JWT>
```

### 2.3 SSE（`/api/events`）—— 只能用 query 传 token

`EventSource` 不能设 header，所以：

```http
GET /api/events?jwt=<JWT>
Accept: text/event-stream
```

**实测**：不带 `jwt` → 401；带 `X-ND-Authorization` header → 200（curl 可以，浏览器 EventSource 不行）。

### 2.4 拿 Token

```bash
curl -s -X POST <BASE>/auth/login -H 'Content-Type: application/json' \
  -d '{"username":"<u>","password":"<p>"}'
```

响应字段（实测）：

```json
{
  "id": "6TfliL0lp3Ae3kpniD8YMp",
  "isAdmin": true,
  "name": "Dev Admin",
  "username": "admin",
  "token": "<JWT，用于 /api/* 与 ?jwt=>",
  "subsonicSalt": "37477d",
  "subsonicToken": "8c4bd9c3093fe0d3fd9dca167b736e88"
}
```

`subsonicToken`/`subsonicSalt` 可直接用于所有 `/rest/*` 调用（实测 `ping` 通过）。

---

## 3. 端点总览（全部实测）

| 用途 | 方法与路径 | 鉴权 | 实测结果 |
|---|---|---|---|
| 会话列表 | `GET /api/playback/sessions` | `X-ND-Authorization` 或 `?jwt=` | 200；带/不带尾部 `/` 都是 200 |
| 单个会话 | `GET /api/playback/sessions/{sessionId}` | 同上 | 存在 200，不存在 **404** |
| **接管** | `POST /api/playback/sessions/{sessionId}/takeover` | 同上 + **会话归属** | 200；目标不存在 **404**；接管他人会话 **403**（管理员不受限） |
| SSE 事件 | `GET /api/events?jwt=<JWT>` | query jwt | 200，事件名 `playbackHandoff` |
| 播放上报 | `GET /rest/reportPlayback?...` | Subsonic `u/t/s` | `status:"ok"` |
| 会话列表（Subsonic 口） | `GET /rest/getNowPlaying?...` | Subsonic `u/t/s` | 字段更全，见 6.2 |
| 播流 | `GET /rest/stream?id=<songId>&...` | Subsonic | — |
| 封面 | `GET /rest/getCoverArt?id=<songId>&...` | Subsonic | — |
| 输出设备列表 | `GET /api/jukebox/devices` | Native | `Jukebox.Enabled=false` → **403** 文本 `jukebox is disabled` |
| 选中输出设备 | `POST /api/jukebox/select` body `{"device_id":"browser"}` | Native | 同上 |
| 远程播控 | `POST /api/jukebox/control` body `{"action":"pause","value":0}` | Native | 同上 |
| 取双语歌词 | `GET /api/lyrics/translate/{songId}?lang=zh-CN` | Native | 未配置引擎 404 |

`takeover` 请求体（**全部可选**，空 body 也返回 200）：

```json
{
  "action": "pause",
  "sourceSessionId": "app-mobile-1",
  "newPlayerName": "Chora (手机端)",
  "targetOutput": "browser"
}
```

- `action`：`pause`（默认）/ `stop`，其他值 → **400** `action must be 'stop' or 'pause'`
- `targetOutput`：`browser`/`local` 或 Jukebox 输出 ID；**留空 = 继承被接管方的输出**

响应：

```json
{
  "status": "ok",
  "action": "pause",
  "takenOverSessionId": "client-A",
  "session": { "…被接管会话的最新快照；目标不存在时整个字段缺失…" }
}
```

---

## 4. 会话生命周期（服务端真实行为，按这个写状态机）

会话存在服务端进程内缓存，**key = 客户端 ID**，TTL 由上报状态决定：

| 上报 `state` | 会话效果 | TTL |
|---|---|---|
| `starting` | 新建/更新；若同 ID 已在 `playing` 则忽略（防乱序） | 剩余曲长 + 5s |
| `playing` | 更新进度与状态 | **剩余曲长 + 5s**（按 `playbackRate` 折算） |
| `paused` | 更新为暂停 | **30 分钟** |
| `stopped` | **立即从列表删除** | — |

**进度插值**：列表接口只对 `state=playing` 做服务端线性外推
`positionMs += (now - lastReport) × playbackRate`，并 clamp 到曲目时长；
`paused`/`starting` 不插值，进度停在最后一次上报值——所以暂停时进度是准的。

**合法 state**：`starting` / `playing` / `paused` / `stopped`；传别的值返回
`{"code":0,"message":"Invalid state: bogus"}`。

**`volume` 的坑（实测）**：`volume=0` 会被当成"没传"——首次上报存成 **100**，
之后保持旧值，**静音状态传不上去**。想保留旧音量就**别传** `volume`
（实测：先传 30，之后不传，仍是 30）。

`outputDevice` / `playMode` 传空串同样保持旧值；`bilingual` 是 bool，
为 `false` 时 JSON 里**字段直接消失**（`omitempty`），客户端按"缺失 = false"处理。

---

## 5. 任务 T1：稳定会话身份 + 播放上报

### 5.1 身份（最关键）

服务端用 **`X-ND-Client-Unique-Id` header** 决定 `sessionId`：

- 必须是本设备/本安装的**稳定唯一 ID**（首次启动生成一次并持久化）。
- 不传会退回 Subsonic player ID（基于 cookie），移动端冷启动可能变化 → **收不到自己的 SSE 停播事件**、
  列表里堆出重复会话。
- 该 header 是**全局中间件**，`/api/*` 与 `/rest/*` 都生效。
- 同时给 Subsonic 调用带 `c=<AppId>`：`playerName` 会显示成 `<AppId> [<userAgent>]`
  （实测 `myapp [curl]`），这就是别人在列表里看到的设备名。

### 5.2 上报

```http
GET /rest/reportPlayback
  ?mediaId=<songId>&mediaType=song&positionMs=<int>          # 必须 >= 0，负数报错
  &state=starting|playing|paused|stopped
  &playbackRate=1.0
  &outputDevice=browser|xiaomi_l7a|...
  &volume=0..100                 # 可选；0 无效，见第 4 节
  &playMode=single|all|order
  &bilingualActive=true|false
  &ignoreScrobble=true           # 测试/接管时避免误计播放次数
  &u=<user>&t=<subToken>&s=<subSalt>&f=json&v=1.8.0&c=<AppId>
X-ND-Client-Unique-Id: <稳定ID>
```

**节奏（必须遵守，否则别人看不见你）**：

1. 起播：先 `starting`，紧接着 `playing`；
2. 播放中：**每 10~15 秒**上报一次 `playing` + 当前 `positionMs`
   （TTL = 剩余曲长 + 5s，间隔超过剩余时长会提前从列表消失；
   自带网页端用的是 60s，长歌才安全，**新客户端不要抄这个值**）；
3. 暂停：**立刻**上报 `paused` + `positionMs`；
4. 切歌/停止：对旧歌上报 `stopped`，否则旧会话会挂着旧歌直到 TTL 结束。

---

## 6. 任务 T2：看见别人（两种口径，建议都支持）

### 6.1 口径 A：Native `GET /api/playback/sessions`

```json
{
  "count": 2,
  "sessions": [{
    "sessionId": "client-A",
    "userId": "6TfliL0lp3Ae3kpniD8YMp",
    "username": "admin",
    "playerName": "myapp [curl]",
    "songId": "4GpHuSUqwxq4OQtfnjpG6b",
    "title": "Title", "artist": "Artist", "album": "Album",
    "artistId": "…", "albumId": "…",
    "duration": 192,
    "positionMs": 56420,
    "positionSec": 56.42,
    "state": "playing",
    "playbackRate": 1.0,
    "coverArtId": "4GpHuSUqwxq4OQtfnjpG6b",
    "lastReport": "2026-09-27T00:39:07+08:00",
    "isCurrentSession": false,
    "outputDevice": "browser",
    "volume": 80,
    "playMode": "all",
    "bilingual": true
  }]
}
```

- 任意登录用户可读，**不需要管理员**；按开播时间倒序；
- 只显示当前用户**有权限访问的曲库**里的会话；
- `isCurrentSession: true` 当且仅当请求带的 `X-ND-Client-Unique-Id` 等于该 `sessionId`（实测）；
- `bilingual:false` 时该字段消失。

### 6.2 口径 B：Subsonic `GET /rest/getNowPlaying`（标准端点，任何 Subsonic 客户端都能用）

`nowPlaying.entry[]` = 标准 `Child` **+ 本 fork 扩展字段**：

```json
{
  "id": "<songId>", "title": "…", "artist": "…", "album": "…",
  "duration": 192, "coverArt": "mf-<songId>",
  "username": "admin", "minutesAgo": 0,
  "playerId": 1,
  "playerName": "myapp [curl]",
  "state": "playing", "positionMs": 56420, "playbackRate": 1.0,
  "sessionId": "client-A",
  "outputDevice": "browser", "volume": 80, "playMode": "all", "bilingual": true
}
```

**易错点**：`playerId` 是**自增序号**（实测恒为 1、2、3…），接管必须用 `sessionId`。
自带网页端用的就是口径 B + `sessionId`。

---

## 7. 任务 T3：接管（黄金 6 步）

1. **读进度**：`target = session.positionMs`（服务端已插值，直接用，别自己算）。
2. **加载音频**：`/rest/stream?id=<songId>`，seek 到 `target` 毫秒。
3. **继承 6 维状态**：

   | 维度 | 来源 | 动作 |
   |---|---|---|
   | 进度 | `positionMs` | `seekTo(positionMs)` |
   | 播放意图 | `state` | `playing` → 直接起播；`paused` → 保持暂停，等用户点 ▶ |
   | 音量 | `volume` (0..100) | 映射到本地音量模型，**缺失或 0 视为 100** |
   | 输出设备 | `outputDevice` | 非 `browser` → 第 8.2 节；`browser` → 本机播放 |
   | 循环模式 | `playMode` | 映射到本地 repeat 模式 |
   | 双语歌词 | `bilingual` | `true` → 拉 `/api/lyrics/translate/{songId}?lang=zh-CN`（8.3 节） |

4. **本地起播之后**，异步发接管（不要串行等待）：

   ```http
   POST /api/playback/sessions/<targetSessionId>/takeover
   X-ND-Authorization: Bearer <JWT>
   Content-Type: application/json

   {"action":"pause","sourceSessionId":"<我的ID>","newPlayerName":"Chora (手机端)","targetOutput":"<继承或 browser>"}
   ```

5. **处理响应**：200 即可；`session` 缺失说明目标已过期，无妨。
6. **本地 UI**：把该会话从列表移除（乐观更新）+ toast `已从 00:56 接管播放：<歌名>`。

> **顺序铁律**：先本地起播，再 POST takeover。反过来会出现"原设备已停、我还没准备好"的空窗。

---

## 8. 任务 T4/T5：SSE 互斥 + 远程输出 + 双语继承

### 8.1 SSE 停播（必须实现）

```js
const es = new EventSource(`${BASE}/api/events?jwt=${token}`)
es.addEventListener('playbackHandoff', (e) => {
  const ev = JSON.parse(e.data)
  if (ev.targetSessionId !== myClientUniqueId) return   // 必须自己过滤
  player.pause()
  toast(`播放已被「${ev.newPlayerName || '其他设备'}」接管，本地已暂停`)
  reportPlayback(currentSongId, localPositionMs, 'paused')
})
// 断线 5s 后重连；服务端周期性发 keepAlive
```

事件体（实测原文）：

```json
{"targetSessionId":"client-A","sourceSessionId":"client-B","action":"pause",
 "songId":"4GpHuSUqwxq4OQtfnjpG6b","positionMs":1000,"newPlayerName":"Chora (手机端)",
 "outputDevice":"browser","volume":80,"playMode":"all"}
```

**实测关键**：服务端是 `broadcastToAll`，**包括发起接管的自己在内，所有在线客户端都收到**，
所以 `targetSessionId` 过滤不可省。`action` 为 `pause`（默认）或 `stop`。

### 8.2 远程输出（音箱）继承

- 接管时 `targetOutput` **留空** = 服务端自动继承被接管方的 `outputDevice`；
- 若选 `browser`/`local`：服务端发现当前选中的不是浏览器，会**自动 pause 远程音箱**，
  避免"手机和客厅一起响"（仅 `Jukebox.Enabled=true` 时生效）；
- 若继承远程音箱：服务端**不会**替你切设备，客户端自己调：

  ```http
  POST /api/jukebox/select   {"device_id":"<outputDevice>"}
  POST /api/jukebox/play     {"song_id":"<id>","stream_url":"<url>","position":<sec>}
  # 之后进度/音量/暂停 走 /api/jukebox/control 与 /api/jukebox/status
  # （详见 docs/jukebox-api.md：状态码 409 = 当前是本机输出）
  ```

- 远程输出时**本地播放器静音**（`audio.muted = true`），进度条用本地时钟渲染，
  每隔几秒用 `/api/jukebox/status` 的 `currentTime` 校准（差距 > 2s 才微调）；
- 音量铁律：本地拖动后 3 秒内不采纳 status 回来的音量，防止抢手。

### 8.3 双语歌词继承

会话只带 `bilingual: true` 标志，**不带歌词正文**，接管方自己拉：

```http
GET  /api/lyrics/translate/{songId}?lang=zh-CN    # 只读磁盘缓存，命中 0ms、不花 AI 额度
POST /api/lyrics/translate {"songId":"…","targetLang":"zh-CN"}   # 缺失时触发翻译
```

返回 `inlineLrc`（同时间戳一行原词+译文，移动端悬浮歌词用）/ `bilingualLrc` /
`combinedLrc` / 结构化 `lines`。无歌词歌曲返回 **404**，UI 要提示"该歌曲暂无可翻译歌词"，不能卡死。

---

## 9. 与 `docs/playback-handoff-api.md` 的差异（**以本节为准**）

| # | 该文档写法 | 实测/代码真相 |
|---|---|---|
| 1 | "亦兼容 `Authorization: Bearer`" | ❌ **只认 `X-ND-Authorization: Bearer`**，标准 `Authorization` 头 → 401；替代方案是 `?jwt=` |
| 2 | 建议 10~15 秒上报 | 服务端无强制，但 TTL = 剩余曲长 + 5s，**间隔必须短于剩余时长**；自带网页端实际用 60s（不可照抄） |
| 3 | 未提 `getNowPlaying` | 实测可用且字段更全；`playerId` 是序号，接管用 `sessionId` |
| 4 | 未提 volume 语义 | `volume=0` → 100/保持旧值，**静音传不上去**；不传参数 = 保持旧值 |
| 4b | 未提 `/rest/scrobble` 的陷阱 | **禁止**用 `/rest/scrobble?submission=false` 上报会话：它硬编码 `bilingual=false`、`playbackRate=1.0`、`positionMs=position*1000`（毫秒被截断到秒），会**无条件覆盖**双语标志与精度。要上报扩展维度只能用 `/rest/reportPlayback` |
| 5 | 未提接管后会话去向 | 接管成功后目标变 `paused` 并**继续留在列表 30 分钟**，UI 要决定展示还是隐藏 |
| 6 | takeover 目标不存在 | **是 404**（`session not found or already stopped`）。早先版本会"照样暂停全局 jukebox 并返回 200"，现已修复 |
| 6b | 未提接管归属 | **必须校验**：`caller.IsAdmin \|\| target.UserId == caller.ID`，否则 **403** `not allowed to take over another user's session`。注意 404 先于 403——无库权限的会话与不存在的会话都返回 404 |
| 6c | 未提 Jukebox 门禁 | takeover 端点**不**被 `Jukebox.Enabled` 挡住；它只在"暂停远程音箱"那一步检查（`Enabled=false` 或 `AdminOnly` 下非管理员则跳过该步，接管仍 200） |
| 7 | 未提 SSE 过滤 | 事件广播给**所有人**，必须自己按 `targetSessionId` 过滤 |
| 8 | 未提 `DevActivityPanel` | 为 `false` 时 `/api/events` 根本不挂载（默认 `true`） |
| 9 | 未提接管后是否切设备 | 服务端**不会**替接管方 `select` 输出设备，客户端要自己调 `/api/jukebox/select` |
| 10 | takeover body 必填项 | 全部可选，空 body 也能成功；`action` 非法才 400 |
| 11 | `playMode=single\|all\|order` | 内核枚举是 `order` / `orderLoop` / `singleLoop` / `shufflePlay`。服务端不校验，按 `single`/`all` 上报会让网页端接管后循环模式不生效 |
| 12 | 未提接管会清空本地队列 | 网页端 `reduceTakeoverTrack` 会把本地队列替换成这一首（`queue:[item], clear:true`）——接管即丢弃播放列表 |

---

## 10. 分阶段任务清单（可直接当 TODO 拆）

### 阶段 1 — 上报与身份
- [ ] 生成并持久化稳定 `clientUniqueId`，所有 `/api/*`、`/rest/*` 请求带 `X-ND-Client-Unique-Id`
- [ ] Subsonic 调用统一带 `c=<AppId>` 与 `u/t/s`（登录响应直接取）
- [ ] 封装 `reportPlayback`（含 `outputDevice/volume/playMode/bilingualActive`）
- [ ] 按 5.2 节节奏接起播/心跳/暂停/停止
- [ ] **自测**：跑完第 11 节脚本，网页端"正在播放"里能看到本 App

### 阶段 2 — 会话列表 UI
- [ ] 轮询 `GET /api/playback/sessions`（建议 5s；或收到 `nowPlayingCount` SSE 再拉）
- [ ] 渲染封面（`/rest/getCoverArt?id=`）、标题歌手、进度条（`positionMs` + 本地时钟）、状态、设备名
- [ ] `isCurrentSession == true` 的自己那条隐藏或标"本机"
- [ ] `state == "paused"` 用不同样式
- [ ] **自测**：两个客户端同时开，互见

### 阶段 3 — 接管（核心）
- [ ] 点击会话 → 按第 7 节 6 步执行（**先起播后 takeover**）
- [ ] 6 维状态继承，尊重 `paused` 意图
- [ ] 远程输出时调 `/api/jukebox/select` + `play`，本地静音
- [ ] `bilingual` 标志 → 拉 `/api/lyrics/translate/{songId}`
- [ ] **自测**：A 播到 30s，B 接管，B 从 30s 起播、A 毫秒级暂停

### 阶段 4 — SSE 互斥
- [ ] 登录后长连 `/api/events?jwt=`，监听 `playbackHandoff`
- [ ] 按 `targetSessionId` 过滤 → 暂停 + toast + 上报 `paused`
- [ ] 断线 5s 重连，忽略 `keepAlive`
- [ ] **自测**：B 接管 A，A 必须收到事件并停下（两边同时出声 = 不合格）

### 阶段 5 — 收尾
- [ ] 音量继承映射到本地音量模型，避免爆音/静音
- [ ] takeover 失败要有提示且不卡死播放
- [ ] 不缓存越权曲库的会话（服务端已过滤）
- [ ] 对照第 0 节 5 条验收

---

## 11. 端到端自测脚本（对任意实例直接跑）

```bash
BASE=http://localhost:4533
U=<用户名>; P=<密码>

# 1) 登录拿凭证
LOGIN=$(curl -s -X POST $BASE/auth/login -H 'Content-Type: application/json' \
  -d "{\"username\":\"$U\",\"password\":\"$P\"}")
JWT=$(echo "$LOGIN" | jq -r .token)
ST=$(echo "$LOGIN" | jq -r .subsonicToken); SS=$(echo "$LOGIN" | jq -r .subsonicSalt)
SUB="u=$U&t=$ST&s=$SS&f=json&v=1.8.0&c=smoketest"

# 2) 取一首歌
ID=$(curl -s "$BASE/rest/getRandomSongs?n=1&$SUB" | jq -r '.["subsonic-response"].randomSongs.song[0].id')
echo "song=$ID"

# 3) 以 client-A 身份上报
curl -s -H "X-ND-Client-Unique-Id: client-A" \
  "$BASE/rest/reportPlayback?mediaId=$ID&mediaType=song&positionMs=5000&state=playing&playbackRate=1.0&outputDevice=browser&volume=80&playMode=all&ignoreScrobble=true&$SUB" \
  | jq -c '.["subsonic-response"].status'        # 期望 "ok"

# 4) 列表里应出现 client-A
curl -s -H "X-ND-Authorization: Bearer $JWT" $BASE/api/playback/sessions | jq .

# 5) 鉴权负例：标准 Authorization 必须是 401，X-ND-Authorization 必须是 200
curl -s -o /dev/null -w "std=%{http_code}\n" -H "Authorization: Bearer $JWT" $BASE/api/playback/sessions
curl -s -o /dev/null -w "nd=%{http_code}\n"  -H "X-ND-Authorization: Bearer $JWT" $BASE/api/playback/sessions

# 6) 开 SSE（另开一个终端）
curl -sN "$BASE/api/events?jwt=$JWT" | grep --line-buffered playbackHandoff

# 7) 用 client-B 身份接管 client-A，第 6 步应立即打出 playbackHandoff
curl -s -X POST -H "X-ND-Authorization: Bearer $JWT" -H "X-ND-Client-Unique-Id: client-B" \
  -H 'Content-Type: application/json' \
  -d '{"action":"pause","sourceSessionId":"client-B","newPlayerName":"B 手机","targetOutput":"browser"}' \
  "$BASE/api/playback/sessions/client-A/takeover" | jq .

# 8) 会话应变成 paused；再报 stopped 应立即消失
curl -s -H "X-ND-Authorization: Bearer $JWT" $BASE/api/playback/sessions | jq -c '.sessions[]|{sessionId,state}'
curl -s -H "X-ND-Client-Unique-Id: client-A" \
  "$BASE/rest/reportPlayback?mediaId=$ID&mediaType=song&positionMs=6000&state=stopped&ignoreScrobble=true&$SUB" >/dev/null
curl -s -H "X-ND-Authorization: Bearer $JWT" $BASE/api/playback/sessions | jq -c '{count}'
```

**通过标准**：步骤 4 能看到会话、5 分别是 401/200、6 在 7 执行瞬间收到事件、
8 先 `paused` 后 `count:0`。

---

## 12. 常见故障速查

| 现象 | 原因 |
|---|---|
| 401 on `/api/*` | 用了标准 `Authorization`，改 `X-ND-Authorization` 或 `?jwt=` |
| 401 on `/api/events` | `?jwt=` 缺失或 JWT 过期 |
| 连上 SSE 但收不到事件 | 对方客户端没带 `X-ND-Client-Unique-Id`，`sessionId` 与你过滤的值对不上 |
| 列表里看不到自己 | 没调 `reportPlayback`，或 `state`/`mediaId` 非法，或会话已过 TTL |
| 会话进度不动 | 上报间隔太长（>剩余曲长）导致过期，或 `state` 不是 `playing`（不插值） |
| 音量显示 100 但实际不是 | `volume=0` 或没传，服务端按 100/旧值存 |
| 接管后两边一起响 | 没实现 SSE `playbackHandoff`，或 `targetSessionId` 没和自己的 ID 比对 |
| 接管后音箱没切过去 | 忘了调 `/api/jukebox/select`（服务端不会替你切） |
| `/api/jukebox/*` 全 403 | 服务端 `Jukebox.Enabled = false` |
| SSE 连接立刻断 | 服务端 `DevActivityPanel = false`，端点未挂载 |

