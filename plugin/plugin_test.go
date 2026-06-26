package main

import (
	"strings"
	"testing"

	"cloud.google.com/go/logging"
)

func TestLogIDFromLogName(t *testing.T) {
	t.Parallel()

	conf := &pluginConfig{projectID: "example-project"}
	tests := []struct {
		name string
		raw  string
		want string
		ok   bool
	}{
		{name: "plain log ID", raw: "app", want: "app", ok: true},
		{name: "plain log ID with allowed symbols", raw: "app.log_1-2", want: "app.log_1-2", ok: true},
		{name: "full log name", raw: "projects/example-project/logs/app", want: "app", ok: true},
		{name: "escaped slash", raw: "projects/example-project/logs/a%2Fb%2Fc"},
		{name: "other project", raw: "projects/other-project/logs/app"},
		{name: "unsupported resource path", raw: "folders/1/logs/app"},
		{name: "unsupported symbol", raw: "app:prod"},
		{name: "too long", raw: strings.Repeat("a", maxLogIDLength+1)},
		{name: "empty", raw: ""},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, ok := conf.logIDFromLogName(test.raw)
			if ok != test.ok || ok && got != test.want {
				t.Fatalf("logIDFromLogName(%q) = %q, %v; want %q, %v", test.raw, got, ok, test.want, test.ok)
			}
		})
	}
}

func TestLoggerForLogNameHonorsMaxLoggers(t *testing.T) {
	t.Parallel()

	conf := &pluginConfig{
		projectID:  "example-project",
		maxLoggers: 1,
		loggers: map[string]*logging.Logger{
			"existing": {},
		},
	}
	if logger := conf.loggerForLogName("new-log"); logger != nil {
		t.Fatalf("loggerForLogName returned %#v, want nil", logger)
	}
}
