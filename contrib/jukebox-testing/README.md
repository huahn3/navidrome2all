# Jukebox 输出设备 E2E 测试桩

用于在没有真实硬件时验证多输出端播放器控制（见 [docs/jukebox.md](../../docs/jukebox.md)）的
全链路：select → play → status → seek → volume → pause → 切回浏览器。

- `fake_mpd.py` — 假 MPD 守护进程，监听 `127.0.0.1:16600`，实现 MPD 文本协议的
  最小子集（status/clear/add/play/pause/stop/seek/setvol），命令日志写入
  `/tmp/nd_fake_mpd.log`
- `fake_dlna.py` — 假 DLNA 渲染器，监听 `127.0.0.1:1800`，提供
  `/rootDesc.xml` 设备描述与 AVTransport/RenderingControl SOAP 端点，
  请求日志写入 `/tmp/nd_fake_dlna.log`

## 用法

```bash
# 1. 启动两个测试桩（无第三方依赖，python3 即可）
python3 contrib/jukebox-testing/fake_mpd.py &
python3 contrib/jukebox-testing/fake_dlna.py &

# 2. 在 navidrome.toml 中配置
# [Jukebox]
# Enabled = true
# AdminOnly = false
# [[Jukebox.Outputs]]
# ID = "mpd-fake"  Name = "Fake MPD"  Type = "mpd"
# Address = "127.0.0.1:16600"
# [[Jukebox.Outputs]]
# ID = "dlna-fake" Name = "Fake DLNA" Type = "dlna"
# Address = "http://127.0.0.1:1800/rootDesc.xml"

# 3. 启动 Navidrome，在网页播放器工具栏切换输出设备并操作播放/暂停/音量/进度
# 4. 检查测试桩日志确认收到的命令
tail -f /tmp/nd_fake_mpd.log /tmp/nd_fake_dlna.log
```

## 验证要点

- `play` 后假 DLNA 日志中应出现 `SetAVTransportURI`，URI 是带 Subsonic 签名
  （`u/t/s` 参数）的 `/rest/stream` 绝对 URL；用 curl 拉该 URL 应返回 200 + 音频流
- `status` 接口应返回假设备回报的状态/进度/音量
- 切回 `browser` 后假设备日志应出现 `Stop`/`stop`
