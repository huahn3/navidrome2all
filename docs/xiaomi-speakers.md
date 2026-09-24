# 小米音箱原生协议调研与实施方案

> 调研日期：2026-09-24。目标设备：小爱同学一代（小米AI音箱 S12）、
> Redmi 小爱音箱 Play（L7A）。结论先行：**S12 支持 DLNA，现有 DLNA 驱动直接可用**；
> **L7A 不支持 DLNA，需要 miIO/MIoT 原生协议驱动**。

## 1. DLNA 支持情况（小米官方手册）

官方支持 DLNA 的型号：小米AI音箱、小爱音箱、小爱音箱Pro、小爱音箱Art、小爱音箱HD。
开启方式：小爱音箱 App → 我的 → DLNA 音乐播放（开关）。

| 设备 | MIoT 型号 | DLNA | 结论 |
|---|---|---|---|
| 小爱同学一代（小米AI音箱） | `xiaomi.wifispeaker.s12` | ✅ 支持 | 用现有 `type = "dlna"` 驱动即可 |
| Redmi 小爱音箱 Play | `xiaomi.wifispeaker.l7a` | ❌ 不在列表 | 需原生协议驱动（见下文） |

DLNA 模式限制：DLNA 播放时不能用语音切歌（但语音可控制播放/暂停）。

## 2. 两台设备的 MIoT-Spec 服务树（实测于 home.miot-spec.com）

### Redmi 小爱音箱 Play（xiaomi.wifispeaker.l7a, urn ...xiaomi-l7a:1）

| 服务 | siid | 内容 |
|---|---|---|
| 扬声器 speaker | 2 | 音量 piid=1（uint8, 3~100）R/W/N；静音 piid=2 |
| 麦克风 microphone | 3 | 静音 piid=1 |
| 播放控制 play-control | 4 | 播放状态 piid=1（0=空闲, 1=播放中）R/N；动作：暂停 aiid=1、播放 aiid=2、下一首 aiid=3、上一首 aiid=4 |
| 智能音箱 intelligent-speaker | 5 | 文本内容 piid=1、指令静默执行 piid=2（uint8: 0=开启 1=关闭）；动作：**播放文本 aiid=1**（入参[text]）、唤醒 aiid=2、播放电台 aiid=3、播放音乐 aiid=4、**执行文本指令 aiid=5**（入参[text, silent]） |

### 小米AI音箱（xiaomi.wifispeaker.s12, urn ...xiaomi-s12:2）

结构相同：siid=2 音量(1~100)、siid=3 麦克风、siid=4 播放控制
（playing-state 多一个 2=Pause）、siid=5 智能音箱
（play-text aiid=1、**execute-text-directive aiid=5**，silent 为 bool）、siid=6 闹钟。

**注意：siid/aiid 因型号而异**（例如 L09A 的 play-text 是 3-1、L15A 是 7-3），
社区维护的对照表见 xiaomusic issue #365 与 songloft-plugin-miot issue #28。
驱动实现必须把每类动作的 siid/aiid 做成**可配置项**（带按型号的默认值）。

## 3. 两条通信通道

### A. 本地 miio（UDP 54321，需 16 字节设备 token，不需要小米账号）

- 协议：UDP + AES-128-CBC（key/iv 由 token MD5 派生），hello 握手拿 deviceId/stamp
- 通用 MIoT 方法：`get_properties` / `set_properties` / `action`（参数 `{did, siid, piid/aiid, value/in}`）
- 老设备 raw 方法：`player_play_operation`、`player_get_play_status`、`player_set_volume` 等
- Go 参考实现：`github.com/icepie/miio.go`（`New(ip).SetToken().SetDid()`、
  `GetProps` / `SetProps` / `DoAction` / `Send`）；python-miio 的 wifispeaker 模块
- **可用于：播放/暂停/上下首（siid=4 动作）、音量（siid=2）、播放状态（siid=4 piid=1）**
- 文本指令动作（siid=5）能否走本地通道**取决于固件，需真机实测**；
  社区主流（xiaomusic/mi-gpt）都走云端

### B. 小米云 MIoT（api.io.mi.com，需小米账号密码）

