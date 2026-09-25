---
name: lyrics-translation
description: Navidrome 歌词翻译与双语对照功能架构、新增翻译引擎 Provider、双层悬浮歌词与抽屉歌词同步机制、缓存与端到端调试指南。当用户要求"新增歌词翻译引擎"、"修改翻译逻辑"、"排查歌词不刷新/翻译失败/双语显示问题"时使用。
---

# 歌词翻译与双语对照

必读前置：仓库根 `AGENTS.md` 第 4 节（歌词翻译与双语模型）与功能文档 `docs/lyrics-translation-api.md`。

---

## 1. 架构总览

本功能为 Navidrome 注入了**按需触发的大模型与传统 API 歌词翻译能力**，支持双语对照并在单行悬浮与抽屉全屏歌词中实时同步。

### 模块分布

```
core/lyrics/
  translation.go         TranslationService 服务单例、配置读写、磁盘缓存、singleflight、LRC 格式合成
  provider_gemini.go     Google Gemini (REST API，支持 gemini-flash-latest / flash-lite 等)
  provider_zhipu.go      智谱清言 GLM-4 (glm-4-flash)
  provider_openai.go     OpenAI 兼容接口 (DeepSeek、Moonshot 等，支持自定义 BaseURL)
  provider_baidu.go      百度翻译 API (通用文本翻译，支持 AppID + SecretKey + MD5 签名)
  provider_google.go     Google 翻译 (免费网页端点接口，无 Key)
server/nativeapi/
  lyrics_translation.go  /api/lyrics/translate (用户翻译端点) 与 /translation/config /test (管理端接口)
ui/src/
  audioplayer/
    TranslateButton.jsx  底栏/控制栏翻译切换按钮（支持内存/磁盘双重缓存即时切换与加载动画）
    Player.jsx           双层歌词同步（底层 audioLists[playIndex].lyric 注入与 initLyricParser 调度）
  lyricsTranslation/
    LyricsTranslation.jsx 独立管理后台页面（路由 /lyrics-translation，参数配置与实时连通性测试）
  layout/
    LyricsTranslationMenu.jsx 侧边栏导航条目
```

---

## 2. 核心状态与两层同步模型（最易出错点）

### Redux 状态（`state.player`）

- `bilingualActive`: boolean，当前曲目是否处于双语/翻译显示模式。
- `originalLyrics`: `{ [trackId]: string }`，缓存曲目原始 LRC 文本。
- `bilingualLyrics`: `{ [trackId]: string }`，缓存曲目翻译后双语 LRC（优先 `inlineLrc`，备选 `bilingualLrc`）。

### 双层歌词同步机制

1. **抽屉/全屏歌词组件**：订阅 Redux `state.player`，响应式更新。
2. **底栏单行悬浮歌词（`music-player-lyric`）**：由 `navidrome-music-player` 内部维护，**只认** `playerRef.current.state.audioLists[playIndex].lyric`。

**切换双语时的三重动作（必须同时触发，见 `TranslateButton.jsx`）**：
```javascript
// 1. 更新 Redux 状态
dispatch(setBilingualActive(targetState))
dispatch(setBilingualLyric(songId, targetLrc))

// 2. 替换播放器内核当前歌曲的 lyric 字段
playerRef.current.state.audioLists[playIndex].lyric = targetLrc

// 3. 立即重置解析器并以当前时间驱动重绘
playerRef.current.initLyricParser()
playerRef.current.update(currentTimeMs)
```
> ⚠️ **关键坑**：只改 Redux 状态不改 `audioLists` 和 `initLyricParser`，底栏桌面悬浮歌词会一直停留在旧歌词，用户会认为"翻译没生效"！

### 队列防回滚保护（`Player.jsx`）

播放器在 `reduceSyncQueue` 和 `reduceCurrent` 中，会在用户调整队列或切歌时从传入列表中覆盖 `audioLists`。
当 `bilingualActive === true` 时，必须保留已经注入的最新双语 `lyric`，禁止被传入的未翻译旧歌词覆盖。

---

## 3. 三种歌词输出格式

后端 `buildTranslationResult`（`core/lyrics/translation.go`）每次翻译都会合成三种格式：

