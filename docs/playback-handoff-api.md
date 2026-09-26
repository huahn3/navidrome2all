# Navidrome 活跃会话与多端同步接管（Playback Handoff & Takeover）API — 第三方客户端集成指南

> **版本**：Navidrome fork（本仓库）原生 REST API。  
> **适用对象**：Chora、Symfonium、Finamp、Tempo 等希望实现类似 **Spotify Connect / Apple Handoff** 跨设备无缝接力与协同播控的第三方客户端。

---

## 1. 功能概览与业务场景

在传统 Subsonic 体系下，每个客户端（网页端、车机、手机 App）是孤立的“信息孤岛”，无法感知局域网内其他设备正在播放什么。

本 Navidrome fork 服务端引入了 **实时播放会话追踪系统（PlayTracker）** 与 **多端同步接管机制（Playback Handoff）**：
- **场景 A（跨设备无缝接力）**：电脑网页端正在播放歌曲《Friendship》到 `00:56`，用户拿起手机打开 **Chora**，在界面上看到电脑正在播放的卡片，点击后手机立即从 `00:56` 开始无缝接管播放，同时电脑端自动暂停，体验极致丝滑。
- **场景 B（远程状态监控与遥控）**：客厅的 NAS 或智能音箱（通过 Jukebox DLNA/MPD）正在放歌，手机端可以实时显示音箱的播放进度条、当前曲目、音量，并能随时接管或控制播放。

---

## 2. 鉴权机制

所有 `/api/playback/` 接口均走 Native API 鉴权，必须在 HTTP Header 中携带有效 Token：

```http
Authorization: Bearer <token>
```
*(亦兼容 `X-ND-Authorization: Bearer <token>`)*

> Token 可通过 `POST /auth/login`（用户名密码）获得。

---

## 3. API 端点规范

### 3.1 获取所有活跃播放会话：`GET /api/playback/sessions`

查询当前局域网/服务器上所有登录客户端正在播放的曲目及其实时推算进度。

```http
GET /api/playback/sessions
Authorization: Bearer <token>
```

**响应示例 (200 OK)**：
```json
{
  "count": 2,
  "sessions": [
    {
      "sessionId": "huhan3-Chora-987abc",
      "userId": "u-001",
      "username": "huhan3",
      "playerName": "Chora (手机端)",
      "songId": "song_12345",
      "title": "Friendship",
      "artist": "Futuristic Swaver",
      "artistId": "ar-678",
      "album": "BFOTY",
      "albumId": "al-999",
      "duration": 192,
      "positionMs": 56420,
      "positionSec": 56.42,
      "state": "playing",
      "playbackRate": 1.0,
      "coverArtId": "song_12345",
      "lastReport": "2026-09-26T16:05:30Z",
      "isCurrentSession": false,
      "outputDevice": "xiaomi_l7a",
      "volume": 65,
      "playMode": "single",
      "bilingual": true
    },
    {
      "sessionId": "huhan3-NavidromeUI-112233",
      "userId": "u-001",
      "username": "huhan3",
      "playerName": "NavidromeUI (电脑端)",
      "songId": "song_88888",
      "title": "夢幻上昇:Mew Rage Up!",
      "artist": "Capper",
      "album": "槍火天使 Gun-fire Angel",
      "duration": 149,
      "positionMs": 56000,
      "positionSec": 56.0,
      "state": "playing",
      "playbackRate": 1.0,
      "coverArtId": "song_88888",
      "lastReport": "2026-09-26T16:05:32Z",
      "isCurrentSession": true,
      "outputDevice": "browser",
      "volume": 80,
      "playMode": "all",
      "bilingual": false
    }
  ]
}
```

#### 核心字段解释（含全量流转接管继承状态）：
| 字段 | 类型 | 说明 |
| :--- | :--- | :--- |
| `sessionId` | string | 客户端会话唯一 ID（通常对应客户端 clientId） |
| `playerName` | string | 客户端设备/应用名（如 `Chora`, `NavidromeUI`, `Symfonium`） |
| `songId` | string | 媒体文件 ID，直接用于调用流媒体接口播放 |
| `positionMs` | integer | **最新高精度实时毫秒进度**（由服务端根据上次上报时间与播放速率自动线性插值推算，非常精准） |
| `positionSec` | float | 秒级进度，方便客户端直接用于播放器跳转 |
| `state` | string | 当前状态：`"playing"`（正在播放）、`"paused"`（暂停中）、`"starting"` |
| `outputDevice` | string | 当前发声输出端：`"browser"`（本机网页/应用），或 Jukebox 输出设备 ID（如 `"xiaomi_l7a"`, `"mpd"` 等） |
| `volume` | integer | 当前播放音量（`0 ~ 100` 整数百分比），接管端需按音量模型等比采纳 |
| `playMode` | string | 当前循环模式：`"single"`（单曲循环）、`"all"`（列表循环）、`"order"`（顺序播放）等 |
| `bilingual` | boolean | 是否开启了歌词双语对照翻译，接管端可自动无缝拉取并展示双语歌词 |
| `isCurrentSession`| boolean | 是否是当前发起请求的客户端自身 |

