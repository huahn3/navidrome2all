# 优化批次风险与回滚手册（2026-09-27）

> 这份文档记录 2026-09-27 这一轮"体检 + 优化"里**所有可能影响现有功能**的改动。
> 每条都给出：改了什么 / 为什么 / 可能坏在哪 / 怎么验证 / **怎么回滚**。
> 出问题先看这里，再决定是 `git revert` 单个提交还是改配置。
>
> 涉及凭据泄漏的 P0 部分**不能靠回滚解决**，必须先轮换凭据（见 §0）。

---

## 0. P0：真实凭据曾被提交到公开仓库（最高优先级，与"优化"无关）

### 事实

`scripts/xiaomi_test_server.go` 曾以字面量形式保存并提交（commit `ba34db51`，在 `origin/master` 上）：

- 小米账号 ID 与 `passToken`（`V1:...`，等同于小米云账号会话凭据）
- 一个 Navidrome JWT（当时有效期到 2026-09-28）
- 站内明文账号密码（`123` / `123123`）
- 内网地址 `192.168.31.246:14534`

仓库是 **public**（`private: false`），因此这些值在 GitHub 上任何人可见。

### 已做的（代码侧）

- 凭据全部改为环境变量读取，缺失即报错退出（`loadSecrets()`）
- 新增 `scripts/.env.example` 作为模板；`/scripts/.env` 已进 `.gitignore`
- 硬编码的内网 IP 从启动横幅里去掉
- 全仓扫描确认工作区已无这些字面量

### 仍需你手工完成（AI 不做：涉及账号与强推）

1. **先轮换，再谈历史**（顺序不能反）：
   - 改掉 Navidrome 那个账号的密码；
   - 小米账号退出全部会话 / 重新登录，让旧 `passToken` 失效；
   - 重新生成 JWT（`POST /auth/login` 取 `.token`）。
2. **清历史**（会改写 `master`，必须你自己执行）：

   ```bash
   # 方式 A：filter-repo（推荐，会彻底移除该文件的所有历史版本）
   brew install git-filter-repo
   git filter-repo --path scripts/xiaomi_test_server.go --invert-paths
   git push --force-with-lease origin master

   # 方式 B：如果不想动历史（历史里仍有凭据，但工作区已干净）
   #   至少保证 §0.1 的轮换已完成——凭据一旦作废，历史里的旧值就没有价值
   ```

   > 只做方式 B 也可以接受，前提是凭据已全部轮换失效。
   > 注意：本仓库历史已重置过一次，`filter-repo` 之后 `upstream` 的 diff 参照点不受影响
   > （基线 commit `ee6dd1bc` 是按路径比对的，不依赖共享祖先）。

3. 如果用方式 A，`docs/fork-and-upstream.md` 里"单条初始提交"的描述需要同步更新。

### 回滚

代码侧回滚无意义也不该回滚（回滚等于把凭据放回去）。工具本身不可用时，
临时用 `git stash`/切回旧版本跑，**但不要把旧文件再提交**。

---

## 1. `GET /api/jukebox/outputs` 改为脱敏返回（影响管理页面）

### 改动

- `server/nativeapi/jukebox_outputs.go`：`outputToDTO` 对 `Password` / `Token` / `PassToken`
  返回掩码（`********`），并新增常量 `secretMask`。
- `outputFromDTO`：收到的值等于掩码时**保留原有密文**，而不是把掩码写进库。
- 响应加 `Cache-Control: no-store`。
- 前端 `ui/src/jukebox/*` 表单：拿到掩码时显示"已保存（留空则不修改）"，留空即不覆盖。

### 为什么

原先 `GET` 把小米音箱的 `passToken`、DLNA/MPD 的 `Password` **原样**回给浏览器，
虽然接口是 admin-only，但 devtools / 抓包 / 代理日志里全是明文；
而同仓库的歌词配置接口早就做了脱敏（`GetMaskedConfig`），两边不一致。

### 可能坏在哪

- **表单回填**：如果前端把掩码原样提交回 `PUT`，旧实现会把密文覆盖成 `********`
  → 设备控制失效（小米音箱云控、MPD 认证失败）。已通过"掩码 = 保留原值"堵住。
- **编辑页显示**：掩码会让用户看不到当前真实口令（预期行为，与歌词配置一致）。

### 验证

```bash
# 1) 创建一个带 Password 的输出，GET 回来应是 ********
curl -s -H "X-ND-Authorization: Bearer $JWT" $BASE/api/jukebox/outputs | jq '.[].password'
# 2) 不改密码直接 PUT 回去，再 GET，掩码应仍在且设备仍能控制
```

### 回滚

`git revert` 掉"outputs 脱敏"那个提交即可恢复原行为（不影响其它功能）。

---

## 2. DLNA Seek 改为"同步发送 + 后台验证"（影响出声链路，最需要盯的一项）

### 改动

`core/jukebox/driver_dlna.go`：

- `Seek()` 现在**只同步发一次 SOAP Seek**（一次 HTTP 往返，约 10ms）就返回；
- "渲染器到底跳没跳"的确认与最多 3 次重试移到**后台 goroutine**（`verifySeek`）；
- 新增 `gen`（generation）计数器：`Play` / `Stop` / `Seek` 都会 `+1`，
  后台验证每一步先比对 generation，不一致立刻放弃；
- 后台验证**睡眠期间不持有 `d.mu`**，所以音量/暂停可以穿插执行。

### 为什么

