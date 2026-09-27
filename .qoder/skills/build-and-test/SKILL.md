---
name: build-and-test
description: 构建、测试、lint 与 Docker 打包发布的标准命令（Go build tags、Ginkgo、Vitest、ESLint/Prettier、linux/amd64 镜像推送与验架构），另含 i18n 三文件校验与浏览器验界面的避坑方法。修改任何 Go 或 ui/ 代码后必须按此验证。
---

# 构建与测试

## Go

```bash
# 跑指定包测试（必须带 build tags，否则编译失败）
make test PKG=./core/jukebox        # 125 specs
make test PKG=./server/nativeapi    # 198 specs
make test PKG=./core/lyrics         # 41 specs
make test PKG=./core/scrobbler      # 104 specs

# 等价裸命令
go test -tags netgo,sqlite_fts5 ./core/jukebox ./server/nativeapi ./core/lyrics

# 全量（Go + JS + i18n）
make testall

# lint（golangci-lint，配置在 .golangci.yml）
make lint
gofmt -l core server conf scripts   # 必须无输出
```

- 测试框架：Ginkgo v2 + Gomega（suite 文件 `*_suite_test.go`）
- 例外：少数内部回归用裸 `testing`（`core/lyrics/batch_internal_test.go`、
  `core/lyrics/translation_internal_test.go`、`core/lyrics/provider_errors_test.go`、
  `core/jukebox/driver_mpd_test.go` 里的 `TestMPDDriver_*`）——它们要访问未导出字段，
  所以文件必须是 `package <pkg>` 而不是 `<pkg>_test`
- 格式化：`gofmt` / `goimports`（`make format` 会同时跑 JS prettier 和 `go mod tidy`）
- 加了新 spec 后**顺手更新本文件与 `AGENTS.md` 第 2 节的数字**，数字漂移会让下一个人
  以为测试没跑到新代码。

## 前端（ui/）

```bash
cd ui
npm run test        # Vitest（94 文件 / 843 用例）
npx vitest run src/audioplayer/VolumeControl.test.jsx   # 只跑音量测试
npx vitest run src/audioplayer/TranslateButton.test.jsx # 只跑歌词翻译测试
npm run lint        # ESLint，--max-warnings 0
npm run prettier    # 格式化 ./src
npm run check-formatting  # 只检查不写入
npx prettier --check src/audioplayer/Player.jsx         # 只查改过的文件
npm run build       # vite 生产构建，产物 ui/build/
```

前端产物被 `go:embed` 打进二进制：**改了任何 `ui/src` 都要 `npm run build` 之后
重新 `go build`**，否则跑的还是旧界面，容易误判成"我的改动没生效"。

i18n 文案是**构建期**打包进 bundle 的，改了 `*.json` 也必须重新 `npm run build`。

### 中文文案必须写三份，且要回读校验

| 文件 | 作用 |
|---|---|
| `ui/src/i18n/en.json` | 英文基线（key 的唯一来源） |
| `resources/i18n/zh-Hans.json` | 简体中文 |
| `resources/i18n/zh-Hant.json` | 繁体中文 |

界面语言由 `ui/src/i18n/provider.js` 的 `deepmerge(en, 服务端语言包)` 决定，**只写代码里的
`_:` 回退不够**——只要 `en.json` 里有同 key，界面就会显示英文。改完跑一次覆盖校验：

```bash
python3 -c "
import json
def paths(o,p=''):
    out=set()
    for k,v in o.items():
        kp=f'{p}.{k}' if p else k
        out |= paths(v,kp) if isinstance(v,dict) else {kp}
    return out
E=paths(json.load(open('ui/src/i18n/en.json',encoding='utf-8')))
for f in ['resources/i18n/zh-Hans.json','resources/i18n/zh-Hant.json']:
    print(f,'缺',len(E-paths(json.load(open(f,encoding='utf-8')))),'条')
"
```

期望输出是两个 `缺 0 条`。**别在循环里 `load()` 同一个文件再 `save()`**——
那样每次改动都会被下一次加载覆盖，脚本静默失效，文案根本没写进文件（真实踩过）。
插值语法是 polyglot 的 `%{name}`，不是 react-i18next 的 `{{name}}`。

还要查**源码引用了但语言包里没有**的 key。注意 `make test-i18n` **查不出这一类**：
它只校验 en.json 与翻译包的对齐，且**只有"翻译包多出 en 没有的 key"（extra）判失败**，
"en 有而某语言缺"（missing）只统计不报错（36 份语言包不可能同步，缺翻译是常态）。
源码引用但语言包完全没有的 key 会静默回退到代码里的 `_:` 默认值，换语言就露馅：

