package main

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"cloud.google.com/go/logging"
	"github.com/google/go-cmp/cmp"
)

func TestEntryFromRecordMapsSpecialFields(t *testing.T) {
	t.Parallel()

	ts := time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		configure  func(*pluginConfig)
		record     map[any]any
		removedKey string
		check      func(*testing.T, logging.Entry)
	}{
		{
			name: "severity",
			configure: func(conf *pluginConfig) {
				conf.mapper.mapping.severityKey = "level"
			},
			record:     map[any]any{"message": "hello", "level": "warn"},
			removedKey: "level",
			check: func(t *testing.T, entry logging.Entry) {
				t.Helper()
				if entry.Severity != logging.Warning {
					t.Fatalf("Severity = %v, want Warning", entry.Severity)
				}
			},
		},
		{
			name: "trace",
			configure: func(conf *pluginConfig) {
				conf.mapper.mapping.traceKey = "trace"
				conf.mapper.mapping.autoformatStackdriverTrace = true
			},
			record:     map[any]any{"message": "hello", "trace": "abc123"},
			removedKey: "trace",
			check: func(t *testing.T, entry logging.Entry) {
				t.Helper()
				if entry.Trace != "projects/example-project/traces/abc123" {
					t.Fatalf("Trace = %q", entry.Trace)
				}
			},
		},
		{
			name: "span ID",
			configure: func(conf *pluginConfig) {
				conf.mapper.mapping.spanIDKey = "span_id"
			},
			record:     map[any]any{"message": "hello", "span_id": "span-1"},
			removedKey: "span_id",
			check: func(t *testing.T, entry logging.Entry) {
				t.Helper()
				if entry.SpanID != "span-1" {
					t.Fatalf("SpanID = %q", entry.SpanID)
				}
			},
		},
		{
			name:       "trace sampled",
			record:     map[any]any{"message": "hello", defaultTraceSampledKey: true},
			removedKey: defaultTraceSampledKey,
			check: func(t *testing.T, entry logging.Entry) {
				t.Helper()
				if !entry.TraceSampled {
					t.Fatal("TraceSampled = false")
				}
			},
		},
		{
			name:       "insert ID",
			record:     map[any]any{"message": "hello", defaultInsertIDKey: "insert-1"},
			removedKey: defaultInsertIDKey,
			check: func(t *testing.T, entry logging.Entry) {
				t.Helper()
				if entry.InsertID != "insert-1" {
					t.Fatalf("InsertID = %q", entry.InsertID)
				}
			},
		},
		{
			name: "operation",
			record: map[any]any{
				"message": "hello",
				defaultOperationKey: map[any]any{
					"id":       "operation-1",
					"producer": "test",
					"first":    true,
					"extra":    "kept",
				},
			},
			check: func(t *testing.T, entry logging.Entry) {
				t.Helper()
				if entry.Operation == nil || entry.Operation.Id != "operation-1" || !entry.Operation.First {
					t.Fatalf("Operation = %#v", entry.Operation)
				}
				assertPayloadNestedField(t, entry, defaultOperationKey, "extra", "kept")
			},
		},
		{
			name: "source location",
			record: map[any]any{
				"message": "hello",
				defaultSourceLocationKey: map[any]any{
					"file":     "main.go",
					"line":     42,
					"function": "main.run",
					"extra":    "kept",
				},
			},
			check: func(t *testing.T, entry logging.Entry) {
				t.Helper()
				if entry.SourceLocation == nil || entry.SourceLocation.File != "main.go" || entry.SourceLocation.Line != 42 {
					t.Fatalf("SourceLocation = %#v", entry.SourceLocation)
				}
				assertPayloadNestedField(t, entry, defaultSourceLocationKey, "extra", "kept")
			},
		},
		{
			name: "HTTP request",
			record: map[any]any{
				"message": "hello",
				defaultHTTPRequestKey: map[any]any{
					"requestMethod": "GET",
					"requestUrl":    "https://example.com/path",
					"status":        200,
					"latency":       "150ms",
					"remoteIp":      "192.0.2.1",
					"userAgent":     "test-agent",
					"extra":         "kept",
				},
			},
			check: func(t *testing.T, entry logging.Entry) {
				t.Helper()
				if entry.HTTPRequest == nil || entry.HTTPRequest.Status != 200 || entry.HTTPRequest.Request == nil {
					t.Fatalf("HTTPRequest = %#v", entry.HTTPRequest)
				}
				assertPayloadNestedField(t, entry, defaultHTTPRequestKey, "extra", "kept")
			},
		},
		{
			name: "monitored resource",
			configure: func(conf *pluginConfig) {
				conf.mapper.mapping.enableResourceOverride = true
			},
			record: map[any]any{
				"message": "hello",
				defaultMonitoredResourceKey: map[any]any{
					"type": "generic_task",
					"labels": map[any]any{
						"project_id": "example-project",
						"location":   "asia-northeast1",
					},
				},
			},
			removedKey: defaultMonitoredResourceKey,
			check: func(t *testing.T, entry logging.Entry) {
				t.Helper()
				if entry.Resource == nil || entry.Resource.Type != "generic_task" || entry.Resource.Labels["location"] != "asia-northeast1" {
					t.Fatalf("Resource = %#v", entry.Resource)
				}
			},
		},
		{
			name: "labels",
			record: map[any]any{
				"message": "hello",
				defaultLabelsKey: map[any]any{
					"service": "api",
					"version": 1,
				},
			},
			removedKey: defaultLabelsKey,
			check: func(t *testing.T, entry logging.Entry) {
				t.Helper()
				want := map[string]string{"service": "api", "version": "1"}
				if diff := cmp.Diff(want, entry.Labels); diff != "" {
					t.Fatalf("Labels mismatch (-want +got):\n%s", diff)
				}
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			conf := testPluginConfig()
			if test.configure != nil {
				test.configure(conf)
			}
			entry := conf.entryFromRecord(ts, test.record)
			if !entry.Timestamp.Equal(ts) {
				t.Fatalf("Timestamp = %v, want %v", entry.Timestamp, ts)
			}
			test.check(t, entry)
			assertPayloadFieldRemoved(t, entry, test.removedKey)
		})
	}
}

