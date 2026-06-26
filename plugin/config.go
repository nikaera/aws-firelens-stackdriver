package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	mrpb "google.golang.org/genproto/googleapis/api/monitoredres"
)

const (
	pluginName = "stackdriver_wif"

	defaultLogID                = "fluent-bit"
	defaultResource             = "global"
	defaultLabelsKey            = "logging.googleapis.com/labels"
	defaultSeverityKey          = "logging.googleapis.com/severity"
	defaultTraceKey             = "logging.googleapis.com/trace"
	defaultSpanIDKey            = "logging.googleapis.com/spanId"
	defaultTraceSampledKey      = "logging.googleapis.com/traceSampled"
	defaultInsertIDKey          = "logging.googleapis.com/insertId"
	defaultOperationKey         = "logging.googleapis.com/operation"
	defaultSourceLocationKey    = "logging.googleapis.com/sourceLocation"
	defaultHTTPRequestKey       = "logging.googleapis.com/http_request"
	defaultMonitoredResourceKey = "logging.googleapis.com/monitored_resource"
	defaultLogNameKey           = "logging.googleapis.com/logName"

	defaultFlushTimeout        = 10 * time.Second
	defaultMaxRecordDepth      = 32
	defaultMaxLabelCount       = 64
	defaultMaxLabelKeyLength   = 512
	defaultMaxLabelValueLength = 4096
	defaultMaxJSONBytes        = 256 * 1024
	defaultMaxStringBytes      = 256 * 1024
	defaultMaxFieldCount       = 256
	defaultMaxArrayItems       = 1024
	defaultMaxPayloadBytes     = 512 * 1024
	defaultMaxLoggers          = 64

	optionProjectID                  = "project_id"
	optionGoogleServiceCredentials   = "google_service_credentials"
	optionEnableIdentityFederation   = "enable_identity_federation"
	optionEnableADC                  = "enable_adc"
	optionLogID                      = "log_id"
	optionLogName                    = "log_name"
	optionLabels                     = "labels"
	optionLabelsKey                  = "labels_key"
	optionSeverityKey                = "severity_key"
	optionTraceKey                   = "trace_key"
	optionAutoformatStackdriverTrace = "autoformat_stackdriver_trace"
	optionSpanIDKey                  = "span_id_key"
	optionTraceSampledKey            = "trace_sampled_key"
	optionInsertIDKey                = "insert_id_key"
	optionOperationKey               = "operation_key"
	optionSourceLocationKey          = "source_location_key"
	optionHTTPRequestKey             = "http_request_key"
	optionMonitoredResourceKey       = "monitored_resource_key"
	optionEnableResourceOverride     = "enable_resource_override"
	optionLogNameKey                 = "log_name_key"
	optionUseTagAsLogID              = "use_tag_as_log_id"
	optionEnableCrossProjectTrace    = "enable_cross_project_trace"
	optionTextPayloadKey             = "text_payload_key"
	optionTagLabelKey                = "tag_label_key"
	optionResource                   = "resource"
	optionResourceLabels             = "resource_labels"
	optionLocation                   = "location"
	optionNamespace                  = "namespace"
	optionFlushTimeout               = "flush_timeout"
	optionMaxRecordDepth             = "max_record_depth"
	optionMaxLabelCount              = "max_label_count"
	optionMaxLabelKeyLength          = "max_label_key_length"
	optionMaxLabelValueLength        = "max_label_value_length"
	optionMaxJSONBytes               = "max_json_bytes"
	optionMaxStringBytes             = "max_string_bytes"
	optionMaxFieldCount              = "max_field_count"
	optionMaxArrayItems              = "max_array_items"
	optionMaxPayloadBytes            = "max_payload_bytes"
	optionMaxLoggers                 = "max_loggers"
	optionAWSRegion                  = "aws_region"
	optionProjectNumber              = "project_number"
	optionPoolID                     = "pool_id"
	optionProviderID                 = "provider_id"
	optionGoogleServiceAccount       = "google_service_account"
)

type configLookup func(key string) string

