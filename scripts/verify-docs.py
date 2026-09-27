#!/usr/bin/env python3
"""核对文档/skills 里引用的路径、符号、行号、配置项是否与真实代码一致。

只做机器能判定的事实检查；语义正确性仍需人工/AI 复核。
"""
import json
import os
import re
import subprocess
import sys

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
DOC_FILES = []
for base in ("docs", ".claude", ".qoder"):
    for dirpath, _, files in os.walk(os.path.join(ROOT, base)):
        for f in files:
            if f.endswith(".md"):
                DOC_FILES.append(os.path.join(dirpath, f))
DOC_FILES.append(os.path.join(ROOT, "AGENTS.md"))

problems = []

# 来自第三方库的方法名，本仓源码里自然搜不到
EXTERNAL_SYMS = {"extend", "get", "map", "set", "has", "isEmpty", "values", "keys"}

# 见 docs/fork-and-upstream.md（说"已删除"）与 docs/risk-notes-optimization.md
# 的历史改动清单（记录当时的文件名，现已重命名/重构）。
HISTORICAL = {
    ".github/dependabot.yml",
    ".github/workflows/",
    "ui/src/jukebox/JukeboxOutputForm.jsx",
    "JukeboxOutputList/Create/Edit/Form.jsx",
    "jukebox/index.js",
    "subsonic/index.js",
    "node-polyglot/index.js",
    "node_modules/react-final-form/dist/react-final-form.cjs.js",
    "material-ui/core/Popover/Popover.js",
    "src/layout/NowPlayingPanel.test.jsx",
    "github.com/icepie/miio.go",
    "icepie/miio.go",
}


def rel(p):
    return os.path.relpath(p, ROOT)


# ---------------------------------------------------------------- 1. 文件路径
# 匹配形如 `path/to/file.go` 或 `path/to/file.go:123` 的引用
PATH_RE = re.compile(r"(?<![\w/.-])((?:[\w.-]+/)+[\w.-]+\.(?:go|jsx?|tsx?|json|py|sh|md|toml|yml|yaml|css))(?::(\d+))?")
SKIP_DIR_PREFIXES = ("http://", "https://")

seen_paths = set()
# 历史/叙述段落里提到的已删除文件不算错；靠"必须带反引号且路径真实"来区分
FENCE_RE = re.compile(r"```.*?```", re.S)
INLINE_CODE_RE = re.compile(r"`([^`\n]+)`")

for doc in DOC_FILES:
    with open(doc, encoding="utf-8") as f:
        raw = f.read()
    # 1) 去掉围栏代码块（```...```）
    raw = FENCE_RE.sub(lambda m: "\n" * m.group(0).count("\n"), raw)
    # 2) 提取行内反引号里的内容作为候选
    cands = []
    for m in INLINE_CODE_RE.finditer(raw):
        cands.append(m.group(1))
    # 不再对全文二次 finditer：会从真实路径中间截出假路径
    # （如 ui/src/i18n/provider.js 被截成 ui/src/i18n/en.js）
    for cand in cands:
        m = PATH_RE.fullmatch(cand.strip())
        if not m:
            continue
        path, line = m.group(1), m.group(2)
        if path.startswith("http") or "://" in path:
            continue
        # 明显是散文/示例的（带占位符、大写开头、非本仓库前缀）
        if any(t in path for t in ("...", "<", ">", "|")):
            continue
        # 已删除/已重命名的文件只在"历史叙述"与"历史改动清单"里出现，
        # 那是当时的事实，不该改写成现在的名字（那会篡改历史）。
        if path in HISTORICAL:
            continue
        if not os.path.exists(os.path.join(ROOT, path)):
            problems.append(("PATH", rel(doc), path, "文件不存在"))
            continue
        seen_paths.add(path)
        if line:
            try:
                n = sum(1 for _ in open(os.path.join(ROOT, path), encoding="utf-8"))
            except Exception:
                continue
            if int(line) > n:
                problems.append(("LINE", rel(doc), f"{path}:{line}", f"文件只有 {n} 行"))

# ------------------------------------------------- 2. 形如 `funcName` 的符号引用
SYM_RE = re.compile(r"`([A-Za-z_][A-Za-z0-9_]*)\(\)`")
go_src = {}
for dirpath, dirs, files in os.walk(ROOT):
    dirs[:] = [d for d in dirs if d not in (".git", "node_modules", "ui/build", "ui/dist", "bin", "vendor")]
    for f in files:
        if f.endswith(".go"):
            fp = os.path.join(dirpath, f)
            try:
                go_src[rel(fp)] = open(fp, encoding="utf-8").read()
            except Exception:
                pass