原来最坏情况（`waitPositionAdvancing` 6s + 3 次重试 × 1.2s + HTTP 往返 ≈ **10s+**）
是在 `DeviceManager` 的**全局互斥锁内**同步等待的。这期间所有设备命令排队；
`POST /api/playback/sessions/{id}/takeover` 内部会同步 `Control("pause")`，
所以接管接口本身也会被拖到 10s。改完后锁占用降到毫秒级。

### 可能坏在哪

- **切歌后被拽回旧位置**：这是异步化最真实的风险。已用 generation 机制挡住
  （`Play`/`Stop` 一调用，等待中的验证立刻作废），并新增测试
  `abandons a pending seek verification when the transport moves on` 守住。
- **重复 Seek 次数**：总数仍是 `dlnaSeekAttempts`（1 次同步 + 2 次后台重试 = 3），
  但到达时间推迟约 1~4 秒。对"接受 Seek 却忽略它"的渲染器，
  最终位置仍然正确，只是纠正得晚一点。
- **日志时序变化**：`DLNA renderer ignored the seek` 与那条 Warn 现在出现在
  Seek 请求返回之后；抓日志排查时别以为"请求失败了"。
- 测试从"同步计数"改成 `Eventually`/`Consistently` 断言（契约变了，断言必须跟着变）。

### 验证

```bash
go test -count=1 -tags=netgo,sqlite_fts5 ./core/jukebox
```
真机验证清单：
1. 拖动进度条 → 音箱应落到目标位置（顽固设备可能晚 1~4 秒纠正）；
2. 拖动后**立刻**切歌 → 音箱必须播新歌，不能跳回旧位置（重点测这条）；
3. 拖动过程中把音量条来回拖 → 应立即生效（以前会被 Seek 阻塞）；
4. 另一个客户端点"接管" → 接口应毫秒级返回（以前可能卡 10s）。

### 回滚

`git revert` 该提交。若只想临时止血：把 `verifySeek` 里的 `go` 去掉并把
`dlnaSeekStartTimeout`/`dlnaSeekConfirmDelay` 调小，能缩短阻塞但会降低顽固设备的纠正能力。


---

## 3. `play_tracker.ReportPlayback` 按状态拆分（影响播放上报与会话列表）

### 改动

`core/scrobbler/play_tracker.go`：`ReportPlayback` 的 4 个 `case` 各抽成一个私有方法
（`reportStarting` / `reportPlayingOrPaused` / `reportStopped` / …），主函数只留分派与
共享前置（时间戳、用户、客户端）。**逻辑逐行搬迁，未改判定条件。**

### 为什么

`gocyclo` 复杂度 40（阈值 30），是本文件里最容易改坏的地方，先拆开才敢继续加功能。

### 可能坏在哪

- **⚠️ 实际踩到过一次（已修）**：拆分时把分支里的 `return nil`（原本是**终止整个
  `ReportPlayback`**）改成了 `return false, nil`（只终止该分支），导致被忽略的乱序
  `starting` 报告仍然会走到函数尾部的 NowPlaying 广播。
  是 `play_tracker_test.go:1010` 这个用例挡下来的（`Consistently(...).Should(BeFalse())`）。
  修法：引入 `errOutOfOrderReport` 哨兵错误，调用处识别它并整体返回 `nil`，
  语义与拆分前完全一致。
  **教训：机械搬迁必须核对控制流（提前返回的作用域），不能只比对语句。**
- 搬迁时漏搬字段或改变 `sessionsMu` 的加锁范围 → 会话重复/丢失。

### 验证

```bash
go test -count=1 -tags=netgo,sqlite_fts5 ./core/scrobbler   # 104 specs，必须全绿
```


### 回滚

`git revert` 该提交即可，行为回到单体函数。

---

## 4. 前端分包（**已回滚，勿再尝试**）与歌词页懒加载

### 4.1 事故记录：`manualChunks` 导致页面白屏

最初给 `ui/vite.config.js` 加了 `manualChunks`，手工把
`react` / `react-admin` / `vendor` 分组。**结果整个页面打不开**：

```
Uncaught TypeError: Cannot set properties of undefined (setting 'AsyncMode')
    at react-is.production.min.js:12
```

原因：手工分组把 React 生态拆散，`react-is` 与 `react` 落到不同 chunk，
页面里出现多份 React 内部对象，模块初始化阶段就抛错，
表现为 index.html 加载完成但**整页空白**（所有 JS 资源都是 200，容易误判成服务端问题）。

**处置**：`manualChunks` 已整段删除，回到 Rollup 默认行为
（单包 `index-*.js`，2.78MB）。懒加载 chunk `LyricsTranslation-*.js`（25KB）保留，
它是 `React.lazy` 自动产生的，安全。

**教训**：不要手工给 React / react-dom / react-is / scheduler 分组。
真要优化首屏，用 `React.lazy` 做路由级拆分（本仓库已做歌词翻译页），
或用 `import()` 拆单个大组件，别碰 `manualChunks`。

### 4.2 保留的改动：歌词翻译控制台懒加载

- `ui/src/routes.jsx` 用 `React.lazy` 加载歌词翻译页；
- 新增 `ui/src/common/RouteFallback.jsx` 作为 Suspense 兜底
  （单独文件是因为 `react-refresh/only-export-components` 不允许同文件导出非组件）。

风险很低：只影响该页首次进入时多一帧 loading。
验证：登录后打开 `/lyrics-translation` 正常渲染。

### 验证（每次改前端后都要做）

