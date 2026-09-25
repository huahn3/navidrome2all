# Navidrome 多输出设备（Jukebox）API — 第三方客户端集成指南

> **版本**：Navidrome fork（本仓库）原生 REST API，不属于 Subsonic 遗留的本机 mpv 协议范围。  
> **适用对象**：Chora、Symfonium、Finamp、Tempo 等任何希望实现局域网多设备投播与播放控制的第三方客户端。

---

## 1. 概览

标准 Subsonic 协议中的 `jukeboxControl` 仅能控制服务器宿主机本地的声卡（通过 mpv 子进程），无法满足现代家庭多音箱、多房间串流播放的需求。

本 Navidrome fork 在服务端构建了原生的 **多输出设备管理器（DeviceManager）**，支持通过标准 HTTP JSON API 将音乐路由到多种局域网设备：
- **MPD (Music Player Daemon)**：驱动 NAS 或 Linux 本机声卡/DAC 无损解码输出
- **DLNA / UPnP 渲染器**：电视、功放、智能音箱（如小爱音箱 DLNA 模式、Sonos 等）
- **小米小爱音箱原生协议**：通过本地 miIO (UDP+AES) / 云端 MIoT，直接向不支持 DLNA 的小爱音箱下发流媒体播放指令

### 核心设计原则
1. **轻量服务端，富客户端**：播放队列（Queue）和控制逻辑依然在客户端本地，服务端 Jukebox 是无状态的单曲代理控制器。
2. **统一音量模型**：音量取值范围为 `0 - 100` 的整数，双向同步（支持音箱物理旋钮/遥控器反向调节与客户端界面同步）。
3. **平滑切歌与无缝流转**：支持播放中随时切换输出设备，并携带当前进度 `position` 实现无缝续播。

---

## 2. 鉴权机制

所有 Native API 端点均位于 `/api/` 路径下，必须携带 Bearer Token：

```http
Authorization: Bearer <token>
```
*(同时兼容 `X-ND-Authorization: Bearer <token>`)*

> `<token>` 通过标准登录接口获取：
> ```http
> POST /auth/login
> Content-Type: application/json
> 
> {"username": "admin", "password": "your-password"}
> ```
> 响应体 JSON 中的 `token` 字段即为有效凭证。

---

## 3. 核心 API 端点

### 3.1 获取所有输出设备与当前选中状态

用于在客户端的「播放输出端」弹窗或设置中展示设备列表。

```http
GET /api/jukebox/devices
Authorization: Bearer <token>
```

**响应示例 (200 OK)**：
```json
{
  "devices": [
    {
      "id": "browser",
      "name": "本机播放",
      "type": "browser"
    },
    {
      "id": "mpd-living",
      "name": "客厅声卡 (MPD)",
      "type": "mpd"
    },
    {
      "id": "dlna-bedroom",
      "name": "卧室小爱音箱",
      "type": "dlna"
    },
    {
      "id": "xiaoai-play",
      "name": "书房小爱音箱 Play",
      "type": "xiaomi"
    }
  ],
  "selected": "dlna-bedroom"
}
```

- `browser`：系统保留的内置设备 ID，表示在客户端本机出声。
- `type` 枚举：`browser`、`mpd`、`dlna`、`xiaomi`。
- `selected`：当前服务端激活的设备 ID。

---

### 3.2 切换激活的输出设备

用户在列表中点击某台设备时调用。

```http
POST /api/jukebox/select
Content-Type: application/json
Authorization: Bearer <token>

{
  "deviceId": "dlna-bedroom"
}
```

**响应**：`200 OK` (切换成功) 或 `404 Not Found` (设备不存在)。  
> **说明**：当切换为 `browser` 时，服务端会自动对先前激活的远程设备发送 `stop` 指令。

---

### 3.3 投播曲目 (Play)

向当前激活的远程设备下发播放指令。

```http
POST /api/jukebox/play
Content-Type: application/json
Authorization: Bearer <token>

{
  "songId": "7a3f8c2b",
  "position": 45
}
```

| 字段 | 类型 | 必填 | 说明 |
| :--- | :--- | :--- | :--- |
| `songId` | string | ✅ | Navidrome 曲目 ID |
| `position` | integer | ❌ | 开始播放的秒数，默认 `0`。切换设备无缝续播时传入客户端当前进度 |
| `streamUrl` | string | ❌ | 可选自定义流地址。不填时服务端会自动生成带签名的局域网直通流 URL |