---

### 3.2 获取指定会话状态：`GET /api/playback/sessions/{sessionId}`

获取某一台特定设备的实时会话详情。

```http
GET /api/playback/sessions/{sessionId}
Authorization: Bearer <token>
```
**响应**：单个会话 JSON 对象（格式同上）。若会话已结束或不存在则返回 `404 Not Found`。

---

### 3.3 接管会话 / 协同停播：`POST /api/playback/sessions/{sessionId}/takeover`
### 3.3 接管会话 / 互斥停播交接：`POST /api/playback/sessions/{sessionId}/takeover`

当设备 B 点击接管设备 A 的播放后，设备 B 应调用此接口告知服务端**将原设备 A 停播或置为暂停**，并触发全局 SSE 广播，实现“A 播 B 停，B 播 A 停”的单发声源互斥。

```http
POST /api/playback/sessions/{sessionId}/takeover
Content-Type: application/json
Authorization: Bearer <token>

{
  "action": "pause",
  "sourceSessionId": "huhan3-Chora-987abc",
  "newPlayerName": "Chora (手机端)",
  "targetOutput": "browser"
}
```

| 字段 | 类型 | 必填 | 取值范围与说明 |
| :--- | :--- | :--- | :--- |
| `action` | string | ❌ | `"pause"`（推荐默认，将原设备置为暂停）或 `"stop"`（停止原设备） |
| `sourceSessionId` | string | ❌ | 当前发起接管的客户端会话唯一 ID，服务端在 SSE 广播中回传 |
| `newPlayerName` | string | ❌ | 当前接管设备的人性化名称（如 `Chora (手机端)`, `Web (Chrome)`），便于原设备弹出提示 |
| `targetOutput` | string | ❌ | 当前接管端选择的声音出口：`"browser"` / `"local"`（本机播放），或 Jukebox 输出设备 ID（缺省时自动继承被接管会话的 `outputDevice`） |

#### 发声出口（Jukebox vs 本机）冲突处理逻辑：
- **若接管端选择“本机播放” (`targetOutput = "browser" / "local"`)**：若此时远端 Jukebox 音箱（DLNA/MPD/小米）正在放歌，服务端会自动暂停远端 Jukebox，确保音乐平滑切换到当前这台设备的耳机或扬声器，**杜绝两端同时发声打架**。
- **若接管端依然选择 Jukebox 外部音箱（或继承被接管端的智能音箱）**：服务端保持客厅音箱持续播放，仅向原控制设备下发交接信号，接管端接管进度条与播控权，**音乐在音箱端平滑不中断**。

**响应示例 (200 OK)**：
```json
{
  "status": "ok",
  "action": "pause",
  "takenOverSessionId": "huhan3-NavidromeUI-112233",
  "session": {
    "sessionId": "huhan3-NavidromeUI-112233",
    "songId": "song_12345",
    "title": "Friendship",
    "state": "paused",
    "positionMs": 56000,
    "positionSec": 56.0,
    "outputDevice": "xiaomi_l7a",
    "volume": 65,
    "playMode": "single",
    "bilingual": true
  }
}
```

---

### 3.4 SSE 实时互斥停播事件：`event: playbackHandoff`

服务端在收到接管请求后，会通过 `/api/events`（Server-Sent Events）向所有在线客户端广播 `playbackHandoff` 事件：

```http
GET /api/events?jwt=<token>
Accept: text/event-stream
```

**SSE 事件推送示例**：
```
event: playbackHandoff
data: {"targetSessionId":"huhan3-NavidromeUI-112233","sourceSessionId":"huhan3-Chora-987abc","action":"pause","songId":"song_12345","positionMs":56000,"newPlayerName":"Chora (手机端)","outputDevice":"xiaomi_l7a","volume":65,"playMode":"single"}
```

#### 客户端响应契约（极其重要）：
客户端收到 `playbackHandoff` 后：
1. 校验 `targetSessionId` 是否与**当前客户端自身的会话 ID**（如 `clientId`）一致；
2. 若一致，说明**本地播放已被其他设备接管**：
   - 立即调用底层播放器暂停音频（如 `exoPlayer.pause()` 或 `audio.pause()`）；
   - 在界面上弹出温和的浮窗提示：`"播放已被「${newPlayerName}」接管，本地已暂停"`；
   - 更新 UI 播放/暂停状态为暂停，停止上报 `playing` 心跳。
