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

- `bilingualTrackId`: string | null，**当前曲目**的双语标记。判定一律用导出的纯函数
  `isBilingualTrack(state, trackId)`（`ui/src/reducers/playerReducer.js`）。
  > 不要引入全局 boolean：那样"给 A 翻译后切到 B，B 的按钮也会显示已双语"。
- `originalLyrics`: `{ [trackId]: string }`，缓存曲目原始 LRC 文本。
- `bilingualLyrics`: `{ [trackId]: string }`，缓存曲目翻译后双语 LRC（优先 `inlineLrc`，备选 `bilingualLrc`）。

### 双层歌词同步机制

1. **抽屉/全屏歌词组件**：订阅 Redux `state.player`，响应式更新。
2. **底栏单行悬浮歌词（`music-player-lyric`）**：由 `navidrome-music-player` 内部维护，
   **只认** `playerRef.current.state.audioLists[playIndex].lyric`。

**切换双语实际只做一件事**——`TranslateButton.jsx` 唯一的 action：
```javascript
dispatch(updateSongLyric(trackId, lrc, isBilingual))
```
（`ui/src/actions/player.js`。仓库里**没有** `setBilingualActive` / `setBilingualLyric`
这类 action，写了就是死代码。）

内核注入与解析器刷新由 `Player.jsx` 里依赖 `playerState.current?.lyric` 的 effect
统一完成（`ui/src/audioplayer/Player.jsx`，约 500 行起）：
1. 按 `trackId` 匹配写 `player.state.audioLists` 的**所有**条目（不是只写当前那条——
   否则 `updateAudioLists` 会从队列数据把它盖回去）；
2. 再写 `audioLists[playIndex].lyric`；
3. 若 `state.lyric !== currentLyric` → `setState({lyric}, cb => player.initLyricParser())`；
   否则只 `player.lyric.update(ms)` 让单行滚动跟上时间。

> ⚠️ **关键坑**：组件里**不要**自己碰 `playerRef`。只 dispatch 就够了，
> 手写一遍内核注入反而会漏掉上面第 1 步的全量匹配。

### 队列侧的保护在 reducer，不在 Player.jsx

`reduceSyncQueue` 在合并队列时，只要队内已有不同且非空的 lyric 就**保留旧值**
（无条件生效，不看双语标记）；`reduceCurrent` 把队列里的 lyric 回填进 `current`，
并**仅在曲目真的变了**时把 `bilingualTrackId` 清空。`isBilingualTrack` 用在两处：
`reduceUpdateSongLyric`（决定新的 `bilingualTrackId`）和 `reduceCurrent`
（判断"当前曲目的原文还没记下来"时跳过写缓存）。

---

## 3. 歌词输出格式（3 种 LRC + 1 种结构化）

后端 `buildTranslationResult`（`core/lyrics/translation.go`）每次翻译都会合成：

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

> **译文与原文相同时，三种 LRC 都只输出一遍原文**（比较前 `TrimSpace`，所以只有空白
> 差异也算相同；译文为空同理）。上面示例若 `translation` 等于 `original`，`bilingualLrc`
> 就是一行 `[00:12.34]Original line`，不会出现重复两行。
> 这是修过的 bug——引擎对专有名词、纯外文歌词、人名常原样返回，重复行非常显眼。
>
> **`lines[]` 不去重**：`translation` 保留引擎返回的原样重复原文。用结构化数据渲染的
> 客户端要自己判断 `translation === original`。

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
- 把新引擎 id 加进 `ENGINE_IDS`，并在 `ENGINE_LABEL_KEYS` 里补一条**字面量** key
  （`{ mynew: 'menu.lyricsTranslation.engine.mynew' }`）
- 在 `ENGINE_NOTE_KEYS` 里补说明卡片的 key
- 按需控制显示模型、Key、BaseURL、ProxyURL 等输入项（未启用引擎的输入项用条件渲染）

