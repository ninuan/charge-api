.PHONY: setup dev check deploy deploy-git reset-local

setup:
	cd backend && go mod download
	cd frontend && npm ci

dev:
	bash scripts/dev.sh

check:
	bash scripts/check.sh

deploy:
	@DEPLOY_HOST="$(DEPLOY_HOST)" \
	DEPLOY_PATH="$(DEPLOY_PATH)" \
	SERVICE_NAME="$(SERVICE_NAME)" \
	HEALTH_URL="$(HEALTH_URL)" \
	SSH_OPTS="$(SSH_OPTS)" \
	SKIP_CHECK="$(SKIP_CHECK)" \
	bash scripts/deploy.sh $(DEPLOY_ARGS)

deploy-git:
	@DEPLOY_HOST="$(DEPLOY_HOST)" \
	DEPLOY_PATH="$(DEPLOY_PATH)" \
	DEPLOY_REMOTE="$(DEPLOY_REMOTE)" \
	DEPLOY_BRANCH="$(DEPLOY_BRANCH)" \
	SERVICE_NAME="$(SERVICE_NAME)" \
	HEALTH_URL="$(HEALTH_URL)" \
	SSH_OPTS="$(SSH_OPTS)" \
	SKIP_CHECK="$(SKIP_CHECK)" \
	bash scripts/deploy_git.sh $(DEPLOY_ARGS)

reset-local:
	@printf "这会删除 .local/ 下的本地测试用户、Cookie 和设备状态，继续吗？[y/N] "; \
	read answer; \
	if [ "$$answer" = "y" ] || [ "$$answer" = "Y" ]; then \
		rm -rf .local; \
		echo "本地测试状态已删除。下次 make dev 会重新创建 admin。"; \
	else \
		echo "已取消。"; \
	fi