- 登录：`account.xiaomi.com/pass/serviceLogin?sid=xiaomiio` → ssecurity；
  之后所有 RPC 用 ssecurity 双重 MD5 签名
- 设备列表：`/home/device_list`（响应里直接带各设备的本地 token 与 did/model）
- 动作调用：`POST /app/miotspec/action`，body
  `{"params":{"did":...,"siid":5,"aiid":5,"in":["播放 http://192.168.1.5:4533/xx.mp3", true]}}`
- Python 参考：miservice；Go 参考：`github.com/lsongdev/miservice-go`（不成熟但可借鉴）
- **播放指定 URL 的已验证做法就是文本指令 `播放 <url>`**（xiaomusic 10k stars，
  明确支持 L7A 与 S12）

## 4. 关键限制（影响我们的 PlayerDriver 语义）

| 能力 | 支持度 | 说明 |
|---|---|---|
| Play(URL) | ✅（云端文本指令） | 文本指令 `播放 <url>`；URL 需音箱可达、建议带 `.mp3` 扩展名（小爱对无扩展名 URL 可能拒播） |
| Pause/Resume | ✅ | siid=4 aiid=1/2（MIoT 动作，本地/云皆可） |
| Stop | ⚠️ | spec 无 stop 动作 → 用 pause 近似 |
| Prev/Next | ✅ 设备支持，**本 fork 不暴露** | siid=4 aiid=4/3。播放队列由浏览器端持有，切歌由前端下发下一首的 `Play`；`PlayerDriver` 里没有 Next/Prev，驱动里也不要有（曾实现过，因不可达已删除） |
| Seek | ❌ | spec 无 seek 动作 → 驱动返回不支持，UI 禁用/忽略 |
| SetVolume | ✅ | siid=2 piid=1（L7A 3~100 / S12 1~100） |
| GetState | ⚠️ | 只有 空闲/播放中（S12 多 Pause）；**无进度、无曲名** → 返回缓存状态 + 设备 playing-state |
| 进度上报 | ❌ | 文本指令播放不回报进度 → 前端跳过漂移校准，进度条纯靠本地时钟（现有静音时钟方案天然适配） |

## 5. 实施方案（`type = "xiaomi"` 新驱动）

> **状态：已实现（待真机验证）。** 驱动为 `core/jukebox/driver_xiaomi.go`
> （+ `xiaomi_miio.go` 本地传输、`xiaomi_cloud.go` 云端登录/RPC）；
> 配置字段已落地为 `Token` / `DID` / `Model` / `Account` / `Password` / `TextDirective`；
> 扩展名别名端点为 Subsonic 侧的 `/rest/stream/{id}.mp3`（`StreamAlias`）；
> `/api/jukebox/status` 新增 `deviceType` 字段，前端据其跳过漂移校准与 seek 转发。

1. **流端点**（已实现）：`server/subsonic` 新增 `/rest/stream/{id}.mp3` 别名端点，
   id 从路径提取后走与 `/rest/stream` 完全相同的逻辑（Subsonic 签名鉴权不变）；
   xiaomi 驱动在 `Play` 时自动重写 URL。对 flac 等格式可复用 Navidrome 转码输出 mp3。
2. **`core/jukebox/driver_xiaomi.go`**（已实现）：
   - 本地 miio 传输层 `xiaomi_miio.go`（UDP 54321 + AES-128-CBC，零外部依赖）：
     hello 握手学习 did/stamp、`get_properties` / `set_properties` / `action`；
     控制/音量/状态全走本地
   - 可选云端客户端 `xiaomi_cloud.go`（仅用于 `Play(URL)` 的文本指令；
     passport 三步登录 + SHA1 签名调 `miotspec/action`；配置了账号时 Play 优先走云端）
   - `JukeboxOutputDevice` 新增字段：`Token`（本地）、`DID`、`Model`
     （决定 siid/aiid 默认值，内置 l7a/s12/l05b 表）、`Account`/`Password`（云端）、
     `TextDirective`（如 `"5-5"`，允许按型号覆盖）
   - `Seek` 返回包装 `ErrInvalidCommand` 的"不支持"错误（→400）；`Stop` 以 Pause 近似；
     `GetState` 返回 playing-state 映射 + 音量；Play 后 10 秒宽限期内忽略 idle 读数
     （音箱拉流缓冲中），无进度（currentTime 恒 0）
