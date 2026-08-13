package main

import (
	"strings"
	"testing"

	"genericagent-admin-go/internal/config"
)

func TestTrayLanguageFollowsTheLocaleTag(t *testing.T) {
	chinese := []string{"zh", "zh-CN", "zh_CN.UTF-8", "zh-Hans-CN", "ZH-TW", " zh_SG "}
	for _, locale := range chinese {
		if !speaksChinese(locale) {
			t.Fatalf("%q was not read as Chinese", locale)
		}
	}
	other := []string{"en-US", "en_GB.UTF-8", "ja-JP", "de", "zhuang"}
	for _, locale := range other {
		if speaksChinese(locale) {
			t.Fatalf("%q was read as Chinese", locale)
		}
	}
}

// A locale lookup that fails must not silently switch a user's tray to a
// language they never asked for.
func TestTrayLanguageKeepsChineseWhenThePlatformStaysSilent(t *testing.T) {
	if trayTextForLocale("").OpenChat != trayZH.OpenChat {
		t.Fatal("an unknown locale did not keep the Chinese menu")
	}
	if trayTextForLocale("en-US").OpenChat != trayEN.OpenChat {
		t.Fatal("an English locale did not switch the menu")
	}
}

func TestTrayLocalePrefersTheExplicitOverride(t *testing.T) {
	t.Setenv("GA_ADMIN_LANG", "en-US")
	t.Setenv("LANG", "zh_CN.UTF-8")
	if got := trayLocale(); got != "en-US" {
		t.Fatalf("locale = %q, want the override", got)
	}
}

// The entry is only in the menu while something is running, so the label only
// has to name the count it would end.
func TestStopServicesLabelCountsWhatItWouldStop(t *testing.T) {
	if got := stopServicesLabel(trayZH, 3); !strings.Contains(got, "3") {
		t.Fatalf("running label = %q, want the count", got)
	}
	if got := stopServicesLabel(trayEN, 1); got != "Stop all services (1 running)" {
		t.Fatalf("english running label = %q", got)
	}
}

func TestTrayAppRunningServicesDefaultsToZero(t *testing.T) {
	if got := (trayApp{}).runningServices(); got != 0 {
		t.Fatalf("running services without a provider = %d", got)
	}
}

// The whole menu has to move together: an English tray that still says
// "访问范围" would be worse than either language alone.
func TestTrayStatusSpeaksTheChosenLanguage(t *testing.T) {
	cfg := config.Default()
	cfg.RemoteAccess = true
	cfg.Port = 8787

	status := describeTrayStatus("0.0.0.0:8787", cfg, true, lanIP(testLAN), trayEN)

	if status.LocalLabel != "Address: 127.0.0.1:8787" {
		t.Fatalf("local label = %q", status.LocalLabel)
	}
	if status.LANLabel != "LAN: 192.168.1.20:8787" {
		t.Fatalf("LAN label = %q", status.LANLabel)
	}
	if status.ScopeLabel != trayEN.ScopeLabel+trayEN.ScopeRemotePassword {
		t.Fatalf("scope = %q", status.ScopeLabel)
	}
	for _, label := range []string{status.LocalLabel, status.LANLabel, status.ScopeLabel, status.Tooltip} {
		for _, r := range label {
			if r > 0x2E7F {
				t.Fatalf("English tray label %q kept a Chinese character", label)
			}
		}
	}
}
