package main

import (
	"strings"
	"testing"
	"time"
)

func TestReadOutputConfigRequiresProjectID(t *testing.T) {
	t.Parallel()

	_, err := readOutputConfig(mapLookup(nil))
	if err == nil {
		t.Fatal("expected missing project_id error")
	}
	if !strings.Contains(err.Error(), optionProjectID) {
		t.Fatalf("error = %q, want project_id", err)
	}
}

func TestReadOutputConfigRequiresWIFOptionsWhenEnabled(t *testing.T) {
	t.Parallel()

	_, err := readOutputConfig(mapLookup(map[string]string{
		optionProjectID:                "example-project",
		optionEnableIdentityFederation: "true",
		optionAWSRegion:                "ap-northeast-1",
	}))
	if err == nil {
		t.Fatal("expected missing WIF options error")
	}

	for _, key := range []string{
		optionProjectNumber,
		optionPoolID,
		optionProviderID,
		optionGoogleServiceAccount,
	} {
		if !strings.Contains(err.Error(), key) {
			t.Fatalf("error = %q, want %s", err, key)
		}
	}
}

func TestReadOutputConfigRequiresExplicitAuthMode(t *testing.T) {
	t.Parallel()

	_, err := readOutputConfig(mapLookup(map[string]string{
		optionProjectID: "example-project",
	}))
	if err == nil {
		t.Fatal("expected auth mode error")
	}
	if !strings.Contains(err.Error(), optionEnableADC) {
		t.Fatalf("error = %q, want %s", err, optionEnableADC)
	}
}

func TestReadOutputConfigRejectsMultipleAuthModes(t *testing.T) {
	t.Parallel()

	_, err := readOutputConfig(mapLookup(map[string]string{
		optionProjectID:                "example-project",
		optionEnableADC:                "true",
		optionGoogleServiceCredentials: "/tmp/service-account.json",
	}))
	if err == nil {
		t.Fatal("expected multiple auth mode error")
	}
	if !strings.Contains(err.Error(), "only one authentication mode") {
		t.Fatalf("error = %q", err)
	}
}

func TestReadOutputConfigRejectsInvalidLogID(t *testing.T) {
	t.Parallel()

	_, err := readOutputConfig(mapLookup(map[string]string{
		optionProjectID: "example-project",
		optionEnableADC: "true",
		optionLogID:     "app/prod",
	}))
	if err == nil {
		t.Fatal("expected invalid log ID error")
	}
	if !strings.Contains(err.Error(), optionLogID) {
		t.Fatalf("error = %q, want %s", err, optionLogID)
	}
}

func TestReadOutputConfigNormalizesStaticLogName(t *testing.T) {
	t.Parallel()

	cfg, err := readOutputConfig(mapLookup(map[string]string{
		optionProjectID: "example-project",
		optionEnableADC: "true",
		optionLogName:   "projects/example-project/logs/app",
	}))
	if err != nil {
		t.Fatalf("readOutputConfig returned error: %v", err)
	}
	if cfg.LogID != "app" {
		t.Fatalf("LogID = %q, want app", cfg.LogID)
	}
}

func TestReadOutputConfigParsesDefaultsAndAliases(t *testing.T) {
	t.Parallel()

	cfg, err := readOutputConfig(mapLookup(map[string]string{
		"ProjectID":                    "example-project",
		optionLabels:                   "service=api, env = dev, ignored",
		optionEnableIdentityFederation: "yes",
		optionAWSRegion:                "ap-northeast-1",
		optionProjectNumber:            "123456789",
		optionPoolID:                   "pool",
		optionProviderID:               "provider",
		optionGoogleServiceAccount:     "logger@example.iam.gserviceaccount.com",
	}))
	if err != nil {
		t.Fatalf("readOutputConfig returned error: %v", err)
	}

	if cfg.ProjectID != "example-project" {
		t.Fatalf("ProjectID = %q", cfg.ProjectID)
	}
	if cfg.LogID != defaultLogID {
		t.Fatalf("LogID = %q, want %q", cfg.LogID, defaultLogID)
	}
	if cfg.Labels["service"] != "api" || cfg.Labels["env"] != "dev" {
		t.Fatalf("Labels = %#v", cfg.Labels)
	}
	if !cfg.EnableIdentityFederation {
		t.Fatal("EnableIdentityFederation = false")
	}
	if cfg.WIF.ProjectNumber != "123456789" {
		t.Fatalf("WIF.ProjectNumber = %q", cfg.WIF.ProjectNumber)
	}
	if cfg.Mapping.traceSampledKey != defaultTraceSampledKey ||
		cfg.Mapping.insertIDKey != defaultInsertIDKey ||
		cfg.Mapping.operationKey != defaultOperationKey ||
		cfg.Mapping.sourceLocationKey != defaultSourceLocationKey ||
		cfg.Mapping.httpRequestKey != defaultHTTPRequestKey ||
		cfg.Mapping.monitoredResourceKey != defaultMonitoredResourceKey ||
		cfg.Mapping.logNameKey != defaultLogNameKey {
		t.Fatalf("special keys were not defaulted: %#v", cfg)
	}
	if cfg.Mapping.enableResourceOverride || cfg.Mapping.enableCrossProjectTrace ||
		cfg.Mapping.autoformatStackdriverTrace || cfg.Mapping.useTagAsLogID {
		t.Fatalf("unsafe mapping options should default to false: %#v", cfg.Mapping)
	}
	if cfg.FlushTimeout != defaultFlushTimeout {
		t.Fatalf("FlushTimeout = %v", cfg.FlushTimeout)
	}
}

