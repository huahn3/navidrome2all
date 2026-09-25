# Navidrome 歌词翻译 API — 第三方客户端集成指南

> **版本**：Navidrome fork（本仓库），自定义 API，不属于 Subsonic 标准协议范围。  
> **适用对象**：Chora、Symfonium、Finamp、Ultrasonic 等任何希望接入歌词翻译功能的第三方客户端。

---

## 概览

Navidrome 在播放侧内置了**按需歌词翻译**能力。翻译只在用户主动点击时触发，不会预加载或后台扫描。结果会在服务器端缓存，第二次请求同一首歌时直接返回缓存，几乎无延迟。

支持的翻译引擎（由服务器管理员配置）：

| 引擎 ID | 名称 | 类型 |
|---|---|---|
| `gemini` | Google Gemini Flash / Flash Lite | 大模型 |
| `zhipu` | 智谱 GLM-4-Flash | 大模型 |
| `openai` | OpenAI 兼容接口（DeepSeek 等） | 大模型 |
| `baidu` | 百度翻译 | 传统 API |
| `google` | Google Translate (免费) | 传统 API |

---

## 鉴权

所有 `/api/` 路由使用 Native API 鉴权头：

```
Authorization: Bearer <token>
```

`<token>` 来自 `POST /auth/login` 的 JSON 响应中的 `token` 字段。  
**注意：Cookie 不生效，必须用 Bearer 头。**

---

## API 端点

### 1. 翻译歌词（核心接口）

```
POST /api/lyrics/translate
Content-Type: application/json
Authorization: Bearer <token>
```

**请求体**

```json
{
  "songId":    "abc123",
  "targetLang": "zh-CN",
  "force":     false
}
```

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `songId` | string | ✅ | Navidrome 媒体文件 ID |
| `targetLang` | string | ❌ | 目标语言，BCP-47 格式，默认 `zh-CN` |
| `force` | bool | ❌ | `true` 时强制重新翻译，跳过缓存 |

**成功响应 (200)**

```json
{
  "songId": "abc123",
  "targetLang": "zh-CN",
  "engine": "gemini",
  "model": "gemini-flash-latest",
  "updatedAt": "2024-01-15T10:30:00Z",
  "inlineLrc": "[00:12.34]Original line / 翻译行\n[00:15.67]Next line / 下一行\n",
  "bilingualLrc": "[00:12.34]Original line\n[00:12.34]翻译行\n[00:15.67]Next line\n[00:15.67]下一行\n",
  "combinedLrc":  "[00:12.34]Original line\n翻译行\n[00:15.67]Next line\n下一行\n",
  "lines": [
    {
      "index": 0,
      "start": 12340,
      "end": 15670,
      "original": "Original line",
      "translation": "翻译行"
    }
  ]
}
```

| 字段 | 说明 |
|---|---|
| `inlineLrc` | **单行悬浮双语歌词（推荐）**：同时间戳合并原文与译文（`原文 / 译文`），专为底栏悬浮及单行播放器设计，彻底避免同时间戳双行歌词竞争闪烁 |
| `bilingualLrc` | 双语 LRC：每行原文后紧跟同时间戳的译文行，适合支持同时间戳双行的全屏滚动歌词器 |
| `combinedLrc` | 组合 LRC：原文行带时间戳，译文行紧跟其后无时间戳 |
| `lines` | 结构化数组，含 `start/end`（毫秒）、`original`、`translation`，适合自定义 UI 渲染 |

**错误响应**

| HTTP | 含义 |
|---|---|
| 403 | 翻译功能未启用（管理员未开启） |
| 404 | 歌曲不存在或无可翻译歌词 |
| 400 | 翻译引擎未配置 API Key |
| 500 | 翻译引擎调用失败（body 含原始错误信息） |

---

### 2. 读取已缓存翻译（快速，无 AI 调用）

```
GET /api/lyrics/translate/{songId}?lang=zh-CN
Authorization: Bearer <token>
```

- 缓存存在 → 200 + 同上 JSON 结构  
- 缓存不存在 → 404

**建议**：先查此接口，404 再调 POST 触发翻译。

---

### 3. 管理端：获取与更新翻译配置 (Admin Only)

