---
name: playback-handoff
description: Navidrome 活跃会话追踪与跨端无缝流转接管（Playback Handoff & Takeover）架构、6大维度全状态继承（输出设备、音量、进度、状态、循环模式、双语歌词）、单发声源互斥停播与 SSE 广播机制、前后端排障与端到端联调指南。
---

# 活跃会话追踪与跨端流转接管（Playback Handoff）

必读前置：仓库根 [AGENTS.md](file:///Users/liubo/Desktop/navidrome2all/AGENTS.md) 第 3 节（音量模型）、第 4 节（双语歌词模型）、第 5 节（Jukebox）与功能文档 [docs/playback-handoff-api.md](file:///Users/liubo/Desktop/navidrome2all/docs/playback-handoff-api.md)。

---

## 1. 架构总览

本功能为 Navidrome 注入了类似 **Spotify Connect / Apple Handoff** 的全状态无缝流转接管能力，彻底打通网页端、第三方客户端（如 Chora）及外部音箱（Jukebox MPD/DLNA/小米）之间的信息孤岛。

```
[客户端 A (Web/App)] --- reportPlayback (携带设备/音量/循环/双语) ---> [PlayTracker]
                                                                        | (内存 PlayMap + TTL)
[客户端 B (Web/App)] <--- SSE /api/events 或 getNowPlaying -------------+
        |
        +-- 点击接管 (Takeover) 
             |-- 1. 继承 6 大状态 (outputDevice, volume, positionMs, state, playMode, bilingual)
             |-- 2. POST /api/playback/sessions/{sessionId}/takeover
                     |-- PlayTracker 标记目标为 paused/stopped
                     |-- SSE 广播 playbackHandoff -> 客户端 A 即时静音暂停
                     +-- Jukebox 智能防冲突 (选本机时停音箱，选音箱时平滑接力)
```

### 核心模块拓扑

```
core/scrobbler/
  play_tracker.go           PlaybackSession 单例缓存、线性进度插值、OutputDevice/Volume/PlayMode/Bilingual 维护
server/subsonic/
  media_annotation.go       reportPlayback 扩展参数解析 (&outputDevice=...&volume=...&playMode=...&bilingualActive=...)
  album_lists.go            getNowPlaying 响应装配全状态
server/events/
  events.go                 PlaybackHandoff SSE 广播事件结构体
server/nativeapi/
  playback_sessions.go      GET /api/playback/sessions 会话列表与 POST /takeover 接口、Jukebox 发声防冲突路由
ui/src/
  subsonic/index.js         reportPlayback 封装，支持透传 extra 参数
  actions/player.js         takeoverTrack(data, positionSec, extraOptions)
  reducers/playerReducer.js reduceTakeoverTrack 状态承接 (outputDevice, volume 换算, pendingState, mode, bilingualActive)
  audioplayer/Player.jsx    音量平方律下发、pendingState 暂停保护、双语歌词自动补全、reportPlayback 心跳全量上报
  layout/NowPlayingPanel.jsx 活跃会话面板、多端状态角标展示 (🔊 设备名 / 音量 % / 本机)、接管意图派发
```

---

## 2. 6 大全量继承维度与核心规则（最重要不变量）

当用户点击接管（Takeover）时，接管端必须**无损且全量**继承被接管端的 6 大维度状态：

| 维度 | 字段 | 类型 | 取值范围与继承规则 |
|---|---|---|---|
| **1. 输出设备** | `outputDevice` | `string` | `"browser"`（本机网页/App）或 Jukebox 设备 ID（如 `"xiaomi_l7a"`, `"mpd"`）。若对方正在音箱播放，接管端默认继承该设备 ID，不中断音箱发声 |
| **2. 音量大小** | `volume` | `integer` | `0 ~ 100` 整数百分比。Redux store 需将其换算为 `0.0 ~ 1.0`（除以 100）。若为 0 必须兜底为 `defaultUIVolume/100` 严禁持久化 0 |
| **3. 播放进度** | `positionMs` | `integer` | 毫秒级精度。服务端根据上次心跳时间戳与 `playbackRate` 进行线性插值（`clampedMs`），接管端通过 `pendingSeekTime` 瞬间定位 |
| **4. 播放状态** | `state` | `string` | `"playing"`、`"paused"`。若原设备是暂停状态，用户点击会话卡片应**保持暂停**；仅当用户主动点击卡片右侧的 `▶` 按钮时强制设为 `playing` |
| **5. 循环模式** | `playMode` | `string` | `"single"`（单曲循环）、`"all"`（列表循环）、`"order"`（顺序播放）。接管端同步设置播放器内核的 `mode` |
| **6. 双语歌词** | `bilingual` | `boolean` | 是否开启歌词翻译。若为 `true`，接管端自动激活双语模式并异步拉取翻译结果（命中服务端磁盘缓存 0ms 渲染） |

---

## 3. 音量与两层播放内核联动铁律

1. **Redux store 是音量唯一真源**：
   - 远程接口上报与拉取值：`0 ~ 100`（整数）；
   - Store 内部存储：`0.0 ~ 1.0` 感知值；
   - 网页本地 `<audio>` 元素：`store²`（物理平方律换算）；
   - 远程 Jukebox 设备：`round(store * 100)`。
2. **状态继承时的暂停保护（`pendingState === 'paused'`）**：
   - `react-jinke-music-player` 在装载新队列（`clear: true`）时默认会尝试 autoPlay；
   - 若接管的目标状态是 `"paused"`，`Player.jsx` 必须在 `options.autoPlay`、`onAudioPlay` 回调以及 `pendingSeekTime` loadedmetadata 时三重拦截并立即调用 `audioInstance.pause()`，确保不误播声音。
3. **双语歌词无缝拉取**：
   - 继承 `bilingualActive: true` 后，若本地 Redux 未缓存该歌曲的双语歌词，`Player.jsx` 自动触发 `POST /api/lyrics/translate`。由于被接管端早已请求过该曲目，服务端磁盘永久缓存直接命中返回，实现无感双语同步。

---

## 4. Jukebox 发声通道防冲突协议

在 `POST /api/playback/sessions/{sessionId}/takeover` 接口中：
- **场景 1：接管端选择“本机发声” (`targetOutput = "browser" / "local"`)**：
  若此时原会话或服务器正通过 Jukebox 外部音箱播放，服务端自动向 Jukebox 下发 `pause`，确保声音平滑转移到当前设备的耳机/扬声器，**杜绝两端同时发声**。
- **场景 2：接管端选择继续在外部音箱播放（`targetOutput = targetSession.outputDevice`）**：
  服务端**绝不暂停 Jukebox**，音箱无缝连续播放，仅向原控制设备下发交接信号，接管端接管播控权与实时进度。

---

## 5. 开发与测试自查清单

1. **测试用例验证**：
   ```bash
   # 后端会话追踪与原生接口测试
   go test -count=1 -tags=netgo,sqlite_fts5 ./core/scrobbler ./server/nativeapi ./server/subsonic
   # 前端状态与面板测试
   cd ui && npm test -- NowPlayingPanel.test.jsx playerReducer.test.js
   ```
2. **代码风格与规范**：
   ```bash
   gofmt -l core/scrobbler server/nativeapi server/subsonic
   cd ui && npm run lint && npm run check-formatting
   ```
3. **真实多端流转交互验证**：
   - 设备 A 播放歌曲并切换为双语歌词、设置音量为 65%、单曲循环模式；
   - 设备 B 打开右上方「正在播放」面板，确认设备 A 的卡片上显示 `[🔊 设备名] [65%]`；
   - 设备 B 点击接管，确认设备 B 继承了 65% 音量、单曲循环、双语歌词与当前进度；
   - 检查设备 A 收到 SSE `playbackHandoff` 提示“播放已被接管”并自动暂停。
