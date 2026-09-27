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
X-ND-Authorization: Bearer <token>
```

`<token>` 来自 `POST /auth/login` 的 JSON 响应中的 `token` 字段。  
**注意：Cookie 不生效，必须用 Bearer 头。**

---

## API 端点

### 1. 翻译歌词（核心接口）

```
POST /api/lyrics/translate
Content-Type: application/json
X-ND-Authorization: Bearer <token>
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
| `lines` | 结构化数组（毫秒精度），每项 `{index, start, end, original, translation}` |

> **译文与原文相同时只输出一遍原文**（比较前会 `TrimSpace`，所以仅有空白差异也
> 算相同；译文为空同样如此）。上面示例里如果 `translation` 等于 `original`，
> 三种 LRC 都会只出现一行原文，**不会**出现 `[00:12.34]xxx\n[00:12.34]xxx`。
> 这是修过的 bug：引擎对专有名词、纯外文歌词、人名常原样返回。
>
> 注意 **`lines[]` 不做这个去重**——`translation` 仍保留引擎原样返回的重复原文。
> LRC 与 `lines` 在这一点上行为不对称，用结构化数据渲染时要自己去重。
| `lines` | 结构化数组，含 `start/end`（毫秒）、`original`、`translation`，适合自定义 UI 渲染 |

**错误响应**

| HTTP | 含义 |
|---|---|
| 403 | 翻译功能未启用（管理员未开启） |
| 404 | 歌曲不存在或无可翻译歌词 |
| 400 | 翻译引擎未配置 API Key |
| 500 | 翻译引擎调用失败。body 固定为 `translation failed, see server logs for details`（`text/plain`），**刻意不含上游原文**——该端点任何登录用户都能调，回显会把引擎密钥/URL 带出去 |

---

### 2. 读取已缓存翻译（快速，无 AI 调用）

```
GET /api/lyrics/translate/{songId}?lang=zh-CN
X-ND-Authorization: Bearer <token>
```

- 缓存存在 → 200 + 同上 JSON 结构  
- 缓存不存在 → 404

**建议**：先查此接口，404 再调 POST 触发翻译。

---

### 3. 管理端：获取与更新翻译配置 (Admin Only)

**获取配置**
```
GET /api/lyrics/translation/config
X-ND-Authorization: Bearer <admin-token>
```

响应 200。脱敏由 `core/lyrics` 的 `MaskSecret` 决定，不是固定的 `******`：

| 长度 | 掩码形态 |
|---|---|
| 0 | `""`（空值原样返回） |
| ≤ 6 | `******` |
| ≤ 12 | `ab****yz`（首 2 + 尾 2） |
| 更长 | `abc****wxyz`（首 3 + 尾 4） |

```json
{
  "enabled": true,
  "engine": "gemini",
  "model": "gemini-flash-latest",
  "apiKey": "abc****wxyz",
  "secretKey": "",
  "appId": "",
  "targetLanguage": "zh-CN",
  "proxyUrl": "",
  "baseUrl": ""
}
```

> **只有 `apiKey` 与 `secretKey` 被脱敏，`appId` 是明文**（它不是密钥）。

**掩码往返契约（重要）**：把带 `****` 的值原样 PUT 回去会被理解为"沿用库中原值"，
而不是把字面量 `abc****wxyz` 存进去。所以客户端可以"读出来→改一个字段→整份提交"，
不用先剥掉掩码。

**更新配置**
```
PUT /api/lyrics/translation/config
Content-Type: application/json
X-ND-Authorization: Bearer <admin-token>
```

响应 `200 {"status":"ok"}`。

### 3.5 缓存与批量重译端点（管理员）

| 方法与路径 | 说明 |
|---|---|
| `GET /api/lyrics/translation/cache` | 列出磁盘缓存，返回 `{"items":[...],"total":N}`。**不检查 `enabled`**——关掉总开关后仍可读 |
| `DELETE /api/lyrics/translation/cache` | 清空全部缓存，返回 `{"status":"ok","cleared":N}` |
| `DELETE /api/lyrics/translation/cache/{id}?lang=` | 删单条。**不带 `lang` 会删掉该歌曲所有语言** |
| `POST /api/lyrics/translation/retranslate-all` | 启动后台批量重译。已在运行时返回 400；无缓存条目也 400 |
| `GET /api/lyrics/translation/retranslate-status` | 进度：`{running, canceling?, total, processed, success, failed, current, lastError?, startedAt}` |
| `POST /api/lyrics/translation/retranslate-cancel` | 请求停止 |

> **取消是"软"的**：`cancel` 只会把 `canceling` 置 true 并让 `running` 保持 true，
> worker 只在**两首歌之间**看标志（每首间隔 350ms），正在处理的那首做完才会停。
> 这样设计是为了防止"界面显示已停止 → 用户再点一次开始 → 两个 worker 互相污染
> 同一份计数器"（服务端用 `batchGen` 代次丢弃过期写入）。客户端轮询到
> `running == false` 才算真的结束。

**连通性测试**
```
POST /api/lyrics/translation/test
Content-Type: application/json
X-ND-Authorization: Bearer <admin-token>
```
请求体**不是**配置对象本身，而是 `{config, sampleText}`：

```json
{
  "config": { "engine": "gemini", "apiKey": "...", "model": "gemini-flash-latest" },
  "sampleText": "Hello world"
}
```

- 成功：`200 {"success":true,"result":"..."}`
- 失败：`400 {"success":false,"error":"..."}`（**是 400，不是 500**；这里会带上
  引擎返回的错误原文，所以只应被管理员用来调试）

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
  -H "X-ND-Authorization: Bearer $TOKEN"

# 触发翻译（缓存 miss 时）
curl -s -X POST https://your-navidrome/api/lyrics/translate \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"songId":"abc123","targetLang":"zh-CN"}'
```

---

*如发现 API 行为与此文档不符，请以仓库 `server/nativeapi/lyrics_translation.go` 实现为准。*
