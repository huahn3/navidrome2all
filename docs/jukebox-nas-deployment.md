# 在 NAS 上部署（飞牛 fnOS 实测路径）

本文覆盖从"这台 fork 的镜像"到"局域网里真的出声"的全过程，含 fnOS 的具体操作位置。
配置项与故障对照表见 [jukebox.md](jukebox.md)，本文只讲部署。

> 文中命令都在 SSH 里执行：fnOS 的设置中开启 SSH（不同版本菜单名略有差异，找"SSH / 终端"一项），
> 用管理员账号密码登录，需要 root 时 `sudo -i`。fnOS 是 Debian 底座，
> `docker` / `docker compose` / `apt` 命令与通用 Linux 一致。
>
> 本文中标注"实测"的结论来自本机真机验证（小爱音箱 DLNA + 假 MPD）；
> 其余 fnOS 界面名称可能随版本变化，以你机器上的实际菜单为准。
>
> 需要提前说明的是：飞牛帮助中心并没有 Docker、SSH、静态 IP、音频输出的官方文档，
> 下文中这些操作位置来自社区经验的交集，属于"社区级"事实而非厂商承诺。

## 0. 先决定：要哪种出声方式

| 目标 | 需要部署什么 | 难度 | 前置条件 |
|---|---|---|---|
| 只有浏览器出声（手机/电脑） | 仅 Navidrome | 1 分钟 | 无（`Enabled = true` 即可） |
| 局域网智能音箱 / 功放 | Navidrome + 音箱 | 低 | 音箱支持 DLNA；`BaseUrl` 配成局域网地址 |
| NAS 本机声卡（3.5mm / HDMI / USB 声卡） | Navidrome + MPD | 中 | 主板确有可用的音频输出设备 |
| 不支持 DLNA 的小爱音箱 | Navidrome + token | 中 | 拿到 miIO token，见 `docs/xiaomi-speakers.md` |

**先确认 NAS 到底有没有声卡**（很多 NAS 主板没有音频口，此时 MPD 方案不成立）：

```bash
cat /proc/asound/cards      # 列出内核识别到的声卡
aplay -l                    # 列出可播放的 ALSA 设备（hw:0,0 之类）
```

两行都为空 → 只能走 DLNA/小米，或插一个 USB 声卡再试（`alsa-utils` 缺失时先 `apt install`）。

飞牛是无头 NAS 系统，**没有桌面音频栈**（默认没有 PulseAudio/PipeWire，不要去挂
`/run/user/1000/pulse` 之类的 socket），直连 ALSA 的 `/dev/snd` 是唯一可靠路径。
板载/核显 HDMI 音频与机型强相关，社区有 N5095 平台 HDMI 音频识别缺失的反馈，
所以"能不能出声"只能以 `aplay` 实测为准。

## 1. 构建本 fork 的镜像

官方 `deluan/navidrome:latest` **不含**本 fork 的多输出端功能，必须用自己构建的镜像。

### 方式 0：直接拉现成镜像（最省事）

本 fork 已推送到 Docker Hub：

```bash
docker pull huhan333/navidrome2all:latest      # 目前只有 linux/amd64
```

**先在 NAS 上 `uname -m` 确认架构**：`x86_64` 才能用这个 tag，`aarch64` 需要另建 arm64 镜像
（见方式 A，把 `IMAGE_PLATFORMS` 换成 `linux/arm64`）。
注意飞牛应用中心里那个一键 Navidrome 装的是上游镜像，没有输出设备页面，而且同样占 4533；
装过就要先停，否则本 fork 的容器 `bind: address already in use`。

### 方式 A：在 NAS 上直接构建（省去镜像搬运）

```bash
sudo -i
mkdir -p /vol1/1000/docker/navidrome2all && cd $_
# 把仓库弄上来：git clone 你的 fork，或本机 scp 过去
git clone <你的仓库地址> src && cd src
IMAGE_PLATFORMS="linux/amd64" DOCKER_TAG="navidrome2all:local" make docker-image
```

