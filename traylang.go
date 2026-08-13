package main

import (
	"os"
	"strings"
	"sync"
)

// trayLanguage resolves once and is shared by everything that writes tray
// text: the system language cannot change under a running process, and the
// lookup costs a helper process on macOS.
var trayLanguage = sync.OnceValue(func() trayText { return trayTextForLocale(trayLocale()) })

// The tray is the one surface the OS draws for this app, and it is up before
// any page loads, so it follows the system language rather than the language
// picked inside the web UI, which this process never learns about.
func trayTextForLocale(locale string) trayText {
	if speaksChinese(locale) {
		return trayZH
	}
	return trayEN
}

// speaksChinese reads the language subtag out of anything a platform hands
// back: "zh-CN", "zh_CN.UTF-8", "zh-Hans-CN" or a bare "zh". A platform that
// will not say keeps the Chinese menu this app shipped with, rather than
// switching a user's tray to English on a locale lookup that merely failed.
func speaksChinese(locale string) bool {
	tag := strings.ToLower(strings.TrimSpace(locale))
	if tag == "" {
		return true
	}
	if cut := strings.IndexAny(tag, ".@"); cut >= 0 {
		tag = tag[:cut]
	}
	tag = strings.ReplaceAll(tag, "_", "-")
	return tag == "zh" || strings.HasPrefix(tag, "zh-")
}

// trayLocale asks the environment first: GA_ADMIN_LANG is there for a user
// whose system language and preferred menu language differ, and the POSIX
// variables are what a shell-launched process actually carries.
func trayLocale() string {
	for _, key := range []string{"GA_ADMIN_LANG", "LC_ALL", "LC_MESSAGES", "LANG"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return systemLocale()
}