3. 若不一致，说明受影响的并非本机，忽略即可。

---

### 3.5 客户端自身的播放状态定时上报（让别人也能看见并全量继承你）

为了让你的客户端（如 Chora）也能在电脑网页端或其他设备上被实时看见并实现**输出设备、音量、进度、状态、循环模式、双语歌词等全状态继承接管**，客户端在播放音乐时必须遵守增强的 `reportPlayback` 规范：

```http
POST /rest/reportPlayback?mediaId={songId}&mediaType=song&positionMs={ms}&state={state}&playbackRate=1.0&outputDevice={dev}&volume={vol}&playMode={mode}&bilingualActive={bool}
```
*(使用常规 Subsonic 鉴权头或 query 参数 `u/t/s`，或在 Header 中携带 `X-ND-Client-Unique-Id: <clientId>`)*

- **上报字段说明**：
  - `outputDevice`: 当前发声输出（`"browser"` 或 Jukebox 设备 ID 如 `"xiaomi_l7a"`）；
  - `volume`: 当前播放音量（整数 `0 ~ 100`）；
  - `playMode`: 播放循环模式（如 `"single"`, `"all"`, `"order"`）；
  - `bilingualActive`: 是否处于双语翻译歌词模式（`true` / `false`）。
- **上报频率建议**：
  - 开始起播时上报一次：`state=starting` 接着 `state=playing`；
  - 正常播放中：每隔 `10 ~ 15 秒` 周期性上报一次 `state=playing` 和当前进度与音量状态；
  - 用户暂停时：立即上报一次 `state=paused` 和当前 `positionMs`；
  - 用户停止或切换歌曲时：对旧歌曲上报 `state=stopped`。

---

## 4. 客户端集成核心铁律

### 铁律 1: 毫秒精度即时载入
当用户点击某条会话卡片进行接管时，直接读取该会话对象的 `positionMs`（或 `positionSec`）。在加载音频后，立即调用底层播放器（ExoPlayer）：
```kotlin
player.seekTo(targetIndex, session.positionMs)
player.play()
```
利用服务端自动插值的 `positionMs`，实现极致平滑的秒级衔接。

### 铁律 2: 及时下发 takeover 协同停播
接管开始后，客户端必须异步发送 `POST /api/playback/sessions/{sessionId}/takeover`（带上 `sourceSessionId` 与 `newPlayerName`）。服务端通过 SSE 广播 `playbackHandoff` 触发原设备毫秒级静音停播，杜绝“两边设备都在唱”的串音尴尬。

### 铁律 3: 监听 SSE 实现单发声源互斥（A 播 B 停，B 播 A 停）
客户端长连 `/api/events`，监听 `playbackHandoff` 事件。一旦被远端设备接管，立即执行 `player.pause()`。无论是在多台浏览器网页之间、还是在浏览器与 Chora App 之间，任何一端起播接管，前一端均毫秒级自动暂停！