func TestTextPayloadMapping(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		record map[any]any
		want   any
	}{
		{
			name:   "uses text payload when only residual field",
			record: map[any]any{"message": "hello"},
			want:   "hello",
		},
		{
			name: "keeps JSON payload when text field is not alone",
			record: map[any]any{
				"message": "hello",
				"service": "api",
			},
			want: map[string]any{"message": "hello", "service": "api"},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			conf := testPluginConfig()
			conf.mapper.mapping.textPayloadKey = "message"
			entry := conf.entryFromRecord(time.Now(), test.record)
			if diff := cmp.Diff(test.want, entry.Payload); diff != "" {
				t.Fatalf("Payload mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestLoggerAndEntryFromRecordAddsTagLabel(t *testing.T) {
	t.Parallel()

	conf := testPluginConfig()
	conf.mapper.mapping.tagLabelKey = "fluentbit_tag"
	_, entry := conf.loggerAndEntryFromRecord(time.Now(), map[any]any{"message": "hello"}, "app.firelens")
	if entry.Labels["fluentbit_tag"] != "app.firelens" {
		t.Fatalf("Labels = %#v", entry.Labels)
	}
}

func TestInvalidInsertIDStaysInPayload(t *testing.T) {
	t.Parallel()

	conf := testPluginConfig()
	_, entry := conf.loggerAndEntryFromRecord(time.Now(), map[any]any{
		"message":          "hello",
		defaultInsertIDKey: 123,
	}, "")
	if entry.InsertID != "" {
		t.Fatalf("InsertID = %q, want empty", entry.InsertID)
	}
	payload, ok := entry.Payload.(map[string]any)
	if !ok {
		t.Fatalf("Payload type = %T", entry.Payload)
	}
	if payload[defaultInsertIDKey] != 123 {
		t.Fatalf("payload insert ID = %#v", payload[defaultInsertIDKey])
	}
}

func TestNormalizeValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		maxBytes int
		value    []byte
		want     any
		wantJSON bool
	}{
		{
			name:     "preserves JSON bytes",
			maxBytes: defaultMaxJSONBytes,
			value:    []byte(`{"ok":true}`),
			want:     `{"ok":true}`,
			wantJSON: true,
		},
		{
			name:     "converts non JSON bytes to string",
			maxBytes: defaultMaxJSONBytes,
			value:    []byte(`not-json`),
			want:     "not-json",
		},
		{
			name:     "skips large JSON validation",
			maxBytes: 2,
			value:    []byte(`{"ok":true}`),
			want:     `{"ok":true}`,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			mapper := testEntryMapper()
			mapper.limits.maxJSONBytes = test.maxBytes
			normalized := mapper.normalizeValue(test.value, 0)
			if test.wantJSON {
				raw, ok := normalized.(json.RawMessage)
				if !ok {
					t.Fatalf("normalized type = %T, want json.RawMessage", normalized)
				}
				if string(raw) != test.want {
					t.Fatalf("raw = %s, want %s", raw, test.want)
				}
				return
			}
			if normalized != test.want {
				t.Fatalf("normalized = %#v, want %#v", normalized, test.want)
			}
		})
	}
}