type outputConfig struct {
	ProjectID                string
	GoogleServiceCredentials string
	EnableIdentityFederation bool
	EnableADC                bool
	LogID                    string
	Labels                   map[string]string
	Mapping                  entryMappingConfig
	ResourceType             string
	ResourceLabels           map[string]string
	Location                 string
	Namespace                string
	FlushTimeout             time.Duration
	Limits                   safetyLimits
	MaxLoggers               int
	WIF                      wifConfig
}

type entryMappingConfig struct {
	projectID                  string
	labelsKey                  string
	severityKey                string
	traceKey                   string
	autoformatStackdriverTrace bool
	spanIDKey                  string
	traceSampledKey            string
	insertIDKey                string
	operationKey               string
	sourceLocationKey          string
	httpRequestKey             string
	monitoredResourceKey       string
	enableResourceOverride     bool
	logNameKey                 string
	useTagAsLogID              bool
	enableCrossProjectTrace    bool
	textPayloadKey             string
	tagLabelKey                string
}

type safetyLimits struct {
	maxRecordDepth      int
	maxLabelCount       int
	maxLabelKeyLength   int
	maxLabelValueLength int
	maxJSONBytes        int
	maxStringBytes      int
	maxFieldCount       int
	maxArrayItems       int
	maxPayloadBytes     int
}

func readOutputConfig(lookup configLookup) (outputConfig, error) {
	cfg := outputConfig{
		ProjectID:                firstConfig(lookup, optionProjectID, "ProjectID", "ProjectId"),
		GoogleServiceCredentials: firstConfig(lookup, optionGoogleServiceCredentials, "Google_Service_Credentials"),
		EnableIdentityFederation: isEnabled(firstConfig(lookup, optionEnableIdentityFederation, "Enable_Identity_Federation")),
		EnableADC:                isEnabled(firstConfig(lookup, optionEnableADC, "Enable_ADC")),
		LogID:                    configOrDefault(lookup, defaultLogID, optionLogID, optionLogName, "LogID", "LogName"),
		Labels:                   parseLabels(firstConfig(lookup, optionLabels, "Labels")),
		ResourceType:             configOrDefault(lookup, defaultResource, optionResource, "Resource"),
		ResourceLabels:           parseLabels(firstConfig(lookup, optionResourceLabels, "Resource_Labels")),
		Location:                 firstConfig(lookup, optionLocation, "Location"),
		Namespace:                firstConfig(lookup, optionNamespace, "Namespace"),
		WIF: wifConfig{
			AWSRegion:            firstConfig(lookup, optionAWSRegion, "AWS_Region"),
			ProjectNumber:        firstConfig(lookup, optionProjectNumber, "Project_Number"),
			PoolID:               firstConfig(lookup, optionPoolID, "Pool_ID"),
			ProviderID:           firstConfig(lookup, optionProviderID, "Provider_ID"),
			GoogleServiceAccount: firstConfig(lookup, optionGoogleServiceAccount, "Google_Service_Account"),
		},
	}
	cfg.Mapping = readEntryMappingConfig(cfg.ProjectID, lookup)
	if err := populateSafetyLimits(&cfg, lookup); err != nil {
		return outputConfig{}, err
	}

	if cfg.ProjectID == "" {
		return outputConfig{}, fmt.Errorf("%s is required", optionProjectID)
	}
	logID, ok := logIDFromLogName(cfg.ProjectID, cfg.LogID)
	if !ok {
		return outputConfig{}, fmt.Errorf("%s contains unsupported characters or is too long", optionLogID)
	}
	cfg.LogID = logID
	if cfg.EnableIdentityFederation {
		if missing := cfg.WIF.missingRequiredOptions(); len(missing) > 0 {
			return outputConfig{}, fmt.Errorf("missing required WIF option(s): %s", strings.Join(missing, ", "))
		}
	}
	authModes := 0
	if cfg.EnableIdentityFederation {
		authModes++
	}
	if cfg.GoogleServiceCredentials != "" {
		authModes++
	}
	if cfg.EnableADC {
		authModes++
	}
	if authModes == 0 {
		return outputConfig{}, fmt.Errorf("one authentication mode is required: %s, %s, or %s", optionEnableIdentityFederation, optionGoogleServiceCredentials, optionEnableADC)
	}
	if authModes > 1 {
		return outputConfig{}, fmt.Errorf("only one authentication mode can be configured: %s, %s, or %s", optionEnableIdentityFederation, optionGoogleServiceCredentials, optionEnableADC)
	}
	if cfg.FlushTimeout <= 0 {
		return outputConfig{}, fmt.Errorf("%s must be greater than zero", optionFlushTimeout)
	}
	for name, value := range map[string]int{
		optionMaxRecordDepth:      cfg.Limits.maxRecordDepth,
		optionMaxLabelCount:       cfg.Limits.maxLabelCount,
		optionMaxLabelKeyLength:   cfg.Limits.maxLabelKeyLength,
		optionMaxLabelValueLength: cfg.Limits.maxLabelValueLength,
		optionMaxJSONBytes:        cfg.Limits.maxJSONBytes,
		optionMaxStringBytes:      cfg.Limits.maxStringBytes,
		optionMaxFieldCount:       cfg.Limits.maxFieldCount,
		optionMaxArrayItems:       cfg.Limits.maxArrayItems,
		optionMaxPayloadBytes:     cfg.Limits.maxPayloadBytes,
		optionMaxLoggers:          cfg.MaxLoggers,
	} {
		if value < 0 {
			return outputConfig{}, fmt.Errorf("%s must not be negative", name)
		}
	}

	return cfg, nil
}

