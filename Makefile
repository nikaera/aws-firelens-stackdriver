AWS_REGION          ?= ap-northeast-1

export AWS_REGION

ifdef AWS_PROFILE
export AWS_PROFILE
endif

.PHONY: plugin-build example-app-build-push example-fluentbit-build-push

plugin-build:
	cd plugin && ./build-linux-plugin.sh

example-app-build-push:
	$(MAKE) -C examples/ecs-firelens app-build-push REPO_ROOT=$(CURDIR)

example-fluentbit-build-push:
	$(MAKE) -C examples/ecs-firelens fluentbit-build-push REPO_ROOT=$(CURDIR)