func TestNormalizeMapAppliesPayloadLimits(t *testing.T) {
	t.Parallel()

	mapper := testEntryMapper()
	mapper.limits.maxFieldCount = 1
	mapper.limits.maxArrayItems = 2
	mapper.limits.maxStringBytes = 4

	payload := mapper.normalizeMap(map[any]any{
		"long":  "abcdef",
		"array": []any{"a", "b", "c"},
	}, 0)

	if payload[payloadTruncatedKey] != true {
		t.Fatalf("payload truncated marker = %#v", payload)
	}
	for key, value := range payload {
		if key == payloadTruncatedKey {
			continue
		}
		if key == "long" && value != "abcd" {
			t.Fatalf("string value = %#v", value)
		}
	}
}

func TestNormalizeMapPrioritizesSpecialFields(t *testing.T) {
	t.Parallel()

	conf := testPluginConfig()
	conf.mapper.mapping.severityKey = "level"
	conf.mapper.limits.maxFieldCount = 1

	_, entry := conf.loggerAndEntryFromRecord(time.Now(), map[any]any{
		"message": "hello",
		"service": "api",
		"level":   "warning",
	}, "")

	if entry.Severity != logging.Warning {
		t.Fatalf("Severity = %v, want Warning", entry.Severity)
	}
	payload, ok := entry.Payload.(map[string]any)
	if !ok {
		t.Fatalf("Payload type = %T", entry.Payload)
	}
	if _, ok := payload["level"]; ok {
		t.Fatalf("payload still contains level: %#v", payload)
	}
	if payload[payloadTruncatedKey] != true {
		t.Fatalf("payload truncated marker = %#v", payload)
	}
}

func TestExtractLabelsAppliesLimits(t *testing.T) {
	t.Parallel()

	mapper := testEntryMapper()
	mapper.limits.maxLabelCount = 1
	mapper.limits.maxLabelKeyLength = 3
	mapper.limits.maxLabelValueLength = 4
	labels, ok := mapper.extractStringMap(map[string]any{
		"labels": map[string]any{
			"abcdef": "123456",
		},
	}, "labels")
	if !ok {
		t.Fatal("extractStringMap ok = false")
	}
	want := map[string]string{"abc": "1234"}
	if diff := cmp.Diff(want, labels); diff != "" {
		t.Fatalf("labels mismatch (-want +got):\n%s", diff)
	}
}

func TestExtractSeverity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		raw  string
		want logging.Severity
		ok   bool
	}{
		{raw: "debug", want: logging.Debug, ok: true},
		{raw: "INFO", want: logging.Info, ok: true},
		{raw: "warn", want: logging.Warning, ok: true},
		{raw: "error", want: logging.Error, ok: true},
		{raw: "fatal", want: logging.Emergency, ok: true},
		{raw: "unknown", want: logging.Default},
	}

	for _, test := range tests {
		test := test
		t.Run(test.raw, func(t *testing.T) {
			t.Parallel()

			got, ok := extractSeverity(map[string]any{"level": test.raw}, "level")
			if ok != test.ok || got != test.want {
				t.Fatalf("extractSeverity(%q) = %v, %v; want %v, %v", test.raw, got, ok, test.want, test.ok)
			}
		})
	}
}

