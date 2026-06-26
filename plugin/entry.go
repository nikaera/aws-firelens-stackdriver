package main

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/logging"
	"cloud.google.com/go/logging/apiv2/loggingpb"
	mrpb "google.golang.org/genproto/googleapis/api/monitoredres"
)

const (
	truncatedMarker      = "[truncated]"
	payloadTruncatedKey  = "logging.googleapis.com/truncated"
	maxRecordDepthMarker = "[max record depth exceeded]"
)

var httpRequestSubfields = []string{
	"requestMethod",
	"requestUrl",
	"requestSize",
	"status",
	"responseSize",
	"latency",
	"serverIp",
	"remoteIp",
	"cacheHit",
	"cacheValidatedWithOriginServer",
	"cacheFillBytes",
	"cacheLookup",
	"protocol",
	"userAgent",
	"referer",
}

type entryMapper struct {
	mapping entryMappingConfig
	limits  safetyLimits
}

func (m entryMapper) entryFromPayload(ts any, payload map[string]any, tag string) logging.Entry {
	entry := logging.Entry{
		Timestamp: timestamp(ts),
		Payload:   payload,
	}

	m.applyResource(payload, &entry)
	m.applyLabels(payload, tag, &entry)
	m.applyTraceFields(payload, &entry)
	m.applyMetadataFields(payload, &entry)
	m.applyTextPayload(payload, &entry)
	return entry
}

func (m entryMapper) applyResource(payload map[string]any, entry *logging.Entry) {
	if !m.mapping.enableResourceOverride {
		return
	}
	if resource, ok := extractMonitoredResource(payload, m.mapping.monitoredResourceKey); ok {
		entry.Resource = resource
		delete(payload, m.mapping.monitoredResourceKey)
	}
}

func (m entryMapper) applyLabels(payload map[string]any, tag string, entry *logging.Entry) {
	if labels, ok := m.extractStringMap(payload, m.mapping.labelsKey); ok {
		entry.Labels = labels
		delete(payload, m.mapping.labelsKey)
	}
	if m.mapping.tagLabelKey != "" && tag != "" {
		if entry.Labels == nil {
			entry.Labels = map[string]string{}
		}
		entry.Labels[truncateString(m.mapping.tagLabelKey, m.limits.maxLabelKeyLength)] = truncateString(tag, m.limits.maxLabelValueLength)
	}
}

func (m entryMapper) applyTraceFields(payload map[string]any, entry *logging.Entry) {
	if severity, ok := extractSeverity(payload, m.mapping.severityKey); ok {
		entry.Severity = severity
		delete(payload, m.mapping.severityKey)
	}

	if trace, ok := extractString(payload, m.mapping.traceKey); ok {
		if formatted, ok := formatTrace(m.mapping.projectID, trace, m.mapping.autoformatStackdriverTrace, m.mapping.enableCrossProjectTrace); ok {
			entry.Trace = formatted
			delete(payload, m.mapping.traceKey)
		}
	}

	if spanID, ok := extractString(payload, m.mapping.spanIDKey); ok {
		entry.SpanID = spanID
		delete(payload, m.mapping.spanIDKey)
	}

	if sampled, ok := extractBool(payload, m.mapping.traceSampledKey); ok {
		entry.TraceSampled = sampled
		delete(payload, m.mapping.traceSampledKey)
	}
}

func (m entryMapper) applyMetadataFields(payload map[string]any, entry *logging.Entry) {
	if insertID, ok := extractStrictString(payload, m.mapping.insertIDKey); ok {
		entry.InsertID = insertID
		delete(payload, m.mapping.insertIDKey)
	}

	if operation, ok := extractOperation(payload, m.mapping.operationKey); ok {
		entry.Operation = operation
		pruneSubfields(payload, m.mapping.operationKey, "id", "producer", "first", "last")
	}

	if sourceLocation, ok := extractSourceLocation(payload, m.mapping.sourceLocationKey); ok {
		entry.SourceLocation = sourceLocation
		pruneSubfields(payload, m.mapping.sourceLocationKey, "file", "line", "function")
	}

	if request, ok := extractHTTPRequest(payload, m.mapping.httpRequestKey); ok {
		entry.HTTPRequest = request
		pruneSubfields(payload, m.mapping.httpRequestKey, httpRequestSubfields...)
	}
}