```bash
python3 scripts/check-i18n-keys.py     # 期望：0 个 key 缺失
```

**key 一律写字面量映射表，不要用模板字符串拼**（`t(`menu.x.${id}`)`）。
拼接出的 key 校验器抓不到，一旦语言包里命名对不上就静默回退——
表现是界面上某个文案变成语言码或英文，而中文界面完全看不出来。
校验器会对本 fork 命名空间下的模板拼 key 主动打印提示。

### 改了 `ui/src` 必须 `npm run build` + `go build`

UI 由 `go:embed` 打进二进制。只跑 `npm run build` 而不重编二进制，跑的还是旧界面——
排查时很容易误判成"我的 CSS/JS 没生效"，白查很久（真实踩过）。同理 i18n 文案也是构建期打包。

### 手机视口验收（自研页面必做）

上游 react-admin 页面在 390px 下正常，溢出基本都出自自研页面，详见
`docs/risk-notes-optimization.md` §11 与 `AGENTS.md` 第 9 节。

```js
// ego-browser 没有 setViewportSize，用 CDP 模拟手机
await page.cdp('Network.enable')
await page.cdp('Network.setBypassServiceWorker', { bypass: true })
await page.cdp('Network.setCacheDisabled', { cacheDisabled: true })
await page.cdp('Emulation.setDeviceMetricsOverride', {
  width: 390, height: 844, deviceScaleFactor: 2, mobile: true,
})
```

验收断言：

```js
const r = await page.evaluate(() => {
  const vw = document.documentElement.clientWidth
  const isOver = (el) => {
    const b = el.getBoundingClientRect()
    return b.width > 0 && b.height > 0 && b.right > vw + 1
  }
  const over = [...document.querySelectorAll('*')].filter(isOver)
  return { docScrollW: document.documentElement.scrollWidth, over: over.length }
})
// 期望 docScrollW === 390 且 over === 0
```

**必须复现用户的真实数据状态**：空表格 / 空列表 / 未登录态会隐藏一整条代码路径。
本项目真实踩过：歌词翻译页的缓存表格连续两轮都没测到，因为测试实例缓存是空的，
表格根本不渲染。

手工造数据比在空状态下断言有效得多：

| 场景 | 怎么造 |
|---|---|
| 歌词翻译页的缓存表格 | 往 `<DataFolder>/lyrics_translations/` 塞两条 `<songId>_zh-Hans.json` |
| 输出设备控制台的卡片 | `POST /api/jukebox/outputs` 建 2-3 个不同 `type` 的设备 |
| 正在播放/接管面板 | 需要真实播放会话；至少让列表有多条记录再量 |
| i18n 界面 | `localStorage.locale` **不能**写成 `JSON.stringify('en')`（带引号会回退到 `_:` 默认值）；正确写法见下 |

```js
// 切语言验证（defaultLocale 读的是原始字符串，且需要一份缓存才会真正切换）
await page.evaluate(() => {
  localStorage.setItem('locale', 'en')
  localStorage.setItem('translation', JSON.stringify({ id: 'en', data: '{}' }))
})
```

用户说"还有问题"时，能上他的实例就直接上他的实例量——他的实例可能有不同的
数据、语言、字号、构建产物，在自建等价环境里反复验证是浪费时间
（真实教训：空数据让同一个 bug 连续两轮测不出来）。

表格类元素记得两条一起写，缺一无效：

```css
.responsive-fields .MuiTableContainer-root {
  overflow-x: auto;      /* 自己滚 */
  contain: inline-size;  /* 少这行，min-content 仍等于表格宽度 */
}
```

再确认控件**可用**（只查溢出不够——控件被压扁也算"无溢出"但没法用）：

```js
const sels = [...document.querySelectorAll('.MuiSelect-select')].filter((e) => e.textContent.trim())
sels.map((e) => ({
  w: Math.round(e.getBoundingClientRect().width),
  t: e.textContent.trim().slice(0, 24),
  cut: e.scrollWidth > e.clientWidth + 1,   // true = 文案被截断了
}))
// 期望：w > 200 且 cut === false
```

**弹窗（Dialog）内容是 portal，也要一起量**——页面上量不到不等于弹窗里没溢出。
定位"谁在撑宽"用离屏探针直接量 min-content，不要靠猜：

