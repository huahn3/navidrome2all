#!/bin/zsh
# Navidrome 本地启动脚本
# 用法: ./start.sh
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

pkill -f 'bin/navidrome' 2>/dev/null && echo "→ 停止旧实例" || true
sleep 1

echo "→ 启动 Navidrome (端口 14534)..."
ND_PORT=14534 \
ND_DATAFOLDER=. \
ND_MUSICFOLDER=./music \
ND_LOGLEVEL=info \
ND_JUKEBOX_ENABLED=true \
ND_BASEURL="http://$(ipconfig getifaddr en0 2>/dev/null || echo localhost):14534" \
nohup ./bin/navidrome > /tmp/navidrome.log 2>&1 &
disown
PID=$!
echo "→ PID=$PID"
sleep 3

if curl -s -L -o /dev/null -w "%{http_code}" http://localhost:14534/ 2>/dev/null | grep -q "200\|302"; then
  echo "✅ 启动成功: http://localhost:14534"
  echo "   日志: tail -f /tmp/navidrome.log"
else
  echo "❌ 启动失败，查看日志: cat /tmp/navidrome.log"
  exit 1
fi