`IMAGE_PLATFORMS` **必须指定为单一平台**（Intel/AMD 机型 `linux/amd64`，
ARM 机型 `linux/arm64`，可用 `make docker-platforms` 查支持列表）：
Makefile 的默认值是多架构列表，那种构建不会把镜像导入本机 docker。
构建很慢（要编译前端 + CGO SQLite），十几分钟正常。

### 方式 B：本机交叉编译后导入（NAS 连不上 Docker Hub 时才用）

国内网络下 NAS 直连 Docker Hub 经常拉不动，此时在开发机构建后 `docker save` 管道过去：

```bash
# 开发机
IMAGE_PLATFORMS="linux/amd64" DOCKER_TAG="navidrome2all:local" make docker-image
docker save navidrome2all:local | ssh admin@<NAS_IP> 'sudo -i docker load'
```

验证镜像可用：

```bash
docker run --rm navidrome2all:local --version     # 打印本 fork 的版本号即说明镜像可用（期望输出形如 `0.0.0-SNAPSHOT (<git sha>)`）
```

## 2. 部署 Navidrome

飞牛 **Docker** 应用里左侧的 **"Compose"**（部分版本叫 **"项目"**）→ **新增项目**：
项目名随意，"路径"选一个存储目录（如 `/vol1/1000/docker/navidrome2all`，
路径不要用中文），把下面的 YAML 粘进去，改两处（音乐目录、`ND_BASEURL` 里的 IP）再点启动。

```yaml
services:
  navidrome:
    image: huhan333/navidrome2all:latest    # 方式 0；本地构建则写 navidrome2all:local
    container_name: navidrome
    restart: unless-stopped
    # 推荐：用 host 网络，"扫描局域网"才能发现音箱（原因见下方第 2 点）
    network_mode: host
    volumes:
      - /vol1/1000/music:/music:ro          # 你的音乐目录
      - /vol1/1000/docker/navidrome2all/data:/data
    environment:
      ND_JUKEBOX_ENABLED: "true"
      ND_JUKEBOX_ADMINONLY: "true"
      # 必须是音箱能访问到的地址：NAS 的局域网 IP + 真实端口，不能是 localhost
      ND_BASEURL: "http://192.168.31.246:4533"
      ND_MUSICFOLDER: "/music"
      ND_DATAFOLDER: "/data"
```

三个最容易填错的地方：

1. **`ND_BASEURL` 的主机**就是音箱实际去拉流的地址。填成 `localhost`/`127.0.0.1` 的症状是
   "设备显示已选中、进度在走、但一点声音都没有"，服务端日志会给出
   `Jukebox stream URL points at the loopback interface`。用 `ip addr` 或路由器后台确认 NAS 的 IP，
   并建议在路由器上给它做静态 DHCP 绑定，否则 IP 一变全部失效。
2. **`host` 网络模式下不要再写 `ports:`**，那是无效的（且会让人误以为端口变了）。
   为什么推荐 host：SSDP 扫描是靠多播完成的，容器在桥接网络里发的多播经过 NAT，
   多数渲染器不会应答，于是"扫描局域网"没有结果。这一项不影响播放本身。
   若必须用桥接网络（例如和别的应用共用端口 4533），改成：
   ```yaml
   # ports:
   #   - "4533:4533"
   # ND_BASEURL 仍写 http://<NAS_IP>:4533（映射后的对外端口）
   ```
   此时 DLNA 的"扫描局域网"可能没有结果——按 [jukebox.md](jukebox.md) 的说明手填
   描述文档 URL 即可，播放本身不受影响。
   host 模式下容器与飞牛共用端口空间，要避开 **5666/5667**
   （飞牛 Web UI）和应用中心各应用声明的端口；被占时容器日志是 `bind: address already in use`。
   不确定就 `ss -ltnp | grep 4533` 先看一眼。
3. **`/music` 建议 `:ro` 只读**。Navidrome 只需要读取；只读也避免误改曲库。
   若用了 Music Tag Web 之类要写标签的工具，再单独放开写权限。

### 权限（`user: 1000:1000` 报 permission denied 时）

社区常见写法是给容器指定 `user: 1000:1000`。飞牛的共享目录属主往往就是 `1000`，
但**新创建的 `/data` 子目录可能属 root**，此时数据库写不进去：