```bash
cd ui && npm run build && cd .. && go build -tags=netgo,sqlite_fts5 -o bin/navidrome .
# 起一个临时实例，只看登录页能否渲染（不需要账号）
curl -s -o /dev/null -w "%{http_code}\n" http://localhost:<port>/ping
```
然后浏览器打开 `/app/`：**必须看到登录框**（React 能启动）。
只跑单元测试是发现不了这类问题的——它只在真实浏览器里炸。

### 回滚

`git revert` 掉 `vite.config.js` 与 `routes.jsx` 的改动即可。


---

## 5. NowPlayingPanel 改为事件驱动（影响"正在播放"列表实时性）

### 改动

`ui/src/layout/NowPlayingPanel.jsx`：

- 服务端每次 `reportPlayback` 都会广播 `nowPlayingCount`，面板已在监听它
  （`state.activity.nowPlayingLastUpdate`），因此**打开面板时的 10 秒轮询被去掉**；
- 新增监听 `state.player.lastHandoff`：别的设备接管后列表立刻刷新；
- 保留两个兜底：SSE 不可用（`config.devActivityPanel === false`）时仍按 10 秒轮询，
  以及**任何情况下**都保留 60 秒慢轮询。

### 可能坏在哪

- 若服务端把 `DevActivityPanel` 关掉且前端拿到的是旧值 → 列表变慢。
  已用 `config.devActivityPanel` 做运行时判断，并保留 60s 兜底，最坏退化为 60s 刷新。
- `state.player` 在某些测试/嵌入场景下可能不存在 → 选择器改为可选链
  `state.player?.lastHandoff`。

### 验证

`npx vitest run src/layout/NowPlayingPanel.test.jsx`（15 用例）；
浏览器里开两个标签页：A 播放 → B 的徽标数应在 1s 内变化。

### 回滚

`git revert`；或把 10 秒轮询无条件加回。

---

## 6. 音量不变量抽成纯函数（低风险，但属于核心链路）

`ui/src/audioplayer/Player.jsx` 里三处换算改为调用 `ui/src/audioplayer/volume.js`
（`perceivedToElementVolume` / `devicePercentToPerceived` / `perceivedToDevicePercent`
+ `VOLUME_EPSILON`），**计算结果与原来逐位相同**，只是搬到了可单测的地方，
并新增 7 个用例（`volume.test.js`），其中包含"设备回报 0 必须被忽略"这条历史 bug 的守卫。

回滚：`git revert`；若只想快速止血，把 `volume.js` 里的实现改回内联逻辑即可。


---

## 7. 低风险改动（一般不会坏，出问题直接 revert 即可）

| 改动 | 文件 | 说明 |
|---|---|---|
| 删除死代码 `currentStamp` | `core/jukebox/xiaomi_miio.go` | lint 判定未使用，确认无调用 |
| `err.(net.Error)` → `errors.As` | `core/jukebox/discover_dlna.go` | 行为等价，兼容性更好 |
| 给 lint 误报加 `//nolint` + 原因 | `xiaomi_cloud.go`、`scripts/*` | `bodyclose`/`misspell`/`gosec` 均为协议或结构决定的误报 |
| 抽取会话转换函数 | `server/nativeapi/playback_sessions.go` | 消除 3 处重复，并补齐 takeover 响应缺失的 `artistId/albumId/isCurrentSession` |
| `.gitignore` 危险模式加前导斜杠 | `.gitignore` | 见 §7 验证 |
| README 去掉上游构建徽章 / 截图改指本仓库 | `README.md` | 纯文档与展示 |
| 文档数字校正（189 specs / 91 文件 793 用例） | `AGENTS.md`、`.claude/skills/build-and-test/SKILL.md` | 纯文档 |

---

## 8. `.gitignore` 改动的验证方法（务必跑）

```bash
# 危险模式应不再命中任意层级
for p in core/foo/music/x.go docs/dist/a.md server/var/y.go a/b/wiki/c.md sub/navidrome.toml; do
  git check-ignore -q "$p" && echo "危险: $p 被忽略" || echo "ok: $p"
done
# 上游文件没有被误伤（必须为空）
git diff --diff-filter=D --name-only ee6dd1bc HEAD
```

> 残留项：`/navidrome-*` 仍会忽略**仓库根目录**下名为 `navidrome-xxx` 的目录
> （原本就是用来忽略根目录的跨平台构建产物），不影响任意层级的源码目录。

---

## 9. 本次改动的完整清单与验证结果（第一批）

### 按风险从高到低

| # | 改动 | 文件 | 风险 |
|---|---|---|---|
| 1 | DLNA Seek 异步化（出声链路 + 锁） | `core/jukebox/driver_dlna.go` + 测试 | **高**（§2） |
| 2 | outputs 凭据脱敏（前后端契约变化） | `server/nativeapi/jukebox_outputs.go`、`ui/src/jukebox/JukeboxOutputForm.jsx`、`ui/src/i18n/en.json` | **中高**（§1） |
| 3 | `ReportPlayback` 拆分 | `core/scrobbler/play_tracker.go` | **中**（§3，测试已守住） |
| 4 | 正在播放面板改事件驱动 | `ui/src/layout/NowPlayingPanel.jsx` | **中低**（§5） |
| 5 | 歌词翻译页懒加载（**分包已回滚**） | `ui/src/routes.jsx`、`ui/src/common/RouteFallback.jsx` | 低（§4） |
| 6 | 会话转换函数抽取 | `server/nativeapi/playback_sessions.go` | 低（纯重构，净减 34 行） |
| 7 | 音量换算抽成纯函数 | `ui/src/audioplayer/volume.js`、`Player.jsx` | 低（§6，结果逐位相同） |
| 8 | 脚本凭据改环境变量 | `scripts/xiaomi_test_server.go`、`scripts/.env.example` | 低（仅本地排障工具） |
| 9 | lint 归零（删死代码/`errors.As`/`nolint` 注明） | `core/jukebox/*` | 低 |
| 10 | `.gitignore` 加前导斜杠、README、文档数字 | 仓库元数据 | 低（§8 验证） |