### 铁律 4: 与 Jukebox（多输出端）联动
结合 `docs/jukebox-api.md`：
- 若接管时选择“本机播放” (`targetOutput = "browser"`），服务端自动暂停外部 Jukebox 音箱；
- 若用户希望手机接管但依然让客厅音箱出声：下发 Jukebox 目标设备 ID，音箱继续播放，手机仅作为遥控面板。

### 铁律 5: 全量继承 6 大维度状态（无缝接力黄金体验）
优秀的流转接管必须全量继承原设备的状态，避免跳变：
1. **输出设备 (`outputDevice`)**：若对方正在推送到小米/DLNA/MPD 音箱，接管端自动绑定该输出设备并保留发声通道；
2. **音量 (`volume`)**：按客户端音量模型换算（如 `0 ~ 100` 映射到 `0.0 ~ 1.0`），避免突然爆音或静音；
3. **播放进度 (`positionMs`)**：毫秒精度插值精准跳转；
4. **播放状态 (`state`)**：尊重原会话的暂停/播放意图（原设备暂停时卡片接管保持暂停，点 `▶` 按钮才强制起播）；
5. **循环模式 (`playMode`)**：保留单曲循环、列表循环或顺序播放配置；
6. **双语对照 (`bilingual`)**：若对方已开启歌词翻译，接管端直接拉取并展示双语歌词（0ms 命中服务端磁盘缓存）。

---

## 5. Android (Kotlin + Retrofit + SSE) 完整接入代码范例

### 5.1 数据模型 (DTO)

```kotlin
data class PlaybackSessionsResponse(
    val count: Int,
    val sessions: List<PlaybackSessionDto>
)

data class PlaybackSessionDto(
    val sessionId: String,
    val userId: String,
    val username: String,
    val playerName: String?,
    val songId: String,
    val title: String,
    val artist: String,
    val album: String,
    val albumId: String?,
    val duration: Int,
    val positionMs: Long,
    val positionSec: Double,
    val state: String,
    val coverArtId: String?,
    val isCurrentSession: Boolean,
    val outputDevice: String = "browser",
    val volume: Int = 100,
    val playMode: String = "all",
    val bilingual: Boolean = false
)

data class TakeoverRequest(
    val action: String = "pause", // "pause" or "stop"
    val sourceSessionId: String? = null,
    val newPlayerName: String? = null,
    val targetOutput: String = "browser" // "browser" or jukebox output id
)

data class TakeoverResponse(
    val status: String,
    val action: String,
    val takenOverSessionId: String,
    val session: PlaybackSessionDto?
)

data class PlaybackHandoffEvent(
    val targetSessionId: String,
    val sourceSessionId: String?,
    val action: String,
    val songId: String?,
    val positionMs: Long,
    val newPlayerName: String?,
    val outputDevice: String? = null,
    val volume: Int? = null,
    val playMode: String? = null
)
```

### 5.2 Retrofit API 接口

```kotlin
interface PlaybackHandoffApi {
    @GET("api/playback/sessions")
    suspend fun getActiveSessions(
        @Header("Authorization") token: String
    ): Response<PlaybackSessionsResponse>

    @POST("api/playback/sessions/{sessionId}/takeover")
    suspend fun takeoverSession(
        @Path("sessionId") sessionId: String,
        @Header("Authorization") token: String,
        @Body request: TakeoverRequest
    ): Response<TakeoverResponse>
}
```

### 5.3 业务层接管与停播监听逻辑 (ViewModel / Manager)

```kotlin
class HandoffManager(
    private val api: PlaybackHandoffApi,
    private val player: Player, // Media3 / ExoPlayer
    private val authManager: AuthManager,
    private val myClientId: String, // 本机唯一客户端 ID
    private val context: Context
) {
    /**
     * 主动全状态继承接管其他设备
     */
    suspend fun takeover(session: PlaybackSessionDto, forcePlay: Boolean = false) {
        val token = "Bearer ${authManager.token}"

        // 1. 获取目标歌曲流并设置到本地播放器
        val mediaItem = MediaItem.Builder()
            .setMediaId(session.songId)
            .setUri(getStreamUrl(session.songId))
            .build()

        player.setMediaItem(mediaItem)
        // 2. 毫秒级跳转到原设备当前的实时进度
        player.seekTo(session.positionMs)
        
        // 3. 继承音量与循环模式
        val targetVolume = (session.volume.toFloat() / 100f).coerceIn(0f, 1f)
        player.volume = targetVolume
        when (session.playMode) {
            "single" -> player.repeatMode = Player.REPEAT_MODE_ONE
            "all" -> player.repeatMode = Player.REPEAT_MODE_ALL
            else -> player.repeatMode = Player.REPEAT_MODE_OFF
        }

        player.prepare()
        // 4. 继承播放/暂停意图
        if (forcePlay || session.state == "playing") {
            player.play()
        }

        // 5. 异步通知服务端互斥停播原设备并广播交接信号（继承对方的输出端）
        try {
            api.takeoverSession(
                sessionId = session.sessionId,
                token = token,
                request = TakeoverRequest(
                    action = "pause",
                    sourceSessionId = myClientId,
                    newPlayerName = "Chora (手机端)",
                    targetOutput = session.outputDevice
                )
            )
        } catch (e: Exception) {
            Log.w("Handoff", "Failed to send takeover request: ${e.message}")
        }
    }

    /**
     * 处理收到的 SSE playbackHandoff 事件（被其他设备接管停播）
     */
    fun onPlaybackHandoffReceived(event: PlaybackHandoffEvent) {
        if (event.targetSessionId == myClientId) {
            // 本机正是被接管的目标设备 -> 立即暂停本地发声
            Handler(Looper.getMainLooper()).post {
                if (player.isPlaying) {
                    player.pause()
                }
                val taker = event.newPlayerName ?: "其他设备"
                Toast.makeText(
                    context,
                    "播放已被「$taker」接管，本地已暂停",
                    Toast.LENGTH_SHORT
                ).show()
            }
        }
    }
}
```
