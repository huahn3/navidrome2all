# 本 fork 与上游的关系

> 给"下次要跟进上游更新"或"想知道这个 fork 到底改了什么"的人/AI 看的。
> 代码层面的接手说明在仓库根 `AGENTS.md`。

## 历史现状

本仓库的 git 历史**被重置过**：只有面向二次开发的最终状态，没有上游的 5000 多条提交。

| 项目 | 值 |
|---|---|
| 上游仓库 | `https://github.com/navidrome/navidrome` |
| 二开基线（上游 commit） | `ee6dd1bc`（上游 2026-09-23 的 master） |
| 本仓库历史 | 单条初始提交，**与上游无共同祖先** |
| 默认分支 | **`master`**，见下 |
| 旧历史备份 | 重置时的完整 `.git` 移到仓库外的 `navidrome2all-git-backup/`，目录里有 `BACKUP-INFO.txt` 说明如何恢复；确认新仓库无误后可删 |

**因此：能 `diff`，不能 `merge`/`rebase`（没有共同祖先，任何合并都是"整棵树对整棵树"）。**
跟进上游要用下面第 4 节的"补丁搬运"流程，而不是 `git merge upstream/master`。

**为什么分支叫 `master` 而不是 `main`**：随上游带过来的
`.github/workflows/pipeline.yml`（第 4、9 行）与 `push-translations.yml`（第 5 行）
把触发分支写死成 `master`。改名要让 CI 继续跑，就得同时改这三个地方，
否则推送后一条流水线都不会触发。

## 1. 一次性配置 remote

`upstream` 已经在本仓库里配好并 fetch 过（`git remote -v` 可见），只需要补上你自己的仓库：

```bash
git remote add origin https://github.com/<你的账号>/<仓库名>.git
git push -u origin master
```

`upstream` 只用来取参照点，**不要**向它 push。

## 2. 这个 fork 到底改了什么

**新增（整目录都是本 fork 自有，跟进上游时可直接覆盖）**

```
core/jukebox/                    多输出端播放后端（驱动 / 设备管理 / SSDP / DB 输出配置）
server/nativeapi/jukebox.go      /api/jukebox/{devices,status,select,play,control} + 流地址生成
server/nativeapi/jukebox_outputs.go   /api/jukebox/outputs CRUD 与 /discover
server/nativeapi/jukebox{,_outputs}_test.go
server/subsonic/stream_alias_test.go  /rest/stream/{id}.mp3 别名端点用例
ui/src/jukebox/                  「管理 → 输出设备」页面
ui/src/audioplayer/{DeviceSelector,VolumeControl}.jsx（含 .test.jsx）
ui/src/audioplayer/jukebox.js    /api/jukebox 封装
docs/                            功能与部署文档
contrib/jukebox-testing/         假 MPD / 假 DLNA 测试桩
.claude/skills/ .qoder/skills/   AI 协作技能
```

**改动的上游文件（跟进上游时逐个手工核对，别整文件覆盖）**