```bash
sudo mkdir -p /vol1/1000/docker/navidrome2all/data
sudo chown -R 1000:1000 /vol1/1000/docker/navidrome2all/data
```

不确定就先不写 `user:`（默认 root 运行，最省事），跑通后再收紧。

这条路在飞牛上走得通：官方说明 v1.2.0 起共享文件夹改用 Windows ACL，
但 **Docker 目录和部分应用目录仍保持 POSIX ACL**，所以上面的 `chown 1000:1000`
在 `/vol1/1000/docker/...` 下是有效的。UI 里"文件右键 → 权限"那一套只管 SMB 侧，
不要指望它调容器内的读写权限。

## 3. 首次启动与管理员

```bash
cd /vol1/1000/docker/navidrome2all && docker compose up -d
docker logs -f navidrome          # 看到 "Navidrome server is ready!" 即可 Ctrl-C
```

浏览器打开 `http://<NAS_IP>:4533`，**第一次访问会要求创建管理员账号**（此时没有默认密码）。
创建后：管理 → 输出设备 → 新增，即可添加远程输出。

看到工具栏出现"输出设备"图标 = `ND_JUKEBOX_ENABLED` 生效；
看不到就是开关没生效（`docker exec navidrome env | grep ND_` 核对）。

如果开了飞牛的防火墙（设置 → 安全性 → 防火墙，默认关闭），给局域网段放行 4533：
**音箱拉流也是入站到 4533 的请求**，只放通浏览器会让音箱拿不到音频。
走 FN Connect 等隧道从外网访问时，`ND_BASEURL` 仍是局域网地址，
远程浏览器能听但局域网外的音箱拉不到流——多输出端按局域网场景设计，勿作外网方案。

## 4. 加一台 MPD 让 NAS 声卡出声

> 社区里**没有**飞牛专用的 MPD 教程，本节是通用 Debian/Docker 做法套在飞牛上，
> 未在本机实测过（本 fork 的 MPD 驱动逻辑是用假 MPD 服务验证的，见 [jukebox.md](jukebox.md)）。

驱动传给 MPD 的是**相对音乐库根目录**的路径（实测入参就是 `add "test-song.mp3"`），
所以 MPD 的 `music_directory` 与 Navidrome 的 `MusicFolder` 只要指向同一份内容即可对齐，
不需要任何路径重写。

### 先决定 MPD 跑在哪

| 方式 | 优点 | 代价 |
|---|---|---|
| **宿主机 apt 装 MPD**（推荐） | 没有声卡权限问题，`Address = 127.0.0.1:6600` 直连 | 多一个系统服务；系统升级可能带更新 |
| MPD 容器 | 与系统隔离、随 compose 一起启停 | 必须直通 `/dev/snd` 并处理 audio 组 GID |

飞牛是 Debian 底座，直接：

```bash
sudo apt update && sudo apt install -y mpd mpc alsa-utils
```

### mpd.conf（两种方式通用）

Debian 的 MPD 默认用 `/etc/mpd.conf`，且默认以 `mpd` 用户运行。要点：

```
music_directory    "/vol1/1000/music"   # 与 Navidrome 挂进容器的音乐目录同一个宿主路径
playlist_directory "/var/lib/mpd/playlists"
db_file            "/var/lib/mpd/database"
log_file           "/var/log/mpd/mpd.log"
state_file         "/var/lib/mpd/state"
bind_to_address    "127.0.0.1"
port               "6600"
auto_update        "yes"
auto_update_depth  "4"

audio_output {
  type   "alsa"
  name   "NAS"
  device "hw:0,0"          # 取 aplay -l 输出里的卡片号
}
```

- `auto_update "yes"` 很关键：Navidrome 的扫描不会让 MPD 收录歌曲，
  缺这一步时表现为 502，body 里是 MPD 原文 `directory or file ... not found`。
- Debian 的 `mpd` 用户默认不在 `audio` 组里，不出声时先查这个：
  ```bash
  sudo adduser mpd audio && sudo systemctl restart mpd
  ```
- 若 MPD 与 Navidrome 不同机，需要放开监听并配 `password`：
  `bind_to_address "0.0.0.0"`（或 `any`）+
  `password = "你的密码@read,add,control"`，网页里 `Password` 填同一个值。