func (m entryMapper) applyTextPayload(payload map[string]any, entry *logging.Entry) {
	if m.mapping.textPayloadKey != "" {
		if textPayload, ok := extractString(payload, m.mapping.textPayloadKey); ok && len(payload) == 1 {
			entry.Payload = textPayload
		}
	}
}

func extractMonitoredResource(payload map[string]any, key string) (*mrpb.MonitoredResource, bool) {
	if key == "" {
		return nil, false
	}
	fields, ok := extractMap(payload, key)
	if !ok {
		return nil, false
	}

	resourceType, ok := stringFromMap(fields, "type")
	if !ok {
		return nil, false
	}

	labels := map[string]string{}
	if rawLabels, ok := fields["labels"]; ok {
		switch values := rawLabels.(type) {
		case map[string]any:
			for key, value := range values {
				labels[key] = fmt.Sprint(value)
			}
		case map[any]any:
			for key, value := range values {
				labels[stringify(key)] = fmt.Sprint(value)
			}
		default:
			return nil, false
		}
	}

	return &mrpb.MonitoredResource{
		Type:   resourceType,
		Labels: labels,
	}, true
}

func (m entryMapper) normalizeValue(value any, depth int) any {
	budget := m.limits.maxPayloadBytes
	return m.normalizeValueWithBudget(value, depth, &budget)
}

func (m entryMapper) normalizeValueWithBudget(value any, depth int, budget *int) any {
	if m.limits.maxRecordDepth > 0 && depth > m.limits.maxRecordDepth {
		return maxRecordDepthMarker
	}

	switch v := value.(type) {
	case map[any]any:
		return m.normalizeMapWithBudget(v, depth, budget)
	case []any:
		count := len(v)
		truncated := false
		if m.limits.maxArrayItems > 0 && count > m.limits.maxArrayItems {
			count = m.limits.maxArrayItems
			truncated = true
		}
		items := make([]any, 0, count+1)
		for _, item := range v[:count] {
			normalized := m.normalizeValueWithBudget(item, depth+1, budget)
			if !m.consumePayloadBudget(budget, approximateSize(normalized)) {
				truncated = true
				break
			}
			items = append(items, normalized)
		}
		if truncated {
			items = append(items, truncatedMarker)
		}
		return items
	case []byte:
		if m.limits.maxJSONBytes > 0 && len(v) <= m.limits.maxJSONBytes && json.Valid(v) {
			if !m.consumePayloadBudget(budget, len(v)) {
				return truncatedMarker
			}
			return json.RawMessage(v)
		}
		return m.truncateString(string(v))
	case string:
		return m.truncateString(v)
	default:
		return v
	}
}

func (m entryMapper) normalizeMap(record map[any]any, depth int) map[string]any {
	budget := m.limits.maxPayloadBytes
	return m.normalizeMapWithBudget(record, depth, &budget)
}

func (m entryMapper) normalizeMapWithBudget(record map[any]any, depth int, budget *int) map[string]any {
	priorityKeys := map[string]struct{}{}
	priorityFields := map[string]any{}
	if depth == 0 {
		priorityKeys = m.priorityPayloadKeys()
		priorityFields = m.priorityPayloadFields(record, priorityKeys)
	}

	fieldCount := len(record) - len(priorityFields)
	truncated := false
	if m.limits.maxFieldCount > 0 && fieldCount > m.limits.maxFieldCount {
		fieldCount = m.limits.maxFieldCount
		truncated = true
	}

	payload := make(map[string]any, fieldCount+len(priorityFields)+1)
	for key, value := range priorityFields {
		if !m.addNormalizedPayloadField(payload, key, value, depth, budget) {
			truncated = true
		}
	}

	added := 0
	for key, value := range record {
		payloadKey := m.truncateString(stringify(key))
		if _, ok := priorityFields[payloadKey]; ok {
			continue
		}
		if added >= fieldCount {
			truncated = true
			break
		}
		if !m.addNormalizedPayloadField(payload, payloadKey, value, depth, budget) {
			truncated = true
			break
		}
		added++
	}
	if truncated {
		payload[payloadTruncatedKey] = true
	}
	return payload
}

