package catalog

import (
	_ "embed"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/jasper0507/what-to-eat/internal/engine"
)

// Taste profile 萃取与元数据解析（ADR-0022）：导入期对菜谱 markdown 做
// 确定性解析——词典子串命中，禁 LLM、禁嵌入，离线可复算。
// 三部词典是仓库数据，一行一词，# 开头为注释。

//go:embed dictionaries/ingredients.txt
var ingredientsDictionary string

//go:embed dictionaries/flavors.txt
var flavorsDictionary string

//go:embed dictionaries/techniques.txt
var techniquesDictionary string

var (
	ingredientTerms = parseDictionary(ingredientsDictionary)
	flavorTerms     = parseDictionary(flavorsDictionary)
	techniqueTerms  = parseDictionary(techniquesDictionary)
	nullRune        = "\x00"

	difficultyPattern = regexp.MustCompile(`预估烹饪难度：(★+)`)
	caloriesPattern   = regexp.MustCompile(`预估卡路里：\s*([0-9]+(?:\.[0-9]+)?)\s*大卡`)
	durationPattern   = regexp.MustCompile(
		`(?:需要|约需)[^。\n]{0,12}?([0-9]+(?:\.[0-9]+)?(?:\s*[-–~至]\s*[0-9]+(?:\.[0-9]+)?)?|[一两二三四五六七八九十半]+个?半?)\s*(分钟|小时)`,
	)
	imagePattern       = regexp.MustCompile(`!\[[^\]]*\]\(\s*([^)\s]+)\s*\)`)
	chineseNumeralUnit = map[rune]float64{
		'一': 1, '二': 2, '两': 2, '三': 3, '四': 4,
		'五': 5, '六': 6, '七': 7, '八': 8, '九': 9, '十': 10,
	}
)

// dictionaryTerm 是一条词典词目。词典行格式（ADR-0022）：
//
//	词条            —— 命中即记录自身
//	别名=主形       —— 命中记录主形（同物异名归一，保证相似度交集可比）
//	!排除词         —— 只掩蔽不记录（防「番茄酱」喂给「番茄」）
//	词条|steps      —— 工艺专用：额外匹配步骤文本
type dictionaryTerm struct {
	surface    string
	canonical  string
	exclude    bool
	matchSteps bool
}

// parseDictionary 解析词典并按 surface 词长降序排——匹配器命中即掩蔽，
// 长词先吃掉字面，短词不再误中。
func parseDictionary(raw string) []dictionaryTerm {
	terms := make([]dictionaryTerm, 0)
	for _, line := range strings.Split(raw, "\n") {
		entry := strings.TrimSpace(line)
		if entry == "" || strings.HasPrefix(entry, "#") {
			continue
		}
		var term dictionaryTerm
		if rest, ok := strings.CutSuffix(entry, "|steps"); ok {
			term.matchSteps = true
			entry = strings.TrimSpace(rest)
		}
		if rest, ok := strings.CutPrefix(entry, "!"); ok {
			term.exclude = true
			entry = rest
		}
		term.surface = entry
		term.canonical = entry
		if surface, canonical, ok := strings.Cut(entry, "="); ok {
			term.surface = surface
			term.canonical = canonical
		}
		terms = append(terms, term)
	}
	slices.SortStableFunc(terms, func(left, right dictionaryTerm) int {
		return len([]rune(right.surface)) - len([]rune(left.surface))
	})
	return terms
}

// recipeSections 是解析出的菜谱分区：描述段（首个 ## 之前）、原料节、
// 操作步骤节全文。
type recipeSections struct {
	description string
	ingredients string
	steps       string
}

func splitSections(content string) recipeSections {
	var sections recipeSections
	lines := strings.Split(content, "\n")
	current := "description"
	var description, ingredients, steps strings.Builder
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			heading := strings.TrimSpace(strings.TrimPrefix(trimmed, "## "))
			switch {
			case strings.Contains(heading, "原料"):
				current = "ingredients"
			case strings.Contains(heading, "操作"):
				current = "steps"
			default:
				current = ""
			}
			continue
		}
		switch current {
		case "description":
			if !strings.HasPrefix(trimmed, "#") && !strings.HasPrefix(trimmed, "![") {
				description.WriteString(trimmed)
				description.WriteString("\n")
			}
		case "ingredients":
			ingredients.WriteString(trimmed)
			ingredients.WriteString("\n")
		case "steps":
			steps.WriteString(trimmed)
			steps.WriteString("\n")
		}
	}
	sections.description = description.String()
	sections.ingredients = ingredients.String()
	sections.steps = steps.String()
	return sections
}

// matchTerms 长词优先扫词典：命中即把字面掩蔽成占位符（防止短词重复
// 误中），排除词只掩蔽不记录，别名归一到主形并去重。stepsHaystack 只对
// 带 |steps 标记的词条开放（工艺词典专用，其余词典传空串）。
func matchTerms(terms []dictionaryTerm, primary, stepsHaystack string) []string {
	hits := make([]string, 0)
	recorded := make(map[string]bool)
	for _, term := range terms {
		mask := strings.Repeat(nullRune, len([]rune(term.surface)))
		matched := false
		if strings.Contains(primary, term.surface) {
			matched = true
			primary = strings.ReplaceAll(primary, term.surface, mask)
		}
		if term.matchSteps && strings.Contains(stepsHaystack, term.surface) {
			matched = true
			stepsHaystack = strings.ReplaceAll(stepsHaystack, term.surface, mask)
		}
		if matched && !term.exclude && !recorded[term.canonical] {
			recorded[term.canonical] = true
			hits = append(hits, term.canonical)
		}
	}
	return hits
}

