# aws-firelens-stackdriver

Fluent Bit output plugin for sending ECS FireLens logs to Google Cloud Logging.

The main artifact is:

```text
plugin/dist/out_stackdriver_wif.so
```

The plugin uses AWS Workload Identity Federation. It reads AWS credentials from the normal AWS SDK credential chain, exchanges them through Google STS, impersonates a Google service account, and writes log entries to Cloud Logging. No Google service account key is required.

## Repository Layout

```text
plugin/                 Fluent Bit Go output plugin and artifact build
examples/ecs-firelens/  ECS Fargate + FireLens sample project
```

`plugin/` is the product. The ECS/Terraform project is only a runnable example.

## Build the Plugin

Requirements:

- Docker with Buildx
- Network access to download Go modules and the Go toolchain during the Docker build

Build the Linux arm64 shared object:

```bash
make plugin-build
```

or:

```bash
cd plugin
./build-linux-plugin.sh
```

Output:

```text
plugin/dist/out_stackdriver_wif.so
```

## Use the Plugin in a FireLens Image

Copy the artifact into the Fluent Bit plugin directory and register it:

```dockerfile
FROM public.ecr.aws/aws-observability/aws-for-fluent-bit:init-3

COPY plugin/dist/out_stackdriver_wif.so /fluent-bit/plugins/out_stackdriver_wif.so
COPY fluentbit/plugins.conf /fluent-bit/etc/plugins.conf
COPY fluentbit/pipeline.conf /fluent-bit/etc/pipeline.conf
```

`plugins.conf`:

```conf
[PLUGINS]
    Path /fluent-bit/plugins/out_stackdriver_wif.so
```

`pipeline.conf`:

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

See [plugin/README.md](plugin/README.md) for the full plugin contract.

## Example

The ECS Fargate sample is in [examples/ecs-firelens](examples/ecs-firelens). It creates AWS and Google Cloud resources with Terraform and runs a small Go app whose JSON logs are routed through this plugin.

## Release Direction

The expected release artifact is the `.so` file produced by `plugin/Dockerfile.build`. CI should treat this artifact as the primary output and attach it to releases. Terraform validation and the ECS sample are secondary checks.