func (m entryMapper) priorityPayloadKeys() map[string]struct{} {
	keys := map[string]struct{}{}
	for _, key := range []string{
		m.mapping.labelsKey,
		m.mapping.severityKey,
		m.mapping.traceKey,
		m.mapping.spanIDKey,
		m.mapping.traceSampledKey,
		m.mapping.insertIDKey,
		m.mapping.operationKey,
		m.mapping.sourceLocationKey,
		m.mapping.httpRequestKey,
		m.mapping.monitoredResourceKey,
		m.mapping.logNameKey,
		m.mapping.textPayloadKey,
	} {
		key = m.truncateString(key)
		if key != "" {
			keys[key] = struct{}{}
		}
	}
	return keys
}

func (m entryMapper) priorityPayloadFields(record map[any]any, priorityKeys map[string]struct{}) map[string]any {
	fields := map[string]any{}
	for key, value := range record {
		payloadKey := m.truncateString(stringify(key))
		if _, ok := priorityKeys[payloadKey]; ok {
			fields[payloadKey] = value
		}
	}
	return fields
}

func (m entryMapper) addNormalizedPayloadField(payload map[string]any, key string, value any, depth int, budget *int) bool {
	normalized := m.normalizeValueWithBudget(value, depth+1, budget)
	if !m.consumePayloadBudget(budget, len(key)+approximateSize(normalized)) {
		return false
	}
	payload[key] = normalized
	return true
}

func stringify(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	default:
		return fmt.Sprint(v)
	}
}

func timestamp(value any) time.Time {
	if ts, ok := value.(time.Time); ok {
		return ts
	}
	return time.Now()
}

func extractString(payload map[string]any, key string) (string, bool) {
	value, ok := payload[key]
	if !ok {
		return "", false
	}

	switch v := value.(type) {
	case string:
		return v, v != ""
	case []byte:
		return string(v), len(v) > 0
	default:
		return fmt.Sprint(v), true
	}
}

func extractStrictString(payload map[string]any, key string) (string, bool) {
	value, ok := payload[key]
	if !ok {
		return "", false
	}

	switch v := value.(type) {
	case string:
		return v, v != ""
	default:
		return "", false
	}
}

func (m entryMapper) extractStringMap(payload map[string]any, key string) (map[string]string, bool) {
	value, ok := payload[key]
	if !ok {
		return nil, false
	}

	labels := map[string]string{}
	add := func(key string, value any) {
		if m.limits.maxLabelCount > 0 && len(labels) >= m.limits.maxLabelCount {
			return
		}
		key = truncateString(strings.TrimSpace(key), m.limits.maxLabelKeyLength)
		if key == "" {
			return
		}
		labels[key] = truncateString(fmt.Sprint(value), m.limits.maxLabelValueLength)
	}

	switch v := value.(type) {
	case map[string]any:
		for k, val := range v {
			add(k, val)
		}
	case map[any]any:
		for k, val := range v {
			add(stringify(k), val)
		}
	default:
		return nil, false
	}

	return labels, len(labels) > 0
}

func extractSeverity(payload map[string]any, key string) (logging.Severity, bool) {
	value, ok := extractString(payload, key)
	if !ok {
		return logging.Default, false
	}

	switch strings.ToLower(strings.TrimSpace(value)) {
	case "debug":
		return logging.Debug, true
	case "info", "information":
		return logging.Info, true
	case "notice":
		return logging.Notice, true
	case "warning", "warn":
		return logging.Warning, true
	case "error", "err":
		return logging.Error, true
	case "critical", "crit":
		return logging.Critical, true
	case "alert":
		return logging.Alert, true
	case "emergency", "emerg", "fatal":
		return logging.Emergency, true
	default:
		return logging.Default, false
	}
}

func formatTrace(projectID, trace string, autoformat, allowCrossProject bool) (string, bool) {
	trace = strings.TrimSpace(trace)
	if trace == "" {
		return "", false
	}

	prefix := "projects/" + projectID + "/traces/"
	if strings.HasPrefix(trace, prefix) {
		return trace, true
	}
	if strings.HasPrefix(trace, "projects/") {
		return trace, allowCrossProject
	}
	if !autoformat {
		return trace, true
	}
	return prefix + trace, true
}

func extractBool(payload map[string]any, key string) (bool, bool) {
	value, ok := payload[key]
	if !ok {
		return false, false
	}

	switch v := value.(type) {
	case bool:
		return v, true
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(v))
		return parsed, err == nil
	case []byte:
		parsed, err := strconv.ParseBool(strings.TrimSpace(string(v)))
		return parsed, err == nil
	default:
		return false, false
	}
}