- 容器方式：可用的现成镜像有 `tobi312/rpi-mpd:alpine`（Tob1as/docker-mpd，配置文件在
  `/etc/mpd.conf`）、`giof71/mpd-alsa`、`johngong/mpd`；**`linuxserver/mpd` 已经不存在**
  （Docker Hub 与文档站都返回 404），别照旧教程抄它。把上面的路径换成容器内的
  （`music_directory "/music"`、`/config` 下放 database/state），compose 里加
  ```yaml
      network_mode: host              # 否则 Navidrome 连不到它的 6600
      devices:
        - /dev/snd:/dev/snd           # 直通声卡（挂目录会漏掉 control 节点）
      group_add:
        - "<stat -c %g /dev/snd 的输出>"   # 让容器进程有权限操作声卡设备
  ```
  缺 `group_add` 时 MPD 的报错通常是 `cannot open ALSA device "hw:0,0": Permission denied`
  或 `cannot find card '0'`，与本项目无关。

### 在网页里添加设备

`Type = MPD 服务器`、`Address = 127.0.0.1:6600`（跨机时填对应 host:port）、
`Password` 按上一步、`Path from/to` 留空。

### 验证 MPD 这一环

出声失败时先分清是"控制没通"还是"曲库没这首歌"：

```bash
mpc update && mpc play          # 或 mpc -h <host>:6600 ...
mpc status                      # 期望 state: play
aplay -l                        # 确认设备号与 mpd.conf 的 device 一致
```

`mpc play` 有声音而 Navidrome 里播放失败 → 是路径/曲库问题（`mpc listall | head`
看 MPD 眼里的相对路径，和 502 body 里 `add` 的路径对一下）。
`mpc play` 也没声音 → 是 `audio_output` / 声卡权限问题，与本项目无关。


## 5. 部署后自检

仓库自带一个只读自检脚本（不会改变选中设备或播放状态），从 NAS 上或任意同网段机器跑：

```bash
bash .qoder/skills/nas-jukebox-deploy/scripts/preflight.sh \
  --base http://<NAS_IP>:4533 --user <管理员> --password '<密码>' \
  --mpd 127.0.0.1:6600 \
  --dlna http://<音箱IP>:<port>/<UDN>.xml \
  --song <一首存在的歌曲ID>
```

5 项分别对应：Web API 可达 → 登录 → 设备列表（能区分"开关没开"）→ MPD 协议握手 →
DLNA 描述文档含 AVTransport → 用 Subsonic 签名模拟音箱拉流。
**最后一项是判断 `BaseUrl` 是否真的可用的决定性检查**：它返回 200/206 才说明音箱拿得到音频。

歌曲 ID 可在网页播放时从地址栏 `.../app/all/songs/xxx/play` 中取，
或用管理 API 查询。DLNA 的描述文档 URL 从"扫描局域网"结果里复制；扫不到时
按 [jukebox.md 的已知限制](jukebox.md#已知限制) 从路由器设备列表或 `NOTIFY` 包里取。

## 6. 验收清单（真机）

1. 浏览器播放一首歌 → 切到目标设备 → **同一首歌在同一位置继续出声**
2. 拖动进度条 → 约 1~3 秒内设备跟着跳（小爱类设备靠驱动重发 seek 才生效）
3. 暂停/继续/音量 → 设备端表现一致；直接在设备上按键 → 网页状态几秒内跟随
4. 播放一整曲后自动进入下一首（队列继续推进）
5. 切回"浏览器" → 远程设备静音/停止，本地恢复出声
6. 刷新页面、重启容器后重新选择设备 → 能正常播放（`ensureSelected` 生效）
7. 非管理员账号：能听、能看到设备，但不能改输出设备配置（`AdminOnly`）

## 7. 卸载 / 回滚

数据只在 `/data`（SQLite + 封面 + 日志）与你的音乐目录（只读）。
`docker compose down` 停服务；彻底清除：删容器 + 删 `/vol1/1000/docker/navidrome2all/data`。
音乐文件不受影响。回滚到官方版本只需把 `image:` 换回 `deluan/navidrome:latest`
（此时多输出端功能消失，但曲库/账号照常可用）。
