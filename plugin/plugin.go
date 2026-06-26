package main

import (
	"C"
	"context"
	"log"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"unsafe"

	"cloud.google.com/go/logging"
	"github.com/fluent/fluent-bit-go/output"
	"google.golang.org/api/option"
)

const maxLogIDLength = 512

var logIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

type pluginConfig struct {
	client        *logging.Client
	logger        *logging.Logger
	loggerOptions []logging.LoggerOption
	loggersMu     sync.Mutex
	loggers       map[string]*logging.Logger
	maxLoggers    int
	projectID     string
	mapper        entryMapper
}

//export FLBPluginRegister
func FLBPluginRegister(def unsafe.Pointer) int {
	return output.FLBPluginRegister(def, pluginName, "Cloud Logging output with AWS Workload Identity Federation")
}

//export FLBPluginInit
func FLBPluginInit(plugin unsafe.Pointer) int {
	ctx := context.Background()

	cfg, err := readOutputConfig(func(key string) string {
		return output.FLBPluginConfigKey(plugin, key)
	})
	if err != nil {
		log.Print(err)
		return output.FLB_ERROR
	}

	opts := []option.ClientOption{}
	if cfg.EnableIdentityFederation {
		tokenSource, err := newWIFTokenSource(ctx, cfg.WIF)
		if err != nil {
			log.Printf("failed to configure identity federation: %v", err)
			return output.FLB_ERROR
		}
		opts = append(opts, option.WithTokenSource(tokenSource))
	} else if cfg.GoogleServiceCredentials != "" {
		opts = append(opts, option.WithAuthCredentialsFile(option.ServiceAccount, cfg.GoogleServiceCredentials))
	}

	client, err := logging.NewClient(ctx, cfg.ProjectID, opts...)
	if err != nil {
		log.Printf("failed to create Cloud Logging client: %v", err)
		return output.FLB_ERROR
	}

	resource := cfg.monitoredResource()
	loggerOptions := []logging.LoggerOption{
		logging.CommonResource(resource),
		logging.CommonLabels(cfg.Labels),
		logging.PartialSuccess(),
		logging.ContextFunc(func() (context.Context, func()) {
			return context.WithTimeout(context.Background(), cfg.FlushTimeout)
		}),
	}
	conf := &pluginConfig{
		client:        client,
		logger:        client.Logger(cfg.LogID, loggerOptions...),
		loggerOptions: loggerOptions,
		loggers:       map[string]*logging.Logger{},
		maxLoggers:    cfg.MaxLoggers,
		projectID:     cfg.ProjectID,
		mapper: entryMapper{
			mapping: cfg.Mapping,
			limits:  cfg.Limits,
		},
	}

	id := storeConfig(conf)
	output.FLBPluginSetContext(plugin, id)
	return output.FLB_OK
}

//export FLBPluginFlushCtx
func FLBPluginFlushCtx(ctx, data unsafe.Pointer, length C.int, tag *C.char) int {
	id, ok := output.FLBPluginGetContext(ctx).(int)
	if !ok {
		log.Print("failed to read plugin context")
		return output.FLB_ERROR
	}

	conf := getConfig(id)
	if conf == nil {
		log.Printf("configuration for context %d was not found", id)
		return output.FLB_ERROR
	}

	dec := output.NewDecoder(data, int(length))
	fluentBitTag := C.GoString(tag)
	usedLoggers := map[*logging.Logger]struct{}{}
	for {
		ret, ts, record := output.GetRecord(dec)
		if ret != 0 {
			break
		}

		logger, entry := conf.loggerAndEntryFromRecord(ts, record, fluentBitTag)
		usedLoggers[logger] = struct{}{}
		logger.Log(entry)
	}

	if err := flushLoggers(loggersFromSet(usedLoggers)); err != nil {
		log.Printf("failed to flush Cloud Logging entries: %v", err)
		return output.FLB_RETRY
	}

	return output.FLB_OK
}