| 文件 | 改动内容 |
|---|---|
| `conf/configuration.go` | `[Jukebox]` 新增 `Outputs` 与 `JukeboxOutputDevice` 结构 |
| `server/serve_index.go` | 向页面注入 `jukeboxEnabled` |
| `server/nativeapi/native_api.go` | 挂载 jukebox 路由、启动时用 DB 输出喂给 DeviceManager |
| `server/subsonic/api.go` | 注册 `/rest/stream/{id}{.ext}` 别名路由 |
| `server/subsonic/stream.go` | 抽出可复用的流处理，供别名端点调用 |
| `ui/src/App.jsx` | 注册 `jukeboxOutput` 资源（admin + `jukeboxEnabled` 才出现） |
| `ui/src/audioplayer/Player.jsx` | 音量三规则、设备切换、静音时钟与进度校准 |
| `ui/src/audioplayer/PlayerToolbar.jsx` | 装配 `DeviceSelector` / `VolumeControl` |
| `ui/src/audioplayer/keyHandlers.jsx` | 键盘音量改走 store |
| `ui/src/audioplayer/styles.js` | 隐藏库自带音量条（只留 `VolumeControl`）、移动端音量独占一行与工具栏排布 |
| `ui/src/actions/player.js` | `setOutputDevice` 与 `BROWSER_DEVICE` |
| `ui/src/reducers/playerReducer.js` | `outputDevice` 状态、清队列时保留设备选择 |
| `ui/src/store/createAdminStore.js` | 持久化白名单加 `outputDevice`、音量 0 兜底 |
| `ui/src/config.js` | `jukeboxEnabled` / `defaultUIVolume` |
| `ui/src/dataProvider/wrapperDataProvider.js` | 输出设备的 REST 适配 |
| `ui/src/i18n/en.json`、`resources/i18n/zh-Hans.json`、`zh-Hant.json` | `jukebox.*` 与 `resources.jukeboxOutput.*` 文案 |
| `ui/vite.config.js` | 开发模式注入 `__APP_CONFIG__`（否则本地跑不出真实配置） |
| `server/serve_index_test.go`、`ui/src/audioplayer/PlayerToolbar.test.jsx`、`ui/src/reducers/playerReducer.test.js`、`ui/src/dataProvider/wrapperDataProvider.test.js` | 上述改动的用例 |
| `README.md`、`go.mod`、`go.sum` | 说明与依赖 |
| `.gitignore` | 移除 `AGENTS.md` 的忽略（本 fork 要跟踪它）；新增**根目录限定**的 `/artwork/` |

> ⚠️ 改 `.gitignore` 时的真实教训：起初写的是不带斜杠的 `artwork/`，它匹配**任意层级**，
> 于是把整个 `core/artwork/` Go 包（81 个源文件）连同上游的封面功能一起从提交里漏掉了。
> 所以第 4 节的流程走完，一定要用
> `git diff --diff-filter=D --name-only <upstream-base> HEAD` 确认为空。

查看全部差异（`ee6dd1bc` 需要先被 `upstream` fetch 到本地）：

```bash
git diff --stat ee6dd1bc HEAD
git diff ee6dd1bc HEAD -- conf/configuration.go server/subsonic/
```

## 3. 只想看"上游最近改了什么、和我们有没有冲突"

```bash
git fetch upstream
git log --oneline ee6dd1bc..upstream/master            # 基线之后的上游提交
git log --oneline ee6dd1bc..upstream/master -- core/jukebox ui/src/audioplayer server/nativeapi
git diff --name-only ee6dd1bc upstream/master          # 上游动了哪些文件
```

第三条命令最重要：它列出"上游改动是否与我们的文件重叠"。

## 4. 跟进上游的标准流程

因为不能 merge，做法是**把我们的改动当成一个补丁搬到新的上游基线上**：

```bash
# ① 以最新上游为参照，导出我们的全部改动
git fetch upstream
git diff --binary upstream/master HEAD > /tmp/fork-changes.patch

# ② 从最新上游开一个工作分支，套用
git checkout -b sync-$(date +%Y%m%d) upstream/master
git apply --3way --stat /tmp/fork-changes.patch    # 先看会改什么
git apply --3way /tmp/fork-changes.patch           # 再真套

# ③ 提交并验证（务必全跑，见 AGENTS.md 第 7 节）
git add -A && git commit -m "sync: rebase fork changes onto upstream <date>"
make test PKG=./core/jukebox && make test PKG=./server/nativeapi
cd ui && npm run test && npm run lint

# ④ 通过后推到自己的仓库（默认分支是 master，不要改名，见开头说明）
```

冲突通常出现在第 2 节表格里"改动的上游文件"——上游也改了同一个文件时，
**以上游为准，手工把我们的那段重新加回去**，不要整文件保留我们的旧版本
（那会静默回退上游的 bug 修复）。

套完记得更新本文档第 1 节的基线 commit 号。

## 5. 许可与署名（不能省）

- 上游是 **GPLv3**。`LICENSE` 文件与各源文件顶部的版权头必须保留。
- 分发编译产物（二进制、Docker 镜像、NAS 应用）时，必须**同时提供完整对应源码**
  （公开仓库即可），并说明本版本相对上游的修改。
- 在 `README.md` 的 fork 段落里保持上游 `navidrome.org` 的指向与致谢。
