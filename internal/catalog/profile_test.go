package catalog

import (
	"reflect"
	"testing"
)

func TestParseCookMinutes(t *testing.T) {
	cases := []struct {
		description string
		want        int
	}{
		{"从备料到出锅大约需要 1 小时。", 60},
		{"大约需要 40 分钟", 40},
		{"约需 30 分钟", 30},
		{"大约需要 2.5 小时", 150},
		{"制作耗时大约需要两小时", 120},
		{"大约需要一小时", 60},
		{"约需半小时", 30},
		{"大约需要两个半小时", 150},
		// 区间取上界；分隔符含多字节字符，必须按 rune 宽度切
		{"大约需要 30-40 分钟", 40},
		{"约需 1.5~2 小时", 120},
		{"大约需要 20至30 分钟", 30},
		{"大约需要 1–1.5 小时", 90},
		// 十位组合
		{"约需二十分钟", 20},
		{"约需二十五分钟", 25},
		{"约需十五分钟", 15},
		// 解析不出 → 0（信息小字尽力而为）
		{"很快就好", 0},
	}
	for _, testCase := range cases {
		if got := parseCookMinutes(testCase.description); got != testCase.want {
			t.Errorf("parseCookMinutes(%q) = %d, want %d", testCase.description, got, testCase.want)
		}
	}
}

func TestEnrichExtractsProfileWithMaskingAndAliases(t *testing.T) {
	content := `# 西红柿炒鸡蛋的做法

酸甜开胃的家常菜，预估烹饪难度：★★

预估卡路里：250 大卡

## 必备原料和工具

* 西红柿
* 鸡蛋
* 番茄酱
* 小米辣

## 操作

1. 热油煎鸡蛋
`
	enrichment := Enrich("vegetable_dish/西红柿炒鸡蛋.md", "西红柿炒鸡蛋", content)
	// 别名归一：西红柿 → 番茄；排除词掩蔽：番茄酱/小米辣不产生 番茄/小米。
	// 顺序 = 词典长词优先的匹配序，确定性。
	if !reflect.DeepEqual(enrichment.Profile.Ingredients, []string{"番茄", "鸡蛋"}) {
		t.Errorf("ingredients = %v, want [番茄 鸡蛋]", enrichment.Profile.Ingredients)
	}
	if !reflect.DeepEqual(enrichment.Profile.Flavors, []string{"酸甜"}) {
		t.Errorf("flavors = %v, want [酸甜]", enrichment.Profile.Flavors)
	}
	// 工艺：菜名的 炒 + 步骤的 煎（|steps 标记）
	if !reflect.DeepEqual(enrichment.Profile.Techniques, []string{"煎", "炒"}) {
		t.Errorf("techniques = %v, want [煎 炒]", enrichment.Profile.Techniques)
	}
	if enrichment.Difficulty == nil || *enrichment.Difficulty != 2 {
		t.Errorf("difficulty = %v, want 2", enrichment.Difficulty)
	}
	if enrichment.Calories == nil || *enrichment.Calories != 250 {
		t.Errorf("calories = %v, want 250", enrichment.Calories)
	}
}

func TestParseImagesResolvesRelativeKeepsExternalDropsAbsolute(t *testing.T) {
	content := "![成品](./1.jpg)\n![步骤](steps/2.png)\n![外链](https://example.com/a.jpg)\n![绝对](/site/b.png)\n![邻目录](../c.png)\n![越界](../../../etc/passwd)\n"
	images := parseImages("meat_dish/口水鸡/口水鸡.md", content)
	want := []string{
		"meat_dish/口水鸡/1.jpg",
		"meat_dish/口水鸡/steps/2.png",
		"https://example.com/a.jpg",
		// 邻目录引用仍在语料树内，正常归位；跳出语料根的越界引用被丢弃
		"meat_dish/c.png",
	}
	if !reflect.DeepEqual(images, want) {
		t.Errorf("images = %v, want %v", images, want)
	}
}