### 终验结果（本机实跑）

- `gofmt -l core server conf model cmd` → 无输出
- `golangci-lint run`（全仓）→ **0 issues**（改动前 19）
- `go test ./core/... ./server/...` → 全绿
- `npm run check-formatting` / `npm run lint`（`--max-warnings 0`）→ 全绿
- `npm run test` → **92 文件 / 800 用例全绿**（连续两次）
- `npm run build` → 成功；`go build` → 成功
- 端到端冒烟（新二进制 + 临时实例，已销毁）：
  - 建带密码的输出 → 响应即掩码；
  - `GET /api/jukebox/outputs` → 掩码 + `Cache-Control: no-store`；
  - **把掩码原样 PUT 回去 → 直接查 SQLite 确认库里密码仍是原值**（关键验证）；
  - handoff 链路：reportPlayback → sessions 列表 → takeover 均正常，
    takeover 响应字段与列表口径一致（`artistId/albumId` 已补齐）。

### 没做的（有意留下）

- 凭据轮换与 git 历史清理：涉及账号和强推，必须你本人执行（§0）。
- 真正的加密存储（设备口令落库加密）：本次只做了 API 侧脱敏，
  库内仍是明文（与 TOML 配置文件同级风险），已用 `//nolint:gosec` 标注并在此登记。
- 拆分 `LyricsTranslation.jsx`（1460 行）、`XiaomiAuthBlock.jsx`（821 行）：纯重构，建议单独批次。
- react-admin Resource 级路由懒加载：需要浏览器实测，不与本批混做。


## 10. 输出设备控制台重做 + 菜单移位（界面改动，行为有一处变化）

### 改动

1. `core/jukebox/manager.go`：`Select()` 幂等——已经是当前输出时直接返回，
   不再重建驱动对象（避免丢掉设备会话，小爱音箱要重做握手），
   也不再重复打 `Jukebox output switched` 日志。
2. 菜单：`ui/src/layout/JukeboxOutputsMenu.jsx` 新增，
   在 `AppBar.jsx` 里插到 **个性化 → 输出设备 → 歌词翻译** 之间
   （原来它在 react-admin 资源列表末尾）。
3. 页面重做为自定义控制台（**不再是 react-admin 标准界面**）：
   - 新增 `ui/src/jukebox/JukeboxOutputs.jsx`（卡片网格 + 当前输出区 + 弹窗编辑）
   - 新增 `ui/src/jukebox/OutputEditorDialog.jsx`（react-final-form 自定义排版）
   - 新增 `ui/src/jukebox/outputConstants.js`、`ui/src/common/RouteFallback.jsx`
   - `ui/src/routes.jsx` 注册 `/jukebox-outputs`（懒加载）
   - `ui/src/App.jsx` 移除 `<Resource name="jukeboxOutput">`
   - 删除旧的标准页：`JukeboxOutputList/Create/Edit/Form.jsx`、`jukebox/index.js`
     （连带 `wrapperDataProvider` 里已无用的 `jukeboxOutput` 映射与对应测试）

### 行为变化（需要注意）

- 旧路径 `/jukebox-outputs/create` 与 `/jukebox-outputs/:id` 不再存在
  （react-admin Resource 已注销），现在统一是 `/jukebox-outputs` + 页内弹窗。
  旧书签会 404。
- 配置文件里定义的设备（`source: "config"`）在卡片上标为只读：不给删除按钮。

### 验证（浏览器实测，2026-09-27）

- 菜单顺序：`Personal → Output Devices → Lyrics Translation` ✓
- 卡片渲染、类型徽标、"设为输出"（服务端 `selected` 同步变化）✓
- 编辑弹窗字段回填正确，ID 只读、token 显示为 `********` ✓
- **保存往返**：改名字保存后直接查 SQLite，原 token（`aaaaaa…`）与
  MPD 密码（`secret@read`）**未被掩码覆盖** ✓

### 过程中踩到并修掉的两个真 bug（都只有浏览器能发现）

1. **表单字段全空**：`Input` 最初写成自引用 `useField` + 展开到 MUI `TextField`，
   `values` 里数据齐全但 DOM 输入框是空的。改为官方 `Field` + `input` prop 写法后正常。
2. **Service Worker / HTTP 缓存导致测到旧代码**：连续三次"改了没效果"，
   实际是浏览器在用旧的 `JukeboxOutputs-*.js` chunk。
   验证前端改动必须 `page.goto(".../?cb=" + Date.now())` 绕缓存，
   必要时先 `navigator.serviceWorker.unregister()` + 清 `caches`。

### 10.1 中文化（同一批追加）

**关键机制**（排查花了一点时间，值得记住）：
UI 的中文**不来自** `ui/src/i18n/en.json`，而来自**服务端** `resources/i18n/zh-Hans.json`
（`ui/src/i18n/provider.js` 里 `deepmerge(en, 服务端语言包)`）。
所以只把中文写进代码的 `translate(key, { _: '中文' })` 回退是**不够**的——
只要 `en.json` 里有这个 key，中文用户就会看到英文。

因此新增界面文案必须同时写：
1. `ui/src/i18n/en.json`（英文基线）
2. `resources/i18n/zh-Hans.json`（简体）
3. `resources/i18n/zh-Hant.json`（繁体）