func TestReadOutputConfigParsesSafetyOptions(t *testing.T) {
	t.Parallel()

	cfg, err := readOutputConfig(mapLookup(map[string]string{
		optionProjectID:                  "example-project",
		optionEnableADC:                  "true",
		optionFlushTimeout:               "1500ms",
		optionMaxRecordDepth:             "4",
		optionMaxLabelCount:              "2",
		optionMaxLabelKeyLength:          "8",
		optionMaxLabelValueLength:        "16",
		optionMaxJSONBytes:               "1024",
		optionMaxStringBytes:             "2048",
		optionMaxFieldCount:              "32",
		optionMaxArrayItems:              "64",
		optionMaxPayloadBytes:            "4096",
		optionMaxLoggers:                 "3",
		optionEnableResourceOverride:     "true",
		optionAutoformatStackdriverTrace: "true",
		optionUseTagAsLogID:              "true",
		optionEnableCrossProjectTrace:    "true",
	}))
	if err != nil {
		t.Fatalf("readOutputConfig returned error: %v", err)
	}

	if cfg.FlushTimeout != 1500*time.Millisecond ||
		cfg.Limits.maxRecordDepth != 4 ||
		cfg.Limits.maxLabelCount != 2 ||
		cfg.Limits.maxLabelKeyLength != 8 ||
		cfg.Limits.maxLabelValueLength != 16 ||
		cfg.Limits.maxJSONBytes != 1024 ||
		cfg.Limits.maxStringBytes != 2048 ||
		cfg.Limits.maxFieldCount != 32 ||
		cfg.Limits.maxArrayItems != 64 ||
		cfg.Limits.maxPayloadBytes != 4096 ||
		cfg.MaxLoggers != 3 ||
		!cfg.Mapping.enableResourceOverride ||
		!cfg.Mapping.autoformatStackdriverTrace ||
		!cfg.Mapping.useTagAsLogID ||
		!cfg.Mapping.enableCrossProjectTrace {
		t.Fatalf("safety options = %#v", cfg)
	}
}

func TestReadOutputConfigRejectsInvalidSafetyOptions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		option      string
		value       string
		wantMessage string
	}{
		{
			name:        "invalid duration",
			option:      optionFlushTimeout,
			value:       "later",
			wantMessage: optionFlushTimeout,
		},
		{
			name:        "invalid integer",
			option:      optionMaxRecordDepth,
			value:       "deep",
			wantMessage: optionMaxRecordDepth,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			values := map[string]string{
				optionProjectID: "example-project",
				optionEnableADC: "true",
				test.option:     test.value,
			}
			_, err := readOutputConfig(mapLookup(values))
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), test.wantMessage) {
				t.Fatalf("error = %q, want %s", err, test.wantMessage)
			}
		})
	}
}

func TestMonitoredResource(t *testing.T) {
	t.Parallel()

	resource := outputConfig{
		ProjectID:      "example-project",
		ResourceType:   "generic_task",
		ResourceLabels: map[string]string{"job": "firelens", "task_id": "task-1"},
		Location:       "asia-northeast1",
		Namespace:      "ecs",
	}.monitoredResource()

	if resource.Type != "generic_task" {
		t.Fatalf("Type = %q", resource.Type)
	}
	if resource.Labels["project_id"] != "example-project" ||
		resource.Labels["location"] != "asia-northeast1" ||
		resource.Labels["namespace"] != "ecs" ||
		resource.Labels["job"] != "firelens" ||
		resource.Labels["task_id"] != "task-1" {
		t.Fatalf("Labels = %#v", resource.Labels)
	}
}

func TestReadOutputConfigParsesResourceLabelsAndTextPayloadKey(t *testing.T) {
	t.Parallel()

	cfg, err := readOutputConfig(mapLookup(map[string]string{
		optionProjectID:      "example-project",
		optionEnableADC:      "true",
		optionResourceLabels: "job=firelens,task_id=task-1",
		optionTextPayloadKey: "message",
		optionTagLabelKey:    "fluentbit_tag",
	}))
	if err != nil {
		t.Fatalf("readOutputConfig returned error: %v", err)
	}
	if cfg.ResourceLabels["job"] != "firelens" || cfg.ResourceLabels["task_id"] != "task-1" {
		t.Fatalf("ResourceLabels = %#v", cfg.ResourceLabels)
	}
	if cfg.Mapping.textPayloadKey != "message" {
		t.Fatalf("TextPayloadKey = %q", cfg.Mapping.textPayloadKey)
	}
	if cfg.Mapping.tagLabelKey != "fluentbit_tag" {
		t.Fatalf("TagLabelKey = %q", cfg.Mapping.tagLabelKey)
	}
}

func mapLookup(values map[string]string) configLookup {
	return func(key string) string {
		return values[key]
	}
}
