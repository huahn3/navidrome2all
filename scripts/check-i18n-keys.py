#!/usr/bin/env python3
"""检查源码里 translate('...') 引用的 key 是否都存在于语言包。

为什么需要它：`make test-i18n`（scripts/validate-translations.sh）只做
"en.json 的 key 是否都在翻译包里"这一个方向的校验，查不出
**组件引用了但三份语言包都没有**的 key —— 那种 key 会静默回退到代码里的
`_:` 默认值，在非默认语言下就表现为"某个界面中英/中繁混排"。

polyglot 在构造时用 extend() 把嵌套对象拍平成带点键（见
node-polyglot/index.js:329），所以查找是扁平的，脚本按同样语义实现。
"""
import json
import os
import re
import sys
import collections

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
PACKS = [
    os.path.join(ROOT, 'ui/src/i18n/en.json'),
    os.path.join(ROOT, 'resources/i18n/zh-Hans.json'),
    os.path.join(ROOT, 'resources/i18n/zh-Hant.json'),
]


def flatten(obj, prefix=''):
    """复刻 polyglot extend() 的拍平规则"""
    out = {}
    for k, v in obj.items():
        key = f'{prefix}.{k}' if prefix else k
        if isinstance(v, dict):
            out.update(flatten(v, key))
        else:
            out[key] = v
    return out


# 语言包的一级命名空间。出现在源码里的同前缀字符串就按 i18n key 校验。
NAMESPACES = ('ra.', 'menu.', 'resources.', 'player.', 'polish.', 'message.')

# translate('x.y') 之外，这些形态也要抓：
#   - 双引号版本
#   - 写进映射表的字面量（如 ENGINE_LABEL_KEYS = { gemini: 'menu...gemini' }）
#   - 模板字符串 `` `menu...${id}` `` 里的静态前缀（用于提醒这种写法抓不全）
NAMED_TEMPLATE = re.compile(
    r'`((?:' + '|'.join(N.rstrip('.') for N in NAMESPACES) + r')[A-Za-z0-9_.]*)\$\{'
)


def referenced_keys():
    used = collections.defaultdict(set)
    src_root = os.path.join(ROOT, 'ui/src')
    patterns = [
        re.compile(r"translate\(\s*'([^']+)'"),
        re.compile(r'translate\(\s*"([^"]+)"'),
    ]
    # 命名空间后面至少还要有一段（要求出现 '.'），避免把 'menu'、'player'
    # 这类 dataProvider/localStorage 的普通字符串误判成 i18n key
    literal = re.compile(
        r"['\"]((?:%s)[A-Za-z0-9_]*\.[A-Za-z0-9_.]+)['\"]"
        % '|'.join(N.rstrip('.') for N in NAMESPACES)
    )
    for dirpath, _, files in os.walk(src_root):
        for name in files:
            if not name.endswith(('.js', '.jsx')):
                continue
            # 测试文件里常出现"故意写错的 key 名"当断言，别当问题报
            if '.test.' in name or '.stories.' in name:
                continue
            path = os.path.join(dirpath, name)
            with open(path, encoding='utf-8') as fh:
                src = fh.read()
            rel = os.path.relpath(path, ROOT)
            for pat in patterns:
                for m in pat.finditer(src):
                    used[m.group(1)].add(rel)
            for m in literal.finditer(src):
                used[m.group(1)].add(rel)
            # 只提醒本 fork 自己的命名空间，上游那批是它自己的写法，不去动
            for m in NAMED_TEMPLATE.finditer(src):
                if not m.group(1).startswith(('menu.lyricsTranslation', 'resources.jukeboxOutput')):
                    continue
                print(
                    f'提示: {rel} 里用模板字符串拼 i18n key（{m.group(1)}...${{}}），'
                    '这类 key 校验不到，请改成字面量映射表'
                )
    return used


def main():
    catalogs = {}
    for p in PACKS:
        with open(p, encoding='utf-8') as fh:
            catalogs[os.path.basename(p)] = flatten(json.load(fh))

    used = referenced_keys()
    problems = 0
    for key in sorted(used):
        missing_in = [name for name, cat in catalogs.items() if key not in cat]
        if missing_in:
            problems += 1
            print(f'缺少 key: {key}')
            print(f'  引用位置: {", ".join(sorted(used[key])[:2])}')
            print(f'  缺失于: {", ".join(missing_in)}')
    print(f'\n共引用 {len(used)} 个 key，{problems} 个在语言包里不存在')
    return 1 if problems else 0


if __name__ == '__main__':
    sys.exit(main())
