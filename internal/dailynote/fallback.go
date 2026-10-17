package dailynote

import "time"

// Original local notes outnumber the 30-day deduplication window, so outages
// can still provide a different note each day without an external dependency.
var localNotes = []string{
	"把平凡的今天写下来，往后的日子会替你珍藏。",
	"留一点时间给自己，也留一点文字给时光。",
	"不用等到特别的日子，今天就值得被记住。",
	"偶尔停下脚步，看看一路陪伴你的微小美好。",
	"有些心情不必急着解释，写下来就已经很好。",
	"在忙碌的日子里，也别忘了听一听自己的声音。",
	"今天走过的路，会慢慢成为属于你的故事。",
	"把心里的小小欢喜，安放在这一页文字里。",
	"认真吃饭，好好休息，让生活有自己的节奏。",
	"与其催促故事快点发生，不如记下此刻的风景。",
	"当你回头看时，那些小事也会闪着自己的光。",
	"每一次真诚的表达，都是与自己的一次相遇。",
	"把没有说完的话，慢慢写给愿意倾听的人。",
	"不必把每一天过得盛大，安稳也有它的分量。",
	"给寻常的生活留个位置，让记忆有处可归。",
	"记录不是为了留住一切，而是珍惜眼前所有。",
	"一本书，一杯茶，都可以成为今天的小小注脚。",
	"在文字里坐一会儿，让心情慢慢找到出口。",
	"带着好奇心出门，也带一段新的故事回来。",
	"愿今天的你，能为一件小事感到由衷开心。",
	"那些安静努力的时刻，也值得被认真看见。",
	"让日子慢一点，让自己听见内心细小的回声。",
	"把路上的见闻记下来，给以后的自己带份礼物。",
	"不用急着为生活下结论，先陪它走过今天。",
	"日常的片段连在一起，就是独一无二的人生。",
	"与朋友分享一点近况，也是在替时光留下温度。",
	"一段被认真记录的时光，会有很长的回响。",
	"在熟悉的地方发现新意，也是生活给你的惊喜。",
	"为今天留下一句话，让平凡的瞬间有迹可循。",
	"不必把所有问题都想明白，先照顾好此刻的自己。",
	"记下窗外的天气，也记下心里正在生长的想法。",
	"生活里那些细小的善意，值得被好好收藏。",
}

func fallback(day time.Time, recent map[string]bool) candidate {
	start := int(day.Unix()/86400) % len(localNotes)
	if start < 0 {
		start += len(localNotes)
	}
	for offset := 0; offset < len(localNotes); offset++ {
		text := localNotes[(start+offset)%len(localNotes)]
		if !recent[contentHash(text)] {
			return candidate{content: text, source: "时光记", provider: "builtin"}
		}
	}
	// The query contains at most one row per day for 30 days; with 32 notes,
	// this branch cannot be reached by persisted community data.
	return candidate{content: localNotes[start], source: "时光记", provider: "builtin"}
}