> 这一页的选项是"id 数组 + key 映射表"，不是 `{id,label}` 对象数组。
> 两种写法混用会让 `value` 全变成 `undefined`，下拉框渲染成空白且菜单打不开
> （真实踩过，见 `docs/risk-notes-optimization.md` §11.7.2）。

### 5. i18n（三份都要写 + 校验）

- `ui/src/i18n/en.json`、`resources/i18n/zh-Hans.json`、`resources/i18n/zh-Hant.json`
- **key 一律写字面量，不要用模板字符串拼**：`` t(`menu.lyricsTranslation.engine.${id}`) ``
  拼出来的 key 校验器抓不到，语言包里命名一对不上就静默回退（界面上显示成语言码，
  而且中文界面完全看不出来）。语言码带连字符（`zh-CN`）而 key 名是 `zhCN`，
  所以必须显式映射表。
- 改完必跑两条：

```bash
python3 scripts/check-i18n-keys.py   # 引用了但语言包里没有的 key（期望 0）
make test-i18n                        # 36 份语言包与 en.json 的 key 对齐
```

### 6. 手机端（这一页是窄屏重灾区）

- 表单容器已挂 `className="responsive-fields"`；字段组用 `row`（grid +
  `minmax(0, 1fr)`，手机单列 / 桌面双列），**不要**改回 `flex: 1 1 240px`
  （窄屏不会真的换行，会被内容顶成两列半宽控件）
- 引擎开关关闭时那个 `Collapse` 必须保留 `unmountOnExit`，否则隐藏内容仍参与
  min-content 计算，把整页撑到 500+px
- 缓存表格容器需要 `overflow-x: auto` **加** `contain: inline-size`，
  只写前者无效（容器是普通块，min-content 仍等于表格宽度）

### 7. 测试
编写单测（参考 `core/lyrics/translation_test.go`），用 `httptest.Server` 模拟 API 响应。
前端 `LyricsTranslation.test.jsx` 里注意：`useTranslate` 被 mock 成
`(key, options) => options?._ || key`（不做插值），所以断言的是**兜底默认值**。

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
| 界面某处文案变成语言码/英文 | i18n key 没对上，静默回退到 `_:` 默认值 | 跑 `python3 scripts/check-i18n-keys.py`；确认没有用模板字符串拼 key |
| 手机端右侧内容被裁 / appbar 图标被挤掉 | 某后代 min-content 超过视口（`Layout` 是 `min-width: fit-content`） | 用 `build-and-test` skill 里的离屏探针逐层量 min-content 定位元凶；**注意空状态不渲染表格/列表，要造真实数据再测** |

---

## 6. 验证命令

```bash
# 后端测试（42 specs）
make test PKG=./core/lyrics
gofmt -l core/lyrics conf server/nativeapi

# 前端测试
cd ui
npx vitest run src/audioplayer/TranslateButton.test.jsx
npx vitest run src/lyricsTranslation                  # 管理页 6 例
npm run lint && npm run check-formatting

# i18n
python3 scripts/check-i18n-keys.py
make test-i18n
```

### 造缓存数据做 UI 验证

管理页的"已翻译歌曲"表格只在**有缓存时**才渲染，空状态下测不到那条路径
（连续两轮漏测就是栽在这里）。手工造两条：

```bash
SONG_ID=$(curl -s -H "X-ND-Authorization: Bearer $TOKEN" \
  'http://localhost:4533/api/song?_end=2&_start=0' | python3 -c 'import json,sys;print(json.load(sys.stdin)[0]["id"])')
mkdir -p <DataFolder>/lyrics_translations
printf '{"songId":"%s","lang":"zh-Hans","lines":[{"start":0,"end":5000,"original":"a","translation":"b"}]}' \
  "$SONG_ID" > <DataFolder>/lyrics_translations/${SONG_ID}_zh-Hans.json
```