//export FLBPluginExitCtx
func FLBPluginExitCtx(ctx unsafe.Pointer) int {
	id, ok := output.FLBPluginGetContext(ctx).(int)
	if !ok {
		return output.FLB_OK
	}

	conf := takeConfig(id)
	if conf == nil {
		return output.FLB_OK
	}

	ret := output.FLB_OK
	if err := flushLoggers(conf.allLoggers()); err != nil {
		log.Printf("failed to flush Cloud Logging entries on exit: %v", err)
		ret = output.FLB_ERROR
	}
	if err := conf.client.Close(); err != nil {
		log.Printf("failed to close Cloud Logging client: %v", err)
		ret = output.FLB_ERROR
	}
	return ret
}

//export FLBPluginExit
func FLBPluginExit() int {
	return output.FLB_OK
}

func main() {
}

func (c *pluginConfig) loggerAndEntryFromRecord(ts any, record map[any]any, tag string) (*logging.Logger, logging.Entry) {
	payload := c.mapper.normalizeMap(record, 0)
	logger := c.logger
	if logName, ok := extractString(payload, c.mapper.mapping.logNameKey); ok {
		if selected := c.loggerForLogName(logName); selected != nil {
			logger = selected
			delete(payload, c.mapper.mapping.logNameKey)
		}
	} else if c.mapper.mapping.useTagAsLogID && tag != "" {
		if selected := c.loggerForLogName(tag); selected != nil {
			logger = selected
		}
	}
	return logger, c.mapper.entryFromPayload(ts, payload, tag)
}

func (c *pluginConfig) entryFromRecord(ts any, record map[any]any) logging.Entry {
	_, entry := c.loggerAndEntryFromRecord(ts, record, "")
	return entry
}

func (c *pluginConfig) loggerForLogName(logName string) *logging.Logger {
	logID, ok := c.logIDFromLogName(logName)
	if !ok {
		return nil
	}

	c.loggersMu.Lock()
	defer c.loggersMu.Unlock()
	if logger, ok := c.loggers[logID]; ok {
		return logger
	}
	if c.maxLoggers > 0 && len(c.loggers) >= c.maxLoggers {
		return nil
	}
	logger := c.client.Logger(logID, c.loggerOptions...)
	c.loggers[logID] = logger
	return logger
}

func (c *pluginConfig) logIDFromLogName(logName string) (string, bool) {
	return logIDFromLogName(c.projectID, logName)
}

func logIDFromLogName(projectID, logName string) (string, bool) {
	logName = strings.TrimSpace(logName)
	if logName == "" {
		return "", false
	}
	prefix := "projects/" + projectID + "/logs/"
	if strings.HasPrefix(logName, prefix) {
		logID, err := url.PathUnescape(strings.TrimPrefix(logName, prefix))
		return logID, err == nil && validLogID(logID)
	}
	if strings.Contains(logName, "/") {
		return "", false
	}
	return logName, validLogID(logName)
}

func (c *pluginConfig) allLoggers() []*logging.Logger {
	c.loggersMu.Lock()
	loggers := make([]*logging.Logger, 0, len(c.loggers)+1)
	loggers = append(loggers, c.logger)
	for _, logger := range c.loggers {
		loggers = append(loggers, logger)
	}
	c.loggersMu.Unlock()
	return loggers
}

func flushLoggers(loggers []*logging.Logger) error {
	for _, logger := range loggers {
		if err := logger.Flush(); err != nil {
			return err
		}
	}
	return nil
}

func validLogID(logID string) bool {
	return logID != "" && len(logID) <= maxLogIDLength && logIDPattern.MatchString(logID)
}

func loggersFromSet(values map[*logging.Logger]struct{}) []*logging.Logger {
	loggers := make([]*logging.Logger, 0, len(values))
	for logger := range values {
		loggers = append(loggers, logger)
	}
	return loggers
}
