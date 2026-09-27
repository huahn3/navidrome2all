package lyrics

import (
	"strings"
	"testing"

	"github.com/navidrome/navidrome/model"
)

// 回归：译文与原文相同时，bilingualLrc 曾把原文写两遍
// （"[01:01.05]xxx\n[01:01.05]xxx"），因为 else 分支的格式串里 orig 出现了两次。
// 触发场景很常见：引擎对专有名词、纯外文歌词、人名会原样返回。
func TestBuildTranslationResult_NoDuplicateWhenTranslationEqualsOriginal(t *testing.T) {
	start1 := int64(61050)
	start2 := int64(70000)
	originals := []model.Line{
		{Start: &start1, Value: "再夹两片培根都放进 burger bun"},
		{Start: &start2, Value: "Burn the bacon"},
	}

	cases := []struct {
		name       string
		translated []string
	}{
		{
			// 引擎原样返回（最常见的触发方式）
			name:       "translation identical to original",
			translated: []string{"再夹两片培根都放进 burger bun", "Burn the bacon"},
		},
		{
			// 只有空白差异：比较前 TrimSpace，不该被当成有译文
			name:       "translation differs only by whitespace",
			translated: []string{"  再夹两片培根都放进 burger bun  ", "\tBurn the bacon\n"},
		},
		{
			// 引擎没给出译文
			name:       "empty translation",
			translated: []string{"", ""},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := buildTranslationResult("s1", "zh-CN", "test", "m", originals, tc.translated)

			// 逐行断言：同一时间戳不能在 bilingualLrc 里出现两次
			assertNoDuplicateLRC(t, "bilingualLrc", res.BilingualLRC)
			assertNoDuplicateLRC(t, "combinedLrc", res.CombinedLRC)

			// inlineLrc 是底栏悬浮歌词用的格式，同一时刻只允许一行
			if lines := nonEmptyLines(res.InlineLRC); len(lines) != len(originals) {
				t.Fatalf("inlineLrc has %d lines, want %d:\n%s",
					len(lines), len(originals), res.InlineLRC)
			}

			// 三种格式都必须保留原文
			for name, lrc := range map[string]string{
				"bilingualLrc": res.BilingualLRC,
				"combinedLrc":  res.CombinedLRC,
				"inlineLrc":    res.InlineLRC,
			} {
				for _, orig := range originals {
					if !strings.Contains(lrc, orig.Value) {
						t.Errorf("%s lost the original line %q:\n%s", name, orig.Value, lrc)
					}
				}
			}
		})
	}
}

// 译文不同时必须双语（防止修复过头，把所有译文都吞掉）
func TestBuildTranslationResult_KeepsDistinctTranslation(t *testing.T) {
	start := int64(1000)
	originals := []model.Line{{Start: &start, Value: "Hello world"}}

	res := buildTranslationResult("s1", "zh-CN", "test", "m", originals, []string{"你好世界"})

	// bilingualLrc：同一时间戳两行（原文 + 译文）
	want := "[00:01.00]Hello world\n[00:01.00]你好世界"
	if strings.TrimRight(res.BilingualLRC, "\n") != want {
		t.Errorf("bilingualLrc =\n%q\nwant\n%q", res.BilingualLRC, want)
	}
	// inlineLrc：同时间戳合成一行
	if !strings.Contains(res.InlineLRC, "Hello world / 你好世界") {
		t.Errorf("inlineLrc should join both on one timestamp, got:\n%s", res.InlineLRC)
	}
	// 结构化 lines 不受影响
	if len(res.Lines) != 1 || res.Lines[0].Translation != "你好世界" {
		t.Errorf("structured lines should keep the translation, got %+v", res.Lines)
	}
}

func nonEmptyLines(lrc string) []string {
	var out []string
	for _, l := range strings.Split(lrc, "\n") {
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return out
}

// assertNoDuplicateLRC 检查同一时间戳不会带出两个不同的行内容
func assertNoDuplicateLRC(t *testing.T, name, lrc string) {
	t.Helper()
	seen := map[string]string{}
	for _, l := range nonEmptyLines(lrc) {
		idx := strings.Index(l, "]")
		if idx < 0 {
			continue
		}
		tag, text := l[:idx+1], l[idx+1:]
		if prev, ok := seen[tag]; ok {
			if prev == text {
				t.Errorf("%s repeats the same line under %s: %q", name, tag, text)
			}
			// 相同 tag 不同内容是合法的双语输出
			continue
		}
		seen[tag] = text
	}
}