本次补了 35 条 jukebox 文案 + 9 条 upstream 遗留的 `quickConnect` 文案
（之前所有语言包都没有它，打开该功能必然是英文）。
校验脚本（改动后必跑，应输出 0）：

```bash
python3 - <<'EOF'
import json
en=json.load(open('ui/src/i18n/en.json',encoding='utf-8'))
def paths(o,p=''):
    out=set()
    for k,v in o.items():
        kp=f"{p}.{k}" if p else k
        out |= paths(v,kp) if isinstance(v,dict) else {kp}
    return out
E=paths(en)
for f in ['resources/i18n/zh-Hans.json','resources/i18n/zh-Hant.json']:
    Z=paths(json.load(open(f,encoding='utf-8')))
    print(f, '缺', len(E-Z), '条')
EOF
```

**验证过的语言切换行为**（上游既有逻辑，非本次改动）：
新装的浏览器在没手动选过语言时，`localStorage.translation` 为空 →
`defaultLocale()` 返回 `en`，所以第一次打开是英文；
用界面上的语言下拉选过一次之后才会切中文。想在测试里验证中文，
需要模拟切换后的缓存：`localStorage.translation = {id:'zh-Hant', data:'<服务端返回的JSON字符串>'}`。

### 10.2 第二批修订（同一界面）

1. **同步 bug（真 bug，用户报）**：页面"设为输出"只调了 `POST /api/jukebox/select`
   并更新本地 state，**没有 dispatch `setOutputDevice`**，而播放 dock 的选择器读的是
   `state.player.outputDevice` → 页面改了、dock 不动。
   修法：`select()` 成功后 dispatch；页面加载时也用服务端 `selected` 反向同步一次
   （覆盖"重启后服务端回落到 browser"和"别的设备改过"两种情况）。
2. **"新增输出设备"移到 header 常驻**（手机端 header 放不下则隐藏，改用右下角 FAB）。
3. **DLNA 地址显示脱敏**：`http://192.168.31.142:9999/e522dfd8-….xml` 在卡片上只显示
   `192.168.31.142:9999`，下面一行提示"设备描述 URL"，完整值放 tooltip。
   **只在显示层脱敏，API 仍返回完整地址**——否则编辑表单会存进被截断的值。
4. **MPD 扫描**（`core/jukebox/discover_mpd.go` + `GET /api/jukebox/discover/mpd`）：
   MPD 没有广播机制，只能探测本机私有网段的 6600 端口。实现要点：
   只扫私网且只扫 /24 及更小（否则一次要拨 6 万个端口）；
   并发上限 32、单次拨号 400ms、整体受 timeout（1-15s，默认 4s）约束；
   读到 `OK MPD <version>` 问候才算命中，并顺带探测是否要密码。
   管理端是 admin-only、按需触发，不做后台任务。
5. **MPD 连接测试**（`POST /api/jukebox/verify/mpd`）：保存前真实登录一次。
   空密码也会发 `password ""`——这样"服务器要密码但没填"会明确报
   `the server requires a password`，而不是假装连上了。
   （这条是写测试时发现的设计缺陷：原实现空密码直接返回成功。）
6. **MPD 密码怎么填的提示**：说明 `mpd.conf` 里的
   `password "abc123@read,add,control,admin"` 要整行原样填。

实测（本地实例 + 真实 NAS）：

```
扫描 → [{"address":"192.168.31.88:6600","version":"0.21.11","needsPassword":true}]
空密码   → mpd: the server requires a password
错密码   → mpd: password rejected
非 MPD   → mpd: 127.0.0.1:4606 did not answer with an MPD greeting
不可达   → mpd: cannot reach 127.0.0.1:6601: connection refused
未登录   → 401
```

### 10.3 第三批：新增/编辑弹窗改成三步向导

原来的弹窗把所有字段一次性铺开（类型下拉 + 基本 + 连接 + 类型专属 + 扫码区块），
又长又难扫，用户反馈"一眼看不懂"。现在：

1. **步骤 1 = 选类型**：三张大卡片（小爱音箱 / MPD / DLNA），带图标、配色、一句话说明，
   选中打勾并直接进入下一步——类型选择从"一个下拉框"变成整屏主角。
2. **步骤 2 = 连接**：只显示该类型需要的字段（小爱只有扫码区块 + IP；MPD 有扫描 + 密码提示；
   DLNA 只有扫描 + 地址），无滚动。
3. **步骤 3 = 高级（可选）**：token/did/model/账号/口令/路径映射，默认折叠到最后，
   并明确说明"一般不用填"。
4. **ID 不再要求用户想**：留空时按名称生成（`Living Room MPD` → `living-room-mpd`，
   纯中文名 → `xiaomi-54hp` 这种 `<类型>-<短哈希>`），输入框下方实时显示
   "将自动生成为：xxx（可自行修改）"，保存时后端才落库。
5. 编辑已有设备直接从第 2 步开始，顶部显示类型徽标 + "更换"按钮。

实测：三步都能走通，中文名/英文名/纯中文名的 ID 生成都正确，保存后落库
（`卧室小爱音箱` → `xiaomi-54hp`）。

**踩到的三个坑（都靠实测才发现，值得记住）**

1. `useFormState` 在本仓库的 react-final-form 6.5.9 里**只接受 `{onChange, subscription}`**，
   传字段名或选择器都会返回**整个 form state**（不是字段值）→ 自动填充的逻辑拿到的
   是对象，`slugify` 全部退化成空串。正确写法是 `useField('name').value`。
   （源码：`node_modules/react-final-form/dist/react-final-form.cjs.js:330`）
