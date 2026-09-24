---
name: add-jukebox-driver
description: 给多输出端播放器（core/jukebox）新增一种输出设备驱动（如 airplay、蓝牙发射器）的完整落点清单与约定。用户要求"新增输出设备类型/驱动""支持某某音箱"时使用。
---

# 新增 Jukebox 输出驱动

必读前置：仓库根 `AGENTS.md` 第 3–5 节（音量模型、两套 jukebox、已知坑）。
功能文档 `docs/jukebox.md`；协议调研范例 `docs/xiaomi-speakers.md`。

**新增一个驱动要动 7 个地方，漏一个就会出现"能选不能控"或"某个语言界面缺文案"。**

## 1. 驱动本体：`core/jukebox/driver_<type>.go`

实现 `driver.go` 的 `PlayerDriver`（**只有这 7 个方法，没有 Next/Prev —— 队列属于浏览器**）：

```go
Play(mediaPath string, streamURL string) error    // 文件型驱动用 mediaPath，网络渲染器用 streamURL
Pause() error
Resume() error
Stop() error
Seek(seconds int) error            // 不支持就返回 fmt.Errorf("%w: ...", ErrInvalidCommand) → HTTP 400
SetVolume(volumePercent int) error // 0-100
GetState() (*PlaybackState, error) // {Status, CurrentTime, Duration, VolumePercent}
```

实现时的硬约定：

- **不要自己加互斥锁**：`DeviceManager` 已经把所有命令串行化了。
- 设备不回报进度时：`CurrentTime` 返 0、`Status` 用最后一次已知的缓存值，
  并考虑给"刚下发播放"留一个宽限期（见 `xiaomiPlayGracePeriod`），
  否则音箱缓冲期间会被判成 `stopped` 而错误推进队列。
- 音量拿不到真值时**不要回报 0**：前端 `adoptDeviceVolume` 会忽略 0
  （0 意味着"还没问过设备"，不是"静音"），用最后一次设定值兜底。
- 网络渲染器只需要 `streamURL`（后端已生成带 Subsonic `u/t/s` 签名的绝对地址），
  **驱动不要自己拼鉴权**，也不要把 URL 记进日志之外的持久化位置。

## 2. 注册：`core/jukebox/manager.go`

常量块加 `TypeXxx = "xxx"`，`newDriver` 的 switch 加一个 case。
需要构造期校验（缺 token 之类）就返回 error，别等到第一次 `Play` 才炸。

## 3. 配置字段：`conf/configuration.go`

`JukeboxOutputDevice` 按需加字段（注释写清用途与格式），并在
`viper.SetDefault("jukebox.outputs", ...)` 附近保持一致；
`[[Jukebox.Outputs]]` 是 TOML 表数组，**环境变量配不了**，只能 TOML 或网页管理页。

## 4. 网页管理表单：`ui/src/jukebox/JukeboxOutputForm.jsx`

- `OUTPUT_TYPE_CHOICES` 加一项（`id` 必须与后端 `TypeXxx` 完全一致）
- 新字段用 `<FormDataConsumer>` 包成"仅该类型可见"（照 xiaomi / mpd 两块写法）
- 每个字段配 `helperText` 文案 key，不要写死英文
- 该目录**只有 list / create / edit 三个页面，没有 show 页**——别顺手加

## 5. i18n：三份都要动

- `ui/src/i18n/en.json`：`resources.jukeboxOutput.types.xxx`、`.fields.*`、`.helpers.*`、`.messages.*`
- `resources/i18n/zh-Hans.json`、`resources/i18n/zh-Hant.json`：后端文案与前端同名 key
- 漏了会导致界面上显示成 key 字符串；`make test-i18n` 会校验翻译文件

## 6. 播放器能力分支：`ui/src/audioplayer/Player.jsx`

按 `/api/jukebox/status` 返回的 `deviceType` 调整行为（现有 xiaomi 就是样板）：

- 不支持 seek → 不转发用户拖动
- 无进度回报 → 跳过漂移校准
- 音量单位仍是统一的 0-100 下发 / `÷100` 采纳，不要为新设备开第二套音量语义

## 7. 测试与文档

- 单测（Ginkgo，仿 `driver_dlna_test.go` / `driver_xiaomi_test.go`）：
  协议一律用假服务器（内存 TCP / `httptest` / 本地 UDP 回环），
  覆盖凭据校验、命令映射、错误分支、状态回报；
  `manager_test.go` 用注入的 `driverFactory` 测分发；
  HTTP 层错误映射测在 `server/nativeapi/jukebox_test.go`
- 端到端：能加进 `contrib/jukebox-testing/` 就加个假设备桩，按 `jukebox-e2e` skill 跑一遍
- 文档：`docs/jukebox.md` 的「字段速查」表、「配置教程」小节、"已知限制"、故障排查表

## 验收

```bash
gofmt -l core/jukebox && make test PKG=./core/jukebox && make test PKG=./server/nativeapi
cd ui && npm run test && npm run lint && npm run check-formatting && npm run build
```

真机验证前先确认 `BaseUrl` 是局域网地址（否则永远"选上了但没声"）。
