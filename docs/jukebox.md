# 多输出端播放器控制（Jukebox Outputs）

本 fork 在 Navidrome 网页播放器中增加了"输出设备切换"功能：除了在浏览器中播放，
还可以把当前歌曲路由到局域网内的其他出声设备——目前支持 **MPD**（驱动 NAS 本机声卡）、
**DLNA/UPnP 渲染器**（智能音箱、功放等）与 **小米音箱原生协议**。

> 要在 NAS（飞牛 fnOS 等）上从零部署，先看
> [NAS 部署指南](jukebox-nas-deployment.md)（镜像构建、compose、BaseUrl、声卡直通、自检）。

## 功能总览

- 播放器工具栏新增设备选择器（`DeviceSelector`），列出所有可用输出
- 切换到远程输出后：播放/暂停/停止/seek/音量都由后端代理到目标设备
- 切回浏览器时自动停止远程设备
- **音量只有一套**：浏览器输出、远程输出、移动端与桌面端共用同一个音量，
  并且与设备实际音量双向同步（详见[音量模型](#音量模型)）
- 「管理 → 输出设备」页面可在网页上增删改输出（存数据库，覆盖 TOML 同名条目），
  DLNA 类型带"扫描局域网"一键填地址
- 前端在远程输出模式下继续驱动"静音本地时钟"：`<audio>` 静音播放以维持
  MediaSession/锁屏界面，进度条以本地时钟为准并按设备回报校准（`Player.jsx`）

> 给 AI / 二次开发者的接手文档是仓库根的 `AGENTS.md`（命令、代码地图、不变量、已知坑）。
> 本文档面向使用者与排障。

## 配置

在 `navidrome.toml` 中：

```toml
[Jukebox]
Enabled = true
AdminOnly = true   # 写操作（select/play/control）仅管理员可用；读取（devices/status）所有登录用户可用

[[Jukebox.Outputs]]
ID = "mpd-living"       # 必填，设备唯一 ID
Name = "客厅声卡"        # UI 显示名
Type = "mpd"            # "mpd" / "dlna" / "xiaomi"
Address = "127.0.0.1:6600"  # MPD: host:port；DLNA: 设备描述文档 URL（推荐，如 http://192.168.1.10:49152/rootDesc.xml；"扫描局域网"填的就是这个），也可填 host:port；xiaomi: 音箱 IP
Password = ""           # MPD 可选密码；xiaomi 云端模式时为小米账号密码
PathFrom = ""           # 可选：路径段替换，见 [MPD 配置教程](#mpd-输出配置教程)
PathTo = ""

[[Jukebox.Outputs]]
ID = "dlna-speaker"
Name = "小爱音箱"
Type = "dlna"
Address = "http://192.168.1.20:49152/rootDesc.xml"

# Redmi 小爱音箱 Play（L7A，不支持 DLNA）——原生协议驱动
[[Jukebox.Outputs]]
ID = "xiaoai-l7a"
Name = "Redmi 小爱"
Type = "xiaomi"
Address = "192.168.1.30"                      # 音箱 IP（本地 miIO，UDP 54321）
Token = "00112233445566778899aabbccddeeff"    # 32 位 hex 设备 token（本地控制/音量/状态）
DID = "123456789"                             # 数字设备 ID（有 token 时可省略，握手自动学习）
Model = "l7a"                                 # 型号后缀，选择 siid/aiid 映射（l7a/s12/l05b，缺省按 Play 系列默认）
Account = "user@example.com"                  # 可选：小米账号（+ Password）启用云端文本指令播放
# TextDirective = "5-5"                       # 可选：execute-text-directive 的 siid-aiid 覆盖
```

> 总开关只有 `Jukebox.Enabled`（`server/serve_index.go` 以 `jukeboxEnabled` 下发给前端）。
> 即使一台远程设备都还没配，工具栏选择器和 **管理 → 输出设备** 页面依然可见——
> 否则无处添加设备；列表里除了"浏览器"以外为空即表示还没有远程输出。

## 配置教程（按输出类型）

设备可在 `navidrome.toml` 里写死，也可以在 **管理 → 输出设备** 页面新增（存进数据库，
按 `ID` 覆盖 TOML 里的同名条目）。表单字段：

- **Id**：唯一 ID，创建后不可改；API 里 `select`/`play` 用的就是它
- **Name**：仅 UI 显示
- **Type**：`mpd` / `dlna` / `xiaomi`（决定用哪个驱动，也决定下面哪些字段有意义）
- 不相关的字段留空即可；`Jukebox.Enabled = true` 才会出现在播放器工具栏

### 浏览器输出

内置，无需配置（ID 固定为 `browser`）。切走时远程设备会被 `stop`，切回时远程停止、
浏览器恢复出声。

### MPD 输出配置教程

MPD 是"让 NAS 本机声卡出声"最省事的方案：Navidrome 只告诉 MPD **放哪个文件**，
解码和出声完全由 MPD 负责。驱动每次操作新开一条 TCP 连接，播放 =
`clear` + `add <路径>` + `play 0`（`core/jukebox/driver_mpd.go`）。

**① 路径语义（最容易踩的坑）**

驱动传给 MPD 的是 **相对音乐库根目录的路径**，也就是数据库 `media_file.path`
（`server/nativeapi/jukebox.go` 直接用 `mediaFile.Path`），例如：

```
add "test-song.mp3"
add "songs/专辑/曲目.flac"
```

已用 `contrib/jukebox-testing/fake_mpd.py` 实测确认：Navidrome 库里
`MusicFolder = /tmp/nd-music`，MPD 收到的却是 `test-song.mp3`，**不带任何前缀**。

> 因此"容器内路径 → 宿主机路径"式的重写通常是不必要的，也是无效的
> （旧版文档里 `PathFrom = "/music"` 的例子匹配不到任何东西）。

结论：**MPD 的 `music_directory` 要与 Navidrome 的 `MusicFolder` 指向同一份内容**。
Docker 场景下两个容器各挂各的挂载点即可，相对路径自然对齐：

```yaml
# navidrome
- /volume1/music:/music
# mpd
- /volume1/music:/mpd-library
# mpd.conf: music_directory "/mpd-library"
```

**② MPD 必须已经收录该文件**

`add` 走的是 MPD 自己的曲库索引，Navidrome 扫描不会让 MPD 看到新歌。
所以要么手动 `mpc update`，要么在 `mpd.conf` 打开自动更新：

```
auto_update "yes"
auto_update_depth "4"
```

否则表现为播放失败、502 body 里是 MPD 原文（`directory or file ... not found` 之类）。

**③ Address / 网络可达**

`Address` = `host:port`，MPD 默认端口 6600。MPD 默认只监听回环，跨机连接需要：

```
bind_to_address "0.0.0.0"
```

同机（含同一 NAS 上的容器网络可达）填 `127.0.0.1:6600` 或容器名端口即可。

**④ Password**

填了就走认证连接（`DialAuthenticated`），对应 `mpd.conf`：

```
password = "secret@read,add,control,admin"
```

权限至少要 `read,add,control`（缺 `add` 会连 `password` 用户名一起报错，
缺 `control` 会让 pause/seek/volume 失败）。留空则匿名连接。

**⑤ 出声设备与音量**

用哪块声卡由 **MPD 的 `audio_output`** 决定，Navidrome 侧不关心：

```
audio_output {
  type     "alsa"
  name     "NAS 声卡"
  device   "hw:0,0"
}
```

音量是 0–100 的 MPD 百分比（`setvol`）。部分声卡混音器不回报数值（`volume: -1`），
此时驱动用最后一次设定的值兜底，进度和状态不受影响。

**⑥ 表单示例**

| 字段 | 值 |
|---|---|
| Id | `mpd-nas` |
| Name | `NAS 声卡` |
| Type | `MPD 服务器` |
| Address | `127.0.0.1:6600` |
| Password | （MPD 密码，无则留空） |
| Path from / Path to | 留空 |

**⑦ PathFrom / PathTo 到底怎么用**

实现是一次字符串替换：`strings.Replace(相对路径, from, to, 1)`——只替换**第一处**、
不要求出现在开头、也不会凭空在开头补目录。只有当 MPD 曲库把同一份文件挂在**不同的
相对目录名**下时才需要，例如 MPD 库里多了一层"无损"目录：

```
Path from = "FLAC/"
Path to   = "无损/FLAC/"
```

正常对齐 `music_directory` 时请留空。

**⑧ 限制**

- 只能播本地文件：网络电台等"只有流地址、没有媒体路径"的条目会失败，
  错误为 `mpd: no local media path for output "..."`
- 切走设备时发 `stop`（清掉当前播放），重新选中后需再点播放
- 进度/状态来自 MPD 的 `status`（`elapsed`/`duration`），精度足够

**⑨ 不接真实 MPD 先验证连通性**

```
python3 contrib/jukebox-testing/fake_mpd.py     # 监听 127.0.0.1:16600
```

在页面临时建一个 `Type = MPD` / `Address = 127.0.0.1:16600` 的设备，选中并播放，
`tail -f /tmp/nd_fake_mpd.log` 会打印 MPD 收到的原始命令（含 `add "..."` 的确切路径）。
确认路径符合预期后删掉该设备，再配真实 MPD。

### DLNA / UPnP 输出配置教程

适用于支持 DLNA 渲染器的音箱/功放（含小爱音箱的 DLNA 接口）。

- **Address 优先填"扫描局域网"给出的设备描述文档 URL**，形如
  `http://192.168.31.142:9999/e522dfd8-....xml`。驱动会下载文档、解析出
  AVTransport 的 `controlURL` 并补全路径。
- 也可以只填 `host:port`：驱动依次探测 `/rootDesc.xml`、`/description.xml`、
  `/<UDN>.xml`，全部失败才回退到约定的 `/AVTransport/control`（会记 warning）。
- **必须配置 `BaseUrl`**（见[上文](#让局域网设备能拉到流baseurl)）。音箱拉的是
  带 Subsonic 签名的 `/rest/stream` 绝对地址，主机名不对就是"选中了但没声音"。
- 扫描原理：SSDP `M-SEARCH`，`MAN: "ssdp:discover"`，同时探测
  `AVTransport:1` 服务与 `MediaRenderer:1` 设备，按 `Location` 去重。
  扫不到多为跨网段/多播被拦（VPN、AP 隔离），可按"已知限制"里的办法手填 URL。
- 控制能力看设备：`pause/resume/stop/volume` 通常都有；`seek` 依赖设备真的执行
  （当前版本会校验回报位置并重发最多 3 次）；进度依赖 `GetPositionInfo`，
  部分设备（实测小爱 S12）`RelTime` 会归零重数，前端已忽略大的向后跳变。

### 小米音箱输出配置教程（原生 miIO / MIoT）

用于**不支持 DLNA** 的小爱音箱（Redmi 小爱音箱 Play L7A、小米小爱音箱 Play L05B 等）。
协议细节与 siid/aiid 差异见 [小米音箱原生协议调研](xiaomi-speakers.md)。

- **Address** = 音箱 IP（不带端口，本地控制走 UDP 54321）
- 凭据三选一/组合（缺项时驱动构造期直接报错）：
  - `Token`（32 位 hex）→ 本地 miIO 控制：**播放/暂停/音量/状态**都有，推荐至少配它。
    报错原文：`xiaomi driver requires token (local miIO) and/or account+password (cloud MIoT)`
  - `Account` + `Password` → 小米云 MIoT：作为播放（文本指令）的优先通道
  - 只用云端（没有 Token）时 **`DID` 必填**：
    `xiaomi driver: cloud-only setups require the did (device ID)`；
    有 Token 时可留空，握手会自动学到
  - 音量必须有 Token：`xiaomi driver: volume control requires the local miIO transport (token)`
- **Model**：`l7a` / `s12` / `l05b`，选择 siid/piid/aiid 映射与音量下限（L7A 是 3）；
  未知型号用 Play 系列默认表，播放指令不同时可填 `TextDirective = "5-5"` 覆盖
- 工作方式：播放是把 `播放 <流地址>` 作为文本指令下发，音箱自己去拉流，
  所以同样依赖 `BaseUrl`；小爱要求 URL 带扩展名，故走 `/rest/stream/{id}.mp3` 别名端点
- 限制：**不支持 seek**（返回 400，前端不会转发用户拖动）、`Stop` 以 Pause 近似、
  无进度回报（`currentTime` 恒 0，前端因此跳过漂移校准）
- 验证状态：上述凭据校验、siid/aiid 映射与错误分支都由单测覆盖（假 miIO UDP 服务器 +
  httptest 假小米云）；**真机上的本地 miIO 控制仍未实测**（手头的音箱走的是 DLNA 接口）。
  第一次配真机时，用 `Address = 音箱IP` + `Token`，从"只点播放"开始逐步验；
  型号不对时先换 `Model`，仍不行就填 `TextDirective` 覆盖 siid-aiid

### 字段速查

| 字段 | browser | mpd | dlna | xiaomi |
|---|---|---|---|---|
| Address | — | `host:6600` | 描述文档 URL（推荐）或 `host:port` | 音箱 IP |
| Password | — | MPD 密码（可选） | — | 小米账号密码（配 Account） |
| Path from / to | — | 一般留空 | — | — |
| Token / DID / Model / Account / TextDirective | — | — | — | 小米专用 |
| 需要 `BaseUrl` | 否 | 否 | 是 | 是 |
| 播放内容 | 本地解码 | MPD 本地文件 | 音箱拉 HTTP 流 | 音箱拉 HTTP 流 |
| 支持进度/seek | 是 | 是 / 是 | 看设备 | 否 / 否 |

## 架构

> 本节说的是本 fork 新增的 `core/jukebox`。仓库里另有一套**上游自带**的 jukebox
> （`core/playback` + Subsonic `jukeboxControl`，用 mpv 子进程在服务器上外放），
> 两者互不相干、共用同一个 `Jukebox.Enabled` 开关。区别见仓库根 `AGENTS.md` 第 4 节。

```
ui/src/audioplayer/DeviceSelector.jsx ──┐
ui/src/audioplayer/jukebox.js (API 封装) │
ui/src/audioplayer/Player.jsx (静音时钟劫持) │  HTTP /api/jukebox/*
ui/src/reducers/playerReducer.js (outputDevice) │
                                             ▼
                        server/nativeapi/jukebox.go  ── chi 路由、鉴权、错误映射
                                             │
                        core/jukebox/manager.go      ── DeviceManager 单例：设备列表/选择/命令分发
                                             │
                        core/jukebox/driver.go       ── PlayerDriver 接口
                                             │
              ┌──────────────────────────────┴───────────────┐
        driver_mpd.go (TCP 6600)    driver_dlna.go (SOAP/UPnP)    driver_xiaomi.go (miIO UDP+AES / MIoT 云端)
```

### 后端

- `PlayerDriver` 接口：`Play(mediaPath, streamURL)` / `Pause` / `Resume` / `Stop` /
  `Seek(seconds)` / `SetVolume(0-100)` / `GetState() (*PlaybackState, error)`
  - MPD 走 `mediaPath`（相对音乐库根目录的文件路径，见
    [MPD 输出配置教程](#mpd-输出配置教程)）；DLNA / xiaomi 走 `streamURL`（HTTP 拉流）
- `DeviceManager`（`GetInstance()` 单例）串行化所有命令（互斥锁），
  维护当前选中设备；内置输出 ID 为 `"browser"`（`BrowserOutputID`）
- `newDriver` 工厂按 `JukeboxOutputDevice.Type` 分发；新增驱动类型只需在此注册
- 哨兵错误：`ErrNoRemoteOutput`（当前是浏览器输出，→ HTTP 409）、
  `ErrInvalidCommand`（未知 action / 越界参数，→ HTTP 400）、其余驱动错误 → 502

### 流 URL

远程渲染器不带浏览器会话，`jukeboxPlay` 会为歌曲生成**带 Subsonic 签名
（u/t/s 盐+MD5）的绝对 `/rest/stream` URL**，目标设备可直接拉流
（已用真实 DLNA 设备验证：200, audio/mpeg）。

小爱音箱要求 URL 路径带音频扩展名，xiaomi 驱动会把 `/rest/stream?id=X`
重写为 `/rest/stream/X.mp3?...`（`server/subsonic` 的 `StreamAlias` 别名端点，
行为与 `/rest/stream` 完全一致，id 从路径提取）。

### 让局域网设备能拉到流：BaseUrl

流地址的主机名由 `core/publicurl` 决定，优先级是 `BaseUrl`（→ `BaseHost`/`BaseScheme`）
> `ShareURL` > 浏览器访问所用的 host。若浏览器用 `http://localhost:14533` 打开界面
且没有配置 `BaseUrl`，下发给音箱的就是 `http://localhost:14533/...`——音箱会去访问
它自己，表现为"设备已选中、没有任何声音"。此时服务端日志会给出明确提示：

```
level=warning msg="Jukebox stream URL points at the loopback interface; ..."
```

修复方式是配置服务器对局域网可达的地址（不要靠改浏览器 URL）：

```toml
BaseUrl = "http://192.168.31.246:14533"
```

`Address` 默认已是 `0.0.0.0`，通常无需改动监听地址。

## 音量模型

音量的**唯一权威是前端 store 里的 `state.player.volume`（0..1，感知音量）**，
浏览器输出、远程输出、移动端与桌面端共用同一个值。三层之间是平方换算关系：

| 位置 | 取值 | 换算 |
|---|---|---|
| store / 滑块 / localStorage 持久化 | 0..1（感知） | 基准 |
| `<audio>` 元素 | 0..1（物理） | `store²` |
| 远程设备（MPD `setvol` / DLNA `SetVolume` / 小米 MIoT） | 0-100 整数 | `round(store×100)` |

用平方是为了让"拖到 50%"听起来大约是"一半响度"——人耳近似对数，播放器的线性音量
按平方映射后滑块手感才线性。

三条同步规则（全部在 `ui/src/audioplayer/Player.jsx`，`VolumeControl.jsx` 只读写 store）：

1. **store → `<audio>`**：effect 把 `store²` 写给元素，并监听 `volumechange` 重新断言。
   必须"重新断言"而不是"写一次"：`navidrome-music-player` 库自己记音量并在开始播放时
   从 0 淡入，会覆盖掉一次性赋值。
2. **store → 远程设备**：远程输出生效时把 `round(store×100)` 下发给设备，200ms 防抖
   （拖动滑块只发最后一次）。
3. **设备 → store**：`/api/jukebox/status` 轮询回报的 `volume`（0-100）反向写回 store，
   于是"用音箱遥控器改音量"也会反映到界面。**这里必须忽略设备回报的 0**：驱动在设备
   第一次真正回答音量查询之前一律回报 0，采纳 0 会让界面静音并被持久化——
   这正是历史上"一刷新音量就 0%"的成因。另外，用户刚拖过滑块的 3 秒内不采纳设备值，
   避免设备回显滞后把用户的选择拽回去。

配套行为：

- 音量随其它播放状态一起存进 localStorage（`state` 键）。因为"静音"存的就是 0，
  而"取消静音要回到多少"只存在于内存，所以读取持久化状态时**会把 0 量替换成
  `config.defaultUIVolume`**（默认 100），避免刷新后是个哑巴播放器。
- 桌面端音量在播放器工具栏一行里；移动端单独占一行（滑块更长，带百分比与静音按钮）。
  两处是同一个组件，没有第二套音量逻辑。
- 键盘快捷键（`Vol+` / `Vol-`）也走 store，而不是直接改 `<audio>`：直接改元素会被
  上面第 1 条立刻夺回，表现为"快捷键没反应"，而且永远传不到远程音箱。
- 静音按钮把 store 置 0，再点回到静音前的百分比（内存里记着）。

## API（`/api/jukebox`）

| 方法 | 路径 | 权限 | 说明 |
|---|---|---|---|
| GET | `/devices` | 登录用户 | `{devices:[{id,name,type}], selected}`；`browser` 为内置输出 |
| GET | `/status` | 登录用户 | `{status,currentTime,duration,volume,deviceId,deviceType}`；浏览器输出时返回 `stopped`；`deviceType` 供前端按设备能力调整行为（如 xiaomi 无进度回报） |
| POST | `/select` | 管理员* | `{device_id}`；切回 `browser` 会停止远程设备 |
| POST | `/play` | 管理员* | `{song_id?, stream_url?, position?}`；position>0 时播放后 seek，并校验设备是否真的跳转（见"故障排查"） |
| POST | `/control` | 管理员* | `{action, value}`；action ∈ `pause/resume/stop/seek/volume` |
| GET | `/discover?timeout=N` | **管理员** | SSDP 扫描结果 `[{usn,name,address,model}]`；`address` 可直接粘进输出的 `Address` |
| GET | `/outputs` | **管理员** | 网页上创建的输出列表（含 TOML 条目时以 DB 覆盖同 ID） |
| POST | `/outputs` | **管理员** | 新建输出；`id` 必须 1-64 位 `[a-zA-Z0-9_-]` |
| GET/PUT/DELETE | `/outputs/{id}` | **管理员** | 读 / 改 / 删单个输出 |

\* `Jukebox.AdminOnly=false` 时所有登录用户可用。`Jukebox.Enabled=false` 时全部 403。

`/outputs` 与 `/discover` **始终是管理员**（不受 `AdminOnly` 影响）：输出配置里存着设备
token 与账号密码。响应体会被剥掉这些密钥字段，写请求需要全量字段。

## 前端

- `ui/src/audioplayer/jukebox.js`：API 封装，缓存服务器端选中设备，
  命令前自动 `ensureSelected`（服务器重启后选中态会重置回浏览器）
- `DeviceSelector.jsx`：工具栏下拉选择器（桌面与移动端共用）
- `VolumeControl.jsx`：唯一的音量控件（图标静音开关 + 滑块 + 百分比）。
  **只读写 store**，所有设备/元素同步都在 `Player.jsx`；桌面端在工具栏同一行，
  移动端独立一行（`compact` 样式）
- `PlayerToolbar.jsx`：把上面两个组件装配进播放器的桌面/移动两套布局
- `keyHandlers.jsx`：键盘快捷键，音量走 `setVolume`
- `Player.jsx`：远程输出时 `<audio>` 元素静音并接管时钟；进度条以本地时钟为准，
  定期用 `/status` 校准漂移；音量三条同步规则（见[音量模型](#音量模型)）；
  `playerReducer` 新增 `outputDevice` 状态
  - 切换输出与首次向设备下发歌曲时都带上本地时钟的 `position`，避免从 0 重播
  - `deviceType === 'xiaomi'` 时跳过漂移校准与用户 seek 转发（设备不支持）
- `ui/src/jukebox/`：「管理 → 输出设备」的 list / create / edit 三个页面
  （**没有 show 页**：列表行点进 edit，无处跳转 show），DLNA 表单内嵌"扫描局域网"区块
- `ui/src/reducers/playerReducer.js` + `ui/src/store/createAdminStore.js`：
  `outputDevice` 与 `volume` 的持久化白名单
- `ui/src/config.js`：读取 `jukeboxEnabled` 决定是否显示选择器，`defaultUIVolume` 是默认音量

## 测试

- Go（Ginkgo）：`make test PKG=./core/jukebox`（98 specs，含 xiaomi 驱动的假 miio
  UDP 服务器与 httptest 假小米云、假 DLNA 渲染器与 SSDP 回放）、
  `make test PKG=./server/nativeapi`（180 specs，含输出 CRUD 的鉴权与 ID 校验）、
  `server/subsonic` 含 `StreamAlias`（`/rest/stream/{id}.mp3`）用例
- 前端（Vitest）：`cd ui && npm run test`（88 文件 / 774 用例，含 `DeviceSelector.test.jsx`、
  `VolumeControl.test.jsx`、`PlayerToolbar.test.jsx`、`playerReducer.test.js`）
- 端到端：`contrib/jukebox-testing/` 提供假 MPD / 假 DLNA 服务器，
  可配合真实 Navidrome 实例做 select→play→status→seek→volume→pause→切回 全流程验证
  （详见该目录 README）
- 手工验证音量：设一个非 0 值 → 刷新页面仍是该值（不被持久化的 0 污染）→
  用音箱遥控器改设备音量 → 界面百分比跟随；命令细节见 `.claude/skills/jukebox-e2e/SKILL.md`

## 故障排查

| 现象 / 日志 | 原因 | 处理 |
|---|---|---|
| `dlna: ... failed: HTTP 501` | 把设备描述文档 URL 当成了控制地址 POST（旧版本行为），或 `Address` 填错路径 | 升级到当前版本后，DLNA 的 `Address` 直接填"扫描局域网"给出的描述文档 URL |
| `dlna: device description ... declares no AVTransport service` | 填的是别的设备的文档（如灯/网关），或该设备不是渲染器 | 换用 `Location` 头里带 `AVTransport:1` 的那个 URL |
| `Could not stop jukebox device`（warning，切换设备时） | 目标设备当时不可达 | 检查 `Address` 与设备是否在线 |
| 选中设备后无声、日志出现 loopback 提示 | 未配置 `BaseUrl`，流地址是 `localhost` | 见上文 [让局域网设备能拉到流](#让局域网设备能拉到流baseurl) |
| `/api/jukebox/status` 持续 502 | 设备命令失败；前端已退避轮询（1s→5s→15s） | 看同时间戳的 `dlna:` 错误行，502 的 body 就是驱动原文 |
| "扫描局域网"没有结果 | 服务器与音箱不在同一二层网段、多播被 VPN/隔离开关拦截，或设备不响应 M-SEARCH | 确认服务器直接跑在局域网机器上；仍扫不到时按下面"手工填写"取 `Location` URL |
| 切到 DLNA 输出后总是从头播放 | 设备忽略了播放刚开始时的 `Seek`（实测小爱 S12 在 `Play` 立刻 `Seek` 时返回 200 但不跳转，约 3 秒后同样调用才生效） | 当前版本驱动会校验回报位置并重发 seek（最多 3 次）；若设备完全不回报位置则只发一次，属设备限制 |
| 设备被识别为 `xiaomi` 类型时进度条不动 | miIO 文本指令播放无进度回报 | 预期行为；改用 DLNA 接口可得 `GetPositionInfo` |
| 选 MPD 后播放 502，body 是 `directory or file ... not found` | MPD 曲库里没有这首歌 | `mpc update` 或开 `auto_update`；确认 `music_directory` 与 `MusicFolder` 是同一份内容 |
| 选 MPD 后 502，body 是 `mpd: no local media path for output ...` | 该条目只有流地址没有本地文件（网络电台） | MPD 输出播不了电台，改用 DLNA/浏览器 |
| MPD 命令都成功但没声音 | `add` 的路径与 MPD 曲库不符，或 `audio_output` 指向了别的声卡 | 用假 MPD 看 `add "..."` 的确切相对路径（见教程 ⑨），再核对 MPD 侧目录 |
| MPD 报认证/权限失败 | `Password` 与 `mpd.conf` 的 `password = "...@read,add,control"` 不匹配 | 密码留空即匿名连接；权限需含 `add,control` |
| 音量刷新后变成 0%（或切设备后一直是 0%） | 驱动在设备首次回答音量查询前回报 0，被界面采纳并持久化 | 已由"忽略设备回报 0 + 读取持久化时把 0 换成默认值"修复；若复现，先查 `/api/jukebox/status` 的 `volume` 与 localStorage 的 `state.player.volume` |
| 拖滑块后设备音量不动 | 当前是浏览器输出（`AdminOnly` 下非管理员被 403），或 `/control volume` 失败 | 看同时间戳的驱动错误（502 body 是设备原文）；确认选中设备与登录用户权限 |
| 键盘 `Vol+/Vol-` 没反应 | 快捷键绕过了 store 直接写 `<audio>`，被"音量权威"effect 夺回 | 现状已修正为走 `setVolume`；二次开发时不要改回直接赋值（见[音量模型](#音量模型)） |
| 用第三方 App（Substream 等）的音量/播放键控制的是另一套设备 | Subsonic `jukeboxControl` 走的是**上游** `core/playback`（mpv 本机播放），与本 fork 的 `core/jukebox` 无关 | 不是 bug：网页输出切换只影响 `/api/jukebox/*`。要给第三方 App 用局域网音箱，需要把它们合并（见 `AGENTS.md` 第 4 节） |

## 已知限制

- DLNA 渲染器的 seek/进度依赖 `GetPositionInfo`，部分设备回报不准（实测小爱 S12
  的 `RelTime` 会滞后若干秒）；小米音箱一次拉走整个文件而不是边下边播
- 渲染器可能接受 `Seek` 却忽略它（见上表）。驱动以"回报位置是否到达目标"为准
  判断是否重发，最多 `dlnaSeekAttempts` 次，之后记 warning 并保持设备自身位置
- SSDP 扫描用标准 `MAN: "ssdp:discover"` 同时探测 `AVTransport:1` 服务与
  `MediaRenderer:1` 设备类型，按 `Location` 去重（同一台设备会回多条）。
  不响应 M-SEARCH 的设备需要手工填 `Address`：用抓 `NOTIFY` 包或路由器
  设备列表里的 `http://<ip>:<port>/<UDN>.xml`
- DLNA 描述文档路径没有统一约定（`/rootDesc.xml`、`/description.xml`、
  `/<UDN>.xml` 都有设备在用）。`Address` 只填 host:port 时驱动会依次探测前三个，
  全部失败才回退到约定的 `/AVTransport/control` 并记录 warning
- MPD 输出走"本地文件"，所以 MPD 的 `music_directory` 必须与 Navidrome 的
  `MusicFolder` 指向同一份内容，且歌曲要被 MPD 收录（`mpc update` / `auto_update`）；
  `PathFrom`/`PathTo` 只能做相对路径的字符串替换，不能替代上面两步
- xiaomi 驱动（`driver_xiaomi.go`）：不支持 seek（返回 `ErrInvalidCommand` → 400）；
  `Stop` 以 Pause 近似；无进度回报（`currentTime` 恒 0，音箱播完自动停止时前端按
  `stopped` 推进队列）；音量需本地 token；播放走云端或本地文本指令（`播放 <url>`）。
  型号差异与协议细节见 [小米音箱原生协议调研](xiaomi-speakers.md)
