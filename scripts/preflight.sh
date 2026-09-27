#!/usr/bin/env bash
# Jukebox 输出链路部署自检：逐项检查 Web API、MPD、DLNA、局域网拉流。
# 只读操作（GET + 一次 HEAD/Range 请求），不会改变选中设备或播放状态。
#
# 用法：
#   ./preflight.sh --base http://192.168.1.10:4533 --user admin --password 'xxx' \
#       [--mpd 127.0.0.1:6600] [--dlna http://192.168.1.20:9999/rootDesc.xml] \
#       [--lan-host 192.168.1.10:4533] [--song <songId>] [--token <jwt>]
#
# --song 给定时才做"音箱能否拉到流"这一项（需 --user 及 --password 或 --subsonic-token/salt）；
# --lan-host 用来单独替换流地址的主机，以复现"音箱实际拿到的 URL"。
set -uo pipefail

BASE="" USER_NAME="" PASSWORD="" MPD="" DLNA="" LAN_HOST="" SONG="" TOKEN_ARG=""
SUBSONIC_TOKEN="" SUBSONIC_SALT=""

fail=0
ok() { printf '  \033[32mOK\033[0m   %s\n' "$1"; }
bad() { printf '  \033[31mFAIL\033[0m %s\n' "$1"; fail=1; }
skip() { printf '  --     %s\n' "$1"; }
note() { printf '\n\033[1m%s\033[0m\n' "$1"; }

while [ $# -gt 0 ]; do
	case "$1" in
	--base) BASE="$2"; shift 2 ;;
	--user) USER_NAME="$2"; shift 2 ;;
	--password) PASSWORD="$2"; shift 2 ;;
	--mpd) MPD="$2"; shift 2 ;;
	--dlna) DLNA="$2"; shift 2 ;;
	--lan-host) LAN_HOST="$2"; shift 2 ;;
	--song) SONG="$2"; shift 2 ;;
	--token) TOKEN_ARG="$2"; shift 2 ;;
	--subsonic-token) SUBSONIC_TOKEN="$2"; shift 2 ;;
	--subsonic-salt) SUBSONIC_SALT="$2"; shift 2 ;;
	-h | --help)
		sed -n '2,12p' "$0"
		exit 0
		;;
	*)
		echo "未知参数：$1" >&2
		exit 2
		;;
	esac
done

md5hex() {
	if command -v md5sum >/dev/null 2>&1; then
		printf '%s' "$1" | md5sum | cut -d' ' -f1
	else
		printf '%s' "$1" | md5 -q
	fi
}

json_field() { # json_field <field> <json> —— 取第一个匹配字段
	printf '%s' "$2" | grep -o "\"$1\"[[:space:]]*:[[:space:]]*\"[^\"]*\"" | head -1 | \
		sed 's/^[^:]*:[[:space:]]*"//; s/"$//'
}

note "1. Navidrome Web API"
if [ -z "$BASE" ]; then
	skip "未给 --base，跳过"
	TOKEN=""
else
	if ! curl -fsS -m 8 -o /dev/null "$BASE/"; then
		bad "$BASE 无法访问（容器未启动 / 端口未映射 / 防火墙）"
		TOKEN=""
	else
		ok "$BASE 可访问"
		TOKEN=""
		if [ -n "$TOKEN_ARG" ]; then
			TOKEN="$TOKEN_ARG"
			ok "使用 --token 传入的 JWT"
		elif [ -n "$USER_NAME" ] && [ -n "$PASSWORD" ]; then
			# 只处理 JSON 里必须转义的两个字符，避免依赖 jq/python
			juser=$(printf '%s' "$USER_NAME" | sed 's/\\/\\\\/g; s/"/\\"/g')
			jpass=$(printf '%s' "$PASSWORD" | sed 's/\\/\\\\/g; s/"/\\"/g')
			login=$(curl -fsS -m 8 -X POST "$BASE/auth/login" -H 'Content-Type: application/json' \
				-d "{\"username\":\"$juser\",\"password\":\"$jpass\"}" 2>/dev/null)
			TOKEN="$(json_field token "$login")"
			if [ -z "$TOKEN" ]; then
				bad "登录失败（用户名或密码错误）"
			else
				ok "登录成功，已拿到 JWT"
			fi
		else
			skip "未给 --user/--password，跳过需要登录的检查"
		fi
	fi
fi