2. 在 render 期间调用 `form.change()` 是渲染副作用，React 会丢弃；
   而且改成 `useEffect` 后又因为 `useRef(!isCreate)` 初值写反而整段被守卫跳过。
   最终方案干脆不做实时写入：**只在提示里预览、保存时生成**，零副作用。
3. polyglot 的插值语法是 `%{name}`，**不是** react-i18next 的 `{{name}}`；
   写错会原样显示 `{{id}}`。

### 回滚

```bash
git checkout -- ui/src/jukebox ui/src/App.jsx ui/src/routes.jsx ui/src/layout/AppBar.jsx \
                ui/src/audioplayer/jukebox.js core/jukebox/discover_mpd*.go
git checkout core/jukebox/manager.go resources/i18n
```
（注意：`Select` 幂等是纯服务端优化，可单独保留。）


- **没有**改 `Jukebox` / `playback` 两套并存的架构（`docs/jukebox.md` 第 5 节已说明取舍）。
- **没有**动 `server/subsonic/*` 的对外语义与 `db/migrations/*`。
- **没有**给 `reportPlayback` 心跳间隔（60s）做改动——经核算 TTL 每次上报都会按
  "剩余曲长 + 5s" 重算，60s 是安全的，改小只会增加请求量。
- **没有**拆分 `LyricsTranslation.jsx`（1460 行）与 `XiaomiAuthBlock.jsx`（821 行）：
  它们有测试但体量大，拆分属于纯重构、收益主要是可读性，建议单独开一个批次做，
  不要和这批安全性/稳定性改动混在一起。

## 11. 手机端适配（横向溢出）+ 小米音箱批量添加

### 11.1 问题现象

用户反馈"手机端很多按钮和文字被隐藏"，并给了 5 张截图。看起来像响应式缺失，
但上游页面（专辑、播放列表、设置）在 390px 下量出来完全正常，**只有本 fork 的自研页面溢出**。

### 11.2 根因（不是"没写媒体查询"，而是内在宽度在传播）

Navidrome 的 `Layout` 是 `min-width: fit-content`。fit-content 的取值是
`min(max-content, max(min-content, 可用宽度))`，所以只要页面上**任何后代**的
min-content 超过视口，整页就被撑宽，右侧内容落到屏幕外——不是被 `overflow:hidden`
藏起来，而是根本够不着，看起来就像"被隐藏"。

歌词翻译页在 390px 下把整页撑到 **561px**，元凶是翻译引擎那个下拉框：
`Google Gemini (支持 gemini-flash-latest / gemini-flash-lite-latest)` 这类长选项文案。

**关键坑：给输入控件加 `min-width: 0` 没用。** 它只影响"收缩下限"，
不影响 min-content 的**贡献值**。实测有效的只有这一种写法：

```css
.MuiSelect-select {
  width: 0;          /* 内在宽度归零 */
  min-width: 100%;   /* 父级有确定宽度后再撑满 */
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
```

验证方式（离屏探针，直接量 min-content，不用猜）：

```js
const probe = document.createElement('div')
probe.style.cssText = 'position:absolute;left:-99999px;width:min-content'
document.body.appendChild(probe)
probe.appendChild(el.cloneNode(true))
probe.firstChild.getBoundingClientRect().width  // 这就是该子树的 min-content
```

### 11.3 第二个坑：JSS 的 `& .MuiXxx` 嵌套选择器在本项目不生成规则

本来想把上面的规则写进 `makeStyles` 的 `'& .MuiSelect-select'`。**它不会出现在样式表里**
（构建产物与运行时 `document.styleSheets` 都查不到，伪类 `&:hover` 才有效）。
所以这类"穿透 MUI 内部类名"的规则必须放**全局样式表** `ui/src/index.css`，
并用作用域类 `.responsive-fields` 限定，页面容器加 `className="responsive-fields"` 即可。

### 11.4 第三个坑：折叠内容仍参与宽度计算

翻译引擎配置区包在 `<Collapse in={!!cfg.enabled}>` 里，开关关闭时内容不可见，
却仍把整页撑到 561px。`overflow: hidden`、`min-width: 0` 全都试过，**只有
`unmountOnExit` 有效**（或对 `.MuiCollapse-hidden` 直接 `display: none`，但那会动到动画）。
现在未启用的配置区根本不渲染，顺带省了渲染开销。

### 11.5 其余三处修复

| 位置 | 问题 | 修法 |
|---|---|---|
| `XiaomiAuthBlock` | 三个登录方式的 Tab 在 390px 下排不下，第三个跑到屏幕外 | MUI 官方做法 `variant="scrollable"`，可滑动 |
| `NowPlayingPanel` | `width: 26em`（≈416px）比手机宽；`maxHeight` 按写死的 120px/条算，标题换行就把最后一条切一半 | 叠 `maxWidth: calc(100vw - 16px)`；`maxHeight` 改 `min(按条数算的值, 60vh)` 并保留滚动 |
| 歌词翻译页缓存表格 | 5 列表格在窄屏把整页撑宽 | 表格容器 `overflow-x: auto` + 表格 `min-width: 520px`，在容器内滚动 |

### 11.6 小米音箱支持多选批量添加

原来一次登录检索到 44 台设备，只能逐台点「选用此音箱」→ 保存 → 再新建，
多台设备等于要重复几十次。现在：