**响应**：`200 OK`。

---

### 3.4 远程播控 (Control)

对当前激活的远程设备进行暂停、继续、停止、跳转和音量控制。

```http
POST /api/jukebox/control
Content-Type: application/json
Authorization: Bearer <token>

{
  "action": "pause"
}
```

#### 支持的 `action` 与参数：

| action | 附加字段 | 示例请求体 | 说明 |
| :--- | :--- | :--- | :--- |
| `pause` | 无 | `{"action": "pause"}` | 暂停播放 |
| `resume` | 无 | `{"action": "resume"}` | 继续播放 |
| `stop` | 无 | `{"action": "stop"}` | 停止播放 |
| `seek` | `value` (秒数, int) | `{"action": "seek", "value": 128}` | 跳转到指定秒数 |
| `volume` | `value` (音量, 0-100, int) | `{"action": "volume", "value": 65}` | 设置设备绝对音量 |

---

### 3.5 轮询设备状态与音量同步 (Status)

客户端在远程播放中建议以 `1s ~ 3s` 间隔轮询该接口，更新 UI 播放进度与音量。

```http
GET /api/jukebox/status
Authorization: Bearer <token>
```

**响应示例 (200 OK)**：
```json
{
  "status": "playing",
  "currentTime": 45,
  "duration": 218,
  "volume": 65,
  "deviceId": "dlna-bedroom",
  "deviceType": "dlna"
}
```

- `status`：`"playing"` | `"paused"` | `"stopped"`
- `currentTime`：当前已播放秒数（部分设备如小爱音箱原生协议不支持进度回报，恒为 `0`）
- `duration`：当前曲目总时长（秒）
- `volume`：设备当前实际音量（`0 - 100` 整数）
- `deviceType`：用于客户端针对性做功能降级判断

---

### 3.6 管理端设备发现与配置 (Admin Only)

- `GET /api/jukebox/discover?timeout=3`：在局域网内进行 SSDP M-SEARCH 扫描，自动发现附近的 DLNA / UPnP 渲染器设备。
- `GET /api/jukebox/outputs`：获取已持久化保存的所有自定义输出设备。
- `POST /api/jukebox/outputs`：创建新的输出设备。
- `PUT /api/jukebox/outputs/{id}`：修改已有设备。
- `DELETE /api/jukebox/outputs/{id}`：删除输出设备。

---

## 4. 客户端集成四大核心铁律 (踩坑经验提炼)

在 Android (如 Chora / ExoPlayer / Media3) 或其他移动端集成多输出端时，**必须严守以下四项铁律**：

### 铁律 1: 0 音量容错与防抢手保护
- **禁止无脑采纳 0 音量**：DLNA 和 MPD 设备在刚开机、刚连接或未就绪时，首次回答音量查询可能会回报 `0`。客户端若将其直接写入本地音量，会导致播放器瞬间被静音并误持久化！**处理规则：当设备回报 `volume == 0` 时，坚决忽略，保留本地音量。**
- **用户拖动防抖与防抢手 (3秒锁定)**：
  1. 用户在客户端拖动音量条时，必须加入 200ms ~ 300ms 防抖，防抖结束后向 `/api/jukebox/control` 发送 `volume`。
  2. 用户松手后的 **3 秒钟内**，忽略 `/api/jukebox/status` 轮询返回的音量，防止因网络延迟或设备反馈滞后将滑块硬生生拽回旧值。

### 铁律 2: 客户端静音与本地时钟驱动 (保持前台媒体服务与锁屏通知)
- 当用户将输出切换为远程设备时，客户端**不能简单地完全销毁本地播放器**。
- **推荐策略**：
  1. 将本地播放器（ExoPlayer）设置为静音（`player.volume = 0f`），或者使用一个本地虚拟计时器（Virtual Clock）驱动进度推进。
  2. 保持 Android 的 `MediaSession` 和前台通知栏服务（`MediaLibraryService`）处于激活状态，以便锁屏、通知栏控制器、车载蓝牙、智能手表等依然能控制上一首、下一首、暂停。
  3. UI 进度条以本地时钟为主渲染，每隔数秒拿 `/api/jukebox/status` 的 `currentTime` 校准轻微漂移（若设备回报有进度且差距大于 2 秒才微调）。

