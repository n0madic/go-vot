// Package service detects which video service a URL belongs to and extracts the
// data (video id, base url, duration) the VOT client needs. Ported from
// @vot.js/node (data/sites.js, utils/videoData.js, helpers/*).
package service

import (
	"net/url"
	"regexp"
)

// Host identifiers (subset used in branching logic). Values match the upstream
// CoreVideoService enum.
const (
	HostYouTube          = "youtube"
	HostCustom           = "custom"
	HostPeertube         = "peertube"
	HostCoursehunterLike = "coursehunterLike"
	HostCloudflareStream = "cloudflarestream"
	HostPatreon          = "patreon"
	HostVimeo            = "vimeo"
)

// matcher reports whether a parsed URL belongs to a service.
type matcher func(u *url.URL) bool

func reHost(pattern string) matcher {
	re := regexp.MustCompile(pattern)
	return func(u *url.URL) bool { return re.MatchString(u.Hostname()) }
}

func inList(list []string) matcher {
	set := make(map[string]struct{}, len(list))
	for _, d := range list {
		set[d] = struct{}{}
	}
	return func(u *url.URL) bool {
		_, ok := set[u.Hostname()]
		return ok
	}
}

func urlPred(fn func(*url.URL) bool) matcher { return fn }

func anyOf(ms ...matcher) matcher {
	return func(u *url.URL) bool {
		for _, m := range ms {
			if m(u) {
				return true
			}
		}
		return false
	}
}

// Service describes one supported video service.
type Service struct {
	Host          string
	BaseURL       string // base url prefix; videoId is appended to it
	Match         matcher
	NeedExtraData bool // requires an HTTP fetch to resolve duration / raw url
	RawResult     bool // the extracted id is already the final url
	Additional    string
}

// alternative-URL lists (YouTube frontends, mirrors). Ported from
// @vot.js/shared/data/alternativeUrls.js.
var (
	sitesInvidious = []string{
		"yewtu.be", "yt.artemislena.eu", "invidious.flokinet.to", "iv.melmac.space",
		"inv.nadeko.net", "inv.tux.pizza", "invidious.private.coffee", "yt.drgnz.club",
		"vid.puffyan.us", "invidious.dhusch.de",
	}
	sitesPiped = []string{
		"piped.video", "piped.tokhmi.xyz", "piped.moomoo.me", "piped.syncpundit.io",
		"piped.mha.fi", "watch.whatever.social", "piped.garudalinux.org", "efy.piped.pages.dev",
		"watch.leptons.xyz", "piped.lunar.icu", "yt.dc09.ru", "piped.mint.lgbt", "il.ax",
		"piped.privacy.com.de", "piped.esmailelbob.xyz", "piped.projectsegfau.lt",
		"piped.in.projectsegfau.lt", "piped.us.projectsegfau.lt", "piped.privacydev.net",
		"piped.palveluntarjoaja.eu", "piped.smnz.de", "piped.adminforge.de", "piped.qdi.fi",
		"piped.hostux.net", "piped.chauvet.pro", "piped.jotoma.de", "piped.pfcd.me",
		"piped.frontendfriendly.xyz",
	}
	sitesProxiTok = []string{
		"proxitok.pabloferreiro.es", "proxitok.pussthecat.org", "tok.habedieeh.re",
		"proxitok.esmailelbob.xyz", "proxitok.privacydev.net", "tok.artemislena.eu",
		"tok.adminforge.de", "tt.vern.cc", "cringe.whatever.social", "proxitok.lunar.icu",
		"proxitok.privacy.com.de",
	}
	sitesPeertube = []string{
		"peertube.1312.media", "tube.shanti.cafe", "bee-tube.fr", "video.sadmin.io",
		"dalek.zone", "review.peertube.biz", "peervideo.club", "tube.la-dina.net",
		"peertube.tmp.rcp.tf", "peertube.su", "video.blender.org", "videos.viorsan.com",
		"tube-sciences-technologies.apps.education.fr", "tube-numerique-educatif.apps.education.fr",
		"tube-arts-lettres-sciences-humaines.apps.education.fr", "beetoons.tv",
		"comics.peertube.biz", "makertube.net",
	}
	sitesPoketube = []string{
		"poketube.fun", "pt.sudovanilla.org", "poke.ggtyler.dev",
		"poke.uk2.littlekai.co.uk", "poke.blahai.gay",
	}
	sitesRicktube         = []string{"ricktube.ru"}
	sitesCoursehunterLike = []string{"coursehunter.net", "coursetrain.net"}
)

// mp4WebmRe matches direct .mp4/.webm links handled as raw custom links.
var mp4WebmRe = regexp.MustCompile(`([^.]+)\.(mp4|webm)`)