- 每台音箱左侧有勾选框，顶部有「全选 / 已选 N 台 / 添加所选 N 台为输出设备」
- 没有局域网 IP 的设备**不可勾选**（驱动靠 IP 控制，没 IP 建了也用不了），按钮置灰
- 批量创建前先 `GET /api/jukebox/outputs` 收集已存在的 ID，再用 `uniqueOutputId`
  追加 `-2/-3` 去重——服务端对重复 ID 直接 400（`an output with this id already exists`）
- 每台音箱建成**独立输出**（各自的 token/did/IP/型号），与"一个账号一个设备"的旧思路不同
- 结果用 `Alert` 汇总：成功 N 台、失败列出设备名与原因；**部分失败时不关弹窗**，
  全部成功才关闭并刷新列表
- 「选用此音箱」（填入下方表单）保留，两种用法互不干扰

### 11.7 验证与回滚

- 单测：`XiaomiAuthBlock.test.jsx` 新增 3 例（批量成功、部分失败、无 IP 不可选）
- 手工：320 / 390 / 1280 三档视口下逐页量
  `document.documentElement.scrollWidth === clientWidth`、弹窗内无 `right > viewportWidth`
  的元素，并**确认下拉框/输入框拿到完整宽度、文案未被截断**（本次三档均为
  `docScrollW === vw`、`over === 0`、Select `cut === false`）
- 回滚：

```bash
git checkout -- ui/src/index.css ui/src/lyricsTranslation ui/src/jukebox \
                ui/src/layout/NowPlayingPanel.jsx ui/src/i18n/en.json \
                resources/i18n/zh-Hans.json resources/i18n/zh-Hant.json
```

### 11.7.1 复盘：上一轮的"修复"其实留下了新问题

第一轮只做到"页面不再横向溢出"就收工，结果给 Select 加的
`width: 0 + min-width: 100%` 把下拉框压到只剩 `Goo` / `gem` 几个字符——
**指标合格（溢出元素 0）但功能是坏的**。用户第二次拿截图反馈"UI 问题很多"。

真正的解法是让**布局**承担收缩，而不是用 CSS 硬压控件：

1. 字段组从 `flex + flex: 1 1 240px` 改成 **grid + `minmax(0, 1fr)`**
   （手机 `1fr` 单列、`sm` 以上两列）。原来的 flex 写法在窄屏**不会真的换行**，
   反而被内容顶成两列半宽控件。
2. `minmax(0, 1fr)` 也不阻止内在宽度向上传播（实测轨道仍按内容取 513px），
   所以**把选项文案改短**才是根治：引擎下拉只留 `Google Gemini`，
   模型下拉只留模型 id，括号说明挪到已有的说明卡片与 helper 文本。
   文案变短后桌面端下拉列表也更好读，算是顺带的体验改善。
3. `.responsive-fields` 里的 Select 规则退回"只截断不改布局"
   （`min-width: 0; max-width: 100%; ellipsis`），作为兜底而非常规手段。

教训：**验收不能只看自己定的指标**。"没有溢出元素"不等于"能用"，
至少要额外断言"关键控件宽度 > 200px 且文案未被截断"。

另外顺手修了一个真 bug：配置里的语言码不在选项列表时（例如手写 API 存了 `zh-Hans`），
Select 会渲染成**一片空白**，用户看不出当前设置。现在会把当前值补进选项并标注
"（当前配置值）"。

### 11.7.2 同一页面的第二轮：界面"中英混排"才是真问题

用户第二次反馈"歌词翻译界面还是有问题，其他界面正常"。这次不是布局，
而是**i18n**：这一页大量文案硬编码中文，英文界面下会出现
"English 标题 + 中文说明卡片 + 中文表格头"的混排。已把 60+ 处文案全部迁移到
`translate()`，并补齐三份语言包。

迁移过程中暴露了一个**仓库级的既有 bug**：全仓 411 个被 `translate()` 引用的 key 里，
**22 个在 en.json / zh-Hans / zh-Hant 里根本不存在**（全是歌词翻译这一页的）。
它们一直静默回退到代码里的中文默认值——所以中文界面看不出问题，
换英文就露馅，而 `make test-i18n` 查不出来（它只校验 en → 翻译包方向）。

新增 `scripts/check-i18n-keys.py`：从源码里抓 `translate('...')` 引用的 key，
按 polyglot 的拍平语义（`extend()` 会把嵌套对象拍成带点键，
见 `node-polyglot/index.js:329`）检查三份语言包是否都有。改动 i18n 后必须跑。

同一轮还抓到自己引入的两个 bug（都是 lint 和原有测试都拦不住的）：

1. `ENGINE_IDS` 是**字符串数组**，我却按对象写了 `o.id` → 5 个选项的 `value`
   全是 `undefined`，下拉框渲染成空白、菜单也打不开。字符串数组 map 出
   `{id,label}` 对象、或直接用字符串，两种写法别混。
2. 批量替换时两个形状相同的 `<MenuItem>` 被一起改了，把语言下拉的文案也换成了
   `engineLabel(...)`，于是语言框只显示 `zh-CN` 这种语言码。
   **批量改 JSX 后必须打开每个下拉框看选项**，光看"页面没报错"不够。

两个都已补单测（`LyricsTranslation.test.jsx` 新增 2 例，断言下拉框显示值与
选项数组），现在 804 个用例。

### 11.7.3 第三轮：表格才是真凶（而且我前两轮根本没测到）

用户第三次反馈"中英文都一样"（截图中 appbar 右侧图标也被挤掉、整页右侧被裁）。
这次直接去**用户的实例**上量（`http://localhost:14534`），320–430px 全部被撑到 **554px**。

钻 min-content 链（离屏探针）得到：