func readEntryMappingConfig(projectID string, lookup configLookup) entryMappingConfig {
	return entryMappingConfig{
		projectID:                  projectID,
		labelsKey:                  configOrDefault(lookup, defaultLabelsKey, optionLabelsKey, "Labels_Key"),
		severityKey:                configOrDefault(lookup, defaultSeverityKey, optionSeverityKey, "Severity_Key"),
		traceKey:                   configOrDefault(lookup, defaultTraceKey, optionTraceKey, "Trace_Key"),
		autoformatStackdriverTrace: isEnabled(firstConfig(lookup, optionAutoformatStackdriverTrace, "Autoformat_Stackdriver_Trace")),
		spanIDKey:                  configOrDefault(lookup, defaultSpanIDKey, optionSpanIDKey, "Span_Id_Key"),
		traceSampledKey:            configOrDefault(lookup, defaultTraceSampledKey, optionTraceSampledKey, "Trace_Sampled_Key"),
		insertIDKey:                configOrDefault(lookup, defaultInsertIDKey, optionInsertIDKey, "Insert_Id_Key"),
		operationKey:               configOrDefault(lookup, defaultOperationKey, optionOperationKey, "Operation_Key"),
		sourceLocationKey:          configOrDefault(lookup, defaultSourceLocationKey, optionSourceLocationKey, "Source_Location_Key"),
		httpRequestKey:             configOrDefault(lookup, defaultHTTPRequestKey, optionHTTPRequestKey, "Http_Request_Key"),
		monitoredResourceKey:       configOrDefault(lookup, defaultMonitoredResourceKey, optionMonitoredResourceKey, "Monitored_Resource_Key"),
		enableResourceOverride:     isEnabled(firstConfig(lookup, optionEnableResourceOverride, "Enable_Resource_Override")),
		logNameKey:                 configOrDefault(lookup, defaultLogNameKey, optionLogNameKey, "Log_Name_Key"),
		useTagAsLogID:              isEnabled(firstConfig(lookup, optionUseTagAsLogID, "Use_Tag_As_Log_ID")),
		enableCrossProjectTrace:    isEnabled(firstConfig(lookup, optionEnableCrossProjectTrace, "Enable_Cross_Project_Trace")),
		textPayloadKey:             firstConfig(lookup, optionTextPayloadKey, "Text_Payload_Key"),
		tagLabelKey:                firstConfig(lookup, optionTagLabelKey, "Tag_Label_Key"),
	}
}