if [ -n "$TOKEN" ]; then
	note "2. 输出设备（/api/jukebox/devices）"
	devices=$(curl -sS -m 8 "$BASE/api/jukebox/devices" -H "X-ND-Authorization: Bearer $TOKEN")
	case "$devices" in
	*jukebox\ is\ disabled*)
		bad "Jukebox.Enabled 未开启：navidrome.toml 加 [Jukebox] Enabled = true 后重启容器"
		;;
	*Not\ authenticated*)
		bad "JWT 不被接受：请确认用的是 --user/--password 登录返回的 token"
		;;
	*)
		ok "设备列表：$devices"
		;;
	esac
else
	skip "无 JWT，跳过设备列表检查（给 --user/--password 才能查）"
fi

note "3. MPD 连通性"
if [ -z "$MPD" ]; then
	skip "未给 --mpd host:port"
else
	host=${MPD%:*}
	port=${MPD##*:}
	greeting=$(
		(
			exec 3<>"/dev/tcp/$host/$port" || exit 1
			IFS= read -r -t 5 line <&3 || line=""
			printf '%s' "$line"
		) 2>/dev/null
	)
	case "$greeting" in
	OK\ MPD*) ok "$MPD 握手成功：$greeting" ;;
	"") bad "$MPD 连不上（MPD 未启动，或 bind_to_address 只监听 127.0.0.1 而两者不同机/不同容器网络）" ;;
	*) bad "$MPD 返回的不是 MPD 协议：$greeting" ;;
	esac
fi

note "4. DLNA 设备描述文档"
if [ -z "$DLNA" ]; then
	skip "未给 --dlna URL"
else
	desc=$(curl -fsS -m 8 "$DLNA" 2>/dev/null)
	if [ -z "$desc" ]; then
		bad "$DLNA 取不到描述文档（音箱离线 / IP 变了 / 端口不对）"
	else
		ok "描述文档可获取（$(printf '%s' "$desc" | wc -c | tr -d ' ') 字节）"
		case "$desc" in
		*AVTransport*) ok "声明了 AVTransport 服务，可作为输出设备" ;;
		*) bad "文档里没有 AVTransport：这台设备不是渲染器，不能当 DLNA 输出" ;;
		esac
	fi
fi

note "5. 局域网能否拉到流（音箱实际用的请求）"
if [ -z "$BASE" ]; then
	skip "未给 --base"
elif [ -z "$SONG" ]; then
	skip "未给 --song <歌曲ID>，跳过拉流检查"
elif [ -z "$USER_NAME" ]; then
	skip "需给 --user（以及 --password 或 --subsonic-token/--subsonic-salt）"
elif [ -z "$PASSWORD" ] && [ -z "$SUBSONIC_TOKEN" ]; then
	skip "需给 --password，或 --subsonic-token/--subsonic-salt"
else
	if [ -z "$SUBSONIC_TOKEN" ]; then
		SUBSONIC_SALT=$(printf '%s%s' "$RANDOM" "$RANDOM" | cut -c1-8)
		SUBSONIC_TOKEN=$(md5hex "$PASSWORD$SUBSONIC_SALT")
	fi
	# 音箱拿到的地址 = BaseUrl 的主机；这里用 --lan-host 复现它（缺省沿用 --base 的主机）
	scheme=${BASE%%://*}
	stream_host=${LAN_HOST:-${BASE#*://}}
	url="$scheme://$stream_host/rest/stream?id=$SONG&u=$USER_NAME&t=$SUBSONIC_TOKEN&s=$SUBSONIC_SALT&v=1.16.1&c=Preflight"
	code=$(curl -sS -m 10 -o /dev/null -w '%{http_code}' -H 'Range: bytes=0-1023' "$url" 2>/dev/null)
	case "$stream_host" in
	localhost* | 127.* | \[::1\]*)
		bad "流地址主机是 $stream_host：音箱会去访问它自己。--base 请用局域网 IP，并在配置里设 BaseUrl = \"http://<NAS的局域网IP>:<端口>\""
		;;
	esac
	if [ "$code" = "200" ] || [ "$code" = "206" ]; then
		ok "签名流可下载（HTTP $code）：$url"
	else
		bad "签名流不可用（HTTP ${code:-无响应}）：$url"
		echo "        检查账号密码、歌曲 ID 是否存在、以及该端口是否对局域网开放"
	fi
fi

printf '\n'
if [ "$fail" = "0" ]; then
	echo "全部检查通过。"
else
	echo "存在失败项，见上面的 FAIL。文档：docs/jukebox.md、docs/jukebox-nas-deployment.md"
fi
exit "$fail"
