---
name: build-and-test
description: 构建、测试与 lint 本 Navidrome fork 的标准命令（Go build tags、Ginkgo、Vitest、ESLint/Prettier）。修改任何 Go 或 ui/ 代码后必须按此验证。
---

# 构建与测试

## Go

```bash
# 跑指定包测试（必须带 build tags，否则编译失败）
make test PKG=./core/jukebox
make test PKG=./server/nativeapi
make test PKG=./core/lyrics

# 等价裸命令
go test -tags netgo,sqlite_fts5 ./core/jukebox ./server/nativeapi ./core/lyrics

# 全量（Go + JS + i18n）
make testall

# lint（golangci-lint，配置在 .golangci.yml）
make lint
gofmt -l core/jukebox server/nativeapi core/lyrics conf   # 必须无输出
```

- 测试框架：Ginkgo v2 + Gomega（suite 文件 `*_suite_test.go`）
- 格式化：`gofmt` / `goimports`（`make format` 会同时跑 JS prettier 和 `go mod tidy`）

## 前端（ui/）

```bash
cd ui
npm run test        # Vitest（89 文件 / 778 用例）
npx vitest run src/audioplayer/VolumeControl.test.jsx   # 只跑音量测试
npx vitest run src/audioplayer/TranslateButton.test.jsx # 只跑歌词翻译测试
npm run lint        # ESLint，--max-warnings 0
npm run prettier    # 格式化 ./src
npm run check-formatting  # 只检查不写入（CI 用）
npx prettier --check src/audioplayer/Player.jsx         # 只查改过的文件
npm run build       # vite 生产构建，产物 ui/build/
```

前端产物被 `go:embed` 打进二进制：**改了任何 `ui/src` 都要 `npm run build` 之后
重新 `go build`**，否则跑的还是旧界面，容易误判成"我的改动没生效"。

## Docker 打包与推送（发布 linux/amd64 镜像）

```bash
# 本地起 colima (macOS 环境)
colima start

# 打包 linux/amd64 镜像并推送到 Docker Hub (huhan333/navidrome2all:latest)
docker buildx build --platform linux/amd64 \
  --build-arg GIT_SHA=$(git rev-parse --short HEAD) \
  --build-arg GIT_TAG=v0.55.0-fork \
  --target final \
  -t huhan333/navidrome2all:latest \
  -t huhan333/navidrome2all:$(git rev-parse --short HEAD) \
  --push .
```

## 起实例与调 API

- 同一时间**只保留一台**实例、一个 URL（多开会让"到底哪个在响"变成玄学）。
  `nohup` 起的进程可能被上层工具回收；验证前先 `curl -s -o /dev/null -w '%{http_code}' <url>/app`。
- native API 鉴权头是 `X-ND-Authorization: Bearer <token>`（`ui/src/dataProvider/httpClient.js`），
  **cookie 不管用**。token 来自 `POST /auth/login`（JSON body，取响应 `token` 字段）。
- 登录态属于用户：**不要自己注册账号，也不要从数据库/日志/会话记录里翻密码**。
  需要真人登录时把实例和 URL 交给用户。
- `BaseUrl` 不配的话，下发给音箱的流地址是 `localhost`，日志会出现
  `Jukebox stream URL points at the loopback interface`——这是"选了设备但没声音"的头号原因。

## 验收清单（改完代码后）

1. 相关 Go 包测试全绿（带 tags）
2. `cd ui && npm run test && npm run lint && npm run check-formatting` 通过
3. `gofmt -l` / prettier 无 diff
4. 如涉及 UI 构建产物：`cd ui && npm run build` + `go build` 通过
5. 涉及音量：确认所有改音量的入口都走 `dispatch(setVolume(...))`，
   没有新的地方直接写 `audioInstance.volume`（会被 store 权威 effect 夺回）
6. 涉及界面：真实登录 + 移动视口点一遍（详见 AGENTS.md 第 7 节）