func populateSafetyLimits(cfg *outputConfig, lookup configLookup) error {
	var err error

	cfg.FlushTimeout, err = durationConfigOrDefault(lookup, defaultFlushTimeout, optionFlushTimeout, "Flush_Timeout")
	if err != nil {
		return err
	}
	cfg.Limits.maxRecordDepth, err = intConfigOrDefault(lookup, defaultMaxRecordDepth, optionMaxRecordDepth, "Max_Record_Depth")
	if err != nil {
		return err
	}
	cfg.Limits.maxLabelCount, err = intConfigOrDefault(lookup, defaultMaxLabelCount, optionMaxLabelCount, "Max_Label_Count")
	if err != nil {
		return err
	}
	cfg.Limits.maxLabelKeyLength, err = intConfigOrDefault(lookup, defaultMaxLabelKeyLength, optionMaxLabelKeyLength, "Max_Label_Key_Length")
	if err != nil {
		return err
	}
	cfg.Limits.maxLabelValueLength, err = intConfigOrDefault(lookup, defaultMaxLabelValueLength, optionMaxLabelValueLength, "Max_Label_Value_Length")
	if err != nil {
		return err
	}
	cfg.Limits.maxJSONBytes, err = intConfigOrDefault(lookup, defaultMaxJSONBytes, optionMaxJSONBytes, "Max_JSON_Bytes")
	if err != nil {
		return err
	}
	cfg.Limits.maxStringBytes, err = intConfigOrDefault(lookup, defaultMaxStringBytes, optionMaxStringBytes, "Max_String_Bytes")
	if err != nil {
		return err
	}
	cfg.Limits.maxFieldCount, err = intConfigOrDefault(lookup, defaultMaxFieldCount, optionMaxFieldCount, "Max_Field_Count")
	if err != nil {
		return err
	}
	cfg.Limits.maxArrayItems, err = intConfigOrDefault(lookup, defaultMaxArrayItems, optionMaxArrayItems, "Max_Array_Items")
	if err != nil {
		return err
	}
	cfg.Limits.maxPayloadBytes, err = intConfigOrDefault(lookup, defaultMaxPayloadBytes, optionMaxPayloadBytes, "Max_Payload_Bytes")
	if err != nil {
		return err
	}
	cfg.MaxLoggers, err = intConfigOrDefault(lookup, defaultMaxLoggers, optionMaxLoggers, "Max_Loggers")
	return err
}

func (c outputConfig) monitoredResource() *mrpb.MonitoredResource {
	labels := map[string]string{"project_id": c.ProjectID}
	for key, value := range c.ResourceLabels {
		labels[key] = value
	}

	if c.Location != "" {
		labels["location"] = c.Location
	}
	if c.Namespace != "" {
		labels["namespace"] = c.Namespace
	}

	return &mrpb.MonitoredResource{
		Type:   c.ResourceType,
		Labels: labels,
	}
}

func firstConfig(lookup configLookup, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(lookup(key)); value != "" {
			return value
		}
	}
	return ""
}

func configOrDefault(lookup configLookup, fallback string, keys ...string) string {
	if value := firstConfig(lookup, keys...); value != "" {
		return value
	}
	return fallback
}

func durationConfigOrDefault(lookup configLookup, fallback time.Duration, keys ...string) (time.Duration, error) {
	value := firstConfig(lookup, keys...)
	if value == "" {
		return fallback, nil
	}
	duration, err := time.ParseDuration(value)
	if err == nil {
		return duration, nil
	}
	seconds, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be a duration or seconds: %q", keys[0], value)
	}
	return time.Duration(seconds) * time.Second, nil
}

func intConfigOrDefault(lookup configLookup, fallback int, keys ...string) (int, error) {
	value := firstConfig(lookup, keys...)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %q", keys[0], value)
	}
	return parsed, nil
}

func parseLabels(raw string) map[string]string {
	labels := map[string]string{}
	for _, field := range strings.Split(raw, ",") {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		key, value, ok := strings.Cut(field, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		labels[key] = strings.TrimSpace(value)
	}
	return labels
}

func isEnabled(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "on", "yes", "1":
		return true
	default:
		return false
	}
}
