---
name: nas-jukebox-deploy
description: 部署与排错本仓库的多输出端播放功能（浏览器 / MPD / DLNA / 小米音箱），尤其是装在 NAS（飞牛 fnOS、群晖、Docker 主机）上时：compose 编排、BaseUrl、声卡直通、SSDP 扫描、局域网拉流。当用户说"怎么部署""配了但不能出声""音箱扫不到""MPD 没声音"或要求生成部署配置时使用。
---

# NAS 多输出端播放部署

## Overview

把"在网页播放器里切换出声设备"这套功能部署到局域网 NAS 上，并用自检脚本定位断在哪一环。
完整人读指南见 `docs/jukebox-nas-deployment.md`；驱动/协议细节见 `docs/jukebox.md`；
改代码前的接手说明见仓库根 `AGENTS.md`（音量模型、两套 jukebox 的区别、已知坑）。

## 先确定目标拓扑

| 想听见的设备 | 需要跑什么 | 关键前置 |
|---|---|---|
| 浏览器（默认） | 只要 Navidrome | `Jukebox.Enabled = true` 即出现选择器 |
| NAS 本机声卡 | Navidrome + MPD（同机或同网络） | MPD `music_directory` 与 `MusicFolder` 同一份内容，且 MPD 已收录歌曲 |
| DLNA/UPnP 音箱 | 只要 Navidrome | `BaseUrl` 必须是局域网地址；音箱与服务器同网段 |
| 不支持 DLNA 的小爱 | 只要 Navidrome | `BaseUrl` + 设备 token（miIO，UDP 54321） |

## 部署流程

1. **拿到镜像**：`docker pull huhan333/navidrome2all:latest`（本 fork 已发布，**仅 linux/amd64**，
   NAS 是 `aarch64` 时要按 `docs/jukebox-nas-deployment.md` 方式 A 自建 arm64）。
   上游 `deluan/navidrome:latest` **没有**多输出端功能，别拿它部署。
2. **打开开关**：`[Jukebox] Enabled = true`（Docker 可用 `ND_JUKEBOX_ENABLED=true`）。
   它只管网页这套；上游的 Subsonic `jukeboxControl`（mpv 播服务器声卡）另有
   `Jukebox.SubsonicEnabled`，**默认关闭，部署时不要顺手打开**。
   设备列表建议**在 Web「管理 → 输出设备」里加**，存数据库；不要指望用环境变量配
   `[[Jukebox.Outputs]]`（viper 的 env 只能给标量，给不了表数组）。
3. **设 `BaseUrl`**：`http://<NAS局域网IP>:<端口>`。这是远程输出能否出声的头号原因——
   它是音箱实际去拉的地址。用 `localhost`/`127.0.0.1` 访问界面不算，必须显式配置。
   端口用 `ports:` 映射即可让音箱从局域网访问；`BaseUrl` 里的端口要和**映射后**的端口一致。
4. **加设备**：`Address` 按类型填（MPD `host:port`；DLNA 填扫描给出的描述文档 URL；
   小米填音箱 IP）。`PathFrom`/`PathTo` 一般留空——MPD 收到的是**相对音乐库根目录**的路径。
5. **DLNA 才需要关心的网络模式**：SSDP 多播扫描要穿透 NAT/网桥才拿得到回包，
   容器内常常扫不到。扫不到不等于功能坏了：从宿主机/路由器取
   `http://<ip>:<port>/<UDN>.xml` 手填即可；要真正扫得到，用 `network_mode: host`
   （host 模式下不要再写 `ports:`，无效且误导）。
6. **MPD 出声**：出声设备由 MPD 的 `audio_output` 决定；容器需拿到声卡
   （`/dev/snd` + `group_add` 填 `/dev/snd` 的 GID），并且**歌曲要在 MPD 侧更新过索引**
   （`mpc update` 或 `auto_update "yes"`）。Navidrome 的扫描不会同步 MPD。
7. **跑自检**：见下。全绿后再到界面上实测一次播放/暂停/seek/音量/切回浏览器。

## 飞牛 fnOS 专有坑

- **应用中心那个一键 Navidrome 同样占 4533**，装过就要先停，否则本 fork 的容器起不来。
- host 网络下与系统共用端口：避开 **5666/5667**（Web UI）和应用中心声明的端口。
- 数据目录用 `/vol1/1000/<共享文件夹>` 约定；**Docker 目录保持 POSIX ACL**
  （v1.2.0 起共享文件夹改走 Windows ACL），所以 `chown 1000:1000` 在这里有效。