1. **`inlineLrc`（单行合并，底栏最推荐）**：
   ```lrc
   [00:12.34]Original line / 翻译行
   [00:15.67]Next line / 下一行
   ```
   **优势**：同一时间戳只保留单行，中间以 ` / ` 分隔。单行悬浮播放器内核在遇到同时间戳多行时经常发生闪烁或只取第一行，`inlineLrc` 完美解决了此问题。
2. **`bilingualLrc`（双行同时间戳）**：
   ```lrc
   [00:12.34]Original line
   [00:12.34]翻译行
   ```
   标准双行 LRC，适合多行滚动的全屏歌词组件。
3. **`combinedLrc`（单时间戳随附翻译）**：
   ```lrc
   [00:12.34]Original line
   翻译行
   ```
4. **`lines`（结构化数组）**：
   `[{ index: 0, start: 12340, end: 15670, original: "...", translation: "..." }]`，供第三方客户端使用。

---

## 4. 新增翻译引擎 Provider（开发清单）

如需接入新的翻译服务（如 Claude、Kimi、火山引擎等）：

### 1. 实现 Provider 接口
在 `core/lyrics/` 创建 `provider_<name>.go`：
```go
package lyrics

import (
    "context"
)

type MyNewProvider struct{}

func (p *MyNewProvider) Translate(ctx context.Context, lines []string, targetLang string, cfg LyricsTranslationConfig) ([]string, error) {
    // 1. 读取 cfg.ApiKey、cfg.BaseURL、cfg.ProxyURL 等
    // 2. 调用外部 API
    // 3. 必须保证返回的 slice 长度与输入的 lines 严格一一对应
    // 4. 返回翻译结果
}
```

### 2. 注册 Provider
在 `core/lyrics/translation.go`：
- 添加常量：`EngineMyNew = "mynew"`
- 在 `NewTranslationService` 中注册：`ts.providers[EngineMyNew] = &MyNewProvider{}`

### 3. 配置支持
若有特殊参数，在 `LyricsTranslationConfig` 中添加字段，若为敏感 Key，在 `GetMaskedConfig` 中添加掩码处理。

### 4. 前端表单扩展
在 `ui/src/lyricsTranslation/LyricsTranslation.jsx`：
- 在 `ENGINE_OPTIONS` 中添加 `{ id: 'mynew', name: '...' }`
- 按需控制显示模型、Key、BaseURL、ProxyURL 等输入项。

### 5. i18n
在 `ui/src/i18n/en.json`、`resources/i18n/zh-Hans.json`、`zh-Hant.json` 补齐相应文案。

### 6. 测试
编写单测（参考 `core/lyrics/translation_test.go`），用 `httptest.Server` 模拟 API 响应。

---

## 5. 调试与排错指南

| 现象 | 可能原因 | 解决办法 |
|---|---|---|
| 点击翻译一直转圈 | 外部 API 超时或网络受阻 | 检查是否需要配置 `proxyUrl`（如 `http://127.0.0.1:7890`）；查看服务端控制台日志 |
| 报错 403 | 翻译总开关未开 | 在 Web「歌词翻译」后台开启总开关并保存 |
| 报错 404 | 歌曲无 LRC 或内嵌歌词 | 属于正常情况，纯音乐或未匹配到歌词的歌曲无法翻译 |
| 报错 400 | API Key 未配置 | 在管理后台填写有效 API Key |
| 报错 500 (Baidu 54001) | 百度签名或 AppID 错误 | 检查 `AppID` 与 `SecretKey` 是否配反或有空格 |
| 抽屉歌词变了但悬浮歌词不变 | 底层播放器内核未重置 | 确保调用了 `initLyricParser()` 和 `update(timeMs)` |
| 重新翻译旧歌曲结果不变 | 命中了磁盘持久化缓存 | 检查 `<DataFolder>/lyrics_translations/`，或用 API 带 `"force": true` 强制刷新 |

---

## 6. 验证命令

```bash
# 后端测试
make test PKG=./core/lyrics
gofmt -l core/lyrics conf server/nativeapi

# 前端测试
cd ui
npx vitest run src/audioplayer/TranslateButton.test.jsx
npm run lint
```
