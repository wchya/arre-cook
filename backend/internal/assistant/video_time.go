package assistant

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const videoTimeNumber = `[0-9零〇一二两三四五六七八九十百]+`

var videoTimeParts = regexp.MustCompile(`(` + videoTimeNumber + `|半)(个半|个|半)?(小时|分钟|秒钟|秒)`)
var overallVideoTime = regexp.MustCompile(`(?:总(?:烹饪|制作)?(?:时间|时长|用时|耗时)|(?:全程|整个(?:烹饪|制作)?过程|整道菜)(?:的)?(?:时间|时长|用时|耗时)?|(?:总共|一共)(?:需要|用时|耗时|用|要)?)(?:大约|大概|约|需要|需|要|用了|用|是|为|[:：\s]){0,3}(?:` + videoTimeNumber + `|半)`)

// Keep only an explicitly stated, unambiguous whole-minute duration. Adjacent
// hours/minutes/seconds may form one duration; separate step times are never added.
func supportsVideoMinutes(evidence string, minutes int) bool {
	if minutes <= 0 || minutes > 600 {
		return false
	}
	for _, ambiguous := range []string{"不要", "不用", "无需", "不能", "不需要", "至少", "至多", "以上", "以下", "不到", "超过", "不止", "不满", "少于", "多于"} {
		if strings.Contains(evidence, ambiguous) {
			return false
		}
	}
	text := strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		if r >= '０' && r <= '９' {
			return r - '０' + '0'
		}
		return r
	}, evidence)
	parts := videoTimeParts.FindAllStringSubmatchIndex(text, -1)
	if len(parts) == 0 {
		return false
	}
	seconds, lastEnd, lastUnit := 0, 0, 3601
	for i, part := range parts {
		if i > 0 && text[lastEnd:part[0]] != "" && text[lastEnd:part[0]] != "又" {
			return false
		}
		before, _ := utf8.DecodeLastRuneInString(text[:part[0]])
		after, _ := utf8.DecodeRuneInString(text[part[1]:])
		if strings.ContainsRune("0123456789零〇一二两三四五六七八九十百点./-—~～至到分之", before) || strings.ContainsRune("半点./-—~～至到", after) {
			return false
		}
		unit := map[string]int{"小时": 3600, "分钟": 60, "秒钟": 1, "秒": 1}[text[part[6]:part[7]]]
		if unit >= lastUnit {
			return false
		}
		number := text[part[2]:part[3]]
		half := number == "半"
		if part[4] >= 0 {
			half = half || strings.Contains(text[part[4]:part[5]], "半")
		}
		value := 0
		if number != "半" {
			var ok bool
			value, ok = videoTimeInteger(number)
			if !ok || value > 36000 {
				return false
			}
		}
		if half && unit == 1 {
			return false
		}
		seconds += value * unit
		if half {
			seconds += unit / 2
		}
		lastEnd, lastUnit = part[1], unit
	}
	return seconds == minutes*60
}

func videoTimeInteger(raw string) (int, bool) {
	if value, err := strconv.Atoi(raw); err == nil {
		return value, value >= 0
	}
	raw = strings.NewReplacer("两", "二", "〇", "零").Replace(raw)
	// Exact spellings avoid interpreting ambiguous phrases such as 两三分钟 or
	// 一百二分钟. Unrecognized expressions remain unspecified rather than guessed.
	for n := 0; n <= 600; n++ {
		if chineseVideoNumber(n) == raw {
			return n, true
		}
	}
	return 0, false
}

func chineseVideoNumber(n int) string {
	digits := []rune("零一二三四五六七八九")
	if n < 10 {
		return string(digits[n])
	}
	if n < 100 {
		tens := "十"
		if n >= 20 {
			tens = string(digits[n/10]) + tens
		}
		if n%10 != 0 {
			tens += string(digits[n%10])
		}
		return tens
	}
	result := string(digits[n/100]) + "百"
	if n%100 == 0 {
		return result
	}
	if n%100 < 10 {
		return result + "零" + string(digits[n%10])
	}
	if n%100 < 20 {
		return result + "一" + chineseVideoNumber(n%100)
	}
	return result + chineseVideoNumber(n%100)
}
