# stackdriver_wif Fluent Bit Plugin

`stackdriver_wif` is a Fluent Bit output plugin for Google Cloud Logging.

It is built as a Linux arm64 shared object:

```text
out_stackdriver_wif.so
```

## Build

From the repository root:

```bash
make plugin-build
```

From this directory:

```bash
./build-linux-plugin.sh
```

The artifact is written to:

```text
plugin/dist/out_stackdriver_wif.so
```

## Runtime Contract

Register the plugin in Fluent Bit:

```conf
[PLUGINS]
    Path /fluent-bit/plugins/out_stackdriver_wif.so
```

Use this output name:

```conf
[OUTPUT]
    Name stackdriver_wif
```

## Required Options for AWS WIF

These options are required when `enable_identity_federation` is enabled:

| Option | Description |
| --- | --- |
| `project_id` | Google Cloud project ID that owns the target Cloud Logging log. |
| `enable_identity_federation` | Set to `true`. |
| `aws_region` | AWS region used by the ECS task credentials. |
| `project_number` | Google Cloud project number that owns the Workload Identity Pool. |
| `pool_id` | Workload Identity Pool ID. |
| `provider_id` | Workload Identity Pool Provider ID. |
| `google_service_account` | Google service account email to impersonate. |

## Common Options

| Option | Default | Description |
| --- | --- | --- |
| `log_id` | `fluent-bit` | Cloud Logging log ID. |
| `resource` | `global` | Monitored resource type. |
| `resource_labels` | empty | Comma-separated monitored resource labels, for example `job=firelens,task_id=task-1`. |
| `flush_timeout` | `10s` | Deadline for Cloud Logging write RPCs used by logger flush. A plain number is treated as seconds. |
| `severity_key` | `logging.googleapis.com/severity` | Payload key used as Cloud Logging severity. |
| `labels_key` | `logging.googleapis.com/labels` | Payload key used as Cloud Logging labels. |
| `trace_key` | `logging.googleapis.com/trace` | Payload key used as Cloud Logging trace. |
| `autoformat_stackdriver_trace` | `false` | Format bare trace IDs as `projects/{project_id}/traces/{trace_id}`. This matches Fluent Bit's upstream option name. |
| `span_id_key` | `logging.googleapis.com/spanId` | Payload key used as Cloud Logging span ID. |
| `trace_sampled_key` | `logging.googleapis.com/traceSampled` | Payload key used as Cloud Logging trace sampled flag. |
| `insert_id_key` | `logging.googleapis.com/insertId` | Payload key used as Cloud Logging insert ID. |
| `operation_key` | `logging.googleapis.com/operation` | Payload key used as Cloud Logging operation metadata. |
| `source_location_key` | `logging.googleapis.com/sourceLocation` | Payload key used as Cloud Logging source location metadata. |
| `http_request_key` | `logging.googleapis.com/http_request` | Payload key used as Cloud Logging HTTP request metadata. |
| `monitored_resource_key` | `logging.googleapis.com/monitored_resource` | Payload key used as per-entry monitored resource metadata when `enable_resource_override` is enabled. |
| `enable_resource_override` | `false` | Allow records to override the configured monitored resource. Keep this disabled unless the input is trusted. |
| `log_name_key` | `logging.googleapis.com/logName` | Payload key used to choose a per-entry log ID. Only plain log IDs matching `A-Za-z0-9._-` or `projects/{project_id}/logs/{log_id}` are accepted. |
| `use_tag_as_log_id` | `false` | Use the Fluent Bit tag as the log ID when `log_name_key` is absent. This is similar to upstream `out_stackdriver`, but it is opt-in here to avoid high-cardinality log IDs. |
| `enable_cross_project_trace` | `false` | Allow full trace names for projects other than `project_id`. |
| `text_payload_key` | empty | Payload key used as Cloud Logging text payload when it is the only remaining payload field. |
| `tag_label_key` | empty | Cloud Logging label key used to store the Fluent Bit tag. |
| `labels` | empty | Comma-separated static labels, for example `service=api,env=dev`. |

Exactly one authentication mode must be configured:

- `enable_identity_federation true` for AWS Workload Identity Federation.
- `google_service_credentials` for a local Google service account JSON file.
- `enable_adc true` for Application Default Credentials.

Use service account files and ADC only for local testing. The intended production path is AWS Workload Identity Federation, which avoids long-lived Google service account keys.

## Safety Limits

