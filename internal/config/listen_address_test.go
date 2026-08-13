package config

import "testing"

func TestListenAddress(t *testing.T) {
	cases := []struct {
		name     string
		cfg      AppConfig
		override int
		want     string
	}{
		{
			name: "default stays on loopback with an ephemeral port",
			cfg:  AppConfig{Host: "127.0.0.1", Port: 8787},
			want: "127.0.0.1:0",
		},
		{
			name:     "port override pins the loopback port",
			cfg:      AppConfig{Host: "127.0.0.1", Port: 8787},
			override: 9001,
			want:     "127.0.0.1:9001",
		},
		{
			name: "remote access binds every interface on the fixed port",
			cfg:  AppConfig{Host: "127.0.0.1", Port: 8787, RemoteAccess: true},
			want: "0.0.0.0:8787",
		},
		{
			name: "remote access keeps an explicit non-loopback host",
			cfg:  AppConfig{Host: "192.168.1.10", Port: 9000, RemoteAccess: true},
			want: "192.168.1.10:9000",
		},
		{
			name: "remote access widens localhost",
			cfg:  AppConfig{Host: "localhost", Port: 9000, RemoteAccess: true},
			want: "0.0.0.0:9000",
		},
		{
			name: "remote access without a usable port falls back to the default",
			cfg:  AppConfig{RemoteAccess: true},
			want: "0.0.0.0:8787",
		},
		{
			name:     "override wins over the remote port",
			cfg:      AppConfig{Port: 8787, RemoteAccess: true},
			override: 9002,
			want:     "0.0.0.0:9002",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ListenAddress(tc.cfg, tc.override); got != tc.want {
				t.Fatalf("ListenAddress = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestValidateRejectsRemoteAccessWithoutFixedPort(t *testing.T) {
	cfg := Default()
	cfg.RemoteAccess = true
	cfg.Port = 0
	if err := Validate(cfg); err == nil {
		t.Fatal("expected remote access without a fixed port to fail validation")
	}
	cfg.Port = 8787
	if err := Validate(cfg); err != nil {
		t.Fatalf("valid remote config rejected: %v", err)
	}
}