```js
const probe = document.createElement('div')
probe.style.cssText = 'position:absolute;left:-99999px;width:min-content'
document.body.appendChild(probe)
probe.appendChild(el.cloneNode(true))
probe.firstChild.getBoundingClientRect().width  // 该子树的 min-content
```

### 浏览器里验界面时先绕开缓存

Service Worker + HTTP 缓存会让你以为改动没生效，验证前先确认加载的是新 bundle：

```
http://localhost:4533/app/?cb=<时间戳>#/jukebox-outputs
```

还不对就注销 Service Worker 并清空 caches 再刷新。断言 DOM 时注意"隐藏但已挂载"的
popover/dialog 也会出现在 `querySelectorAll` 结果里，要按尺寸或可见性过滤。

## Docker 打包与推送（发布 linux/amd64 镜像）

```bash
# 本地起 colima (macOS 环境)
colima start

# 打包 linux/amd64 镜像并推送到 Docker Hub (huhan333/navidrome2all:latest)
docker buildx build --platform linux/amd64 \
  --build-arg GIT_SHA=$(git rev-parse --short HEAD) \
  --build-arg GIT_TAG=v0.55.0-fork \
  --target final \
  -t huhan333/navidrome2all:latest \
  -t huhan333/navidrome2all:$(git rev-parse --short HEAD) \
  --push .
```

- 仓库**没有 CI**，镜像只能本地构建后 `--push`；`GIT_SHA`/`GIT_TAG` 是唯一写进二进制的
  版本信息（`docker logs` 首屏那行 banner 就能核对：`Version: 0.55.0-fork (bfdd120f)`）。
  本仓库历史被重置过、没有 tag，所以 `GIT_TAG` 只能手写。
- 推送的是**工作区当前状态**，不限于已提交内容。发版前先 `git status --short` 确认
  要不要先把改动提交上。
- macOS 是 arm64，**必须显式 `--platform linux/amd64`**，否则 NAS 拉下来会 `exec format error`。

### 推完必须验架构（macOS 本地验不出来）

```bash
# 从 registry 反查 tag 的真实平台
docker buildx imagetools inspect huhan333/navidrome2all:latest | grep -E "Platform|Digest"
```

期望看到 `Platform: linux/amd64`。`unknown/unknown` 那条是 buildx 默认附带的
provenance attestation，正常现象，不用管。

### 冒烟：起一次容器确认镜像能用

```bash
docker run -d --name nd-smoke -p 14607:4533 huhan333/navidrome2all:latest
sleep 25                       # arm64 上跑 amd64 镜像靠模拟器，起得慢
curl -s -o /dev/null -w '%{http_code}\n' http://localhost:14607/ping   # 期望 200
curl -s http://localhost:14607/app/ | grep -c jukebox                   # 期望 >0，确认 UI 已内嵌
docker logs nd-smoke 2>&1 | grep -i version
docker rm -f nd-smoke
```

## 起实例与调 API

- 同一时间**只保留一台**实例、一个 URL（多开会让"到底哪个在响"变成玄学）。
  `nohup` 起的进程可能被上层工具回收；验证前先 `curl -s -o /dev/null -w '%{http_code}' <url>/app`。
- native API 鉴权头是 `X-ND-Authorization: Bearer <token>`（`ui/src/dataProvider/httpClient.js`），
  **cookie 不管用**。token 来自 `POST /auth/login`（JSON body，取响应 `token` 字段）。
- 登录态属于用户：**不要自己注册账号，也不要从数据库/日志/会话记录里翻密码**。
  需要真人登录时把实例和 URL 交给用户。
- `BaseUrl` 不配的话，下发给音箱的流地址是 `localhost`，日志会出现
  `Jukebox stream URL points at the loopback interface`——这是"选了设备但没声音"的头号原因。

## 验收清单（改完代码后）

1. 相关 Go 包测试全绿（带 tags）
2. `cd ui && npm run test && npm run lint && npm run check-formatting` 通过
3. `gofmt -l` / prettier 无 diff
4. 如涉及 UI 构建产物：`cd ui && npm run build` + `go build` 通过
5. 涉及音量：确认所有改音量的入口都走 `dispatch(setVolume(...))`，
   没有新的地方直接写 `audioInstance.volume`（会被 store 权威 effect 夺回）
6. 涉及界面：真实登录 + 移动视口点一遍（详见 AGENTS.md 第 9 节）