| Option | Default | Description |
| --- | --- | --- |
| `max_record_depth` | `32` | Maximum nested payload depth. Deeper values are replaced with a marker string. |
| `max_label_count` | `64` | Maximum labels accepted from `labels_key`. Extra labels are ignored. |
| `max_label_key_length` | `512` | Maximum label key length. Longer keys are truncated. |
| `max_label_value_length` | `4096` | Maximum label value length. Longer values are truncated. |
| `max_json_bytes` | `262144` | Maximum `[]byte` size checked for JSON auto-detection. Larger values are sent as strings. |
| `max_string_bytes` | `262144` | Maximum string size kept in the payload. Longer strings are truncated. |
| `max_field_count` | `256` | Maximum fields kept per payload object. Extra fields are dropped and a truncation marker is added. |
| `max_array_items` | `1024` | Maximum items kept per payload array. Extra items are dropped and a truncation marker is added. |
| `max_payload_bytes` | `524288` | Approximate maximum payload size after normalization. Extra fields are dropped and a truncation marker is added. |
| `max_loggers` | `64` | Maximum per-entry loggers created from `log_name_key`. Extra log IDs fall back to the default logger. |

## Minimal Output Configuration

```conf
[OUTPUT]
    Name                       stackdriver_wif
    Match                      *
    project_id                 ${GCP_PROJECT_ID}
    log_id                     ${CLOUD_LOGGING_LOG_ID}
    resource                   global
    severity_key               level
    enable_identity_federation true
    aws_region                 ${AWS_REGION}
    project_number             ${GCP_PROJECT_NUMBER}
    pool_id                    ${WIF_POOL_ID}
    provider_id                ${WIF_PROVIDER_ID}
    google_service_account     ${GOOGLE_SERVICE_ACCOUNT}
```

## Authentication Flow

1. The plugin loads AWS credentials through the AWS SDK credential chain.
2. Google external account support signs an AWS identity request.
3. Google STS exchanges that AWS identity for a federated token.
4. IAMCredentials impersonates the configured Google service account.
5. The Cloud Logging client writes entries with that access token.

The ECS task role must be allowed to impersonate the Google service account through Workload Identity Federation.

## Log Mapping

Each Fluent Bit record is converted to a Cloud Logging entry.

- `severity_key` maps a payload field to Cloud Logging severity.
- `labels_key` maps a payload object to Cloud Logging labels.
- `trace_key` maps a payload field to Cloud Logging trace. Bare trace IDs are kept as-is unless `autoformat_stackdriver_trace` is enabled. Full trace names for other projects are ignored unless `enable_cross_project_trace` is enabled.
- `span_id_key` maps a payload field to Cloud Logging span ID.
- `trace_sampled_key` maps a payload field to Cloud Logging trace sampled.
- `insert_id_key` maps a non-empty string payload field to Cloud Logging insert ID.
- `operation_key` maps `id`, `producer`, `first`, and `last` to Cloud Logging operation. Unknown subfields remain in the JSON payload.
- `source_location_key` maps `file`, `line`, and `function` to Cloud Logging source location. Unknown subfields remain in the JSON payload.
- `http_request_key` maps common HTTP request fields such as `requestMethod`, `requestUrl`, `status`, `latency`, `remoteIp`, and `userAgent`. Unknown subfields remain in the JSON payload.
- `monitored_resource_key` maps `type` and `labels` to the entry monitored resource only when `enable_resource_override` is enabled.
- `log_name_key` accepts either a plain log ID or `projects/{project_id}/logs/{log_id}`. Other projects and unsupported log IDs are ignored.
- `text_payload_key` writes a string text payload only when that field is the only remaining payload field.
- `tag_label_key` adds the Fluent Bit tag as a Cloud Logging label.

After mapping, special fields are removed from the JSON payload when they are fully consumed.

## Upstream Compatibility Notes

The plugin follows the upstream `out_stackdriver` log-entry mapping where it fits this AWS WIF plugin. Some upstream features are intentionally pending:

- `project_id_key` and `export_to_project_id`: left out to avoid record-driven cross-project routing.
- Kubernetes resource parsing from `logging.googleapis.com/local_resource_id`, `tag_prefix`, and `custom_k8s_regex`: left out because this project targets FireLens/ECS first.
- `cloud_logging_base_url`, `metadata_server`, `service_account_email`, and `service_account_secret`: left out because authentication is handled by WIF, service account files, or ADC.

## Build Integrity

`Dockerfile.build` can verify the downloaded Go toolchain when `GO_TARBALL_SHA256` is set:

```bash
GO_TARBALL_SHA256=<sha256> ./build-linux-plugin.sh
```

Use the SHA-256 value published by the official Go download page for the selected `GO_VERSION`.

## Artifact Compatibility

The current build targets:

```text
GOOS=linux
GOARCH=arm64
buildmode=c-shared
```

The sample image uses:

```text
public.ecr.aws/aws-observability/aws-for-fluent-bit:init-3
```

If you change the Fluent Bit base image, verify that the generated `.so` can be loaded by that image.