- 无桌面音频栈（没有 PulseAudio/PipeWire），MPD 只能用 ALSA `hw:0,0`；
  板载/核显 HDMI 音频随机型有驱动缺失问题，先 `aplay -l` 实测再承诺 MPD 方案。
- 飞牛防火墙默认关闭，开启后要放行 4533 给**整个局域网段**：音箱拉流也是入站请求。
- 官方帮助中心没有 Docker/SSH/静态 IP/音频的文档，界面名称按版本变通，MPD 一节是通用 Debian 做法。

## 自检脚本

只读检查，不会改变选中设备或播放状态。`--song` 需要一首存在的歌曲 ID。

```bash
bash .qoder/skills/nas-jukebox-deploy/scripts/preflight.sh \
  --base http://192.168.1.10:4533 --user <admin> --password '<pw>' \
  --mpd 127.0.0.1:6600 \
  --dlna http://192.168.1.20:9999/rootDesc.xml \
  --song <songId>
```

分段含义：1 Web API 可达 → 登录 → 2 设备列表（能区分 `jukebox is disabled`）→
3 MPD 协议握手（只看 TCP，不发送凭据）→ 4 DLNA 描述文档可取且声明 AVTransport →
5 用 Subsonic 签名（`md5(密码+salt)` 的 `u/t/s`）模拟音箱拉流，验证局域网地址 + 端口 +
签名链路。逐项 FAIL 的成因见 `docs/jukebox.md` 的"故障排查"表。

`--lan-host` 用来单独复现"音箱拿到的主机"（例如想验证换端口后的地址）：
它替换流 URL 的 host，不替换 `--base`。

## 排错切入点

- **有设备但完全不出声**：99% 是 `BaseUrl`。看服务器日志有没有
  `Jukebox stream URL points at the loopback interface`。
- **DLNA 报 501 / 描述文档相关错误**：`Address` 要填描述文档 URL，不是 `/AVTransport/control`。
- **切到 DLNA 后总从头播放**：设备忽略了 `Play` 之后的即时 `Seek`（实测约 3 秒窗口），
  驱动已重发最多 3 次；设备完全不回报位置时只能接受从头播放。
- **MPD 连接成功但不响**：`add` 的路径不在 MPD 曲库里。用假 MPD 看确切入参：
  `python3 contrib/jukebox-testing/fake_mpd.py`（监听 127.0.0.1:16600，
  命令记到 `/tmp/nd_fake_mpd.log`），把临时设备的 `Address` 指过去播一首歌。
- **小米音箱**：无 token 时不能控音量；没有本地 token 又没填 DID 会直接报错；
  该类型不支持 seek（前端不转发拖动是预期行为）。
- **音量显示 0% 或拖了没反应**：先分清"设备没收到"还是"界面被污染"。
  `/api/jukebox/status` 的 `volume` 是设备真实值；localStorage 里 `state.player.volume`
  如果是 0，说明历史版本把"设备尚未回答音量查询"时的 0 采纳并持久化了
  （现版本读取时会换成 `defaultUIVolume`，见 `AGENTS.md` 第 3 节）。
- **界面音量正常、音箱音量不对**：界面向是感知值 0..1，下发设备的是 `round(×100)`，
  只有 `<audio>` 元素用平方映射。若发现"拖到 50% 听起来像 25%"，就是有人把平方用到了设备上。
- **第三方 App 里根本没有 jukebox/音量控制**：Subsonic `jukeboxControl` 属于**上游**那套
  （mpv 播服务器本机），由独立开关 `Jukebox.SubsonicEnabled` 控制，默认关闭，
  和网页的 `Jukebox.Enabled` 互不影响。这是有意的解耦，不是部署错误；
  只有确实要让 App 控制 NAS 声卡时才打开它（还要配 `Devices`/`Default`，见 `AGENTS.md` 第 4 节）。

## 部署时的自我约束

- **只保留一台实例、一个 URL**：多台会让"到底哪个在响"变成玄学，也让自检结果对不上。
- 登录凭据归用户：**不要自己注册管理员账号**，也不要从数据库/日志/历史会话里翻密码。
  需要真人登录验证时，把实例起好、URL 交给用户。
- 排错要给根因证据（日志行、代码位置、`/status` 原文），不要靠"你再刷新试试"。

## Resources

- `scripts/preflight.sh` — 上述 5 段只读自检，输出 OK/FAIL/SKIP，任一 FAIL 时退出码非 0
- `docs/jukebox-nas-deployment.md` — 面向用户的分步部署指南（含 compose 与 mpd.conf 全文）
- `docs/jukebox.md` — 配置项、API、驱动行为、故障排查总表
- `contrib/jukebox-testing/` — 假 MPD / 假 DLNA，用于不接真设备复现链路