// Enrichment 是导入期对一份菜谱的全部富化产物。
type Enrichment struct {
	Profile     engine.Profile
	Images      []string
	Difficulty  *int
	Calories    *int
	CookMinutes *int
}

// Enrich 解析菜谱 markdown，萃取 Taste profile 与元数据。
// sourcePath 用于品类桶与相对图片引用的归位。
func Enrich(sourcePath, name, content string) Enrichment {
	sections := splitSections(content)

	enrichment := Enrichment{
		Profile: engine.Profile{
			Ingredients: matchTerms(ingredientTerms, name+"\n"+sections.ingredients, ""),
			Flavors:     matchTerms(flavorTerms, name+"\n"+sections.description, ""),
			// 工艺默认只认菜名；带 |steps 标记的词条才看步骤（ADR-0022）
			Techniques: matchTerms(techniqueTerms, name, sections.steps),
			Category:   PathCategory(sourcePath),
		},
		Images: parseImages(sourcePath, content),
	}
	if match := difficultyPattern.FindStringSubmatch(content); match != nil {
		stars := len([]rune(match[1]))
		enrichment.Difficulty = &stars
	}
	if match := caloriesPattern.FindStringSubmatch(content); match != nil {
		if value, err := strconv.ParseFloat(match[1], 64); err == nil {
			calories := int(value)
			enrichment.Calories = &calories
		}
	}
	if minutes := parseCookMinutes(sections.description); minutes > 0 {
		enrichment.CookMinutes = &minutes
	}
	return enrichment
}

// parseCookMinutes 从描述段解析耗时。区间取上界（保守），汉字数词按
// 一~十/两/半/X个半 换算。解析不出返回 0（耗时是尽力而为的信息小字）。
func parseCookMinutes(description string) int {
	match := durationPattern.FindStringSubmatch(description)
	if match == nil {
		return 0
	}
	quantity := parseQuantity(match[1])
	if quantity <= 0 {
		return 0
	}
	if match[2] == "小时" {
		quantity *= 60
	}
	return int(quantity)
}

func parseQuantity(raw string) float64 {
	raw = strings.TrimSpace(raw)
	// 区间「30-40」「1.5~2」取上界；分隔符含多字节字符（–、至），必须按
	// rune 宽度跳过，不能 index+1 的字节切片（会切进半个字符）。
	if index := strings.IndexAny(raw, "-–~至"); index > 0 {
		_, width := utf8.DecodeRuneInString(raw[index:])
		raw = strings.TrimSpace(raw[index+width:])
		raw = strings.TrimLeft(raw, "-–~至 ")
	}
	if value, err := strconv.ParseFloat(raw, 64); err == nil {
		return value
	}
	return chineseQuantity(raw)
}

// chineseQuantity 换算汉字数词：一~十、十位组合（二十、二十五）、半（0.5）、
// 「X个半」（X+0.5）。「个」是连接字不计值。
func chineseQuantity(raw string) float64 {
	tens, units := "", raw
	if index := strings.IndexRune(raw, '十'); index >= 0 {
		tens = raw[:index]
		units = raw[index+len("十"):]
	}
	sum := func(part string) float64 {
		total := 0.0
		for _, character := range part {
			switch character {
			case '半':
				total += 0.5
			case '个':
			default:
				if value, ok := chineseNumeralUnit[character]; ok {
					total += value
				}
			}
		}
		return total
	}
	if strings.ContainsRune(raw, '十') {
		tensValue := sum(tens)
		if tensValue == 0 {
			tensValue = 1
		}
		return tensValue*10 + sum(units)
	}
	return sum(units)
}

// parseImages 收集图片引用：相对路径归位为 Catalog 相对路径（供静态挂载），
// 绝对 URL 原样保留。
func parseImages(sourcePath, content string) []string {
	directory := path.Dir(sourcePath)
	images := make([]string, 0)
	seen := make(map[string]bool)
	for _, match := range imagePattern.FindAllStringSubmatch(content, -1) {
		reference := match[1]
		if strings.HasPrefix(reference, "http://") ||
			strings.HasPrefix(reference, "https://") {
			// 外链图原样保留
		} else if strings.HasPrefix(reference, "/") {
			// 站点绝对路径无从归位到 Catalog 目录，丢弃（否则产出永远 404 的条目）
			continue
		} else {
			reference = strings.TrimPrefix(reference, "./")
			if directory != "." {
				reference = directory + "/" + reference
			}
			reference = path.Clean(reference)
			if strings.HasPrefix(reference, "../") {
				continue
			}
		}
		if !seen[reference] {
			seen[reference] = true
			images = append(images, reference)
		}
	}
	return images
}
