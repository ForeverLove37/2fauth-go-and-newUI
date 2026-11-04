package services

import "strings"

// iconAliases maps the names people give a service to the slugs the icon sets use.
// Keys are lower-cased with separators removed. Each slug is tried in order, then
// the name itself.
var iconAliases = map[string][]string{
	"aws":                 {"amazon-web-services", "aws"},
	"amazonwebservices":   {"amazon-web-services", "aws"},
	"amazonaws":           {"amazon-web-services", "aws"},
	"office365":           {"microsoft-365"},
	"o365":                {"microsoft-365"},
	"microsoft365":        {"microsoft-365"},
	"microsoftaccount":    {"microsoft"},
	"outlook":             {"microsoft-outlook", "microsoft"},
	"hotmail":             {"microsoft-outlook", "microsoft"},
	"msn":                 {"microsoft"},
	"azuread":             {"azure"},
	"entraid":             {"azure"},
	"googleauthenticator": {"google"},
	"googleaccount":       {"google"},
	"youtube":             {"youtube", "google"},
	"tutanota":            {"tuta"},
	"protonmail":          {"proton-mail", "proton"},
	"battlenet":           {"battledotnet", "blizzard"},
	"blizzard":            {"blizzardentertainment", "battledotnet"},
	"gog":                 {"gogdotcom"},
	"epic":                {"epic-games"},
	"epicgames":           {"epic-games"},
	"ea":                  {"ea"},
	"psn":                 {"playstation"},
	"playstationnetwork":  {"playstation"},
	"sony":                {"playstation"},
	"xboxlive":            {"xbox"},
	"nintendoaccount":     {"nintendo"},
	"riot":                {"riot-games"},
	"riotgames":           {"riot-games"},
	"twitterx":            {"x", "twitter"},
	"dockerhub":           {"docker"},
	"facebookmeta":        {"facebook"},
	"digitalocean":        {"digitalocean", "digital-ocean"},
	"aliyun":              {"aliyun", "alibaba-cloud"},
	"alibabacloud":        {"alibaba-cloud", "aliyun"},
	"tencentcloud":        {"tencent", "tencent-qq"},
	"sinaweibo":           {"sina-weibo", "weibo"},
	"jd":                  {"jd", "jingdong"},
	"douyin":              {"tiktok"},

	"谷歌":   {"google"},
	"微软":   {"microsoft"},
	"苹果":   {"apple"},
	"亚马逊":  {"amazon"},
	"腾讯":   {"tencent"},
	"腾讯云":  {"tencent"},
	"腾讯qq": {"qq"},
	"微信":   {"wechat"},
	"支付宝":  {"alipay"},
	"阿里":   {"alibaba-cloud", "aliyun"},
	"阿里云":  {"aliyun", "alibaba-cloud"},
	"淘宝":   {"taobao"},
	"百度":   {"baidu"},
	"微博":   {"weibo", "sina-weibo"},
	"新浪微博": {"weibo", "sina-weibo"},
	"哔哩哔哩": {"bilibili"},
	"b站":   {"bilibili"},
	"小米":   {"xiaomi"},
	"华为":   {"huawei"},
	"华为云":  {"huawei"},
	"抖音":   {"tiktok"},
	"京东":   {"jd", "jingdong"},
	"知乎":   {"zhihu"},
	"网易":   {"netease"},
	"蒸汽平台": {"steam"},
	"电报":   {"telegram"},
	"推特":   {"x", "twitter"},
	"脸书":   {"facebook"},
	"领英":   {"linkedin"},
}

// iconBrands are companies whose products often have no icon of their own: "Google
// Workspace" falls back on the Google logo. Only these names fall back on a first
// word, because "Bank of America" must not become whatever "bank" happens to find.
var iconBrands = map[string]bool{
	"google": true, "microsoft": true, "amazon": true, "apple": true, "adobe": true, "meta": true,
	"oracle": true, "ibm": true, "cisco": true, "samsung": true, "tencent": true, "alibaba": true,
	"huawei": true, "xiaomi": true, "atlassian": true, "autodesk": true, "github": true, "gitlab": true,
}