3. **`newDriver` 注册** `TypeXiaomi = "xiaomi"`（已实现）。
4. **前端**（已实现）：`/api/jukebox/status` 返回 `deviceType`；`Player.jsx` 在
   `deviceType === 'xiaomi'` 时跳过 `/status` 漂移校准与用户 seek 转发
   （设备无进度），进度条纯本地时钟；音箱播完回报 idle → 前端按 `stopped` 推进队列。
5. **测试**（已实现，仿 `driver_dlna_test.go` 模式）：
   - Go 内假 miio UDP 服务器（hello/加密包/属性与动作回显）测本地通道
   - httptest 假小米云测登录流程与请求签名校验
   - ~~`contrib/jukebox-testing/` 增加 `fake_xiaomi.py`~~（未做；Go 侧假设备已覆盖，
     真机联调更有意义）

### 配置（已落地）

```toml
[[Jukebox.Outputs]]
ID = "xiaoai-play"
Name = "Redmi 小爱音箱 Play"
Type = "xiaomi"
Address = "192.168.1.30"       # 音箱 IP（本地 miio，UDP 54321；纯云端模式也必须填一个地址字段）
Token = "00112233445566778899aabbccddeeff"  # 本地 miio token（32 hex）
DID = "123456789"              # 数字设备 ID；有 token 时可省略（握手自动学习），纯云端必填
Model = "l7a"                  # 决定 siid/aiid 默认值（l7a/s12/l05b；其他型号用 Play 系列默认 + TextDirective 覆盖）
# TextDirective = "5-5"        # 可选：execute-text-directive 的 siid-aiid 覆盖
# 云端（配置了 Account+Password 时 Play 优先走云端文本指令）：
Account = "13800000000"
Password = "..."
```

> 说明：`Address` 目前是必填字段（驱动工厂统一校验）。纯云端模式暂时也需填
> 音箱 IP（不会用到）；后续如需纯云端可放宽。

### 凭据获取

- 本地 token：米家 App 备份提取，或先用小米账号登录一次云端
  `/home/device_list` 自动拿到（可以做成 `navidrome doctor` 式的辅助命令）
- 账号密码仅存于服务端配置文件，绝不下发前端

### 风险

- 文本指令频繁调用可能被小米风控限流
- 固件升级可能改变本地文本指令的可用性 → 保留云端回退
- 播放 URL 是"指令"语义：音箱可能回一句语音确认（`silent-execution` 参数抑制）

## 6. 落地顺序建议

1. **S12（一代）**：小爱音箱 App 打开 DLNA 开关 → 现有 DLNA 驱动直接用，零新代码
2. **L7A（Redmi Play）**：`driver_xiaomi.go` **已实现**（本地 miio 控制/音量/状态 +
   云端文本指令播放）。**待真机验证**：
   - 本地 miio 握手/属性/动作是否被 L7A 固件接受（token 正确性）
   - 本地文本指令播放是否可行（可行则无需云端）；云端 `miotspec/action` 作为回退
   - `播放 <url>.mp3` 文本指令的实际播放行为与静默参数（l7a 为 uint8 0=开启静默）
   - 云端登录在真实账号下的表现（验证码/风控时 code≠0，会返回明确错误）

   真机验证所需：L7A 的 IP、miio token（32 hex）、（云端回退用）小米账号密码。

## 参考来源

- MIoT-Spec 设备库：home.miot-spec.com/spec/xiaomi.wifispeaker.l7a 与 .s12
- xiaomusic（github.com/hanxi/xiaomusic）及 issue #365（各型号 ttsCommand 对照表）
- 小米官方 HA 集成 wiki：play-text / execute-text-directive 参数语义
  （action 的 in 数组按 piid 顺序取值）
- songloft-plugin-miot issue #28（ttsCommand 型号表，含 L7A=5-1、S12=5-1）
- python-miio wifispeaker 模块（本地 miio raw 方法）
- icepie/miio.go、lsongdev/miservice-go（Go 协议参考）