```
.layout 554 → jss45 554 → main 554 → card 554 → CardContent 554
  → 已翻译歌曲管理 section 522 → 缓存表格容器 524 → table 485
```

**元凶是"已翻译歌曲"表格**。前两轮我都在自己的测试实例上验证，而那个实例
**缓存是空的，表格根本不渲染**，所以永远测不到这条路径——这是我的测试方法漏洞：
**验收必须复现用户的真实数据状态**（现在会先往
`<DataFolder>/lyrics_translations/` 塞两条缓存记录再测）。

修法上有个反直觉的坑：**光有 `overflow-x: auto` 没用**。表格容器是普通块
（不是 flex/grid 项），它的 min-content 仍等于表格宽度。必须加
`contain: inline-size`（语义就是"算我自己的尺寸时忽略内容的行内尺寸"）：

```css
.responsive-fields .MuiTableContainer-root {
  overflow-x: auto;
  contain: inline-size; /* 少了这行，min-content 仍是 524px */
}
```

实测对比（430px 视口）：`overflow-x:auto` 单独 → layout 554；
加 `contain: inline-size` → layout 415（≤ 视口），表格在自己容器内横向滚动。
另外把表格 `min-width` 从 520 降到 480，少一点无谓的横向滚动。

修完在 320/375/390/430 四档实测：`docScrollW === 视口宽度`、页面不能横向滚动、
表格容器 `scrollWidth > clientWidth`（自己在滚）。

### 11.7.4 模板字符串拼 i18n key：校验器抓不到的一类错

用户反馈"翻译目标语言显示成 zh-CN，帮我改成简体中文"。查下来是我上一轮引入的：
语言包里 key 叫 `lang.zhCN`，代码却用模板拼 `` `menu.lyricsTranslation.lang.${id}` ``
—— `id` 是 `zh-CN`，拼出来的是 `lang.zh-CN`，**语言包里没有这个 key**，
于是所有语言选项静默回退成语言码（en/ja/ko… 那几个因为 id 和 key 同名所以侥幸没事）。

这类错的特点是**校验器看不见**：`scripts/check-i18n-keys.py` 原来只抓
`translate('字面量')`，模板拼出来的 key 不在其中。两条修法：

1. 代码侧：key 一律用**字面量映射表**，不要用模板拼接
   （`LANG_LABEL_KEYS = { 'zh-CN': 'menu.lyricsTranslation.lang.zhCN', ... }`）。
2. 校验侧：脚本改为同时抓"写得像 i18n key 的字符串字面量"（覆盖映射表），
   并对本 fork 自己的命名空间提示"不要用模板拼 key"。

顺带修了脚本自身两个 bug：正则里 `|` 优先级最低，
`(ra|menu|...|message[A-Za-z0-9_]*\.)` 只有最后一个分支带了后续要求，
导致 `'player'`、`'menu'` 这类普通字符串被误报；另外测试文件里常出现
"故意写错的 key 名"当断言，已排除 `.test.` / `.stories.`。

现在 517 个 key 全部有定义，校验器对模板拼 key 会主动提示。
### 11.8 本轮发布

- 镜像：`huhan333/navidrome2all:latest` 与 `:bfdd120f`（同一 digest
  `sha256:285eba4b…`），已从 registry 反查确认 `linux/amd64`，
  冒烟验证 `responsive-fields` / `contain:inline-size` 规则已内嵌
- 回归：`golangci-lint` 0 issues、前端 92 文件 / 804 用例、
  `make test-i18n` 36 份语言包、`scripts/check-i18n-keys.py` 517 key 无缺失
- 本轮改动清单见 `git status --short`（未提交）

### 11.9 给后来者的纪律

1. **改了 `ui/src` 必须 `npm run build` + `go build`**，只跑前者时二进制里还是旧 UI，
   会让人以为自己的 CSS 没生效（这次就白排查了很久）。
2. **浏览器验证前先关掉 Service Worker 与 HTTP 缓存**：
   `?cb=<时间戳>` 只能绕过 HTML，JS/CSS 仍可能被 SW 缓存：

   ```js
   await page.cdp('Network.enable')
   await page.cdp('Network.setBypassServiceWorker', { bypass: true })
   await page.cdp('Network.setCacheDisabled', { cacheDisabled: true })
   ```

3. **模拟手机视口用 CDP**（ego-browser 没有 `setViewportSize`）：

   ```js
   await page.cdp('Emulation.setDeviceMetricsOverride', {
     width: 390, height: 844, deviceScaleFactor: 2, mobile: true,
   })
   ```

4. 自研表单页的容器加 `className="responsive-fields"`，窄屏溢出基本就免了。
5. **验收必须复现用户的真实数据状态**。空表格 / 空列表 / 未登录态会隐藏一整条
   代码路径——歌词翻译页的表格问题连续两轮都因为"我的实例没有缓存"而测不到。
6. 用户说"还有问题"时，**直接去他的实例上量**（同一台机器的话），别在自己
   构造的等价环境里反复验证：他的实例可能有不同的数据、语言、字号、构建产物。

7. **验收不能只看"没有溢出元素"**——那可能意味着控件被压到只剩几个字符。
   还要断言关键控件宽度足够（如 Select > 200px）且文案未被截断
   （`el.scrollWidth <= el.clientWidth`）。
8. **i18n key 一律写字面量映射表，不要用模板字符串拼**。拼出来的 key 校验器抓不到，
   一旦语言包里命名对不上就是静默回退（表现为"某处文案变成语言码/英文"，
   而且中文界面完全看不出来）。改完 i18n 跑 `python3 scripts/check-i18n-keys.py`。