all_go = "\n".join(go_src.values())

# 文档里也会引用前端符号（useStyle/adoptDeviceVolume/initLyricParser…），
# 只扫 Go 会把它们全报成"找不到"。
js_src = []
ui_root = os.path.join(ROOT, "ui", "src")
for dirpath, dirs, files in os.walk(ui_root):
    dirs[:] = [d for d in dirs if d not in ("node_modules", "build", "dist")]
    for f in files:
        if f.endswith((".js", ".jsx", ".ts", ".tsx")):
            try:
                js_src.append(open(os.path.join(dirpath, f), encoding="utf-8").read())
            except Exception:
                pass
all_js = "\n".join(js_src)
for doc in DOC_FILES:
    with open(doc, encoding="utf-8") as f:
        text = f.read()
    for m in SYM_RE.finditer(text):
        sym = m.group(1)
        # 只校验看起来像本项目自定义的符号（排除通用词）
        if re.search(r"\b(func|var|const|type)\s+" + re.escape(sym) + r"\b", all_go):
            continue
        if re.search(r"\b" + re.escape(sym) + r"\b", all_go):
            continue
        if re.search(r"\b" + re.escape(sym) + r"\b", all_js):
            continue
        # 外部库的方法（lodash 之类）不在本仓源码里
        if sym in EXTERNAL_SYMS:
            continue
        problems.append(("SYM", rel(doc), f"{sym}()", "Go 和 ui/src 里都找不到这个符号"))

# ----------------------------------------------------- 3. 配置项 [Jukebox] 字段
conf = open(os.path.join(ROOT, "conf/configuration.go"), encoding="utf-8").read()
CONF_RE = re.compile(r"`(Jukebox\.[A-Z][A-Za-z]*|LyricsTranslation\.[A-Za-z]+|LastFM\.[A-Za-z]+|Prometheus\.[A-Za-z]+)`(?![\w.])")
for doc in DOC_FILES:
    with open(doc, encoding="utf-8") as f:
        text = f.read()
    for m in CONF_RE.finditer(text):
        key = m.group(1)
        # 形如 LyricsTranslation.jsx 是文件名，不是配置项
        if key.endswith((".js", ".jsx", ".json", ".go")):
            continue
        field = key.split(".", 1)[1]
        # 字段必须出现在 configuration.go 里
        if re.search(r"\b" + field + r"\b", conf) is None:
            problems.append(("CONF", rel(doc), key, f"conf 里没有字段 {field}"))

# ------------------------------------------------------------ 4. 测试数字
def spec_count(pkg):
    try:
        out = subprocess.run(
            ["go", "test", "-count=1", "-tags=netgo,sqlite_fts5", pkg, "-v"],
            cwd=ROOT, capture_output=True, text=True, timeout=900)
    except Exception:
        return None
    m = re.search(r"Ran (\d+) of (\d+) Specs", out.stdout)
    return int(m.group(1)) if m else None


CLAIM_RE = re.compile(r"(\d+)\s*specs")
doc_claims = {}
for doc in DOC_FILES:
    with open(doc, encoding="utf-8") as f:
        text = f.read()
    for m in CLAIM_RE.finditer(text):
        line_start = text.rfind("\n", 0, m.start()) + 1
        line_end = text.find("\n", m.end())
        line = text[line_start:line_end if line_end > 0 else len(text)]
        pk = re.search(r"(?:PKG=)?(\./(?:core|server)/[\w/]+)", line)
        if pk:
            doc_claims.setdefault(pk.group(1), set()).add((int(m.group(1)), rel(doc)))

print("=" * 70)
print("1) 路径/行号检查完成")
print("=" * 70)
if problems:
    for kind, doc, what, why in problems:
        print(f"[{kind}] {doc}: {what}  -> {why}")
else:
    print("所有文件路径与行号引用均有效")

print()
print("=" * 70)
print("2) Ginkgo spec 数字核对（较慢，实时跑）")
print("=" * 70)
for pkg, claims in sorted(doc_claims.items()):
    real = spec_count(pkg)
    if real is None:
        print(f"{pkg}: 跑不出来，跳过")
        continue
    for claimed, doc in sorted(claims):
        flag = "OK " if claimed == real else "!! "
        print(f"{flag}{doc}: 声称 {claimed}，实际 {real}   ({pkg})")

sys.exit(1 if problems else 0)