// iconTLDs are the suffixes removed from a service named after its domain.
var iconTLDs = map[string]bool{
	"com": true, "net": true, "org": true, "io": true, "co": true, "ai": true, "app": true, "dev": true,
	"me": true, "tv": true, "cn": true, "uk": true, "de": true, "fr": true, "jp": true, "eu": true, "us": true,
}

// maxIconCandidates bounds the names tried for one service, so that a lookup that
// finds nothing stays within the budget of the chain.
const maxIconCandidates = 4

// iconCandidates lists the names to look a service up under, best first.
func iconCandidates(service string) []iconSlugs {
	var names []string
	seen := map[string]bool{}
	add := func(name string) {
		if name != "" && !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}

	names = append(names, iconAliases[iconAliasKey(service)]...)
	for _, name := range names {
		seen[name] = true
	}

	first := buildIconSlugs(service)
	if first.slug != "" {
		// The service itself is searched under its own spelling.
		if !seen[first.slug] {
			seen[first.slug] = true
			names = append(names, service)
		}
		base := domainLabel(first.slug)
		add(base)
		for _, alias := range iconAliases[iconAliasKey(base)] {
			add(alias)
		}
		if strings.Contains(first.slug, "-") {
			add(first.alnum)
			if word, _, _ := strings.Cut(first.slug, "-"); iconBrands[word] {
				add(word)
			}
		}
	}

	out := make([]iconSlugs, 0, maxIconCandidates)
	for _, name := range names {
		if slugs := buildIconSlugs(name); slugs.slug != "" {
			out = append(out, slugs)
		}
		if len(out) == maxIconCandidates {
			break
		}
	}
	return out
}

// iconAliasKey normalises a name for the alias table.
func iconAliasKey(name string) string {
	return strings.ToLower(strings.NewReplacer(" ", "", "-", "", "_", "", ".", "", "\t", "").Replace(strings.TrimSpace(name)))
}

// domainLabel reduces a host name to the label naming the service: "mail.google.com"
// gives "google". Anything else is returned unchanged.
func domainLabel(slug string) string {
	labels := strings.Split(slug, ".")
	end := len(labels)
	for end > 1 && iconTLDs[labels[end-1]] {
		end--
	}
	return labels[end-1]
}

// searchPrefixes are the icon sets preferred among the hits of a search: colour logos
// first, then monochrome brand marks.
var searchPrefixes = []string{"logos", "devicon", "skill-icons", "vscode-icons", "simple-icons", "fa6-brands", "fa-brands", "cib", "mdi"}

// bestSearchHit picks the icon of a search response. A search always answers with
// something, so only an icon named exactly after the service counts: the first hit
// for an unknown service would otherwise be an unrelated picture.
func bestSearchHit(icons []string, slugs iconSlugs) (string, string, bool) {
	wanted := map[string]bool{}
	for _, base := range []string{slugs.slug, slugs.alnum} {
		if base != "" {
			wanted[base] = true
			wanted[base+"-icon"] = true
			wanted[base+"-logo"] = true
		}
	}
	bestRank := len(searchPrefixes) + 1
	var bestPrefix, bestName string
	for _, raw := range icons {
		prefix, name, ok := strings.Cut(strings.TrimSpace(raw), ":")
		if !ok || !wanted[name] || !validSearchIdentifier(prefix) || !validSearchIdentifier(name) {
			continue
		}
		rank := len(searchPrefixes)
		for i, candidate := range searchPrefixes {
			if candidate == prefix {
				rank = i
				break
			}
		}
		if rank < bestRank {
			bestRank, bestPrefix, bestName = rank, prefix, name
		}
	}
	return bestPrefix, bestName, bestName != ""
}