func TestInt64FromMapRejectsUnsafeFloats(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value any
	}{
		{name: "fraction", value: 200.5},
		{name: "NaN", value: math.NaN()},
		{name: "positive infinity", value: math.Inf(1)},
		{name: "negative infinity", value: math.Inf(-1)},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got, ok := int64FromMap(map[string]any{"value": test.value}, "value"); ok {
				t.Fatalf("int64FromMap = %d, true; want false", got)
			}
		})
	}
}

func TestFormatTrace(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		trace             string
		autoformat        bool
		allowCrossProject bool
		want              string
		ok                bool
	}{
		{name: "keeps bare trace by default", trace: "abc", want: "abc", ok: true},
		{name: "autoformats bare trace", trace: "abc", autoformat: true, want: "projects/example-project/traces/abc", ok: true},
		{name: "rejects cross project by default", trace: "projects/other/traces/abc"},
		{name: "allows cross project when enabled", trace: "projects/other/traces/abc", allowCrossProject: true, want: "projects/other/traces/abc", ok: true},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, ok := formatTrace("example-project", test.trace, test.autoformat, test.allowCrossProject)
			if ok != test.ok || ok && got != test.want {
				t.Fatalf("formatTrace = %q, %v; want %q, %v", got, ok, test.want, test.ok)
			}
		})
	}
}

func testPluginConfig() *pluginConfig {
	return &pluginConfig{
		projectID: "example-project",
		mapper:    testEntryMapper(),
	}
}

func testEntryMapper() entryMapper {
	return entryMapper{
		mapping: entryMappingConfig{
			projectID:            "example-project",
			labelsKey:            defaultLabelsKey,
			severityKey:          defaultSeverityKey,
			traceKey:             defaultTraceKey,
			spanIDKey:            defaultSpanIDKey,
			traceSampledKey:      defaultTraceSampledKey,
			insertIDKey:          defaultInsertIDKey,
			operationKey:         defaultOperationKey,
			sourceLocationKey:    defaultSourceLocationKey,
			httpRequestKey:       defaultHTTPRequestKey,
			monitoredResourceKey: defaultMonitoredResourceKey,
			logNameKey:           defaultLogNameKey,
		},
		limits: safetyLimits{
			maxRecordDepth:      defaultMaxRecordDepth,
			maxLabelCount:       defaultMaxLabelCount,
			maxLabelKeyLength:   defaultMaxLabelKeyLength,
			maxLabelValueLength: defaultMaxLabelValueLength,
			maxJSONBytes:        defaultMaxJSONBytes,
			maxStringBytes:      defaultMaxStringBytes,
			maxFieldCount:       defaultMaxFieldCount,
			maxArrayItems:       defaultMaxArrayItems,
			maxPayloadBytes:     defaultMaxPayloadBytes,
		},
	}
}

func assertPayloadFieldRemoved(t *testing.T, entry logging.Entry, key string) {
	t.Helper()

	payload, ok := entry.Payload.(map[string]any)
	if !ok {
		t.Fatalf("Payload type = %T", entry.Payload)
	}
	if _, ok := payload[key]; ok {
		t.Fatalf("payload still contains %q: %#v", key, payload)
	}
	if payload["message"] != "hello" {
		t.Fatalf("message = %#v", payload["message"])
	}
}

func assertPayloadNestedField(t *testing.T, entry logging.Entry, key, nestedKey string, want any) {
	t.Helper()

	payload, ok := entry.Payload.(map[string]any)
	if !ok {
		t.Fatalf("Payload type = %T", entry.Payload)
	}
	nested, ok := payload[key].(map[string]any)
	if !ok {
		t.Fatalf("Payload[%q] = %#v", key, payload[key])
	}
	if nested[nestedKey] != want {
		t.Fatalf("Payload[%q][%q] = %#v, want %#v", key, nestedKey, nested[nestedKey], want)
	}
	if payload["message"] != "hello" {
		t.Fatalf("message = %#v", payload["message"])
	}
}