func extractOperation(payload map[string]any, key string) (*loggingpb.LogEntryOperation, bool) {
	fields, ok := extractMap(payload, key)
	if !ok {
		return nil, false
	}

	operation := &loggingpb.LogEntryOperation{}
	if value, ok := stringFromMap(fields, "id"); ok {
		operation.Id = value
	}
	if value, ok := stringFromMap(fields, "producer"); ok {
		operation.Producer = value
	}
	if value, ok := boolFromMap(fields, "first"); ok {
		operation.First = value
	}
	if value, ok := boolFromMap(fields, "last"); ok {
		operation.Last = value
	}

	return operation, operation.Id != "" || operation.Producer != "" || operation.First || operation.Last
}

func extractSourceLocation(payload map[string]any, key string) (*loggingpb.LogEntrySourceLocation, bool) {
	fields, ok := extractMap(payload, key)
	if !ok {
		return nil, false
	}

	location := &loggingpb.LogEntrySourceLocation{}
	if value, ok := stringFromMap(fields, "file"); ok {
		location.File = value
	}
	if value, ok := int64FromMap(fields, "line"); ok {
		location.Line = value
	}
	if value, ok := stringFromMap(fields, "function"); ok {
		location.Function = value
	}

	return location, location.File != "" || location.Line != 0 || location.Function != ""
}

func extractHTTPRequest(payload map[string]any, key string) (*logging.HTTPRequest, bool) {
	fields, ok := extractMap(payload, key)
	if !ok {
		return nil, false
	}

	request := &logging.HTTPRequest{}
	if value, ok := int64FromMap(fields, "requestSize"); ok {
		request.RequestSize = value
	}
	if value, ok := intFromMap(fields, "status"); ok {
		request.Status = value
	}
	if value, ok := int64FromMap(fields, "responseSize"); ok {
		request.ResponseSize = value
	}
	if value, ok := durationFromMap(fields, "latency"); ok {
		request.Latency = value
	}
	if value, ok := stringFromMap(fields, "serverIp"); ok {
		request.LocalIP = value
	}
	if value, ok := stringFromMap(fields, "remoteIp"); ok {
		request.RemoteIP = value
	}
	if value, ok := boolFromMap(fields, "cacheHit"); ok {
		request.CacheHit = value
	}
	if value, ok := boolFromMap(fields, "cacheValidatedWithOriginServer"); ok {
		request.CacheValidatedWithOriginServer = value
	}
	if value, ok := int64FromMap(fields, "cacheFillBytes"); ok {
		request.CacheFillBytes = value
	}
	if value, ok := boolFromMap(fields, "cacheLookup"); ok {
		request.CacheLookup = value
	}

	method := optionalStringFromMap(fields, "requestMethod")
	requestURL := optionalStringFromMap(fields, "requestUrl")
	protocol := optionalStringFromMap(fields, "protocol")
	userAgent := optionalStringFromMap(fields, "userAgent")
	referer := optionalStringFromMap(fields, "referer")
	if method != "" || requestURL != "" || protocol != "" || userAgent != "" || referer != "" {
		request.Request = buildHTTPRequest(method, requestURL, protocol, userAgent, referer)
	}

	return request, request.Request != nil ||
		request.RequestSize != 0 ||
		request.Status != 0 ||
		request.ResponseSize != 0 ||
		request.Latency != 0 ||
		request.LocalIP != "" ||
		request.RemoteIP != "" ||
		request.CacheHit ||
		request.CacheValidatedWithOriginServer ||
		request.CacheFillBytes != 0 ||
		request.CacheLookup
}

func extractMap(payload map[string]any, key string) (map[string]any, bool) {
	value, ok := payload[key]
	if !ok {
		return nil, false
	}
	switch v := value.(type) {
	case map[string]any:
		return v, true
	case map[any]any:
		return normalizeAnyMap(v), true
	default:
		return nil, false
	}
}

func pruneSubfields(payload map[string]any, key string, fields ...string) {
	values, ok := extractMap(payload, key)
	if !ok {
		return
	}
	for _, field := range fields {
		delete(values, field)
	}
	if len(values) == 0 {
		delete(payload, key)
		return
	}
	payload[key] = values
}