// sites is the ordered service registry. Detection picks the first match.
// Ported entry-for-entry from @vot.js/node/data/sites.js.
var sites = []Service{
	{Host: HostYouTube, BaseURL: "https://youtu.be/", Match: reHost(`^((www.|m.)?youtube(-nocookie|kids)?.com)|(youtu.be)$`)},
	{Host: "invidious", BaseURL: "https://youtu.be/", Match: inList(sitesInvidious)},
	{Host: "piped", BaseURL: "https://youtu.be/", Match: inList(sitesPiped)},
	{Host: "poketube", BaseURL: "https://youtu.be/", Match: inList(sitesPoketube)},
	{Host: "ricktube", BaseURL: "https://youtu.be/", Match: inList(sitesRicktube)},
	{Host: "vk", BaseURL: "https://vk.com/video?z=", Match: anyOf(reHost(`^(www.|m.)?vk.(com|ru)$`), reHost(`^(www.|m.)?vkvideo.ru$`))},
	{Host: "nine_gag", BaseURL: "https://9gag.com/gag/", Match: reHost(`^9gag.com$`)},
	{Host: "twitch", BaseURL: "https://twitch.tv/", Match: anyOf(reHost(`^m.twitch.tv$`), reHost(`^(www.)?twitch.tv$`), reHost(`^clips.twitch.tv$`), reHost(`^player.twitch.tv$`))},
	{Host: "proxitok", BaseURL: "https://www.tiktok.com/", Match: inList(sitesProxiTok)},
	{Host: "tiktok", BaseURL: "https://www.tiktok.com/", Match: reHost(`^(www.)?tiktok.com$`)},
	{Host: HostVimeo, BaseURL: "https://vimeo.com/", Match: reHost(`^vimeo.com$`), NeedExtraData: true},
	{Host: HostVimeo, BaseURL: "https://player.vimeo.com/", Match: reHost(`^player.vimeo.com$`), Additional: "embed", NeedExtraData: true},
	{Host: "xvideos", BaseURL: "https://www.xvideos.com/", Match: anyOf(reHost(`^(www.)?xvideos(-ar)?.com$`), reHost(`^(www.)?xvideos(\d\d\d).com$`), reHost(`^(www.)?xv-ru.com$`))},
	{Host: "pornhub", BaseURL: "https://rt.pornhub.com/view_video.php?viewkey=", Match: reHost(`^[a-z]+.pornhub.(com|org)$`)},
	{Host: "twitter", BaseURL: "https://twitter.com/i/status/", Match: reHost(`^(twitter|x).com$`)},
	{Host: "rumble", BaseURL: "https://rumble.com/", Match: reHost(`^rumble.com$`)},
	{Host: "facebook", BaseURL: "https://facebook.com/", Match: urlPred(func(u *url.URL) bool {
		return regexp.MustCompile(`facebook\.com`).MatchString(u.Host) &&
			(regexp.MustCompile(`/videos/`).MatchString(u.Path) || regexp.MustCompile(`/reel/`).MatchString(u.Path))
	})},
	{Host: "rutube", BaseURL: "https://rutube.ru/video/", Match: reHost(`^rutube.ru$`)},
	{Host: "bilibili", BaseURL: "https://www.bilibili.com/", Match: reHost(`^(www|m|player).bilibili.com$`)},
	{Host: "mailru", BaseURL: "https://my.mail.ru/", Match: reHost(`^my.mail.ru$`)},
	{Host: "bitchute", BaseURL: "https://www.bitchute.com/video/", Match: reHost(`^(www.)?bitchute.com$`)},
	{Host: "eporner", BaseURL: "https://www.eporner.com/", Match: reHost(`^(www.)?eporner.com$`)},
	{Host: HostPeertube, BaseURL: "stub", Match: inList(sitesPeertube)},
	{Host: "dailymotion", BaseURL: "https://dai.ly/", Match: reHost(`^(www.)?dailymotion.com|dai.ly$`)},
	{Host: "trovo", BaseURL: "https://trovo.live/s/", Match: reHost(`^trovo.live$`)},
	{Host: "yandexdisk", BaseURL: "https://yadi.sk/", Match: reHost(`^disk.yandex.(ru|kz|com(\.(am|ge|tr))?|by|az|co\.il|ee|lt|lv|md|net|tj|tm|uz)|yadi.sk$`), NeedExtraData: true},
	{Host: "okru", BaseURL: "https://ok.ru/video/", Match: reHost(`^ok.ru$`)},
	{Host: "googledrive", BaseURL: "https://drive.google.com/file/d/", Match: reHost(`^drive.google.com$`)},
	{Host: "bannedvideo", BaseURL: "https://madmaxworld.tv/watch?id=", Match: reHost(`^(www.)?banned.video|madmaxworld.tv$`), NeedExtraData: true},
	{Host: "weverse", BaseURL: "https://weverse.io/", Match: reHost(`^weverse.io$`), NeedExtraData: true},
	{Host: "newgrounds", BaseURL: "https://www.newgrounds.com/", Match: reHost(`^(www.)?newgrounds.com$`)},
	{Host: "egghead", BaseURL: "https://egghead.io/", Match: reHost(`^egghead.io$`)},
	{Host: "youku", BaseURL: "https://v.youku.com/", Match: reHost(`^v.youku.com$`)},
	{Host: "archive", BaseURL: "https://archive.org/details/", Match: reHost(`^archive.org$`)},
	{Host: "kodik", BaseURL: "stub", Match: reHost(`^kodik.(info|biz|cc)$`), NeedExtraData: true},
	{Host: HostPatreon, BaseURL: "stub", Match: reHost(`^(www.)?patreon.com$`), NeedExtraData: true},
	{Host: "reddit", BaseURL: "stub", Match: reHost(`^(www.|new.|old.)?reddit.com$`), NeedExtraData: true},
	{Host: "kick", BaseURL: "https://kick.com/", Match: reHost(`^kick.com$`), NeedExtraData: true},
	{Host: "appledeveloper", BaseURL: "https://developer.apple.com/", Match: reHost(`^developer.apple.com$`), NeedExtraData: true},
	{Host: "epicgames", BaseURL: "https://dev.epicgames.com/community/learning/", Match: reHost(`^dev.epicgames.com$`), NeedExtraData: true},
	{Host: "odysee", BaseURL: "stub", Match: reHost(`^odysee.com$`), NeedExtraData: true},
	{Host: HostCoursehunterLike, BaseURL: "stub", Match: inList(sitesCoursehunterLike), NeedExtraData: true},
	{Host: "sap", BaseURL: "https://learning.sap.com/courses/", Match: reHost(`^learning.sap.com$`), NeedExtraData: true},
	{Host: "watchpornto", BaseURL: "https://watchporn.to/", Match: reHost(`^watchporn.to$`)},
	{Host: "linkedin", BaseURL: "https://www.linkedin.com/learning/", Match: reHost(`^(www.)?linkedin.com$`), NeedExtraData: true},
	{Host: "incestflix", BaseURL: "https://www.incestflix.net/watch/", Match: reHost(`^(www.)?incestflix.(net|to|com)$`), NeedExtraData: true},
	{Host: "porntn", BaseURL: "https://porntn.com/videos/", Match: reHost(`^porntn.com$`), NeedExtraData: true},
	{Host: "dzen", BaseURL: "https://dzen.ru/video/watch/", Match: reHost(`^dzen.ru$`)},
	{Host: HostCloudflareStream, BaseURL: "stub", Match: reHost(`^(watch|embed|iframe|customer-[^.]+).cloudflarestream.com$`)},
	{Host: "loom", BaseURL: "https://www.loom.com/share/", Match: reHost(`^(www.)?loom.com$`)},
	{Host: "rtnews", BaseURL: "https://www.rt.com/", Match: reHost(`^(www.)?rt.com$`), NeedExtraData: true},
	{Host: "bitview", BaseURL: "https://www.bitview.net/watch?v=", Match: reHost(`^(www.)?bitview.net$`), NeedExtraData: true},
	{Host: "thisvid", BaseURL: "https://thisvid.com/", Match: reHost(`^(www.)?thisvid.com$`)},
	{Host: "ign", BaseURL: "https://de.ign.com/", Match: reHost(`^(\w{2}.)ign.com$`), NeedExtraData: true},
	{Host: "ign", BaseURL: "https://www.ign.com/", Match: reHost(`^(www.)?ign.com$`), NeedExtraData: true},
	{Host: "bunkr", BaseURL: "https://bunkr.site/", Match: reHost(`^bunkr.(site|black|cat|media|red|site|ws|org|s[kiu]|c[ir]|fi|p[hks]|ru|la|is|to|a[cx])$`), NeedExtraData: true},
	{Host: "imdb", BaseURL: "https://www.imdb.com/video/", Match: reHost(`^(www.)?imdb.com$`)},
	{Host: "telegram", BaseURL: "https://t.me/", Match: reHost(`^t.me$`)},
	{Host: HostCustom, BaseURL: "stub", Match: urlPred(func(u *url.URL) bool { return mp4WebmRe.MatchString(u.Path) }), RawResult: true},
}

// Sites returns the service registry (read-only).
func Sites() []Service { return sites }