### 铁律 3: 依据 `deviceType` 动态降级
- **`xiaomi` 原生协议**：
  - 不支持精确 `seek`（调用会报 400 错误）。当 `deviceType == "xiaomi"` 时，客户端应在 UI 上将进度滑块禁用，或拦截拖拽事件并轻提示“该音箱不支持拖动跳转”。
  - 进度回报恒为 `0`，此时进度条必须完全依靠客户端本地播放估算时长推进。
- **`dlna` 协议**：
  - 部分小爱音箱在刚收到 `Play` 指令时立即 `Seek` 会被忽略，服务端已内置 3 次校验重试，客户端只需发一次。

### 铁律 4: 切换设备时的无缝衔接
- **切到远程**：记录本地当前播放位置 `val pos = player.currentPosition / 1000` -> 调用 `POST /api/jukebox/select` -> 调用 `POST /api/jukebox/play` 传入 `position = pos` -> 本地播放器静音并同步起步。
- **切回本机**：调用 `POST /api/jukebox/select` 传入 `deviceId = "browser"`（服务端会自动暂停/停止远程设备） -> 本地播放器恢复音量并 `player.play()` 从原进度继续出声。

---

## 5. Android (Kotlin + Retrofit) 快速接入示例

### 5.1 数据契约模型

```kotlin
data class JukeboxDevicesResponse(
    val devices: List<JukeboxDevice>,
    val selected: String
)

data class JukeboxDevice(
    val id: String,
    val name: String,
    val type: String // "browser", "mpd", "dlna", "xiaomi"
)

data class JukeboxSelectRequest(
    val deviceId: String
)

data class JukeboxPlayRequest(
    val songId: String,
    val position: Long = 0L,
    val streamUrl: String? = null
)

data class JukeboxControlRequest(
    val action: String, // "pause", "resume", "stop", "seek", "volume"
    val value: Long? = null
)

data class JukeboxStatusResponse(
    val status: String, // "playing", "paused", "stopped"
    val currentTime: Long,
    val duration: Long,
    val volume: Int,
    val deviceId: String,
    val deviceType: String
)
```

### 5.2 Retrofit 服务接口

```kotlin
interface JukeboxService {
    @GET("api/jukebox/devices")
    suspend fun getDevices(
        @Header("Authorization") token: String
    ): Response<JukeboxDevicesResponse>

    @POST("api/jukebox/select")
    suspend fun selectDevice(
        @Header("Authorization") token: String,
        @Body request: JukeboxSelectRequest
    ): Response<Unit>

    @POST("api/jukebox/play")
    suspend fun play(
        @Header("Authorization") token: String,
        @Body request: JukeboxPlayRequest
    ): Response<Unit>

    @POST("api/jukebox/control")
    suspend fun control(
        @Header("Authorization") token: String,
        @Body request: JukeboxControlRequest
    ): Response<Unit>

    @GET("api/jukebox/status")
    suspend fun getStatus(
        @Header("Authorization") token: String
    ): Response<JukeboxStatusResponse>
}
```

---

## 6. HTTP 状态码与故障排查速查

| HTTP 状态码 | 含义 | 原因与排查方向 |
| :--- | :--- | :--- |
| **401** | 未鉴权 | Token 缺失、无效或已过期，请重新调用 `/auth/login` |
| **403** | 权限不足或未开启 | 服务端 `Jukebox.Enabled = false`，或者服务端配置了 `Jukebox.AdminOnly = true` 且当前用户非管理员 |
| **404** | 未找到 | 目标 `songId` 不存在，或者选择的 `deviceId` 不存在 |
| **409** | 当前为本机输出 | 在 `selected == "browser"` 状态下向 `/api/jukebox/control` 发送了设备播控指令 |
| **400** | 无效命令 / 参数越界 | 设备不支持该指令（如向 xiaomi 发送 seek），或音量值超出 0-100 范围 |
| **502** | 设备驱动通信失败 | 目标音箱离线、网络不可达、或返回错误。**Response Body 中包含设备原生的详细错误原因** |