func normalizeAnyMap(record map[any]any) map[string]any {
	fields := make(map[string]any, len(record))
	for key, value := range record {
		fields[stringify(key)] = value
	}
	return fields
}

func stringFromMap(fields map[string]any, key string) (string, bool) {
	value, ok := fields[key]
	if !ok {
		return "", false
	}
	switch v := value.(type) {
	case string:
		return v, v != ""
	case []byte:
		return string(v), len(v) > 0
	default:
		return fmt.Sprint(v), true
	}
}

func optionalStringFromMap(fields map[string]any, key string) string {
	value, ok := stringFromMap(fields, key)
	if !ok {
		return ""
	}
	return value
}

func boolFromMap(fields map[string]any, key string) (bool, bool) {
	value, ok := fields[key]
	if !ok {
		return false, false
	}
	switch v := value.(type) {
	case bool:
		return v, true
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(v))
		return parsed, err == nil
	case []byte:
		parsed, err := strconv.ParseBool(strings.TrimSpace(string(v)))
		return parsed, err == nil
	default:
		return false, false
	}
}

func intFromMap(fields map[string]any, key string) (int, bool) {
	value, ok := int64FromMap(fields, key)
	if !ok || value > int64(math.MaxInt) || value < int64(math.MinInt) {
		return 0, false
	}
	return int(value), true
}

func int64FromMap(fields map[string]any, key string) (int64, bool) {
	value, ok := fields[key]
	if !ok {
		return 0, false
	}
	switch v := value.(type) {
	case int:
		return int64(v), true
	case int8:
		return int64(v), true
	case int16:
		return int64(v), true
	case int32:
		return int64(v), true
	case int64:
		return v, true
	case uint:
		if uint64(v) > uint64(math.MaxInt64) {
			return 0, false
		}
		return int64(v), true
	case uint8:
		return int64(v), true
	case uint16:
		return int64(v), true
	case uint32:
		return int64(v), true
	case uint64:
		if v > uint64(1<<63-1) {
			return 0, false
		}
		return int64(v), true
	case float32:
		value := float64(v)
		if math.IsNaN(value) || math.IsInf(value, 0) || math.Trunc(value) != value ||
			value > float64(math.MaxInt64) || value < float64(math.MinInt64) {
			return 0, false
		}
		return int64(value), true
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) || math.Trunc(v) != v ||
			v > float64(math.MaxInt64) || v < float64(math.MinInt64) {
			return 0, false
		}
		return int64(v), true
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		return parsed, err == nil
	case []byte:
		parsed, err := strconv.ParseInt(strings.TrimSpace(string(v)), 10, 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func durationFromMap(fields map[string]any, key string) (time.Duration, bool) {
	value, ok := stringFromMap(fields, key)
	if !ok {
		return 0, false
	}
	duration, err := time.ParseDuration(value)
	return duration, err == nil
}

func buildHTTPRequest(method, requestURL, protocol, userAgent, referer string) *http.Request {
	if requestURL == "" {
		requestURL = "/"
	}
	parsedURL, err := url.Parse(requestURL)
	if err != nil {
		parsedURL = &url.URL{Path: requestURL}
	}

	request := &http.Request{
		Method: method,
		URL:    parsedURL,
		Proto:  protocol,
		Header: http.Header{},
	}
	if userAgent != "" {
		request.Header.Set("User-Agent", userAgent)
	}
	if referer != "" {
		request.Header.Set("Referer", referer)
	}
	return request
}

func truncateString(value string, maxLength int) string {
	if maxLength <= 0 || len(value) <= maxLength {
		return value
	}
	return value[:maxLength]
}

func (m entryMapper) truncateString(value string) string {
	return truncateString(value, m.limits.maxStringBytes)
}

func (m entryMapper) consumePayloadBudget(budget *int, size int) bool {
	if m.limits.maxPayloadBytes <= 0 {
		return true
	}
	if size > *budget {
		return false
	}
	*budget -= size
	return true
}

func approximateSize(value any) int {
	switch v := value.(type) {
	case string:
		return len(v)
	case json.RawMessage:
		return len(v)
	case []byte:
		return len(v)
	case map[string]any:
		size := 0
		for key, value := range v {
			size += len(key) + approximateSize(value)
		}
		return size
	case []any:
		size := 0
		for _, item := range v {
			size += approximateSize(item)
		}
		return size
	default:
		return len(fmt.Sprint(v))
	}
}
