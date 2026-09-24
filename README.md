<a href="https://www.navidrome.org"><img src="resources/logo-192x192.png" alt="Navidrome logo" title="navidrome" align="right" height="60px" /></a>

# Navidrome Music Server &nbsp;[![Tweet](https://img.shields.io/twitter/url/http/shields.io.svg?style=social)](https://twitter.com/intent/tweet?text=Tired%20of%20paying%20for%20music%20subscriptions%2C%20and%20not%20finding%20what%20you%20really%20like%3F%20Roll%20your%20own%20streaming%20service%21&url=https://navidrome.org&via=navidrome)

[![Last Release](https://img.shields.io/github/v/release/navidrome/navidrome?logo=github&label=latest&style=flat-square)](https://github.com/navidrome/navidrome/releases)
[![Build](https://img.shields.io/github/actions/workflow/status/navidrome/navidrome/pipeline.yml?branch=master&logo=github&style=flat-square)](https://nightly.link/navidrome/navidrome/workflows/pipeline/master)
[![Downloads](https://img.shields.io/github/downloads/navidrome/navidrome/total?logo=github&style=flat-square)](https://github.com/navidrome/navidrome/releases/latest)
[![Docker Pulls](https://img.shields.io/docker/pulls/deluan/navidrome?logo=docker&label=pulls&style=flat-square)](https://hub.docker.com/r/deluan/navidrome)
[![Dev Chat](https://img.shields.io/discord/671335427726114836?logo=discord&label=discord&style=flat-square)](https://discord.gg/xh7j7yF)
[![Subreddit](https://img.shields.io/reddit/subreddit-subscribers/navidrome?logo=reddit&label=/r/navidrome&style=flat-square)](https://www.reddit.com/r/navidrome/)
[![Contributor Covenant](https://img.shields.io/badge/Contributor%20Covenant-v2.0-ff69b4.svg?style=flat-square)](CODE_OF_CONDUCT.md)
[![Gurubase](https://img.shields.io/badge/Gurubase-Ask%20Navidrome%20Guru-006BFF?style=flat-square)](https://gurubase.io/g/navidrome)

Navidrome is an open source web-based music collection server and streamer. It gives you freedom to listen to your
music collection from any browser or mobile device. It's like your personal Spotify!

---

## 本 fork 的二次开发功能

本仓库基于 Navidrome 上游（基线 commit `ee6dd1bc`）开发，git 历史已重置为面向二开的干净起点，
与上游没有共同祖先（跟进上游的流程见 [docs/fork-and-upstream.md](docs/fork-and-upstream.md)）。

- **多输出端播放器控制**：在网页播放器中切换出声设备（浏览器 / MPD / DLNA 渲染器 /
  小爱音箱），配 SSDP 一键扫描与网页版输出设备管理，见 [docs/jukebox.md](docs/jukebox.md)
- **统一音量管理**：浏览器输出与远程设备共用一个音量（感知值，元素按平方映射、
  设备按 0-100 下发），并与设备实际音量双向同步；桌面与移动端同一套逻辑，
  移动端不再被强制最大音量
- **小米音箱原生协议驱动**（`type = "xiaomi"`，miIO 本地控制 + MIoT 云端文本指令；
  支持 Redmi 小爱音箱 Play L7A 等无 DLNA 型号；本地 miIO 路径仍待真机验证），
  见 [docs/xiaomi-speakers.md](docs/xiaomi-speakers.md)
- **NAS 部署指南**（飞牛 fnOS / Docker / MPD 声卡直通 / 输出设备配置 / 自检），
  见 [docs/jukebox-nas-deployment.md](docs/jukebox-nas-deployment.md)
- E2E 测试桩（假 MPD / 假 DLNA）：[contrib/jukebox-testing](contrib/jukebox-testing)
- **AI / 二次开发接手文档**：仓库根 [AGENTS.md](AGENTS.md)（命令、代码地图、音量不变量、
  两套 jukebox 的区别、已知坑、验收清单），配套技能在 `.claude/skills` 与 `.qoder/skills`

---


**Note**: The `master` branch may be in an unstable or even broken state during development. 
Please use [releases](https://github.com/navidrome/navidrome/releases) instead of 
the `master` branch in order to get a stable set of binaries.

## [Check out our Live Demo!](https://www.navidrome.org/demo/)

__Any feedback is welcome!__ If you need/want a new feature, find a bug or think of any way to improve Navidrome, 
please file a [GitHub issue](https://github.com/navidrome/navidrome/issues) or join the discussion in our 
[Subreddit](https://www.reddit.com/r/navidrome/). If you want to contribute to the project in any other way 
([ui/backend dev](https://www.navidrome.org/docs/developers/), 
[translations](https://www.navidrome.org/docs/developers/translations/), 
[themes](https://www.navidrome.org/docs/developers/creating-themes)), please join the chat in our 
[Discord server](https://discord.gg/xh7j7yF). 

## Installation

See instructions on the [project's website](https://www.navidrome.org/docs/installation/)

## Cloud Hosting

[PikaPods](https://www.pikapods.com) has partnered with us to offer you an 
[officially supported, cloud-hosted solution](https://www.navidrome.org/docs/installation/managed/#pikapods). 
A share of the revenue helps fund the development of Navidrome at no additional cost for you.

[![PikaPods](https://www.pikapods.com/static/run-button.svg)](https://www.pikapods.com/pods?run=navidrome)

## Features
 
 - Handles very **large music collections**
 - Streams virtually **any audio format** available
 - Reads and uses all your beautifully curated **metadata**
 - Great support for **compilations** (Various Artists albums) and **box sets** (multi-disc albums)
 - **Multi-user**, each user has their own play counts, playlists, favourites, etc...
 - Very **low resource usage**
 - **Multi-platform**, runs on macOS, Linux and Windows. **Docker** images are also provided
 - Ready to use binaries for all major platforms, including **Raspberry Pi**
 - Automatically **monitors your library** for changes, importing new files and reloading new metadata 
 - Supports **lyrics** from sidecar .ttml, .yaml/.yml Lyricsfile, .elrc, .lrc, .srt, .txt files and embedded TTML, Enhanced LRC, LRC, SRT, and plain-text tags (via `lyricspriority`)
 - **Themeable**, modern and responsive **Web interface** based on [Material UI](https://material-ui.com)
 - **Compatible** with all Subsonic/Madsonic/Airsonic [clients](https://www.navidrome.org/docs/overview/#apps)
 - **Transcoding** on the fly. Can be set per user/player. **Opus encoding is supported**
 - Translated to **various languages**

## Translations

Navidrome uses [POEditor](https://poeditor.com/) for translations, and we are always looking 
for [more contributors](https://www.navidrome.org/docs/developers/translations/)

<a href="https://poeditor.com/"> 
<img height="32" src="https://github.com/user-attachments/assets/c19b1d2b-01e1-4682-a007-12356c42147c">
</a>

## Documentation
All documentation can be found in the project's website: https://www.navidrome.org/docs. 
Here are some useful direct links:

- [Overview](https://www.navidrome.org/docs/overview/)
- [Installation](https://www.navidrome.org/docs/installation/)
  - [Docker](https://www.navidrome.org/docs/installation/docker/)
  - [Binaries](https://www.navidrome.org/docs/installation/pre-built-binaries/)
  - [Build from source](https://www.navidrome.org/docs/installation/build-from-source/)
- [Development](https://www.navidrome.org/docs/developers/)
- [Subsonic API Compatibility](https://www.navidrome.org/docs/developers/subsonic-api/)

## Screenshots

<p align="left">
    <img height="550" src="https://raw.githubusercontent.com/navidrome/navidrome/master/.github/screenshots/ss-mobile-login.png">
    <img height="550" src="https://raw.githubusercontent.com/navidrome/navidrome/master/.github/screenshots/ss-mobile-player.png">
    <img height="550" src="https://raw.githubusercontent.com/navidrome/navidrome/master/.github/screenshots/ss-mobile-album-view.png">
    <img width="550" src="https://raw.githubusercontent.com/navidrome/navidrome/master/.github/screenshots/ss-desktop-player.png">
</p>
