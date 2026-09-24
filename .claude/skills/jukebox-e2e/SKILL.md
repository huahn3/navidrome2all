---
name: jukebox-e2e
description: 用 contrib/jukebox-testing 的假 MPD/DLNA 服务器对多输出端功能做端到端验证（含音量双向同步与纯 curl 路径）。改动 jukebox 输出链路、音量模型后使用。
---

# Jukebox 端到端验证

必读前置：`AGENTS.md` 第 3 节（音量三规则）与第 5 节（BaseUrl / 单实例约定）。

## 前置

- **只保留一台 Navidrome 实例、一个 URL**。先 `pgrep -fl bin/navidrome` 确认。
- 假设备不需要 `BaseUrl`（`127.0.0.1` 就能拉到），但**真机验证必须先配**
  `BaseUrl = "http://<局域网IP>:<端口>"`，否则永远"选上了但没声音"。

## 步骤

1. 启动测试桩（日志在 `/tmp/nd_fake_mpd.log`、`/tmp/nd_fake_dlna.log`）：
   ```bash
   python3 contrib/jukebox-testing/fake_mpd.py &
   python3 contrib/jukebox-testing/fake_dlna.py &
   ```
2. 配置 `navidrome.toml`（见 `contrib/jukebox-testing/README.md`）：
   `Jukebox.Enabled = true`、`Jukebox.AdminOnly = false`（方便用 API 验证），
   MPD 指 `127.0.0.1:16600`，DLNA 指 `http://127.0.0.1:1800/rootDesc.xml`
3. `go build -tags=netgo,sqlite_fts5 -o bin/navidrome .` 后启动实例，取一首歌曲 ID
4. 两条验证路径任选（**API 路径更快，界面路径才能验音量 UI**）：
   - **API**：`POST /auth/login` 拿 token → 带 `X-ND-Authorization: Bearer <t>`
     依次调 `/api/jukebox/select` → `/play` → `/status` → `/control`
   - **界面**：真实登录（登录态归用户，不要自己注册账号），
     在工具栏设备选择器里切换，再点播放

   顺序：select → play → status 轮询 → seek → **volume（双向）** → pause → 切回 browser

## 断言点

**协议层（假设备日志）**

- 假 MPD：`clear/add/play`、seek 收到 `seek 0 <秒>`、`setvol <0-100>`、切走收到 `stop`
- 假 DLNA：`SetAVTransportURI`（URI 是带 `u/t/s` 签名的 `/rest/stream` 绝对 URL）、
  `Play/Pause/Stop/Seek/SetVolume`；`GetTransportInfo/GetPositionInfo/GetVolume` 被轮询
- 用 curl 拉 `SetAVTransportURI` 日志里的 URL：必须 200 且 `Content-Type: audio/mpeg`

**状态与错误**

- `/api/jukebox/status` 的 `currentTime/duration/volume` 与假设备一致
- 错误映射：浏览器输出时下发命令 → 409；未知 action → 400；设备不可达 → 502

**音量（本项目最容易改坏的一块，必须专门验）**

- 下发：界面拖到 X% → 设备日志出现 `setvol X` / `SetVolume X`（**是 `round(store×100)`，
  不是 `store²×100`**；平方只用在 `<audio>` 元素上）
- 采纳：改假设备的音量 → 下一轮 `/status` 轮询后界面百分比跟随
- **0 不被采纳**：让假设备在 `GetVolume` 上先回 0，界面音量**不能**变成 0%
  （这是历史上"一刷新音量 0%"的回归点）
- 持久化：设一个非 0 值 → 刷新页面仍是该值；把 localStorage 的
  `state.player.volume` 手工改成 0 再刷新 → 应回到 `defaultUIVolume`，不是哑巴
- 键盘：`Vol+`/`Vol-` 要真的改变界面百分比并下发设备（绕过 store 的写法会被立刻夺回）
- 移动端视口（`Emulation.setDeviceMetricsOverride` 到 390×844）：音量独占一行、可拖动、
  与桌面共用同一个值

## 收尾

```bash
pkill -f fake_mpd.py; pkill -f fake_dlna.py
rm -f /tmp/nd_fake_mpd.log /tmp/nd_fake_dlna.log
```

真机验证过的话，把设备音量恢复到用户原来的值再走（音箱是共享的）。