**获取配置**
```
GET /api/lyrics/translation/config
Authorization: Bearer <admin-token>
```

响应 200（密钥字段脱敏为 `******`）：
```json
{
  "enabled": true,
  "engine": "gemini",
  "model": "gemini-flash-latest",
  "apiKey": "******",
  "secretKey": "",
  "appId": "",
  "targetLanguage": "zh-CN",
  "proxyUrl": "",
  "baseUrl": ""
}
```

**更新配置**
```
PUT /api/lyrics/translation/config
Content-Type: application/json
Authorization: Bearer <admin-token>
```

**连通性测试**
```
POST /api/lyrics/translation/test
Content-Type: application/json
Authorization: Bearer <admin-token>
```
请求体同配置对象，后端将使用给定的配置翻译一句测试歌词，成功返回 200 `{"status":"ok","sample":"..."}`，失败返回 500 原始错误。

---

### 4. Subsonic getLyricsBySongId 扩展参数

本 fork 对标准 Subsonic 接口增加了两个非标准扩展参数：

```
GET /rest/getLyricsBySongId?id={songId}&translate=true&bilingual=true
```

| 参数 | 说明 |
|---|---|
| `translate` | true 时触发翻译（有延迟），将翻译后内容写入歌词响应 |
| `bilingual` | true 时在响应中附加 `bilingualLrc` 字段 |

> ⚠️ 这两个参数为本 fork 新增，标准 Navidrome 不支持，请做特性检测再启用。

---

## 推荐集成流程

```
用户点击"翻译"按钮
    │
    ▼
GET /api/lyrics/translate/{songId}?lang=<targetLang>
    ├─ 200 → 直接使用缓存结果 ✅
    └─ 404 →
            ▼
        显示加载状态
            ▼
        POST /api/lyrics/translate { songId, targetLang }
            ├─ 200 → 使用结果，更新 UI ✅
            ├─ 403 → 提示"请联系管理员开启翻译功能"
            ├─ 404 → 提示"此歌曲暂无可翻译歌词"
            └─ 其他 → 提示翻译失败
```

---

## UI 展示建议

### 双语对照

```
♪ Can't buy me love                ← 原文（高亮同步）
  买不来爱情                          ← 译文（灰色，字体略小）

  Everybody tells me so
  人人都这么说
```

### 数据字段选择

| 场景 | 推荐字段 |
|---|---|
| LRC 播放器（时间轴同步） | `bilingualLrc` |
| 纯文本/滚动列表 | `lines[]` 数组 |
| 保留原歌词时间轴 | `lines[i].start` + `lines[i].translation` |

---

## 目标语言参考

| BCP-47 | 语言 |
|---|---|
| `zh-CN` | 简体中文（默认） |
| `zh-TW` | 繁體中文 |
| `en` | English |
| `ja` | 日本語 |
| `ko` | 한국어 |
| `fr` | Français |
| `de` | Deutsch |
| `es` | Español |

---

## 特性检测

```
GET /api/lyrics/translate/{anyValidSongId}?lang=zh-CN
```

- 返回 200 或 404 → 服务器支持 ✅
- 网络错误或 5xx → 服务器为标准 Navidrome，不支持 ❌

---

## 缓存说明

- 存储在服务器端 `<DataFolder>/lyrics_translations/` 目录
- 命名格式：`{songId}_{lang}.json`
- 永久有效，直到管理员清除或 `force=true` 强制刷新
- 客户端无需自行持久化翻译结果

---

## curl 示例

```bash
TOKEN=$(curl -s -X POST https://your-navidrome/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"user","password":"pass"}' | jq -r .token)

# 查缓存
curl -s -o /dev/null -w "%{http_code}" \
  "https://your-navidrome/api/lyrics/translate/abc123?lang=zh-CN" \
  -H "Authorization: Bearer $TOKEN"

# 触发翻译（缓存 miss 时）
curl -s -X POST https://your-navidrome/api/lyrics/translate \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"songId":"abc123","targetLang":"zh-CN"}'
```

---

*如发现 API 行为与此文档不符，请以仓库 `server/nativeapi/lyrics_translation.go` 实现为准。*
