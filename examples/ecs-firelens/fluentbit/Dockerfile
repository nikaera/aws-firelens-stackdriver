FROM public.ecr.aws/aws-observability/aws-for-fluent-bit:init-3

COPY plugin/dist/out_stackdriver_wif.so /fluent-bit/plugins/out_stackdriver_wif.so
COPY examples/ecs-firelens/fluentbit/parsers.conf /fluent-bit/etc/parsers.conf
COPY examples/ecs-firelens/fluentbit/pipeline.conf /fluent-bit/etc/pipeline.conf
COPY examples/ecs-firelens/fluentbit/plugins.conf /fluent-bit/etc/plugins.conf
