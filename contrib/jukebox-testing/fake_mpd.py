import socket, threading, time

STATUS = {"state": "stop", "playlistlength": "0", "elapsed": "0", "duration": "215", "volume": "60"}
LOG = "/tmp/nd_fake_mpd.log"

def log(msg):
    with open(LOG, "a") as f:
        f.write(msg + "\n")

def handle(conn):
    conn.sendall(b"OK MPD 0.23.5\n")
    f = conn.makefile("rwb")
    for line in f:
        cmd = line.decode().strip()
        if cmd == "close":
            break
        log(cmd)
        if cmd == "status":
            out = "".join(f"{k}: {v}\n" for k, v in sorted(STATUS.items())) + "OK\n"
        elif cmd == "clear":
            STATUS["playlistlength"] = "0"
            out = "OK\n"
        elif cmd.startswith("add"):
            STATUS["playlistlength"] = "1"
            STATUS["song"] = "0"
            out = "OK\n"
        elif cmd.startswith("play"):
            STATUS["state"] = "play"
            out = "OK\n"
        elif cmd.startswith("pause"):
            STATUS["state"] = "pause" if cmd.endswith(" 1") else "play"
            out = "OK\n"
        elif cmd == "stop":
            STATUS["state"] = "stop"
            out = "OK\n"
        elif cmd.startswith("seek"):
            STATUS["elapsed"] = cmd.split()[2]
            out = "OK\n"
        elif cmd.startswith("setvol"):
            STATUS["volume"] = cmd.split()[1]
            out = "OK\n"
        else:
            out = "OK\n"
        f.write(out.encode())
        f.flush()
    conn.close()

s = socket.socket()
s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
s.bind(("127.0.0.1", 16600))
s.listen(16)
log("listening")
while True:
    c, _ = s.accept()
    threading.Thread(target=handle, args=(c,), daemon=True).start()
