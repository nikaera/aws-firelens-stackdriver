AWS_REGION          ?= ap-northeast-1
PROJECT_NAME        ?= firelens-stackdriver-demo
APP_IMAGE_TAG       ?= latest
FLUENTBIT_IMAGE_TAG ?= latest
REPO_ROOT           ?= ../..

export AWS_REGION

ifdef AWS_PROFILE
export AWS_PROFILE
endif

.PHONY: app-build-push fluentbit-build-push

app-build-push:
	AWS_ACCOUNT_ID="$$(aws sts get-caller-identity --query Account --output text)"; \
	ECR_REGISTRY="$${AWS_ACCOUNT_ID}.dkr.ecr.$(AWS_REGION).amazonaws.com"; \
	APP_REPO="$${ECR_REGISTRY}/$(PROJECT_NAME)-app"; \
	aws ecr get-login-password --region $(AWS_REGION) | \
		docker login --username AWS --password-stdin "$${ECR_REGISTRY}"; \
	docker buildx build --platform linux/arm64 -t "$${APP_REPO}:$(APP_IMAGE_TAG)" --push app

fluentbit-build-push:
	$(MAKE) -C "$(REPO_ROOT)" plugin-build
	AWS_ACCOUNT_ID="$$(aws sts get-caller-identity --query Account --output text)"; \
	ECR_REGISTRY="$${AWS_ACCOUNT_ID}.dkr.ecr.$(AWS_REGION).amazonaws.com"; \
	FLUENTBIT_REPO="$${ECR_REGISTRY}/$(PROJECT_NAME)-fluentbit"; \
	aws ecr get-login-password --region $(AWS_REGION) | \
		docker login --username AWS --password-stdin "$${ECR_REGISTRY}"; \
	docker buildx build --platform linux/arm64 -f "$(REPO_ROOT)/examples/ecs-firelens/fluentbit/Dockerfile" -t "$${FLUENTBIT_REPO}:$(FLUENTBIT_IMAGE_TAG)" --push "$(REPO_ROOT)"
